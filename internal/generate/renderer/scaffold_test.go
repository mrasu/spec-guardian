package renderer

import (
	"encoding/json"
	"flag"
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
	sources, err := GenerateScaffold("example.test/project", []fizzbee.ActionCase{{Action: fizzbee.ActionRef{Role: "Reader", Action: "Read"}, Input: fizzbee.Input{Roles: []fizzbee.RoleState{role}}, Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{outputRole}}}}})
	require.NoError(t, err)
	return sources
}

func TestGenerateProducesParseableDeterministicSources(t *testing.T) {
	cases := []fizzbee.ActionCase{
		newRendererActionCase("Reader", "Read"),
		newRendererActionCase("Writer", "Write"),
	}
	first, err := GenerateScaffold("example.test/project", cases)
	require.NoError(t, err)
	second, err := GenerateScaffold("example.test/project", cases)
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

func TestGenerateSharesStateTypesAcrossActions(t *testing.T) {
	cases := []fizzbee.ActionCase{
		newRendererActionCase("Reader", "Read"),
		newRendererActionCase("Reader", "Refresh"),
	}
	sources, err := GenerateScaffold("example.test/project", cases)
	require.NoError(t, err)

	caseSource := string(sources.RoleCases["Reader"])
	assert.Equal(t, 1, strings.Count(caseSource, "type FizzbeeReaderState struct"))
	assert.Contains(t, caseSource, "func (e *ReaderConformanceEnvironment) ExecuteRead")
	assert.Contains(t, caseSource, "func (e *ReaderConformanceEnvironment) ExecuteRefresh")
	assert.Contains(t, caseSource, "func (e *ReaderConformanceEnvironment) ObserveReadState")
	assert.Contains(t, caseSource, "func (e *ReaderConformanceEnvironment) BuildAllowedRefreshStates")
	conformanceSource := string(sources.RoleConformances["Reader"])
	assert.Contains(t, conformanceSource, "func TestReader_Read_Conformance")
	assert.Contains(t, conformanceSource, "func TestReader_Refresh_Conformance")
	roleSource := string(sources.RoleScaffolds["Reader"])
	assert.Equal(t, 1, strings.Count(roleSource, "func (e *ReaderEnvironment) ObserveCurrentState"))
	assert.NotContains(t, roleSource, "ToAllowedObservedState")
	assert.Equal(t, 1, strings.Count(roleSource, "func (e *ReaderEnvironment) BuildAllowedReadState"))
	assert.Equal(t, 1, strings.Count(roleSource, "func (e *ReaderEnvironment) BuildAllowedRefreshState"))
}

func TestGenerateRejectsDifferentRoleSchemasAcrossActions(t *testing.T) {
	read := newRendererActionCase("Reader", "Read")
	refresh := newRendererActionCase("Reader", "Refresh")
	refresh.Input.Roles[0].Fields = fizzbee.JSONObject{"other": "INIT"}
	refresh.Outputs[0].Roles[0].Fields = fizzbee.JSONObject{"other": "DONE"}

	_, err := GenerateScaffold("example.test/project", []fizzbee.ActionCase{read, refresh})
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
