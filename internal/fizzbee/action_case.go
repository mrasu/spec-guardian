package fizzbee

import (
	"fmt"
	"maps"
	"slices"
)

// ActionCase groups the output states for one Action input state.
//
// It conceptually serializes as:
//
//	{
//	  "action":"Request#2.Process", "input":{"roles":[...]},
//	  "outputs":[{"roles":[...]}]
//	}
type ActionCase struct {
	// Action identifies the concrete role instance and Action being extracted.
	Action ActionRef
	// Input is the selected state at the source Node of the Action-start Link.
	Input Input
	// Outputs contains every distinct selected state where the tracked thread has ended.
	Outputs []Output
}

// NewActionCase returns a validated Action case.
func NewActionCase(action ActionRef, input Input, outputs []Output) (ActionCase, error) {
	validatedAction, err := NewActionRef(action.Role, action.Ref, action.Action)
	if err != nil {
		return ActionCase{}, err
	}
	validatedInput, err := NewInput(input.Roles)
	if err != nil {
		return ActionCase{}, err
	}
	if len(outputs) == 0 {
		return ActionCase{}, fmt.Errorf("action case outputs are empty")
	}
	validatedOutputs := make([]Output, 0, len(outputs))
	for _, output := range outputs {
		validatedOutput, err := NewOutput(output.Roles)
		if err != nil {
			return ActionCase{}, err
		}
		validatedOutputs = append(validatedOutputs, validatedOutput)
	}
	return ActionCase{Action: validatedAction, Input: validatedInput, Outputs: validatedOutputs}, nil
}

// NewActionCases returns validated Action cases with a consistent state shape per Action.
func NewActionCases(cases []ActionCase) ([]ActionCase, error) {
	if len(cases) == 0 {
		return nil, fmt.Errorf("action cases are empty")
	}
	validated := make([]ActionCase, 0, len(cases))
	for _, actionCase := range cases {
		validatedCase, err := NewActionCase(actionCase.Action, actionCase.Input, actionCase.Outputs)
		if err != nil {
			return nil, err
		}
		validated = append(validated, validatedCase)
	}
	if err := validateActionCases(validated); err != nil {
		return nil, err
	}
	return validated, nil
}

func validateActionCases(cases []ActionCase) error {
	firstByAction := make(map[string]ActionCase)
	for _, actionCase := range cases {
		if err := validateCaseOutputs(actionCase); err != nil {
			return err
		}
		key := actionCase.Action.Role + "\x00" + actionCase.Action.Action
		first, ok := firstByAction[key]
		if !ok {
			firstByAction[key] = actionCase
			continue
		}
		if err := validateRoleStateShape(first.Input.Roles, actionCase.Input.Roles); err != nil {
			return err
		}
	}
	return nil
}

func validateCaseOutputs(actionCase ActionCase) error {
	for _, output := range actionCase.Outputs {
		if err := validateRoleStateShape(actionCase.Input.Roles, output.Roles); err != nil {
			return err
		}
	}
	return nil
}

func validateRoleStateShape(first, candidate []RoleState) error {
	if len(candidate) != len(first) {
		return fmt.Errorf("got %d roles, want %d", len(candidate), len(first))
	}
	for index, role := range candidate {
		baseline := first[index]
		if role.Name != baseline.Name {
			return fmt.Errorf("role %d got %q, want %q", index, role.Name, baseline.Name)
		}
		if err := validateObjectShape("params", baseline.Params, role.Params); err != nil {
			return err
		}
		if err := validateObjectShape("fields", baseline.Fields, role.Fields); err != nil {
			return err
		}
	}
	return nil
}

func validateObjectShape(name string, first, candidate JSONObject) error {
	firstKeys := slices.Sorted(maps.Keys(first))
	candidateKeys := slices.Sorted(maps.Keys(candidate))
	if len(candidateKeys) != len(firstKeys) {
		return fmt.Errorf("%s got %d keys, want %d", name, len(candidateKeys), len(firstKeys))
	}
	for index, key := range candidateKeys {
		if key != firstKeys[index] {
			return fmt.Errorf("%s key %d got %q, want %q", name, index, key, firstKeys[index])
		}
	}
	return nil
}
