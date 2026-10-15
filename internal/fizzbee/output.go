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
	Roles      []RoleState `json:"roles"`
	SgMetadata SgMetadata  `json:"sg_metadata"`
}

// SgMetadata contains the result derived from the tracked Action's execution path.
type SgMetadata struct {
	ActionCrashed bool `json:"action_crashed"`
}

// NewOutput returns a validated Action-completion boundary state.
func NewOutput(roles []RoleState) (Output, error) {
	validated, err := newRoleStates(roles)
	if err != nil {
		return Output{}, err
	}
	return Output{Roles: validated}, nil
}
