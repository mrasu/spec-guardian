package artifact

// ConformanceTargetRole identifies the non-Init actions of a @conformance role.
//
// It is derived from a spec_ast.json role such as:
//
//	{
//	  "name":"Request", "decorators":[{"name":"conformance"}],
//	  "actions":[{"name":"Init"}, {"name":"Process"}]
//	}
type ConformanceTargetRole struct {
	// Name is the role name from `role.name`, such as "Request".
	Name string
	// Actions contains every Action name except the reserved "Init", such as ["Process"].
	Actions []string
}

func NewConformanceTargetRole(name string, actions []string) *ConformanceTargetRole {
	return &ConformanceTargetRole{
		Name:    name,
		Actions: actions,
	}
}
