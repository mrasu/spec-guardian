package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/internal/fizzbee/pb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ReadArtifacts reads the FizzBee artifacts in dir.
func ReadArtifacts(dir string) (*Artifacts, error) {
	conformanceTargetRoles, err := readConformanceTargetRoles(filepath.Join(dir, "spec_ast.json"))
	if err != nil {
		return nil, err
	}
	stateConfig, err := readStateConfig(filepath.Join(dir, "state_config.json"))
	if err != nil {
		return nil, err
	}
	nodes, err := readNodes(filepath.Join(dir, "nodes_*.pb"))
	if err != nil {
		return nil, err
	}
	links, err := readLinks(filepath.Join(dir, "adjacency_lists_*.pb"))
	if err != nil {
		return nil, err
	}
	graph, err := newGraph(nodes, links)
	if err != nil {
		return nil, err
	}
	graph.conformanceTargetRoles = make(map[string]bool, len(conformanceTargetRoles))
	for _, target := range conformanceTargetRoles {
		graph.conformanceTargetRoles[target.Name] = true
	}

	return NewArtifacts(stateConfig, graph, conformanceTargetRoles), nil
}

func readConformanceTargetRoles(name string) ([]*ConformanceTargetRole, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", name)
	}
	var spec pb.File
	if err := protojson.Unmarshal(data, &spec); err != nil {
		return nil, errors.Wrapf(err, "decode %s", name)
	}

	var roles []*ConformanceTargetRole
	for _, role := range spec.GetRoles() {
		if !hasDecorator(role, "conformance") {
			continue
		}
		name := role.GetName()
		var actions []string
		for _, action := range role.GetActions() {
			if action.GetName() == "Init" {
				continue
			}
			if actionHasParallel(action) {
				return nil, fmt.Errorf("action %s.%s contains parallel execution", name, action.GetName())
			}
			actions = append(actions, action.GetName())
		}
		if len(actions) == 0 {
			return nil, fmt.Errorf("no non-Init action found for @conformance role %s in %s", name, name)
		}
		roles = append(roles, NewConformanceTargetRole(name, actions))
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("no role with @conformance decorator found in %s", name)
	}

	return roles, nil
}

func actionHasParallel(action *pb.Action) bool {
	return action.GetFlow() == pb.Flow_FLOW_PARALLEL || blockHasParallel(action.GetBlock())
}

func blockHasParallel(block *pb.Block) bool {
	if block == nil {
		return false
	}
	if block.GetFlow() == pb.Flow_FLOW_PARALLEL {
		return true
	}
	for _, statement := range block.GetStmts() {
		if statementHasParallel(statement) {
			return true
		}
	}
	return false
}

func statementHasParallel(statement *pb.Statement) bool {
	if blockHasParallel(statement.GetBlock()) {
		return true
	}
	if ifStatement := statement.GetIfStmt(); ifStatement != nil {
		if ifStatement.GetFlow() == pb.Flow_FLOW_PARALLEL {
			return true
		}
		for _, branch := range ifStatement.GetBranches() {
			if blockHasParallel(branch.GetBlock()) {
				return true
			}
		}
	}
	if forStatement := statement.GetForStmt(); forStatement != nil {
		return forStatement.GetFlow() == pb.Flow_FLOW_PARALLEL || blockHasParallel(forStatement.GetBlock())
	}
	if anyStatement := statement.GetAnyStmt(); anyStatement != nil {
		return anyStatement.GetFlow() == pb.Flow_FLOW_PARALLEL || blockHasParallel(anyStatement.GetBlock())
	}
	if whileStatement := statement.GetWhileStmt(); whileStatement != nil {
		return whileStatement.GetFlow() == pb.Flow_FLOW_PARALLEL || blockHasParallel(whileStatement.GetBlock())
	}
	return false
}

func hasDecorator(role *pb.Role, name string) bool {
	for _, decorator := range role.GetDecorators() {
		if decorator.GetName() == name && len(decorator.GetArgs()) == 0 {
			return true
		}
	}
	return false
}

func readStateConfig(name string) (*pb.StateSpaceOptions, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", name)
	}
	var config pb.StateSpaceOptions
	if err := protojson.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrapf(err, "decode %s", name)
	}
	return &config, nil
}

func readNodes(pattern string) ([]*Node, error) {
	files, err := artifactFiles(pattern, "node")
	if err != nil {
		return nil, err
	}
	var nodes []*Node
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, errors.Wrapf(err, "read %s", name)
		}
		decoded, err := decodeNodes(data)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, decoded...)
	}
	return nodes, nil
}

func readLinks(pattern string) ([]*Link, error) {
	files, err := artifactFiles(pattern, "link")
	if err != nil {
		return nil, err
	}
	var links []*Link
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, errors.Wrapf(err, "read %s", name)
		}
		decoded, err := decodeLinks(data)
		if err != nil {
			return nil, err
		}
		links = append(links, decoded...)
	}
	return links, nil
}

func artifactFiles(pattern, kind string) ([]string, error) {
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, errors.Wrapf(err, "glob %s artifacts", kind)
	}
	slices.Sort(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("required %s artifacts matching %s not found", kind, pattern)
	}
	return files, nil
}

func newGraph(nodes []*Node, links []*Link) (*Graph, error) {
	graph := &Graph{Nodes: nodes, Links: links}
	for i := range graph.Links {
		link := graph.Links[i]
		if link.Src < 0 || link.Src >= int64(len(graph.Nodes)) {
			return nil, fmt.Errorf("link %d source node %d is out of range", i, link.Src)
		}
		if link.Dst < 0 || link.Dst >= int64(len(graph.Nodes)) {
			return nil, fmt.Errorf("link %d destination node %d is out of range", i, link.Dst)
		}
		graph.Nodes[link.Src].Outbound = append(graph.Nodes[link.Src].Outbound, i)
		graph.Nodes[link.Dst].Inbound = append(graph.Nodes[link.Dst].Inbound, i)
	}
	return graph, nil
}

func decodeNodes(data []byte) ([]*Node, error) {
	var artifact pb.Nodes
	if err := proto.Unmarshal(data, &artifact); err != nil {
		return nil, errors.Wrap(err, "decode nodes")
	}
	nodes := make([]*Node, 0, len(artifact.Json))
	for _, state := range artifact.Json {
		node, err := NewNode(json.RawMessage(state))
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func decodeLinks(data []byte) ([]*Link, error) {
	var artifact pb.Links
	if err := proto.Unmarshal(data, &artifact); err != nil {
		return nil, errors.Wrap(err, "decode links")
	}
	links := make([]*Link, 0, len(artifact.Links))
	for _, link := range artifact.Links {
		converted, err := NewLink(link)
		if err != nil {
			return nil, err
		}
		links = append(links, converted)
	}
	return links, nil
}
