package renderer

import (
	"fmt"

	"github.com/cockroachdb/errors"
	"go.yaml.in/yaml/v3"
)

// Hook names an optional user environment lifecycle method.
type Hook string

const (
	HookSetupCase      Hook = "setup_case"
	HookCleanupCase    Hook = "cleanup_case"
	HookSetupAttempt   Hook = "setup_attempt"
	HookCleanupAttempt Hook = "cleanup_attempt"
)

// UnmarshalYAML decodes a supported user hook name.
func (h *Hook) UnmarshalYAML(node *yaml.Node) error {
	var value string
	if err := node.Decode(&value); err != nil {
		return errors.Wrap(err, "decode hook")
	}
	hook := Hook(value)
	switch hook {
	case HookSetupCase, HookCleanupCase, HookSetupAttempt, HookCleanupAttempt:
		*h = hook
		return nil
	default:
		return fmt.Errorf("unknown hook %q", value)
	}
}

// Hooks selects the user lifecycle methods generated for every action.
type Hooks struct {
	SetupCase      bool
	CleanupCase    bool
	SetupAttempt   bool
	CleanupAttempt bool
}

// ResolveHooks converts declared hook names to generation settings.
func ResolveHooks(names []Hook) Hooks {
	var hooks Hooks
	for _, name := range names {
		switch name {
		case HookSetupCase:
			hooks.SetupCase = true
		case HookCleanupCase:
			hooks.CleanupCase = true
		case HookSetupAttempt:
			hooks.SetupAttempt = true
		case HookCleanupAttempt:
			hooks.CleanupAttempt = true
		}
	}
	return hooks
}
