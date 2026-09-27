// Package artifact decodes FizzBee exploration artifacts into a state graph.
package artifact

import "github.com/mrasu/spec-guardian/internal/fizzbee/pb"

// Artifacts contains the decoded FizzBee artifact set used for conformance extraction.
//
// The files are `state_config.json`, `spec_ast.json`, `nodes_*.pb`, and
// `adjacency_lists_*.pb`. For example, `spec_ast.json` contains:
//
//	{
//	  "roles": [{"name":"Request", "decorators":[{"name":"conformance"}]}]
//	}
type Artifacts struct {
	// StateConfig is the protobuf decoded from state_config.json. For example,
	// `{ "options": { "maxActions": "100" } }` becomes StateConfig.Options.MaxActions.
	StateConfig *pb.StateSpaceOptions
	// Graph is the state graph assembled from nodes_*.pb and adjacency_lists_*.pb.
	Graph *Graph
	// ConformanceTargetRoles is the @conformance role and its non-Init Actions from spec_ast.json.
	ConformanceTargetRoles []*ConformanceTargetRole
}

func NewArtifacts(config *pb.StateSpaceOptions, graph *Graph, conformanceTargetRoles []*ConformanceTargetRole) *Artifacts {
	return &Artifacts{
		StateConfig:            config,
		Graph:                  graph,
		ConformanceTargetRoles: conformanceTargetRoles,
	}
}
