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

func TestGenerateResultObserver_PassesOutputAndErrorToObservation(t *testing.T) {
	role := fizzbee.RoleState{Name: "Reader", Fields: fizzbee.JSONObject{"failed": true}}
	cases := []fizzbee.ActionCase{{Action: fizzbee.ActionRef{Role: "Reader", Action: "Read"}, Input: fizzbee.Input{Roles: []fizzbee.RoleState{role}}, Outputs: []fizzbee.Output{{Roles: []fizzbee.RoleState{role}}}}}
	declarations := Declarations{Observers: []Observer{{Field: mustField(t, "Reader.failed"), ObserverID: "custom", Result: true, PluginImportPath: "example.test/generated/custom", PluginAlias: "custom", Type: Type{Kind: KindBool}}}}
	sources, err := GenerateScaffold("example.test/generated", fizzbee.Model{Cases: cases}, declarations)
	require.NoError(t, err)
	dir := t.TempDir()
	guardianDir, err := filepath.Abs("../../../guardian")
	require.NoError(t, err)
	scaffold := strings.ReplaceAll(string(sources.RoleScaffolds["Reader"]), `require.NoError(t, errors.New("TODO: implement me"))`, "")
	scaffold = strings.ReplaceAll(scaffold, `"errors"`, "")
	scaffold = strings.ReplaceAll(scaffold, `"github.com/stretchr/testify/require"`, "")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "custom"), 0700))
	files := map[string][]byte{
		"go.mod":         []byte(fmt.Sprintf("module example.test/generated\n\ngo 1.27.0\n\nrequire github.com/mrasu/spec-guardian/guardian v0.0.0\nreplace github.com/mrasu/spec-guardian/guardian => %s\n", filepath.ToSlash(guardianDir))),
		"cases_test.go":  sources.RoleCases["Reader"],
		"common_test.go": sources.Common,
		"impl_test.go":   []byte(scaffold),
		"custom/observer.go": []byte(`package custom
import (
 "context"
 "fmt"
 "github.com/mrasu/spec-guardian/guardian/observer"
)
type Config struct{}
type Observer struct { Output any; Err error }
var _ observer.ResultObserver = (*Observer)(nil)
func New(Config, map[string]any, []observer.FieldRequest) (*Observer, error) { return &Observer{}, nil }
func (*Observer) PrepareCase(context.Context) error { return nil }
func (*Observer) PrepareAttempt(context.Context) error { return nil }
func (*Observer) CleanupCase(context.Context) error { return nil }
func (*Observer) CleanupAttempt(context.Context) error { return nil }
func (o *Observer) Observe(_ context.Context, field string, output any, actionErr error) (any,error) {
 if field != "Reader.failed" { return nil, fmt.Errorf("unexpected field %s",field) }
 o.Output, o.Err = output, actionErr
 return actionErr != nil,nil
}`),
		"runtime_test.go": []byte(`//go:build specguardian
package specguardian_test
import (
 "errors"
 "testing"
 "example.test/generated/custom"
  "github.com/mrasu/spec-guardian/guardian/gcmp"
)
func TestResultObservation(t *testing.T) {
 plugin := &custom.Observer{}
 e := &ReaderConformanceEnvironment{ReaderEnvironment:&ReaderEnvironment{},observerCustom:plugin}
 actionErr := errors.New("action failed")
 output := ReaderReadOutput{}
 actual := e.ObserveReadState(t,FizzbeeReaderState{},output,actionErr)
 if plugin.Output != output || plugin.Err != actionErr { t.Fatal("action result was not forwarded") }
 expected := e.BuildAllowedReadState(t, FizzbeeReaderState{})
 expected.Fields = ReaderComparisonField{Reader:ReaderReaderComparisonField{Failed:gcmp.Equal(true)}}
 comparison := gcmp.NewObservedStateComparator[*ReaderComparisonState](actual)
 if err := comparison.Validate(); err != nil { t.Fatal(err) }
 if diffs := comparison.Match(&expected); len(diffs) != 0 { t.Fatal(diffs) }
}`),
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
