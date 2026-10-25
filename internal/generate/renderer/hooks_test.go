package renderer

import (
	"strings"
	"testing"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestGenerateScaffold_OnlyDeclaredHooksForEveryAction(t *testing.T) {
	tests := []struct {
		name  string
		names []Hook
	}{
		{name: "omitted"},
		{name: "empty", names: []Hook{}},
		{name: "setup case", names: []Hook{HookSetupCase}},
		{name: "cleanup case", names: []Hook{HookCleanupCase}},
		{name: "setup attempt", names: []Hook{HookSetupAttempt}},
		{name: "cleanup attempt", names: []Hook{HookCleanupAttempt}},
		{name: "all", names: []Hook{HookSetupCase, HookCleanupCase, HookSetupAttempt, HookCleanupAttempt}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hooks := ResolveHooks(tt.names)
			sources, err := GenerateScaffold("example.test/project", fizzbee.Model{Cases: []fizzbee.ActionCase{
				newRendererActionCase("Reader", "Read"), newRendererActionCase("Writer", "Write"),
			}}, Declarations{Hooks: hooks})
			require.NoError(t, err)
			for role, action := range map[string]string{"Reader": "Read", "Writer": "Write"} {
				for suffix, enabled := range map[string]bool{
					"Setup" + action + "Case": hooks.SetupCase, "Cleanup" + action + "Case": hooks.CleanupCase,
					"Setup" + action + "Attempt": hooks.SetupAttempt, "Cleanup" + action + "Attempt": hooks.CleanupAttempt,
				} {
					require.Equal(t, enabled, strings.Contains(string(sources.RoleScaffolds[role]), suffix+"("), suffix)
					require.Equal(t, enabled, strings.Contains(string(sources.RoleCases[role]), suffix+"("), suffix)
				}
			}
		})
	}
}

func TestHook_UnmarshalYAML(t *testing.T) {
	for _, name := range []Hook{HookSetupCase, HookCleanupCase, HookSetupAttempt, HookCleanupAttempt} {
		var got Hook
		require.NoError(t, yaml.Unmarshal([]byte(name), &got))
		require.Equal(t, name, got)
	}
	var got Hook
	require.ErrorContains(t, yaml.Unmarshal([]byte("unknown"), &got), "unknown hook")
	require.Empty(t, got)
}
