package renderer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
	"github.com/stretchr/testify/require"
)

func TestGenerateScalarObserver_ObservesAndComparesDeclaredTypes(t *testing.T) {
	messageFields := map[string]Type{"cnt": {Kind: KindInt64}, "source": {Kind: KindString, SkipComparison: true}}
	tests := []struct {
		name   string
		spec   Type
		value  any
		goType string
	}{
		{"Bool", Type{Kind: KindBool}, true, "bool"},
		{"Double", Type{Kind: KindDouble}, json.Number("1.5"), "float64"},
		{"Bytes", Type{Kind: KindBytes}, "AP8=", "[]byte"},
		{"NullableBool", Type{Kind: KindBool, Nullable: true}, nil, "*bool"},
		{"NullableDouble", Type{Kind: KindDouble, Nullable: true}, nil, "*float64"},
		{"NullableBytes", Type{Kind: KindBytes, Nullable: true}, nil, "*[]byte"},
		{"NullableMap", Type{Kind: KindMap, Nullable: true, Values: &Type{Kind: KindDouble}}, nil, "*map[string]float64"},
		{"NullableList", Type{Kind: KindList, Nullable: true, IgnoreOrder: true, Values: &Type{Kind: KindBool}}, nil, "*ReaderNullableListValue"},
		{"Nested", Type{Kind: KindMessage, Fields: map[string]Type{
			"rows":    {Kind: KindMap, Nullable: true, Values: &Type{Kind: KindList, Nullable: true, IgnoreOrder: true, Values: &Type{Kind: KindBytes}}},
			"ignored": {Kind: KindDouble, SkipComparison: true},
		}}, map[string]any{"rows": nil, "ignored": json.Number("1")}, "ReaderNestedValue"},
		{"String", Type{Kind: KindString}, "ready", "string"},
		{"Int", Type{Kind: KindInt64}, json.Number("9007199254740993"), "int64"},
		{"NullableString", Type{Kind: KindString, Nullable: true}, nil, "*string"},
		{"NullableInt", Type{Kind: KindInt64, Nullable: true}, nil, "*int64"},
		{"Message", Type{Kind: KindMessage, Fields: messageFields}, map[string]any{"cnt": json.Number("1"), "source": "model"}, "ReaderMessageValue"},
		{"NullableMessage", Type{Kind: KindMessage, Nullable: true, Fields: messageFields}, nil, "*ReaderNullableMessageValue"},
	}
	fields := fizzbee.JSONObject{}
	var declarations Declarations
	for _, tt := range tests {
		fields[tt.name] = tt.value
		declarations.Observers = append(declarations.Observers, Observer{Field: mustField(t, "Reader."+tt.name), ObserverID: "custom", PluginImportPath: "example.test/generated/custom", PluginAlias: "custom", Type: tt.spec})
	}
	role := fizzbee.RoleState{Name: "Reader", Fields: fields}
	cases := []fizzbee.ActionCase{{Action: fizzbee.ActionRef{Role: "Reader", Action: "Read"}, Input: fizzbee.Input{Roles: []fizzbee.RoleState{role}}, Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{role}}}}}
	sources, err := GenerateScaffold("example.test/generated", cases, declarations)
	require.NoError(t, err)
	repeated, err := GenerateScaffold("example.test/generated", cases, declarations)
	require.NoError(t, err)
	require.Equal(t, sources, repeated)
	for _, tt := range tests {
		require.Contains(t, string(sources.RoleCases["Reader"]), "Convert["+tt.goType+"]")
	}
	dir := t.TempDir()
	guardianDir, err := filepath.Abs("../../../guardian")
	require.NoError(t, err)
	module := fmt.Sprintf("module example.test/generated\n\ngo 1.27.0\n\nrequire github.com/mrasu/spec-guardian/guardian v0.0.0\nreplace github.com/mrasu/spec-guardian/guardian => %s\n", filepath.ToSlash(guardianDir))
	scaffold := string(sources.RoleScaffolds["Reader"])
	scaffold = strings.ReplaceAll(scaffold, `require.NoError(t, errors.New("TODO: implement me"))`, "")
	scaffold = strings.ReplaceAll(scaffold, `"errors"`, "")
	scaffold = strings.ReplaceAll(scaffold, `"github.com/stretchr/testify/require"`, "")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "custom"), 0700))
	files := map[string][]byte{
		"go.mod":         []byte(module),
		"common_test.go": sources.Common,
		"cases_test.go":  sources.RoleCases["Reader"],
		"impl_test.go":   []byte(scaffold),
		"custom/observer.go": []byte(`package custom
import ("context"; "github.com/mrasu/spec-guardian/guardian/observer")
type Config struct{}
type Observer struct { Values map[string]any }
func New(Config, map[string]any, []observer.FieldRequest) (*Observer, error) { return &Observer{}, nil }
func (o *Observer) Observe(ctx context.Context, field string) (any, error) { return o.Values[field], nil }
func (*Observer) PrepareCase(context.Context) error { return nil }
func (*Observer) PrepareAttempt(context.Context) error { return nil }
func (*Observer) CleanupCase(context.Context) error { return nil }
func (*Observer) CleanupAttempt(context.Context) error { return nil }
`),
		"runtime_test.go": []byte(`package specguardian_test
import (
 "encoding/json"
 "strings"
 "testing"
 "example.test/generated/custom"
 "github.com/mrasu/spec-guardian/guardian/gcmp"
 "github.com/mrasu/spec-guardian/guardian/observer"
)
func TestObserveScalarAndMessageFields(t *testing.T) {
 tests := []struct { name string; nullable bool }{{"null", false}, {"non-null", true}, {"empty", false}}
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   values := map[string]any{
    "Reader.String": "ready", "Reader.Int": int64(9007199254740993),
    "Reader.Bool": true, "Reader.Double": float64(1.5), "Reader.Bytes": []byte{0,255},
    "Reader.NullableBool": nil, "Reader.NullableDouble": nil, "Reader.NullableBytes": nil,
    "Reader.NullableMap": nil, "Reader.NullableList": nil,
    "Reader.Nested": map[string]any{"rows": map[string]any{"null": nil, "empty": []any{}, "values": []any{[]byte{1}, []byte{2}}}, "ignored": float64(2)},
    "Reader.NullableString": nil, "Reader.NullableInt": nil,
    "Reader.Message": map[string]any{"cnt": int64(1), "source": "actual"},
    "Reader.NullableMessage": nil,
   }
   model := map[string]any{"String": "ready", "Int": int64(9007199254740993), "NullableString": nil, "NullableInt": nil, "Message": map[string]any{"cnt": int64(1), "source": "model"}, "NullableMessage": nil}
   model["Bool"], model["Double"], model["Bytes"] = true, float64(1.5), "AP8="
   model["NullableBool"], model["NullableDouble"], model["NullableBytes"] = nil, nil, nil
   model["NullableMap"], model["NullableList"] = nil, nil
   model["Nested"] = map[string]any{"rows": map[string]any{"null": nil, "empty": []any{}, "values": []any{"Ag==", "AQ=="}}, "ignored": float64(1)}
   if tt.name == "empty" {
    values["Reader.NullableBytes"], model["NullableBytes"] = []byte{}, ""
    values["Reader.NullableMap"], model["NullableMap"] = map[string]any{}, map[string]any{}
    values["Reader.NullableList"], model["NullableList"] = []bool{}, []bool{}
   }
   if tt.nullable {
    values["Reader.NullableBool"], model["NullableBool"] = false, false
    values["Reader.NullableDouble"], model["NullableDouble"] = float64(2.5), float64(2.5)
    values["Reader.NullableBytes"], model["NullableBytes"] = []byte{1}, "AQ=="
    values["Reader.NullableMap"], model["NullableMap"] = map[string]any{"a": float64(3)}, map[string]any{"a": float64(3)}
    values["Reader.NullableList"], model["NullableList"] = []bool{false, true}, []bool{true, false}
    values["Reader.NullableString"], model["NullableString"] = "text", "text"
    values["Reader.NullableInt"], model["NullableInt"] = int64(3), int64(3)
    values["Reader.NullableMessage"] = map[string]any{"cnt": int64(4), "source": "actual"}
    model["NullableMessage"] = map[string]any{"cnt": int64(4), "source": "model"}
   }
   data, err := json.Marshal(map[string]any{"fields": map[string]any{"Reader": model}})
   if err != nil { t.Fatal(err) }
   var allowed FizzbeeReaderState
   if err := json.Unmarshal(data, &allowed); err != nil { t.Fatal(err) }
   e := &ReaderConformanceEnvironment{ReaderEnvironment: &ReaderEnvironment{}, observerCustom: &custom.Observer{Values: values}}
   observed := e.ObserveReadState(t, FizzbeeReaderState{}, ReaderReadOutput{}, nil)
   comparison := e.BuildAllowedReadState(t, allowed)
   comparator := gcmp.NewObservedStateComparator[*ReaderComparisonState](observed.Output)
   if err := comparator.Validate(); err != nil { t.Fatal(err) }
   if diffs := comparator.Match(&comparison); len(diffs) != 0 { t.Fatal(diffs) }
   mismatches := []struct { name string; value any }{
    {"String", "different"}, {"Int", int64(9007199254740992)},
    {"Bool", false}, {"Double", float64(2.5)}, {"Bytes", []byte{0,254}},
    {"NullableBool", true}, {"NullableDouble", float64(0)}, {"NullableBytes", []byte{2}},
    {"Nested", map[string]any{"rows": map[string]any{"null": []any{}, "empty": []any{}, "values": []any{[]byte{1}, []byte{2}}}, "ignored": float64(2)}},
    {"NullableString", "different"}, {"NullableInt", int64(0)},
    {"Message", map[string]any{"cnt": int64(2), "source": "actual"}},
    {"NullableMessage", map[string]any{"cnt": int64(5), "source": "actual"}},
   }
   if tt.name == "null" {
    mismatches = append(mismatches, struct {name string; value any}{"NullableMap", map[string]any{}}, struct {name string; value any}{"NullableList", []bool{}})
   } else {
    mismatches = append(mismatches, struct {name string; value any}{"NullableMap", nil}, struct {name string; value any}{"NullableList", nil})
   }
   for _, mismatch := range mismatches {
    original := values["Reader."+mismatch.name]
    values["Reader."+mismatch.name] = mismatch.value
    got := e.ObserveReadState(t, FizzbeeReaderState{}, ReaderReadOutput{}, nil)
    diffs := gcmp.NewObservedStateComparator[*ReaderComparisonState](got.Output).Match(&comparison)
    if len(diffs) == 0 || !strings.Contains(strings.Join(diffs, "\n"), mismatch.name) { t.Fatalf("%s mismatch: %v", mismatch.name, diffs) }
    values["Reader."+mismatch.name] = original
   }
  })
 }
}
func TestConversionRejectsInvalidDeclaredValues(t *testing.T) {
 if got, err := observer.Convert[int64](float64(1)); err == nil || got != 0 { t.Fatalf("got %v, error %v", got, err) }
 if got, err := observer.Convert[string](nil); err == nil || got != "" { t.Fatalf("got %v, error %v", got, err) }
 if got, err := observer.Convert[*int64]("1"); err == nil || got != nil { t.Fatalf("got %v, error %v", got, err) }
 tests := []any{nil, map[string]any{"cnt": int64(1)}, map[string]any{"cnt": int64(1), "source": "x", "extra": "x"}, map[string]any{"cnt": float64(1), "source": "x"}}
 for _, value := range tests {
  got, err := observer.Convert[ReaderMessageValue](value)
  if err == nil || got != (ReaderMessageValue{}) { t.Fatalf("got %v, error %v", got, err) }
 }
}
`),
	}
	for name, source := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), source, 0600))
	}
	command := exec.Command("go", "test", "-mod=mod", "-tags=specguardian", "./...")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}
