package fizzbee

import "fmt"

// Input contains the Action-start state exposed at the conformance test boundary.
//
// Its JSON form is:
//
//	{
//	  "roles": [{"params":{"key":1}, "fields":{"status":"READY"}}]
//	}
type Input struct {
	// Roles is the selected role state before the Action begins.
	Roles []RoleState `json:"roles"`
}

// NewInput returns a validated Action-start boundary state.
func NewInput(roles []RoleState) (Input, error) {
	validated, err := newRoleStates(roles)
	if err != nil {
		return Input{}, err
	}
	return Input{Roles: validated}, nil
}

func newRoleStates(roles []RoleState) ([]RoleState, error) {
	if len(roles) == 0 {
		return nil, fmt.Errorf("roles are empty")
	}
	validated := make([]RoleState, 0, len(roles))
	for _, role := range roles {
		validatedRole, err := NewRoleState(role.Name, role.Params, role.Fields)
		if err != nil {
			return nil, err
		}
		validated = append(validated, validatedRole)
	}
	return validated, nil
}
