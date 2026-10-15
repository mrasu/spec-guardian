package renderer

import (
	"bytes"
	"encoding/json"
	"flag"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
)

var updateGolden = flag.Bool("update-golden", false, "update renderer golden files")

func mustField(t *testing.T, value string) *Field {
	t.Helper()
	field, err := NewField(value)
	require.NoError(t, err)
	return field
}

func TestGeneratedTemplateGoldens(t *testing.T) {
	t.Parallel()

	sources := minimalScaffold(t)
	goldens := map[string][]byte{
		"reader_conformance_cases_generated_test.go.golden": sources.RoleCases["Reader"],
		"reader_conformance_generated_test.go.golden":       sources.RoleConformances["Reader"],
		"reader_conformance_impl_test.go.golden":            sources.RoleScaffolds["Reader"],
		"reader_read_cases_generated.json.golden":           sources.CaseData["reader_read_cases_generated.json"],
	}
	for name, got := range goldens {
		name, got := name, got
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			golden := filepath.Join("testdata", name)
			if *updateGolden {
				require.NoError(t, os.WriteFile(golden, got, 0o644))
			}
			want, err := os.ReadFile(golden)
			require.NoError(t, err)

			if diff := cmp.Diff(string(want), string(got)); diff != "" {
				t.Errorf("%s mismatch (-want +got):\n%s", name, diff)
			}
		})
	}
}

func minimalScaffold(t *testing.T) *Scaffold {
	t.Helper()
	role := fizzbee.RoleState{Name: "Reader", Params: fizzbee.JSONObject{"key": json.Number("1")}, Fields: fizzbee.JSONObject{"status": "INIT"}}
	outputRole := fizzbee.RoleState{Name: "Reader", Params: fizzbee.JSONObject{"key": json.Number("1")}, Fields: fizzbee.JSONObject{"status": "DONE"}}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{{Action: fizzbee.ActionRef{Role: "Reader", Action: "Read"}, Input: fizzbee.Input{Roles: []fizzbee.RoleState{role}}, Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{outputRole}}}}}}, Declarations{})
	require.NoError(t, err)
	return sources
}

func TestGenerateScaffold_ActionCrashedComparisonDefaultsAndOverrides(t *testing.T) {
	trueValue, falseValue := true, false
	tests := []struct {
		name         string
		crashOnYield bool
		compare      *bool
		want         string
	}{
		{name: "crash enabled default", crashOnYield: true, want: "true"},
		{name: "crash disabled default", want: "false"},
		{name: "explicit disable", crashOnYield: true, compare: &falseValue, want: "false"},
		{name: "explicit enable", compare: &trueValue, want: "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actionCase := newRendererActionCase("Reader", "Read")
			actionCase.Outputs[0].SgMetadata.ActionCrashed = true
			sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Options: fizzbee.ModelOptions{CrashOnYield: tt.crashOnYield}, Cases: []fizzbee.ActionCase{actionCase}}, Declarations{CompareActionCrashed: tt.compare})
			require.NoError(t, err)
			require.Contains(t, string(sources.RoleCases["Reader"]), map[string]string{"true": "gcmp.Equal(allowed.SgMetadata.ActionCrashed)", "false": "gcmp.Ignore[bool]"}[tt.want])
			require.Contains(t, string(sources.CaseData["reader_read_cases_generated.json"]), `"action_crashed": true`)
		})
	}
}

func TestGenerateProducesParseableDeterministicSources(t *testing.T) {
	cases := []fizzbee.ActionCase{
		newRendererActionCase("Reader", "Read"),
		newRendererActionCase("Writer", "Write"),
	}
	first, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: cases}, Declarations{})
	require.NoError(t, err)
	second, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: cases}, Declarations{})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Contains(t, first.RoleConformances, "Reader")
	require.Contains(t, first.RoleConformances, "Writer")
	require.Contains(t, first.RoleCases, "Reader")
	require.Contains(t, first.RoleCases, "Writer")
	require.Contains(t, first.CaseData, "reader_read_cases_generated.json")
	require.Contains(t, first.CaseData, "writer_write_cases_generated.json")
	require.Contains(t, string(first.RoleCases["Reader"]), "type ReaderActions interface")
	require.NotContains(t, string(first.RoleCases["Reader"]), "type WriterActions interface")

	for name, source := range first.RoleCases {
		parseSource(t, name+"_conformance_cases_generated_test.go", source)
	}
	for name, source := range first.RoleConformances {
		parseSource(t, name+"_conformance_generated_test.go", source)
	}
	for name, source := range first.RoleScaffolds {
		parseSource(t, name+"_test.go", source)
	}
}

