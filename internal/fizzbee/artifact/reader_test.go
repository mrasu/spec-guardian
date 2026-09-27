package artifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mrasu/spec-guardian/internal/fizzbee/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestReadArtifacts(t *testing.T) {
	dir := filepath.Join("..", "testdata", "read_write_db", "fizzbee_output")
	artifacts, err := ReadArtifacts(dir)
	require.NoError(t, err)
	assert.EqualValues(t, 100, artifacts.StateConfig.GetOptions().GetMaxActions())
	assert.EqualValues(t, 2, artifacts.StateConfig.GetOptions().GetMaxConcurrentActions())
	assert.Equal(t, []*ConformanceTargetRole{
		{Name: "Reader", Actions: []string{"Read"}},
		{Name: "Writer", Actions: []string{"Write"}},
	}, artifacts.ConformanceTargetRoles)
	assert.NotEmpty(t, artifacts.Graph.Nodes)
	assert.NotEmpty(t, artifacts.Graph.Links)
	for nodeIndex, node := range artifacts.Graph.Nodes {
		var original map[string]json.RawMessage
		assert.NoError(t, json.Unmarshal(node.JSON, &original))
		state := node.State
		assert.JSONEq(t, string(original["state"]), string(state.Globals))
		assert.JSONEq(t, string(original["roles"]), string(state.Roles))
		assert.JSONEq(t, string(original["channels"]), string(state.Channels))
		assert.JSONEq(t, string(original["channel_messages"]), string(state.ChannelMessages))
		for _, linkIndex := range node.Inbound {
			assert.Equal(t, int64(nodeIndex), artifacts.Graph.Links[linkIndex].Dst)
		}
		for _, linkIndex := range node.Outbound {
			assert.Equal(t, int64(nodeIndex), artifacts.Graph.Links[linkIndex].Src)
		}
	}
}

func TestDecodeLinks(t *testing.T) {
	data, err := proto.Marshal(&pb.Links{Links: []*pb.Link{{
		Src: 2, Dest: 3, Name: "request.Process", Labels: []string{"label"}, ReqId: 4, NewToOldThreads: map[int64]int64{5: 4}, Type: "action",
	}}})
	assert.NoError(t, err)

	links, err := decodeLinks(data)
	assert.NoError(t, err)
	assert.Equal(t, []*Link{{Src: 2, Dst: 3, Name: "request.Process", Labels: []string{"label"}, ReqID: 4, NewToOldThreads: map[int64]int64{5: 4}, Type: "action"}}, links)
}

func TestDecodeLinksRejectsInvalidThreadIdentity(t *testing.T) {
	for _, link := range []*pb.Link{
		{ReqId: -1},
		{NewToOldThreads: map[int64]int64{-1: 0}},
		{NewToOldThreads: map[int64]int64{0: -1}},
		{NewToOldThreads: map[int64]int64{0: 1, 1: 1}},
	} {
		data, err := proto.Marshal(&pb.Links{Links: []*pb.Link{link}})
		require.NoError(t, err)

		links, err := decodeLinks(data)
		assert.ErrorContains(t, err, "thread")
		assert.Nil(t, links)
	}
}

func TestReadConformanceTargetRolesRequiresArgumentlessConformanceDecorator(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "argumentless", content: `{"roles":[{"name":"Worker","decorators":[{"name":"conformance","args":[]}],"actions":[{"name":"Init"},{"name":"Run"}]}]}`},
		{name: "with argument", content: `{"roles":[{"name":"Worker","decorators":[{"name":"conformance","args":[{"name":"value"}]}],"actions":[{"name":"Run"}]}]}`, wantErr: "no role with @conformance decorator"},
		{name: "case sensitive", content: `{"roles":[{"name":"Worker","decorators":[{"name":"Conformance"}],"actions":[{"name":"Run"}]}]}`, wantErr: "no role with @conformance decorator"},
		{name: "Init only", content: `{"roles":[{"name":"Worker","decorators":[{"name":"conformance"}],"actions":[{"name":"Init"}]}]}`, wantErr: "no non-Init action"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "spec_ast.json")
			require.NoError(t, os.WriteFile(name, []byte(testCase.content), 0o600))
			conformanceRoles, err := readConformanceTargetRoles(name)
			if testCase.wantErr != "" {
				assert.ErrorContains(t, err, testCase.wantErr)
				assert.Nil(t, conformanceRoles)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, []*ConformanceTargetRole{{Name: "Worker", Actions: []string{"Run"}}}, conformanceRoles)
		})
	}
}

