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

func TestGenerateActionCrashed_ProviderManualAndDisabled(t *testing.T) {
	falseValue := false
	tests := []struct {
		name     string
		provider bool
		custom   bool
		compare  *bool
	}{
		{name: "provider", provider: true},
		{name: "custom provider", provider: true, custom: true},
		{name: "manual"},
		{name: "disabled", provider: true, compare: &falseValue},
		{name: "default disabled", provider: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := fizzbee.RoleState{Name: "Reader", Fields: fizzbee.JSONObject{}}
			crashOnYield := true
			cases := []fizzbee.ActionCase{{Action: fizzbee.ActionRef{Role: "Reader", Action: "Read"}, Input: fizzbee.Input{Roles: []fizzbee.RoleState{role}}, Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{role}}}}}
			if tt.name == "default disabled" {
				crashOnYield = false
			}
			declarations := Declarations{CompareActionCrashed: tt.compare}
			if tt.provider {
				declarations.ActionCrashedProvider = &Provider{Field: mustField(t, "sg_metadata.action_crashed"), Metadata: true, Result: true, ProviderID: "crash_result", SourceImportPath: "github.com/mrasu/spec-guardian/guardian/provider/errorascrashed", SourceAlias: "errorascrashedprovider", Type: Type{Kind: KindBool}}
				if tt.custom {
					declarations.ActionCrashedProvider.SourceImportPath = "example.test/generated/custom"
					declarations.ActionCrashedProvider.SourceAlias = "custom"
				}
			}
			sources, err := GenerateScaffold("example.test/generated", fizzbee.Model{Options: fizzbee.ModelOptions{CrashOnYield: crashOnYield}, Cases: cases}, declarations)
			require.NoError(t, err)
			source := string(sources.RoleCases["Reader"])
			if tt.name == "default disabled" || tt.compare != nil && !*tt.compare {
				require.NotContains(t, source, "providerCrashResult")
				require.Empty(t, sources.Common)
				require.Empty(t, generatedStructFields(t, source, "ReaderManualObservation"))
				require.Contains(t, source, "gcmp.Ignore[bool]")
			} else if tt.provider {
				require.Contains(t, source, "providerCrashResult.Observe")
				if tt.custom {
					require.Contains(t, source, "*custom.Provider")
				} else {
					require.Contains(t, source, "*errorascrashedprovider.Provider")
				}
				require.Equal(t, map[string]string{"SgMetadata": "guardian.SgFetchedMetadata"}, generatedStructFields(t, source, "ReaderFetchedObservation"))
				require.Contains(t, string(sources.Common), "return map[string]any{}")
				require.Contains(t, string(sources.Common), "Config: map[string]any{}")
			} else {
				require.NotContains(t, source, "providerCrashResult")
				require.Equal(t, map[string]string{"SgMetadata": "guardian.SgObservedMetadata"}, generatedStructFields(t, source, "ReaderManualObservation"))
				require.Contains(t, source, "ActionCrashed: manual.SgMetadata.ActionCrashed")
			}
			manual := "return ReaderManualObservation{}"
			if !tt.provider {
				manual = "return ReaderManualObservation{SgMetadata: guardian.SgObservedMetadata{ActionCrashed: gcmp.Observed(actionErr != nil)}}"
			}
			impl := strings.ReplaceAll(string(sources.RoleScaffolds["Reader"]), `require.NoError(t, errors.New("TODO: implement me"))`, "")
			impl = strings.ReplaceAll(impl, `"errors"`, "")
			impl = strings.ReplaceAll(impl, `"github.com/stretchr/testify/require"`, "")
			impl = strings.ReplaceAll(impl, "return ReaderManualObservation{}", manual)
			if !tt.provider {
				impl = strings.Replace(impl, `"testing"`, "\"testing\"\n\"github.com/mrasu/spec-guardian/guardian/gcmp\"", 1)
			}
			runMetadataScaffold(t, sources, impl)
		})
	}
}