func TestRepeatedRolesKeepSourceNameForExclusions(t *testing.T) {
	writer := fizzbee.RoleState{Name: "Writer", Params: fizzbee.JSONObject{}, Fields: fizzbee.JSONObject{"op": map[string]any{"state": "READY"}}}
	table := fizzbee.RoleState{Name: "Table", Params: fizzbee.JSONObject{}, Fields: fizzbee.JSONObject{"result": "READY"}}
	roleStates := []fizzbee.RoleState{writer, writer, table}
	cases := []fizzbee.ActionCase{{Action: fizzbee.ActionRef{Role: "Table", Action: "Append"}, Input: fizzbee.Input{Roles: roleStates}, Outputs: []fizzbee.Output{{Roles: roleStates}}}}
	data, err := BuildTemplateData("example.test/project", "Table", fizzbee.Model{Cases: cases}, Declarations{Unobservable: []UnobservableField{{Field: mustField(t, "Writer.op"), Reason: "internal"}}})
	require.NoError(t, err)
	require.Equal(t, "Writer1", data.Roles[0].Name)
	require.Equal(t, "Writer2", data.Roles[1].Name)
	require.Equal(t, "Writer", data.Roles[0].SourceName)
	require.Equal(t, "Writer", data.Roles[1].SourceName)
	require.True(t, data.Roles[0].Fields[0].Unobservable)
	require.True(t, data.Roles[1].Fields[0].Unobservable)
}

func TestTypeOnlyDeclarationIsGeneratedOnceInCommonFile(t *testing.T) {
	cases := []fizzbee.ActionCase{newRendererActionCase("Reader", "Read"), newRendererActionCase("Reader", "Refresh")}
	declarations := Declarations{TypedFields: []TypedField{{Field: mustField(t, "Reader.status"), Type: Type{Kind: KindString}}}}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: cases}, declarations)
	require.NoError(t, err)
	common := string(sources.Common)
	require.Contains(t, common, "type ReaderReaderStatusValue string")
	require.Equal(t, 1, strings.Count(common, "type ReaderReaderStatusValue"))
	require.NotContains(t, string(sources.RoleCases["Reader"]), "type ReaderReaderStatusValue")
	require.Contains(t, string(sources.RoleCases["Reader"]), "Status ReaderReaderStatusValue")
	parseSource(t, "conformance_common_generated_test.go", sources.Common)
}

func TestGenerateSharesStateTypesAcrossActions(t *testing.T) {
	cases := []fizzbee.ActionCase{
		newRendererActionCase("Reader", "Read"),
		newRendererActionCase("Reader", "Refresh"),
	}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: cases}, Declarations{})
	require.NoError(t, err)

	caseSource := string(sources.RoleCases["Reader"])
	assert.Equal(t, 1, strings.Count(caseSource, "type FizzbeeReaderState struct"))
	assert.Contains(t, caseSource, "func (e *ReaderConformanceEnvironment) ExecuteRead")
	assert.Contains(t, caseSource, "func (e *ReaderConformanceEnvironment) ExecuteRefresh")
	assert.Contains(t, caseSource, "func (e *ReaderConformanceEnvironment) ObserveReadState")
	assert.Equal(t, 1, strings.Count(caseSource, "func (e *ReaderConformanceEnvironment) BuildAllowedReadState("))
	assert.Equal(t, 1, strings.Count(caseSource, "func (e *ReaderConformanceEnvironment) BuildAllowedRefreshState("))
	assert.Contains(t, caseSource, "Status: gcmp.Equal(allowed.Fields.Reader.Status)")
	assert.Contains(t, caseSource, "func (e *ReaderConformanceEnvironment) BuildAllowedRefreshStates")
	conformanceSource := string(sources.RoleConformances["Reader"])
	assert.Contains(t, conformanceSource, "func TestReader_Read_Conformance")
	assert.Contains(t, conformanceSource, "func TestReader_Refresh_Conformance")
	roleSource := string(sources.RoleScaffolds["Reader"])
	assert.Equal(t, 1, strings.Count(caseSource, "type ReaderFetchedObservation struct"))
	assert.Equal(t, 1, strings.Count(caseSource, "type ReaderManualObservation struct"))
	assert.NotContains(t, caseSource, "type ReaderObservedParams")
	assert.NotContains(t, caseSource, "type ReaderComparisonParams")
	assert.Equal(t, 1, strings.Count(roleSource, "func (e *ReaderEnvironment) BuildReadManualObservation"))
	assert.Equal(t, 1, strings.Count(roleSource, "func (e *ReaderEnvironment) BuildRefreshManualObservation"))
	assert.NotContains(t, roleSource, "ToAllowedObservedState")
	assert.NotContains(t, roleSource, "BuildAllowedReadState")
	assert.NotContains(t, roleSource, "BuildAllowedRefreshState")
}

