package renderer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
	"github.com/stretchr/testify/require"
)

func TestGenerateMapProvider_ConvertsAndComparesNestedValues(t *testing.T) {
	tests := []struct {
		name                      string
		values                    Type
		actual, allowed, mismatch string
	}{
		{"Booleans", Type{Kind: KindList, IgnoreOrder: true, Values: &Type{Kind: KindBool}}, `map[string]any{"a": []bool{true, false}}`, `map[string]any{"a": []bool{false, true}}`, `map[string]any{"a": []bool{true, true}}`},
		{"Doubles", Type{Kind: KindMap, Values: &Type{Kind: KindDouble}}, `map[string]any{"a": map[string]any{"b": float64(1.5)}}`, `map[string]any{"a": map[string]any{"b": float64(1.5)}}`, `map[string]any{"a": map[string]any{"b": float64(2.5)}}`},
		{"Bytes", Type{Kind: KindList, Values: &Type{Kind: KindBytes}}, `map[string]any{"a": [][]byte{{0,255}}}`, `map[string]any{"a": [][]byte{{0,255}}}`, `map[string]any{"a": [][]byte{{0,254}}}`},
		{"NullableMaps", Type{Kind: KindMap, Nullable: true, Values: &Type{Kind: KindBool}}, `map[string]any{"a": nil, "b": map[string]any{}}`, `map[string]any{"a": nil, "b": map[string]any{}}`, `map[string]any{"a": map[string]any{}, "b": map[string]any{}}`},
		{"NullableLists", Type{Kind: KindList, Nullable: true, IgnoreOrder: true, Values: &Type{Kind: KindBool}}, `map[string]any{"a": nil, "b": []bool{true, false}}`, `map[string]any{"a": nil, "b": []bool{false, true}}`, `map[string]any{"a": []bool{}, "b": []bool{false, true}}`},
		{"IgnoredNullableMap", Type{Kind: KindMap, Nullable: true, SkipComparison: true, Values: &Type{Kind: KindBool}}, `map[string]any{"a": nil}`, `map[string]any{"a": map[string]any{"b": true}}`, `map[string]any{"b": nil}`},
		{"IgnoredNullableList", Type{Kind: KindList, Nullable: true, SkipComparison: true, Values: &Type{Kind: KindBool}}, `map[string]any{"a": nil}`, `map[string]any{"a": []bool{true}}`, `map[string]any{"b": nil}`},
		{"Ints", Type{Kind: KindInt64}, `map[string]any{"a": int64(9007199254740993)}`, `map[string]any{"a": int64(9007199254740993)}`, `map[string]any{"a": int64(9007199254740992)}`},
		{"Nullable", Type{Kind: KindInt64, Nullable: true}, `map[string]any{"a": nil, "b": int64(3)}`, `map[string]any{"a": nil, "b": int64(3)}`, `map[string]any{"a": int64(0), "b": int64(3)}`},
		{"Nested", Type{Kind: KindMap, Values: &Type{Kind: KindInt64}}, `map[string]any{"a": map[string]any{"b": int64(3)}}`, `map[string]any{"a": map[string]any{"b": int64(3)}}`, `map[string]any{"a": map[string]any{"c": int64(3)}}`},
		{"Lists", Type{Kind: KindList, IgnoreOrder: true, Values: &Type{Kind: KindMap, Values: &Type{Kind: KindInt64}}}, `map[string]any{"a": []any{map[string]any{"b": int64(1)}, map[string]any{"b": int64(2)}}}`, `map[string]any{"a": []any{map[string]any{"b": int64(2)}, map[string]any{"b": int64(1)}}}`, `map[string]any{"a": []any{map[string]any{"b": int64(1)}, map[string]any{"b": int64(1)}}}`},
		{"Ignored", Type{Kind: KindInt64, SkipComparison: true}, `map[string]any{"a": int64(1)}`, `map[string]any{"a": int64(2)}`, `map[string]any{"b": int64(1)}`},
		{"IgnoredMap", Type{Kind: KindMap, SkipComparison: true, Values: &Type{Kind: KindInt64}}, `map[string]any{"a": map[string]any{"b": int64(1)}}`, `map[string]any{"a": map[string]any{"c": int64(2)}}`, `map[string]any{"b": map[string]any{"b": int64(1)}}`},
		{"Messages", Type{Kind: KindMap, Values: &Type{Kind: KindMessage, Fields: map[string]Type{"cnt": {Kind: KindInt64}, "source": {Kind: KindString, SkipComparison: true}}}}, `map[string]any{"a": map[string]any{"b": map[string]any{"cnt": int64(1), "source": "actual"}}}`, `map[string]any{"a": map[string]any{"b": map[string]any{"cnt": int64(1), "source": "model"}}}`, `map[string]any{"a": map[string]any{"b": map[string]any{"cnt": int64(2), "source": "actual"}}}`},
	}
	caseData := newRendererActionCase("Reader", "Read")
	var declarations Declarations
	var runtimeTests strings.Builder
	for _, tt := range tests {
		spec, err := NewType(KindMap, false, &tt.values, nil)
		require.NoError(t, err)
		fieldName := tt.name
		state := fizzbee.RoleState{Name: "DB", Fields: fizzbee.JSONObject{fieldName: map[string]any{}}}
		// All declared fields belong to one DB role.
		if len(caseData.Input.Roles) == 1 {
			caseData.Input.Roles = append(caseData.Input.Roles, state)
			caseData.Outputs[0].Roles = append(caseData.Outputs[0].Roles, state)
		} else {
			caseData.Input.Roles[1].Fields[fieldName] = map[string]any{}
		}
		declarations.Providers = append(declarations.Providers, Provider{Field: mustField(t, "DB."+fieldName), ProviderID: "db", SourceImportPath: "example.test/db", SourceAlias: "dbprovider", Type: *spec})
		shape := toDeclaredShape(*spec, "DB"+tt.name, true)
		fmt.Fprintf(&runtimeTests, "func Test%s(t *testing.T) { check(t, %s, %s, %s, %s, %s) }\n", tt.name, tt.actual, tt.allowed, tt.mismatch, shape.ObservedConvert, shape.ComparisonConvert)
	}
	sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{caseData}}, declarations)
	require.NoError(t, err)
	repeated, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{caseData}}, declarations)
	require.NoError(t, err)
	require.Equal(t, sources, repeated)
	// The plain int64 map uses the generic field wrapper, so supply the equivalent
	// adapters for the runtime test while checking the generated conversion target.
	require.Contains(t, string(sources.RoleCases["Reader"]), "Convert[map[string]int64]")
	for _, tt := range tests {
		if tt.name == "Ints" {
			continue
		}
		spec := Type{Kind: KindMap, Values: &tt.values}
		shape := toDeclaredShape(spec, "DB"+tt.name, true)
		require.Contains(t, string(sources.RoleCases["Reader"]), "Convert["+shape.RawType+"]")
		require.Contains(t, string(sources.RoleCases["Reader"]), shape.ObservedConvert+"(fetched.Fields.DB."+tt.name+")")
		require.Contains(t, string(sources.RoleCases["Reader"]), shape.ComparisonConvert+"(allowed.Fields.DB."+tt.name+")")
	}
	runtimeTests.WriteString(`
func _sgToDBIntsObservedField(value map[string]int64) *gcmp.ObservedField[map[string]int64] { return gcmp.Observed(value) }
func _sgToDBIntsComparisonField(value map[string]int64) *gcmp.ComparisonField[map[string]int64] { return gcmp.Equal(value) }
func check[T, O, A any](t *testing.T, actual, allowed, mismatch any, observe func(T) O, compare func(T) A) {
 t.Helper()
 got, err := provider.Convert[T](actual)
 if err != nil { t.Fatal(err) }
 want, err := provider.Convert[T](allowed)
 if err != nil { t.Fatal(err) }
 bad, err := provider.Convert[T](mismatch)
 if err != nil { t.Fatal(err) }
 observed := &struct{ Value O }{observe(got)}
 comparator := gcmp.NewObservedStateComparator[*struct{ Value A }](observed)
 if err := comparator.Validate(); err != nil { t.Fatal(err) }
 if diffs := comparator.Match(&struct{ Value A }{compare(want)}); len(diffs) != 0 { t.Fatal(diffs) }
 if diffs := comparator.Match(&struct{ Value A }{compare(bad)}); len(diffs) == 0 { t.Fatal("mismatch was accepted") }
}
func TestNestedIntegerTypeError(t *testing.T) {
 got, err := provider.Convert[map[string][]map[string]int64](map[string]any{"a": []any{map[string]any{"b": float64(3)}}})
 if err == nil || got != nil || !strings.Contains(err.Error(), "want int64") { t.Fatalf("got %v, error %v", got, err) }
}
`)
	dir := t.TempDir()
	guardianDir, err := filepath.Abs("../../../guardian")
	require.NoError(t, err)
	module := fmt.Sprintf("module example.test/generated\n\ngo 1.27.0\n\nrequire github.com/mrasu/spec-guardian/guardian v0.0.0\nreplace github.com/mrasu/spec-guardian/guardian => %s\n", filepath.ToSlash(guardianDir))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "common_test.go"), sources.Common, 0600))
	harness := "package specguardian_test\nimport (\"testing\"; \"strings\"; \"github.com/mrasu/spec-guardian/guardian/gcmp\"; \"github.com/mrasu/spec-guardian/guardian/provider\")\n" + runtimeTests.String()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "runtime_test.go"), []byte(harness), 0600))
	command := exec.Command("go", "test", "-mod=mod", "-tags=specguardian", "./...")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}
