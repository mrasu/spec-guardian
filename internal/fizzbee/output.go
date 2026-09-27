package fizzbee

// Output contains one Action-completion state exposed at the conformance test boundary.
//
// Its JSON form is:
//
//	{
//	  "roles": [{"params":{"key":1}, "fields":{"status":"DONE"}}]
//	}
type Output struct {
	// Roles is the selected role state after the tracked Action thread ends.
	Roles []RoleState `json:"roles"`
}

// NewOutput returns a validated Action-completion boundary state.
func NewOutput(roles []RoleState) (Output, error) {
	validated, err := newRoleStates(roles)
	if err != nil {
		return Output{}, err
	}
	return Output{Roles: validated}, nil
}