func TestGeneratedObserverFieldUsesOneConstant(t *testing.T) {
	cases := []fizzbee.ActionCase{
		newRendererActionCase("Reader", "Read"),
		newRendererActionCase("Reader", "Refresh"),
	}
	observers := []Observer{
		{
			Field:            mustField(t, "Reader.status"),
			ObserverID:       "state",
			PluginImportPath: "example.test/observer",
			PluginAlias:      "stateobserver",
			ValueKind:        "string",
		},
	}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: cases}, Declarations{Observers: observers})
	require.NoError(t, err)
	source := string(sources.RoleCases["Reader"])
	common := string(sources.Common)
	require.Equal(t, 1, strings.Count(common, "_sgFieldReaderStatus = \"Reader.status\""))
	require.Equal(t, 1, strings.Count(common, "func _sgObserverStateSettings()"))
	require.NotContains(t, source, "\"Reader.status\"")
	require.Contains(t, source, "_sgObserverStateSettings()")
	parseSource(t, "reader_conformance_cases_generated_test.go", sources.RoleCases["Reader"])
	parseSource(t, "conformance_common_generated_test.go", sources.Common)
}

func TestGenerateSharedObserverFieldAcrossRoles(t *testing.T) {
	cases := []fizzbee.ActionCase{
		newRendererActionCase("Reader", "Read"),
		newRendererActionCase("Writer", "Write"),
	}
	for index := range cases {
		database := fizzbee.RoleState{Name: "DB", Fields: fizzbee.JSONObject{"records": map[string]any{"key": "value"}}}
		cases[index].Input.Roles = append(cases[index].Input.Roles, database)
		cases[index].Outputs[0].Roles = append(cases[index].Outputs[0].Roles, database)
	}
	declarations := Declarations{Observers: []Observer{{
		Field: mustField(t, "DB.records"), ObserverID: "db", PluginImportPath: "example.test/observer", PluginAlias: "dbobserver", ValueKind: "message",
		ObserverConfig: map[string]any{"endpoint": "example"}, FieldConfig: map[string]any{"query": "SELECT records"},
		Type: Type{Kind: KindMap, Values: &Type{Kind: KindMessage, Fields: map[string]Type{
			"name": {Kind: KindString},
		}}},
	}}}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: cases}, declarations)
	require.NoError(t, err)
	common := string(sources.Common)
	require.Equal(t, 1, strings.Count(common, "_sgFieldDBRecords = \"DB.records\""))
	require.Equal(t, 1, strings.Count(common, "func _sgObserverDbSettings()"))
	require.Equal(t, 1, strings.Count(common, "\"endpoint\": \"example\""))
	require.Equal(t, 1, strings.Count(common, "\"query\": \"SELECT records\""))
	require.Contains(t, common, "type DBRecordsField struct")
	require.Contains(t, common, "Name string `json:\"name\"`")
	for role, source := range sources.RoleCases {
		require.Contains(t, string(source), "_sgObserverDbSettings()")
		require.NotContains(t, string(source), "\"endpoint\": \"example\"")
		require.NotContains(t, string(source), "\"query\": \"SELECT records\"")
		require.Contains(t, string(source), "Convert[map[string]DBRecordsField](valueDBRecords)")
		require.Contains(t, string(source), "Records: typedDBRecords,")
		require.NotContains(t, string(source), "fetched.Fields.DB.Records = typedDBRecords")
		require.Contains(t, string(source), "_sgToDBRecordsObservedField(fetched.Fields.DB.Records)")
		require.Contains(t, string(source), "_sgToDBRecordsComparisonField(allowed.Fields.DB.Records)")
		require.Equal(t, map[string]string{"Records": "map[string]DBRecordsField"}, generatedStructFields(t, string(source), role+"DBFetchedField"))
		require.NotContains(t, string(source), "type ReaderDBRecordsValue struct")
	}
}