func TestReadConformanceTargetRolesRejectsParallelAction(t *testing.T) {
	name := filepath.Join(t.TempDir(), "spec_ast.json")
	content := []byte(`{"roles":[{"name":"Worker","decorators":[{"name":"conformance"}],"actions":[{"name":"Run","block":{"flow":"FLOW_PARALLEL"}}]}]}`)
	require.NoError(t, os.WriteFile(name, content, 0o600))

	roles, err := readConformanceTargetRoles(name)
	assert.ErrorContains(t, err, "Worker.Run contains parallel execution")
	assert.Nil(t, roles)
}

func TestReadArtifactsRejectsInvalidInputs(t *testing.T) {
	validNodes, err := proto.Marshal(&pb.Nodes{Json: []string{`{"roles":[]}`}})
	require.NoError(t, err)
	invalidNodeJSON, err := proto.Marshal(&pb.Nodes{Json: []string{`{"roles":{}}`}})
	require.NoError(t, err)
	validLinks, err := proto.Marshal(&pb.Links{Links: []*pb.Link{{Src: 0, Dest: 0}}})
	require.NoError(t, err)
	valid := map[string][]byte{
		"spec_ast.json":                       []byte(`{"roles":[{"name":"Worker","decorators":[{"name":"conformance"}],"actions":[{"name":"Run"}]}]}`),
		"state_config.json":                   []byte(`{}`),
		"nodes_000000_of_000000.pb":           validNodes,
		"adjacency_lists_000000_of_000000.pb": validLinks,
	}
	tests := []struct {
		name     string
		omit     string
		replace  string
		contents []byte
		want     string
	}{
		{name: "missing spec AST", omit: "spec_ast.json", want: "read"},
		{name: "missing state config", omit: "state_config.json", want: "read"},
		{name: "missing nodes", omit: "nodes_000000_of_000000.pb", want: "required node artifacts"},
		{name: "missing links", omit: "adjacency_lists_000000_of_000000.pb", want: "required link artifacts"},
		{name: "invalid spec AST", replace: "spec_ast.json", contents: []byte(`{`), want: "decode"},
		{name: "invalid state config", replace: "state_config.json", contents: []byte(`{`), want: "decode"},
		{name: "invalid nodes protobuf", replace: "nodes_000000_of_000000.pb", contents: []byte{0x0a, 0x02, 'x'}, want: "decode"},
		{name: "invalid node JSON shape", replace: "nodes_000000_of_000000.pb", contents: invalidNodeJSON, want: "cannot unmarshal object"},
		{name: "invalid links protobuf", replace: "adjacency_lists_000000_of_000000.pb", contents: []byte{0x12, 0x02, 'x'}, want: "decode"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, contents := range valid {
				if name == test.omit {
					continue
				}
				if name == test.replace {
					contents = test.contents
				}
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), contents, 0o600))
			}
			artifacts, err := ReadArtifacts(dir)
			assert.ErrorContains(t, err, test.want)
			assert.Nil(t, artifacts)
		})
	}
}

func TestNewGraphRejectsOutOfRangeNode(t *testing.T) {
	for _, testCase := range []struct {
		name string
		link Link
	}{
		{name: "source", link: Link{Src: 1}},
		{name: "destination", link: Link{Dst: 1}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			graph, err := newGraph([]*Node{{}}, []*Link{&testCase.link})
			assert.Error(t, err)
			assert.Empty(t, graph)
		})
	}
}