func runMetadataScaffold(t *testing.T, sources *Scaffold, impl string) {
	t.Helper()
	dir := t.TempDir()
	guardianDir, err := filepath.Abs("../../../guardian")
	require.NoError(t, err)
	files := map[string][]byte{
		"go.mod":        []byte(fmt.Sprintf("module example.test/generated\n\ngo 1.27.0\n\nrequire github.com/mrasu/spec-guardian/guardian v0.0.0\nreplace github.com/mrasu/spec-guardian/guardian => %s\n", filepath.ToSlash(guardianDir))),
		"cases_test.go": sources.RoleCases["Reader"],
		"impl_test.go":  []byte(impl),
		"runtime_test.go": []byte(`//go:build specguardian
package specguardian_test
import (
 "errors"
 "testing"
 "github.com/mrasu/spec-guardian/guardian"
)
func TestMetadata(t *testing.T) {
 e := NewReaderConformanceEnvironment(t,nil)
 for _, failed := range []bool{false,true} {
  var actionErr error
  if failed { actionErr = errors.New("failed") }
  actual := e.ObserveReadState(t,FizzbeeReaderState{},ReaderReadOutput{},actionErr)
  expected := FizzbeeReaderState{SgMetadata:guardian.SgFizzbeeMetadata{ActionCrashed:failed}}
  allowed := e.BuildAllowedReadStates(t,[]FizzbeeReaderState{expected})
  guardian.AssertAllowed(t,allowed,actual,nil)
 }
}`),
	}
	if len(sources.Common) > 0 {
		files["common_test.go"] = sources.Common
	}
	if strings.Contains(string(sources.RoleCases["Reader"]), "example.test/generated/custom") {
		require.NoError(t, os.Mkdir(filepath.Join(dir, "custom"), 0700))
		files["custom/provider.go"] = []byte(`package custom
import (
 "context"
 "fmt"
 "github.com/mrasu/spec-guardian/guardian/provider"
)
type Config struct{}
type Provider struct{}
func New(_ Config, config map[string]any, fields []provider.FieldRequest) (*Provider,error) {
 if config == nil || len(fields) != 1 || fields[0].Config == nil { return nil,fmt.Errorf("config must be non-nil") }
 return &Provider{},nil
}
func (*Provider) PrepareCase(context.Context) error { return nil }
func (*Provider) PrepareAttempt(context.Context) error { return nil }
func (*Provider) CleanupCase(context.Context) error { return nil }
func (*Provider) CleanupAttempt(context.Context) error { return nil }
func (*Provider) ObserveResult(_ context.Context, field string, output any, actionErr error) (any,error) {
 if field != "sg_metadata.action_crashed" || output == nil { return nil,fmt.Errorf("missing action result") }
 return actionErr != nil,nil
}`)
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

func TestGenerateActionCrashed_DisabledSharedProviderKeepsOtherFields(t *testing.T) {
	falseValue := false
	role := fizzbee.RoleState{Name: "Reader", Fields: fizzbee.JSONObject{"failed": false}}
	cases := []fizzbee.ActionCase{{Action: fizzbee.ActionRef{Role: "Reader", Action: "Read"}, Input: fizzbee.Input{Roles: []fizzbee.RoleState{role}}, Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{role}}}}}
	shared := Provider{ProviderID: "shared", Result: true, SourceImportPath: "example.test/source", SourceAlias: "custom", Type: Type{Kind: KindBool}}
	field := shared
	field.Field = mustField(t, "Reader.failed")
	metadata := shared
	metadata.Field, metadata.Metadata = mustField(t, "sg_metadata.action_crashed"), true
	sources, err := GenerateScaffold("example.test/generated", fizzbee.Model{Options: fizzbee.ModelOptions{CrashOnYield: true}, Cases: cases}, Declarations{CompareActionCrashed: &falseValue, ActionCrashedProvider: &metadata, Providers: []Provider{field}})
	require.NoError(t, err)
	require.Contains(t, string(sources.RoleCases["Reader"]), "providerShared.PrepareCase")
	require.Contains(t, string(sources.RoleCases["Reader"]), "providerShared.Observe")
	require.NotContains(t, string(sources.RoleCases["Reader"]), "typedSgMetadataActionCrashed")
	require.NotContains(t, string(sources.Common), "ActionCrashedField")
}