func TestGenerateListObserverUsesDeclaredFieldType(t *testing.T) {
	caseData := newRendererActionCase("Reader", "Read")
	files := []any{map[string]any{"file_path": "first", "rows": []any{[]any{"a", "b"}}}}
	caseData.Input.Roles = append(caseData.Input.Roles, fizzbee.RoleState{Name: "ObjectStore", Fields: fizzbee.JSONObject{"data_files": files}})
	caseData.Outputs[0].Roles = append(caseData.Outputs[0].Roles, fizzbee.RoleState{Name: "ObjectStore", Fields: fizzbee.JSONObject{"data_files": files}})
	declarations := Declarations{Observers: []Observer{{
		Field: mustField(t, "ObjectStore.data_files"), ObserverID: "files", PluginImportPath: "example.test/files", PluginAlias: "filesobserver",
		Type: Type{Kind: KindList, Values: &Type{Kind: KindMessage, Fields: map[string]Type{
			"file_path": {Kind: KindString, SkipComparison: true},
			"rows":      {Kind: KindList, Values: &Type{Kind: KindList, Values: &Type{Kind: KindString}}},
		}}},
	}}}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{caseData}}, declarations)
	require.NoError(t, err)
	roleSource := string(sources.RoleCases["Reader"])
	commonSource := string(sources.Common)
	require.Contains(t, roleSource, "Convert[ObjectStoreDataFilesValue](valueObjectStoreDataFiles)")
	require.Contains(t, roleSource, "DataFiles ObjectStoreDataFilesValue")
	require.Contains(t, roleSource, "DataFiles: _sgToObjectStoreDataFilesComparisonList(allowed.Fields.ObjectStore.DataFiles)")
	require.NotContains(t, commonSource, "gcmp.Ignore[string](\"compare: false\")")
	require.Contains(t, commonSource, "type ObjectStoreDataFilesValue []ObjectStoreDataFilesField")
	require.Equal(t, map[string]string{"Rows": "*gcmp.ObservedList[*gcmp.ObservedList[*gcmp.ObservedField[string]]]"}, generatedStructFields(t, commonSource, "ObjectStoreDataFilesObservedField"))
	parseSource(t, "reader_conformance_cases_generated_test.go", sources.RoleCases["Reader"])
	parseSource(t, "conformance_common_generated_test.go", sources.Common)
}

func TestGenerateMapObserverIgnoresNestedField(t *testing.T) {
	caseData := newRendererActionCase("Reader", "Read")
	records := map[string]any{"first": map[string]any{"name": "Alice", "source": "remote"}}
	caseData.Input.Roles = append(caseData.Input.Roles, fizzbee.RoleState{Name: "DB", Fields: fizzbee.JSONObject{"records": records}})
	caseData.Outputs[0].Roles = append(caseData.Outputs[0].Roles, fizzbee.RoleState{Name: "DB", Fields: fizzbee.JSONObject{"records": records}})
	declarations := Declarations{Observers: []Observer{{
		Field: mustField(t, "DB.records"), ObserverID: "db", PluginImportPath: "example.test/db", PluginAlias: "dbobserver", ValueKind: "message",
		Type: Type{Kind: KindMap, Values: &Type{Kind: KindMessage, Fields: map[string]Type{
			"name": {Kind: KindString}, "source": {Kind: KindString, SkipComparison: true},
		}}},
	}}}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{caseData}}, declarations)
	require.NoError(t, err)
	roleSource := string(sources.RoleCases["Reader"])
	require.Contains(t, roleSource, "Records: _sgToDBRecordsComparisonField(allowed.Fields.DB.Records)")
	require.NotContains(t, string(sources.Common), "gcmp.Ignore[string](\"compare: false\")")
	require.Contains(t, roleSource, "Records: _sgToDBRecordsObservedField(fetched.Fields.DB.Records)")
	parseSource(t, "reader_conformance_cases_generated_test.go", sources.RoleCases["Reader"])
}

