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

func TestGeneratedSeed_PreparesAllProvidersBeforeSeedingAndCleansUpFailures(t *testing.T) {
	role := fizzbee.RoleState{Name: "Reader", Fields: fizzbee.JSONObject{"rows": []any{"a", "b"}, "ready": true}}
	model := fizzbee.Model{Cases: []fizzbee.ActionCase{{Action: fizzbee.ActionRef{Role: "Reader", Action: "Read"}, Input: fizzbee.Input{Roles: []fizzbee.RoleState{role}}, Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{role}}}}}}
	declarations := Declarations{Providers: []Provider{
		{Field: mustField(t, "Reader.rows"), ProviderID: "custom", SourceImportPath: "example.test/generated/custom", SourceAlias: "custom", Capabilities: []string{"seed", "observe"}, Type: Type{Kind: KindList, IgnoreOrder: true, Values: &Type{Kind: KindString}}},
		{Field: mustField(t, "Reader.ready"), ProviderID: "later", SourceImportPath: "example.test/generated/custom", SourceAlias: "custom", Capabilities: []string{"observe"}, Type: Type{Kind: KindBool}},
	}}
	sources, err := GenerateScaffold("example.test/generated", model, declarations)
	require.NoError(t, err)
	repeated, err := GenerateScaffold("example.test/generated", model, declarations)
	require.NoError(t, err)
	require.Equal(t, sources, repeated)
	dir := t.TempDir()
	guardianDir, err := filepath.Abs("../../../guardian")
	require.NoError(t, err)
	scaffold := string(sources.RoleScaffolds["Reader"])
	scaffold = strings.ReplaceAll(scaffold, `require.NoError(t, errors.New("TODO: implement me"))`, "")
	scaffold = strings.ReplaceAll(scaffold, `"errors"`, "")
	scaffold = strings.ReplaceAll(scaffold, `"github.com/stretchr/testify/require"`, "")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "custom"), 0700))
	files := map[string][]byte{
		"go.mod":         []byte(fmt.Sprintf("module example.test/generated\n\ngo 1.27.0\n\nrequire github.com/mrasu/spec-guardian/guardian v0.0.0\nreplace github.com/mrasu/spec-guardian/guardian => %s\n", filepath.ToSlash(guardianDir))),
		"common_test.go": sources.Common,
		"cases_test.go":  sources.RoleCases["Reader"],
		"impl_test.go":   []byte(scaffold),
		"custom/provider.go": []byte(`package custom
import (
 "context"
 "errors"
 "os"
 "reflect"
 "github.com/mrasu/spec-guardian/guardian/provider"
)
var Prepared int
type Config struct{}
type Provider struct { Prepared bool }
func New(Config, map[string]any, []provider.FieldRequest) (*Provider, error) { return &Provider{}, nil }
func (*Provider) PrepareCase(context.Context) error { return nil }
func (*Provider) CleanupCase(context.Context) error { return nil }
func (p *Provider) PrepareAttempt(context.Context) error {
 p.Prepared = true
 Prepared++
 if os.Getenv("SEED_TEST_MODE") == "prepare failure" { return errors.New("preparation failed") }
 return nil
}
func (*Provider) CleanupAttempt(context.Context) error {
 return os.WriteFile(os.Getenv("SEED_TEST_CLEANUP"), []byte("cleaned"), 0600)
}
func (p *Provider) Seed(ctx context.Context, request provider.SeedRequest) (provider.SeedResult, error) {
 if !p.Prepared || Prepared != 2 { return provider.SeedResult{}, errors.New("seed before all preparations") }
 if len(request.Values) != 1 || request.Values[0].Field != "Reader.rows" { return provider.SeedResult{}, errors.New("incorrect fields") }
 result := provider.SeedResult{Values: []provider.SeedValue{{Field: "Reader.rows", Value: []string{"a", "b"}}}}
 switch os.Getenv("SEED_TEST_MODE") {
 case "seed failure": return provider.SeedResult{}, errors.New("seed failed")
 case "reordered list": result.Values[0].Value = []string{"b", "a"}
 case "missing field": result.Values = nil
 case "mutated input":
  reflect.ValueOf(request.Values[0].Value).Index(0).SetString("changed")
  return provider.SeedResult{Values: request.Values}, nil
 case "mutated field":
  request.Values[0].Field = "renamed"
  return provider.SeedResult{Values: request.Values}, nil
 }
 return result, nil
}
func (*Provider) Observe(context.Context, string) (any, error) { return []string{"a", "b"}, nil }
`),
		"runtime_test.go": []byte(`package specguardian_test
import (
 "testing"
 "example.test/generated/custom"
)
func TestAttempt(t *testing.T) {
 p := &custom.Provider{}
 e := &ReaderConformanceEnvironment{ReaderEnvironment: &ReaderEnvironment{}, providerCustom: p, providerLater: &custom.Provider{}}
 input := FizzbeeReaderState{Fields: ReaderField{Reader: ReaderReaderField{Rows: ReaderRowsValue{"a", "b"}}}}
 t.Cleanup(func() {
  if input.Fields.Reader.Rows[0] != "a" { t.Error("model input changed") }
 })
 e.PrepareReadAttempt(t, input)
 if !p.Prepared { t.Fatal("provider was not prepared") }
}
`),
	}
	for name, data := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0600))
	}
	tests := []struct {
		name string
		want string
	}{
		{"success", ""},
		{"prepare failure", "preparation failed"},
		{"seed failure", "seed failed"},
		{"reordered list", "seed value differs from input"},
		{"missing field", "seed returned 0 fields"},
		{"mutated input", "Reader.rows[0]: seed value differs from input"},
		{"mutated field", "seed field 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "cleanup")
			command := exec.Command("go", "test", "-mod=mod", "-count=1", "-tags=specguardian", "./...")
			command.Dir = dir
			command.Env = append(os.Environ(), "GOWORK=off", "SEED_TEST_MODE="+tt.name, "SEED_TEST_CLEANUP="+marker)
			output, err := command.CombinedOutput()
			require.NotContains(t, string(output), "model input changed")
			if tt.want == "" {
				require.NoError(t, err, string(output))
			} else {
				require.Error(t, err, string(output))
				require.Contains(t, string(output), tt.want)
			}
			cleanup, err := os.ReadFile(marker)
			require.NoError(t, err, string(output))
			require.Equal(t, "cleaned", string(cleanup))
		})
	}
}
