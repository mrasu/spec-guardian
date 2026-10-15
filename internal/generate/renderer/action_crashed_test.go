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

func TestGenerateActionCrashed_ObserverManualAndDisabled(t *testing.T) {
	falseValue := false
	tests := []struct {
		name     string
		observer bool
		custom   bool
		compare  *bool
	}{
		{name: "observer", observer: true},
		{name: "custom observer", observer: true, custom: true},
		{name: "manual"},
		{name: "disabled", observer: true, compare: &falseValue},
		{name: "default disabled", observer: true},
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
			if tt.observer {
				declarations.ActionCrashedObserver = &Observer{Field: mustField(t, "sg_metadata.action_crashed"), Metadata: true, Result: true, ObserverID: "crash_result", PluginImportPath: "github.com/mrasu/spec-guardian/guardian/observer/errorascrashed", PluginAlias: "errorascrashedobserver", Type: Type{Kind: KindBool}}
				if tt.custom {
					declarations.ActionCrashedObserver.PluginImportPath = "example.test/generated/custom"
					declarations.ActionCrashedObserver.PluginAlias = "custom"
				}
			}
			sources, err := GenerateScaffold("example.test/generated", fizzbee.Model{Options: fizzbee.ModelOptions{CrashOnYield: crashOnYield}, Cases: cases}, declarations)
			require.NoError(t, err)
			source := string(sources.RoleCases["Reader"])
			if tt.name == "default disabled" || tt.compare != nil && !*tt.compare {
				require.NotContains(t, source, "observerCrashResult")
				require.Empty(t, sources.Common)
				require.Empty(t, generatedStructFields(t, source, "ReaderManualObservation"))
				require.Contains(t, source, "gcmp.Ignore[bool]")
			} else if tt.observer {
				require.Contains(t, source, "observerCrashResult.Observe")
				if tt.custom {
					require.Contains(t, source, "*custom.Observer")
				} else {
					require.Contains(t, source, "*errorascrashedobserver.Observer")
				}
				require.Equal(t, map[string]string{"SgMetadata": "guardian.SgFetchedMetadata"}, generatedStructFields(t, source, "ReaderFetchedObservation"))
				require.Contains(t, string(sources.Common), "return map[string]any{}")
				require.Contains(t, string(sources.Common), "Config: map[string]any{}")
			} else {
				require.NotContains(t, source, "observerCrashResult")
				require.Equal(t, map[string]string{"SgMetadata": "guardian.SgObservedMetadata"}, generatedStructFields(t, source, "ReaderManualObservation"))
				require.Contains(t, source, "ActionCrashed: manual.SgMetadata.ActionCrashed")
			}
			manual := "return ReaderManualObservation{}"
			if !tt.observer {
				manual = "return ReaderManualObservation{SgMetadata: guardian.SgObservedMetadata{ActionCrashed: gcmp.Observed(actionErr != nil)}}"
			}
			impl := strings.ReplaceAll(string(sources.RoleScaffolds["Reader"]), `require.NoError(t, errors.New("TODO: implement me"))`, "")
			impl = strings.ReplaceAll(impl, `"errors"`, "")
			impl = strings.ReplaceAll(impl, `"github.com/stretchr/testify/require"`, "")
			impl = strings.ReplaceAll(impl, "return ReaderManualObservation{}", manual)
			if !tt.observer {
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
		files["custom/observer.go"] = []byte(`package custom
import (
 "context"
 "fmt"
 "github.com/mrasu/spec-guardian/guardian/observer"
)
type Config struct{}
type Observer struct{}
func New(_ Config, config map[string]any, fields []observer.FieldRequest) (*Observer,error) {
 if config == nil || len(fields) != 1 || fields[0].Config == nil { return nil,fmt.Errorf("config must be non-nil") }
 return &Observer{},nil
}
func (*Observer) PrepareCase(context.Context) error { return nil }
func (*Observer) PrepareAttempt(context.Context) error { return nil }
func (*Observer) CleanupCase(context.Context) error { return nil }
func (*Observer) CleanupAttempt(context.Context) error { return nil }
func (*Observer) Observe(_ context.Context, field string, output any, actionErr error) (any,error) {
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

func TestGenerateActionCrashed_DisabledSharedObserverKeepsOtherFields(t *testing.T) {
	falseValue := false
	role := fizzbee.RoleState{Name: "Reader", Fields: fizzbee.JSONObject{"failed": false}}
	cases := []fizzbee.ActionCase{{Action: fizzbee.ActionRef{Role: "Reader", Action: "Read"}, Input: fizzbee.Input{Roles: []fizzbee.RoleState{role}}, Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{role}}}}}
	shared := Observer{ObserverID: "shared", Result: true, PluginImportPath: "example.test/plugin", PluginAlias: "custom", Type: Type{Kind: KindBool}}
	field := shared
	field.Field = mustField(t, "Reader.failed")
	metadata := shared
	metadata.Field, metadata.Metadata = mustField(t, "sg_metadata.action_crashed"), true
	sources, err := GenerateScaffold("example.test/generated", fizzbee.Model{Options: fizzbee.ModelOptions{CrashOnYield: true}, Cases: cases}, Declarations{CompareActionCrashed: &falseValue, ActionCrashedObserver: &metadata, Observers: []Observer{field}})
	require.NoError(t, err)
	require.Contains(t, string(sources.RoleCases["Reader"]), "observerShared.PrepareCase")
	require.Contains(t, string(sources.RoleCases["Reader"]), "observerShared.Observe")
	require.NotContains(t, string(sources.RoleCases["Reader"]), "typedSgMetadataActionCrashed")
	require.NotContains(t, string(sources.Common), "ActionCrashedField")
}