func TestGenerateListComparison_IgnoresDeclaredOrder(t *testing.T) {
	tests := []struct {
		name     string
		observed bool
	}{
		{"observer field", true},
		{"manual field", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caseData := newRendererActionCase("Reader", "Read")
			files := []any{map[string]any{"file_path": "first", "rows": []any{[]any{"a", "b"}}}}
			caseData.Input.Roles = append(caseData.Input.Roles, fizzbee.RoleState{Name: "ObjectStore", Fields: fizzbee.JSONObject{"data_files": files}})
			caseData.Outputs[0].Roles = append(caseData.Outputs[0].Roles, fizzbee.RoleState{Name: "ObjectStore", Fields: fizzbee.JSONObject{"data_files": files}})
			output := Type{Kind: KindList, IgnoreOrder: true, Values: &Type{Kind: KindMessage, Fields: map[string]Type{
				"file_path": {Kind: KindString, SkipComparison: true},
				"rows":      {Kind: KindList, IgnoreOrder: true, Values: &Type{Kind: KindList, Values: &Type{Kind: KindString}}},
			}}}
			field := mustField(t, "ObjectStore.data_files")
			var declarations Declarations
			if tt.observed {
				declarations.Observers = []Observer{{Field: field, ObserverID: "files", PluginImportPath: "example.test/files", PluginAlias: "filesobserver", Type: output}}
			} else {
				declarations.TypedFields = []TypedField{{Field: field, Type: output}}
			}
			sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{caseData}}, declarations)
			require.NoError(t, err)
			require.Equal(t, 2, strings.Count(string(sources.Common), "return gcmp.ListUnordered(result)"))
			require.NotContains(t, string(sources.Common), "gcmp.Ignore[string](\"compare: false\")")
			require.NotContains(t, string(sources.RoleCases["Reader"]), "EqualIgnoring")
			require.NotContains(t, string(sources.Common), "IgnoreOrder:")
			parseSource(t, "reader_conformance_cases_generated_test.go", sources.RoleCases["Reader"])
		})
	}
}

func TestGenerateBuildsFetchedObservationAfterAllConversions(t *testing.T) {
	actionCase := newRendererActionCase("Reader", "Read")
	for _, role := range []fizzbee.RoleState{actionCase.Input.Roles[0], actionCase.Outputs[0].Roles[0]} {
		role.Fields["first"] = map[string]any{"one": "value"}
		role.Fields["second"] = map[string]any{"two": "value"}
	}
	declarations := Declarations{Observers: []Observer{
		{Field: mustField(t, "Reader.first"), ObserverID: "first", PluginImportPath: "example.test/observer", PluginAlias: "stateobserver", ValueKind: "string"},
		{Field: mustField(t, "Reader.second"), ObserverID: "second", PluginImportPath: "example.test/observer", PluginAlias: "stateobserver", ValueKind: "string"},
	}}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{actionCase}}, declarations)
	require.NoError(t, err)
	source := string(sources.RoleCases["Reader"])

	first := strings.Index(source, "typedReaderFirst, convertErr :=")
	second := strings.Index(source, "typedReaderSecond, convertErr :=")
	fetched := strings.Index(source, "fetched := ReaderFetchedObservation{")
	require.Greater(t, first, -1)
	require.Greater(t, second, first)
	require.Greater(t, fetched, second)
	require.Contains(t, source, "First:  typedReaderFirst,")
	require.Contains(t, source, "Second: typedReaderSecond,")
	require.NotContains(t, source, "fetched.Fields.Reader.First =")
	require.NotContains(t, source, "fetched.Fields.Reader.Second =")
}

