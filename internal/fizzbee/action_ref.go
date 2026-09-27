package fizzbee

import "fmt"

// ActionRef identifies a concrete FizzBee role instance and Action.
//
// It is parsed from an Action-start Link name such as "Request#2.Process".
type ActionRef struct {
	// Role is the role name before '#', such as "Request".
	Role string
	// Ref is the concrete role instance number after '#', such as 2.
	Ref uint64
	// Action is the Action name after '.', such as "Process".
	Action string
}

// NewActionRef returns a validated concrete role Action reference.
func NewActionRef(role string, ref uint64, action string) (ActionRef, error) {
	if role == "" {
		return ActionRef{}, fmt.Errorf("action role is empty")
	}
	if action == "" {
		return ActionRef{}, fmt.Errorf("action name is empty")
	}
	return ActionRef{Role: role, Ref: ref, Action: action}, nil
}
