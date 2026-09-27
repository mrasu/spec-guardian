package artifact

import "encoding/json"

// Graph is an index-addressable FizzBee state graph assembled from protobuf artifacts.
//
// An adjacency_lists_*.pb Link with `src: 12` and `dest: 13` connects
// Nodes[12] to Nodes[13].
type Graph struct {
	// Nodes contains states from nodes_*.pb in their artifact order.
	Nodes []*Node
	// Links contains transitions from adjacency_lists_*.pb in their artifact order.
	Links                  []*Link
	conformanceTargetRoles map[string]bool
}

// IsConformanceTargetRole reports whether name is selected by @conformance in spec_ast.json.
func (graph *Graph) IsConformanceTargetRole(name string) bool {
	return graph.conformanceTargetRoles[name]
}

// RoleState is the role representation embedded in a node JSON roles array.
//
// For example:
//
//	{
//	  "name":"Request", "ref":2,
//	  "params":{"key":1}, "fields":{"status":"DONE"}
//	}
type RoleState struct {
	// Name is the AST role name from the JSON name field.
	Name string `json:"name"`
	// Ref distinguishes concrete instances of the same role, from the JSON ref field.
	Ref uint64 `json:"ref"`
	// Params is the JSON params object passed when the role instance was created.
	Params json.RawMessage `json:"params"`
	// Fields is the JSON fields object holding the role's current state.
	Fields json.RawMessage `json:"fields"`
}