func TestUnobservableFieldRemainsInModelButNotComparison(t *testing.T) {
	read := newRendererActionCase("Reader", "Read")
	read.Input.Roles[0].Fields["history"] = map[string]any{"previous": "value"}
	read.Outputs[0].Roles[0].Fields["history"] = map[string]any{"previous": "value"}
	read.Input.Roles[0].Fields["records"] = map[string]any{"key": "value"}
	read.Outputs[0].Roles[0].Fields["records"] = map[string]any{"key": "value"}
	refresh := newRendererActionCase("Reader", "Refresh")
	refresh.Input.Roles[0].Fields["history"] = map[string]any{"previous": "value"}
	refresh.Outputs[0].Roles[0].Fields["history"] = map[string]any{"previous": "value"}
	refresh.Input.Roles[0].Fields["records"] = map[string]any{"key": "value"}
	refresh.Outputs[0].Roles[0].Fields["records"] = map[string]any{"key": "value"}

	declarations := Declarations{
		Observers:    []Observer{{Field: mustField(t, "Reader.records"), ObserverID: "store", PluginImportPath: "example.test/observer", PluginAlias: "storeobserver", ValueKind: "string"}},
		Unobservable: []UnobservableField{{Field: mustField(t, "Reader.history"), Reason: "not retained"}},
	}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{read, refresh}}, declarations)
	if err != nil {
		t.Fatal(err)
	}
	source := string(sources.RoleCases["Reader"])
	for _, test := range []struct {
		name string
		want map[string]string
	}{
		{"ReaderReaderField", map[string]string{"History": "map[string]any", "Records": "map[string]string", "Status": "string"}},
		{"ReaderReaderFetchedField", map[string]string{"Records": "map[string]string"}},
		{"ReaderReaderManualField", map[string]string{"Status": "*gcmp.ObservedField[string]"}},
		{"ReaderReaderObservedField", map[string]string{"Records": "*gcmp.ObservedField[map[string]string]", "Status": "*gcmp.ObservedField[string]"}},
		{"ReaderReaderComparisonField", map[string]string{"Records": "*gcmp.ComparisonField[map[string]string]", "Status": "*gcmp.ComparisonField[string]"}},
	} {
		if diff := cmp.Diff(test.want, generatedStructFields(t, source, test.name)); diff != "" {
			t.Errorf("%s fields mismatch (-want +got):\n%s", test.name, diff)
		}
	}
	for _, unwanted := range []string{"manual.Fields.Reader.History", "Unobservable[map[string]any]", "Ignore[map[string]any]"} {
		if strings.Contains(source, unwanted) {
			t.Errorf("generated source contains %q", unwanted)
		}
	}
	require.Contains(t, source, "Records: gcmp.Equal(allowed.Fields.Reader.Records)")
	require.Contains(t, source, "Status:  gcmp.Equal(allowed.Fields.Reader.Status)")
	require.NotContains(t, source, "allowed.Fields.Reader.History")
	state := func(status string) map[string]any {
		return map[string]any{
			"params": map[string]any{"Reader": map[string]any{"key": float64(1)}},
			"fields": map[string]any{"Reader": map[string]any{
				"history": map[string]any{"previous": "value"},
				"records": map[string]any{"key": "value"},
				"status":  status,
			}},
		}
	}
	allowedState := state("DONE")
	allowedState["sg_metadata"] = map[string]any{"action_crashed": false}
	wantCases := []map[string]any{{"name": "case1", "input": state("INIT"), "allowed": []any{allowedState}}}
	for _, action := range []string{"read", "refresh"} {
		var gotCases []map[string]any
		name := "reader_" + action + "_cases_generated.json"
		if err := json.Unmarshal(sources.CaseData[name], &gotCases); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		if diff := cmp.Diff(wantCases, gotCases); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", name, diff)
		}
	}
}

func TestGenerateOmitsRolesWithoutManualFields(t *testing.T) {
	read := newRendererActionCase("Reader", "Read")
	read.Input.Roles = append(read.Input.Roles,
		fizzbee.RoleState{Name: "DB", Fields: fizzbee.JSONObject{"records": map[string]any{"key": "value"}}},
		fizzbee.RoleState{Name: "Cache", Fields: fizzbee.JSONObject{"cached_records": map[string]any{"key": "value"}}},
	)
	read.Outputs[0].Roles = append(read.Outputs[0].Roles,
		fizzbee.RoleState{Name: "DB", Fields: fizzbee.JSONObject{"records": map[string]any{"key": "value"}}},
		fizzbee.RoleState{Name: "Cache", Fields: fizzbee.JSONObject{"cached_records": map[string]any{"key": "value"}}},
	)
	declarations := Declarations{
		Observers: []Observer{
			{Field: mustField(t, "DB.records"), ObserverID: "db", PluginImportPath: "example.test/observer", PluginAlias: "dbobserver", ValueKind: "string"},
			{Field: mustField(t, "Cache.cached_records"), ObserverID: "cache", PluginImportPath: "example.test/observer", PluginAlias: "cacheobserver", ValueKind: "string"},
		},
	}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{read}}, declarations)
	require.NoError(t, err)

	source := string(sources.RoleCases["Reader"])
	require.Equal(t, map[string]string{"Reader": "ReaderReaderManualField"}, generatedStructFields(t, source, "ReaderManualField"))
	require.Equal(t, map[string]string{"Status": "*gcmp.ObservedField[string]"}, generatedStructFields(t, source, "ReaderReaderManualField"))
	require.NotContains(t, source, "type ReaderDBManualField struct")
	require.NotContains(t, source, "type ReaderCacheManualField struct")
}

func TestGenerateOmitsEmptyObservationAndComparisonFields(t *testing.T) {
	tests := []struct {
		name     string
		excluded bool
	}{
		{name: "no model fields"},
		{name: "all fields excluded", excluded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read := newRendererActionCase("Reader", "Read")
			declarations := Declarations{}
			if tt.excluded {
				declarations.Unobservable = []UnobservableField{{Field: mustField(t, "Reader.status"), Reason: "internal state"}}
			} else {
				read.Input.Roles[0].Fields = fizzbee.JSONObject{}
				read.Outputs[0].Roles[0].Fields = fizzbee.JSONObject{}
			}
			sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{read}}, declarations)
			require.NoError(t, err)
			source := string(sources.RoleCases["Reader"])
			for _, name := range []string{"FetchedObservation", "ManualObservation"} {
				require.Empty(t, generatedStructFields(t, source, "Reader"+name))
			}
			require.Equal(t, map[string]string{"SgMetadata": "guardian.SgObservedMetadata"}, generatedStructFields(t, source, "ReaderObservedState"))
			require.Equal(t, map[string]string{"SgMetadata": "guardian.SgComparisonMetadata"}, generatedStructFields(t, source, "ReaderComparisonState"))
			for _, name := range []string{"ObservedField", "ComparisonField", "FetchedField", "ManualField"} {
				require.NotContains(t, source, "type Reader"+name+" struct")
				require.NotContains(t, source, "type ReaderReader"+name+" struct")
			}
			require.NotContains(t, source, "manual.Fields")
			require.Contains(t, source, "gcmp.Ignore[bool]")
			if tt.excluded {
				require.Equal(t, map[string]string{"Status": "string"}, generatedStructFields(t, source, "ReaderReaderField"))
			}
		})
	}
}

func TestGenerateOmitsEmptyManualAndRoleFields(t *testing.T) {
	read := newRendererActionCase("Reader", "Read")
	read.Input.Roles[0].Fields = fizzbee.JSONObject{"records": map[string]any{"key": "value"}}
	read.Outputs[0].Roles[0].Fields = fizzbee.JSONObject{"records": map[string]any{"key": "value"}}
	read.Input.Roles = append(read.Input.Roles, fizzbee.RoleState{Name: "Internal", Fields: fizzbee.JSONObject{"status": "INIT"}})
	read.Outputs[0].Roles = append(read.Outputs[0].Roles, fizzbee.RoleState{Name: "Internal", Fields: fizzbee.JSONObject{"status": "DONE"}})
	declarations := Declarations{
		Observers:    []Observer{{Field: mustField(t, "Reader.records"), ObserverID: "store", PluginImportPath: "example.test/observer", PluginAlias: "storeobserver", ValueKind: "string"}},
		Unobservable: []UnobservableField{{Field: mustField(t, "Internal.status"), Reason: "internal state"}},
	}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{read}}, declarations)
	require.NoError(t, err)
	source := string(sources.RoleCases["Reader"])
	require.Empty(t, generatedStructFields(t, source, "ReaderManualObservation"))
	require.NotContains(t, source, "type ReaderManualField struct")
	for _, name := range []string{"ObservedField", "ComparisonField", "FetchedField"} {
		require.Equal(t, map[string]string{"Reader": "ReaderReader" + name}, generatedStructFields(t, source, "Reader"+name))
		require.NotContains(t, source, "type ReaderInternal"+name+" struct")
		require.NotContains(t, source, "Internal: ReaderInternal"+name)
	}
}

func generatedStructFields(t *testing.T, source, name string) map[string]string {
	t.Helper()
	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, "generated.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != name {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("generated type %s is not a struct", name)
			}
			fields := make(map[string]string, len(structure.Fields.List))
			for _, field := range structure.Fields.List {
				var formatted bytes.Buffer
				if err := format.Node(&formatted, fileset, field.Type); err != nil {
					t.Fatal(err)
				}
				for _, fieldName := range field.Names {
					fields[fieldName.Name] = formatted.String()
				}
			}
			return fields
		}
	}
	t.Fatalf("generated type %s is missing", name)
	return nil
}

func TestGenerateRejectsDifferentRoleSchemasAcrossActions(t *testing.T) {
	read := newRendererActionCase("Reader", "Read")
	refresh := newRendererActionCase("Reader", "Refresh")
	refresh.Input.Roles[0].Fields = fizzbee.JSONObject{"other": "INIT"}
	refresh.Outputs[0].Roles[0].Fields = fizzbee.JSONObject{"other": "DONE"}

	_, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{read, refresh}}, Declarations{})
	require.ErrorContains(t, err, "different role schema")
}

func newRendererActionCase(role, action string) fizzbee.ActionCase {
	input := fizzbee.RoleState{Name: role, Params: fizzbee.JSONObject{"key": json.Number("1")}, Fields: fizzbee.JSONObject{"status": "INIT"}}
	output := fizzbee.RoleState{Name: role, Params: fizzbee.JSONObject{"key": json.Number("1")}, Fields: fizzbee.JSONObject{"status": "DONE"}}
	return fizzbee.ActionCase{
		Action:  fizzbee.ActionRef{Role: role, Action: action},
		Input:   fizzbee.Input{Roles: []fizzbee.RoleState{input}},
		Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{output}}},
	}
}

func parseSource(t *testing.T, name string, source []byte) {
	t.Helper()
	_, err := parser.ParseFile(token.NewFileSet(), name, source, parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerateListObserver_NullableAndWholeContainerIgnore(t *testing.T) {
	caseData := newRendererActionCase("Reader", "Read")
	caseData.Input.Roles = append(caseData.Input.Roles, fizzbee.RoleState{Name: "ObjectStore", Fields: fizzbee.JSONObject{"records": []any{}}})
	caseData.Outputs[0].Roles = append(caseData.Outputs[0].Roles, fizzbee.RoleState{Name: "ObjectStore", Fields: fizzbee.JSONObject{"records": []any{}}})
	declarations := Declarations{Observers: []Observer{{
		Field: mustField(t, "ObjectStore.records"), ObserverID: "records", PluginImportPath: "example.test/records", PluginAlias: "recordsobserver",
		Type: Type{Kind: KindList, Values: &Type{Kind: KindMessage, Fields: map[string]Type{
			"snapshot": {Kind: KindMessage, Nullable: true, Fields: map[string]Type{"id": {Kind: KindInt64, Nullable: true}}},
			"detail":   {Kind: KindMessage, SkipComparison: true, Fields: map[string]Type{"name": {Kind: KindString}}},
			"rows":     {Kind: KindList, SkipComparison: true, Values: &Type{Kind: KindString}},
		}}},
	}}}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{caseData}}, declarations)
	require.NoError(t, err)
	common := string(sources.Common)
	require.Equal(t, map[string]string{
		"Snapshot": "*ObjectStoreRecordsSnapshotField",
		"Detail":   "ObjectStoreRecordsDetailField",
		"Rows":     "[]string",
	}, generatedStructFields(t, common, "ObjectStoreRecordsField"))
	require.Equal(t, map[string]string{
		"Snapshot": "*gcmp.ObservedMessage[ObjectStoreRecordsSnapshotObservedField]",
	}, generatedStructFields(t, common, "ObjectStoreRecordsObservedField"))
	require.Equal(t, map[string]string{"Id": "*gcmp.ObservedField[*int64]"}, generatedStructFields(t, common, "ObjectStoreRecordsSnapshotObservedField"))
	require.Contains(t, common, "if value == nil {\n\t\treturn gcmp.MessageObserved[ObjectStoreRecordsSnapshotObservedField](nil)\n\t}")
	require.Contains(t, common, "if value == nil {\n\t\treturn gcmp.MessageEqual[ObjectStoreRecordsSnapshotComparisonField](nil)\n\t}")
	require.NotContains(t, common, "ObjectStoreRecordsDetailComparisonField")
	require.NotContains(t, common, "_sgToObjectStoreRecordsRowsObservedList")
	require.NotContains(t, string(sources.RoleCases["Reader"]), "EqualIgnoring")
	parseSource(t, "conformance_common_generated_test.go", sources.Common)
}
