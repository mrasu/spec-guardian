package fizzbee

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mrasu/spec-guardian/internal/fizzbee/artifact"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadActionCases(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want []ActionCase
	}{
		{
			name: "Read and DB",
			dir:  "read_db",
			want: []ActionCase{
				{
					Action: ActionRef{Role: "Reader", Action: "Read"},
					Input: Input{Roles: []RoleState{
						{Name: "DB", Params: JSONObject{}, Fields: JSONObject{"value": "before"}},
						{Name: "Reader", Params: JSONObject{}, Fields: JSONObject{"status": "INIT", "value": nil}},
					}},
					Outputs: []Output{{Roles: []RoleState{
						{Name: "DB", Params: JSONObject{}, Fields: JSONObject{"value": "before"}},
						{Name: "Reader", Params: JSONObject{}, Fields: JSONObject{"status": "DONE", "value": "before"}},
					}}},
				},
			},
		},
		{
			name: "Read Write and DB",
			dir:  "read_write_db",
			want: []ActionCase{
				{
					Action: ActionRef{Role: "Reader", Action: "Read"},
					Input: Input{Roles: []RoleState{
						{Name: "DB", Params: JSONObject{}, Fields: JSONObject{"value": "after"}},
						{Name: "Reader", Params: JSONObject{}, Fields: JSONObject{"status": "INIT", "value": nil}},
					}},
					Outputs: []Output{{Roles: []RoleState{
						{Name: "DB", Params: JSONObject{}, Fields: JSONObject{"value": "after"}},
						{Name: "Reader", Params: JSONObject{}, Fields: JSONObject{"status": "DONE", "value": "after"}},
					}}},
				},
				{
					Action: ActionRef{Role: "Reader", Action: "Read"},
					Input: Input{Roles: []RoleState{
						{Name: "DB", Params: JSONObject{}, Fields: JSONObject{"value": "before"}},
						{Name: "Reader", Params: JSONObject{}, Fields: JSONObject{"status": "INIT", "value": nil}},
					}},
					Outputs: []Output{{Roles: []RoleState{
						{Name: "DB", Params: JSONObject{}, Fields: JSONObject{"value": "before"}},
						{Name: "Reader", Params: JSONObject{}, Fields: JSONObject{"status": "DONE", "value": "before"}},
					}}},
				},
				{
					Action: ActionRef{Role: "Writer", Action: "Write"},
					Input: Input{Roles: []RoleState{
						{Name: "DB", Params: JSONObject{}, Fields: JSONObject{"value": "before"}},
						{Name: "Writer", Params: JSONObject{}, Fields: JSONObject{"status": "INIT"}},
					}},
					Outputs: []Output{{Roles: []RoleState{
						{Name: "DB", Params: JSONObject{}, Fields: JSONObject{"value": "after"}},
						{Name: "Writer", Params: JSONObject{}, Fields: JSONObject{"status": "DONE"}},
					}}},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ReadActionCases(filepath.Join("testdata", test.dir, "fizzbee_output"))
			require.NoError(t, err)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("ReadActionCases() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExtractActionCasesAcrossRoleExecutionOrders(t *testing.T) {
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	tests := []struct {
		name    string
		states  []string
		links   []*artifact.Link
		targets []string
		want    map[string]ActionCase
		wantErr map[string]string
	}{
		{
			name: "one role runs from input to output",
			states: []string{
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"READY"}}],"threads":[]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"RUNNING"}}],"threads":[{}]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"DONE"}}],"threads":[]}`,
			},
			links: []*artifact.Link{
				{Src: 0, Dst: 1, Name: "First#1.Run", ReqID: 0, Type: "action"},
				{Src: 1, Dst: 2, Name: "thread-0", ReqID: 0},
			},
			targets: []string{"First"},
			want: map[string]ActionCase{
				"First": {
					Action: ActionRef{Role: "First", Ref: 1, Action: "Run"},
					Input: Input{Roles: []RoleState{
						{Name: "First", Params: JSONObject{}, Fields: JSONObject{"status": "READY"}},
					}},
					Outputs: []Output{{Roles: []RoleState{
						{Name: "First", Params: JSONObject{}, Fields: JSONObject{"status": "DONE"}},
					}}},
				},
			},
		},
		{
			name: "second role starts and finishes before first role",
			states: []string{
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"READY"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"READY"}}],"threads":[]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"RUNNING"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"READY"}}],"threads":[{}]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"RUNNING"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"RUNNING"}}],"threads":[{},{}]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"RUNNING"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"DONE"}}],"threads":[{}]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"DONE"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"DONE"}}],"threads":[]}`,
			},
			links: []*artifact.Link{
				{Src: 0, Dst: 1, Name: "First#1.Run", ReqID: 0, Type: "action"},
				{Src: 1, Dst: 2, Name: "Second#2.Run", ReqID: 1, Type: "action"},
				{Src: 2, Dst: 3, Name: "thread-1", ReqID: 1},
				{Src: 3, Dst: 4, Name: "thread-0", ReqID: 0},
			},
			targets: []string{"First", "Second"},
			want: map[string]ActionCase{
				"Second": {
					Action: ActionRef{Role: "Second", Ref: 2, Action: "Run"},
					Input: Input{Roles: []RoleState{
						{Name: "First", Params: JSONObject{}, Fields: JSONObject{"status": "RUNNING"}},
						{Name: "Second", Params: JSONObject{}, Fields: JSONObject{"status": "READY"}},
					}},
					Outputs: []Output{{Roles: []RoleState{
						{Name: "First", Params: JSONObject{}, Fields: JSONObject{"status": "RUNNING"}},
						{Name: "Second", Params: JSONObject{}, Fields: JSONObject{"status": "DONE"}},
					}}},
				},
			},
			wantErr: map[string]string{"First": "no completed action executions found"},
		},
		{
			name: "roles run sequentially",
			states: []string{
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"READY"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"READY"}}],"threads":[]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"RUNNING"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"READY"}}],"threads":[{}]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"DONE"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"READY"}}],"threads":[]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"DONE"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"RUNNING"}}],"threads":[{}]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"DONE"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"DONE"}}],"threads":[]}`,
			},
			links: []*artifact.Link{
				{Src: 0, Dst: 1, Name: "First#1.Run", ReqID: 0, Type: "action"},
				{Src: 1, Dst: 2, Name: "thread-0", ReqID: 0},
				{Src: 2, Dst: 3, Name: "Second#2.Run", ReqID: 0, Type: "action"},
				{Src: 3, Dst: 4, Name: "thread-0", ReqID: 0},
			},
			targets: []string{"First", "Second"},
			want: map[string]ActionCase{
				"First": {
					Action: ActionRef{Role: "First", Ref: 1, Action: "Run"},
					Input: Input{Roles: []RoleState{
						{Name: "First", Params: JSONObject{}, Fields: JSONObject{"status": "READY"}},
						{Name: "Second", Params: JSONObject{}, Fields: JSONObject{"status": "READY"}},
					}},
					Outputs: []Output{{Roles: []RoleState{
						{Name: "First", Params: JSONObject{}, Fields: JSONObject{"status": "DONE"}},
						{Name: "Second", Params: JSONObject{}, Fields: JSONObject{"status": "READY"}},
					}}},
				},
				"Second": {
					Action: ActionRef{Role: "Second", Ref: 2, Action: "Run"},
					Input: Input{Roles: []RoleState{
						{Name: "First", Params: JSONObject{}, Fields: JSONObject{"status": "DONE"}},
						{Name: "Second", Params: JSONObject{}, Fields: JSONObject{"status": "READY"}},
					}},
					Outputs: []Output{{Roles: []RoleState{
						{Name: "First", Params: JSONObject{}, Fields: JSONObject{"status": "DONE"}},
						{Name: "Second", Params: JSONObject{}, Fields: JSONObject{"status": "DONE"}},
					}}},
				},
			},
		},
		{
			name: "first role finishes while second role is running",
			states: []string{
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"READY"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"READY"}}],"threads":[]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"RUNNING"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"READY"}}],"threads":[{}]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"RUNNING"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"RUNNING"}}],"threads":[{},{}]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"DONE"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"RUNNING"}}],"threads":[{}]}`,
				`{"roles":[{"name":"First","ref":1,"params":{},"fields":{"status":"DONE"}},{"name":"Second","ref":2,"params":{},"fields":{"status":"DONE"}}],"threads":[]}`,
			},
			links: []*artifact.Link{
				{Src: 0, Dst: 1, Name: "First#1.Run", ReqID: 0, Type: "action"},
				{Src: 1, Dst: 2, Name: "Second#2.Run", ReqID: 1, Type: "action"},
				{Src: 2, Dst: 3, Name: "thread-0", ReqID: 0, NewToOldThreads: map[int64]int64{0: 1}},
				{Src: 3, Dst: 4, Name: "thread-0", ReqID: 0},
			},
			targets: []string{"First", "Second"},
			wantErr: map[string]string{
				"First":  "no completed action executions found",
				"Second": "no completed action executions found",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := newTestGraph(t, test.states...)
			graph.Links = test.links
			connectGraph(t, graph)

			for _, target := range test.targets {
				cases, err := extractActionCases(graph, target, "Run")
				if wantErr := test.wantErr[target]; wantErr != "" {
					assert.ErrorContains(t, err, wantErr)
					assert.Nil(t, cases)
					continue
				}

				require.NoError(t, err)
				require.Len(t, cases, 1)
				if diff := cmp.Diff(test.want[target], cases[0]); diff != "" {
					t.Errorf("extractActionCases() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
	assert.Empty(t, logs.String())
}

func TestExtractActionCasesWarnsWhenOnlySomeStartsComplete(t *testing.T) {
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	graph := newTestGraph(t,
		`{"roles":[{"name":"Worker","ref":1,"params":{},"fields":{"status":"READY"}}],"threads":[]}`,
		`{"roles":[{"name":"Worker","ref":1,"params":{},"fields":{"status":"RUNNING"}}],"threads":[{}]}`,
		`{"roles":[{"name":"Worker","ref":1,"params":{},"fields":{"status":"DONE"}}],"threads":[]}`,
		`{"roles":[{"name":"Worker","ref":2,"params":{},"fields":{"status":"READY"}}],"threads":[]}`,
		`{"roles":[{"name":"Worker","ref":2,"params":{},"fields":{"status":"RUNNING"}}],"threads":[{}]}`,
	)
	graph.Links = []*artifact.Link{
		{Src: 0, Dst: 1, Name: "Worker#1.Run", ReqID: 0, Type: "action"},
		{Src: 1, Dst: 2, Name: "thread-0", ReqID: 0},
		{Src: 3, Dst: 4, Name: "Worker#2.Run", ReqID: 0, Type: "action"},
	}
	connectGraph(t, graph)

	cases, err := extractActionCases(graph, "Worker", "Run")
	require.NoError(t, err)
	assert.Len(t, cases, 1)
	assert.Contains(t, logs.String(), "cannot extract completed action execution")
}

func TestGraphActionCasesTracksThreadIdentity(t *testing.T) {
	graph := newTestGraph(t,
		`{"roles":[{"name":"Worker","ref":1,"params":{"id":1},"fields":{"status":"READY"}}],"threads":[]}`,
		`{"roles":[{"name":"Worker","ref":1,"params":{"id":1},"fields":{"status":"RUNNING"}}],"threads":[{}]}`,
		`{"roles":[{"name":"Worker","ref":1,"params":{"id":1},"fields":{"status":"RUNNING"}}],"threads":[null,{}]}`,
		`{"roles":[{"name":"Worker","ref":1,"params":{"id":1},"fields":{"status":"DONE"}}],"threads":[]}`,
		`{"roles":[{"name":"Worker","ref":1,"params":{"id":1},"fields":{"status":"INTERVENED"}}],"threads":[]}`,
	)
	graph.Links = []*artifact.Link{
		{Src: 0, Dst: 1, Name: "Worker#1.Run", ReqID: 0, Type: "action"},
		{Src: 1, Dst: 2, Name: "thread-0", ReqID: 0, NewToOldThreads: map[int64]int64{1: 0}},
		{Src: 2, Dst: 3, Name: "thread-1", ReqID: 1},
		{Src: 1, Dst: 4, Name: "Other#1.Run", ReqID: 0, Type: "action"},
	}
	connectGraph(t, graph)

	cases, err := extractActionCases(graph, "Worker", "Run")
	require.NoError(t, err)
	require.Len(t, cases, 1)

	want := ActionCase{
		Action: ActionRef{Role: "Worker", Ref: 1, Action: "Run"},
		Input: Input{Roles: []RoleState{
			{Name: "Worker", Params: JSONObject{"id": json.Number("1")}, Fields: JSONObject{"status": "READY"}},
		}},
		Outputs: []Output{{Roles: []RoleState{
			{Name: "Worker", Params: JSONObject{"id": json.Number("1")}, Fields: JSONObject{"status": "DONE"}},
		}}},
	}

	if diff := cmp.Diff(want, cases[0]); diff != "" {
		t.Errorf("extractActionCases() mismatch (-want +got):\n%s", diff)
	}
}

func TestGraphActionCasesIncludesStartsAfterOtherActionsAndCombinesInputs(t *testing.T) {
	graph := newTestGraph(t,
		`{"roles":[{"name":"Worker","ref":1,"params":{"id":1},"fields":{"status":"READY"}}],"threads":[]}`,
		`{"roles":[{"name":"Worker","ref":1,"params":{"id":1},"fields":{"status":"RUNNING"}}],"threads":[{}]}`,
		`{"roles":[{"name":"Worker","ref":1,"params":{"id":1},"fields":{"status":"FIRST"}}],"threads":[]}`,
		`{"roles":[{"name":"Worker","ref":2,"params":{"id":1},"fields":{"status":"READY"}}],"threads":[]}`,
		`{"roles":[{"name":"Worker","ref":2,"params":{"id":1},"fields":{"status":"RUNNING"}}],"threads":[{}]}`,
		`{"roles":[{"name":"Worker","ref":2,"params":{"id":1},"fields":{"status":"SECOND"}}],"threads":[]}`,
	)
	graph.Links = []*artifact.Link{
		{Src: 0, Dst: 1, Name: "Worker#1.Run", ReqID: 0, Type: "action"},
		{Src: 1, Dst: 2, Name: "thread-0", ReqID: 0},
		{Src: 3, Dst: 4, Name: "Worker#2.Run", ReqID: 0, Type: "action"},
		{Src: 4, Dst: 5, Name: "thread-0", ReqID: 0},
	}
	connectGraph(t, graph)

	cases, err := extractActionCases(graph, "Worker", "Run")
	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Len(t, cases[0].Outputs, 2)

	want := ActionCase{
		Action: ActionRef{
			Role:   "Worker",
			Ref:    1,
			Action: "Run",
		},
		Input: Input{Roles: []RoleState{
			{Name: "Worker", Params: JSONObject{"id": json.Number("1")}, Fields: JSONObject{"status": "READY"}},
		}},
		Outputs: []Output{
			{
				Roles: []RoleState{
					{Name: "Worker", Params: JSONObject{"id": json.Number("1")}, Fields: JSONObject{"status": "FIRST"}},
				},
			},
			{
				Roles: []RoleState{
					{Name: "Worker", Params: JSONObject{"id": json.Number("1")}, Fields: JSONObject{"status": "SECOND"}},
				},
			},
		},
	}

	if diff := cmp.Diff(want, cases[0]); diff != "" {
		t.Errorf("extractActionCases() mismatch (-want +got):\n%s", diff)
	}
}

func newTestGraph(t *testing.T, states ...string) *artifact.Graph {
	t.Helper()
	nodes := make([]*artifact.Node, 0, len(states))
	for _, state := range states {
		node, err := artifact.NewNode(json.RawMessage(state))
		require.NoError(t, err)
		nodes = append(nodes, node)
	}
	return &artifact.Graph{Nodes: nodes}
}

func connectGraph(t *testing.T, graph *artifact.Graph) {
	t.Helper()
	for index, link := range graph.Links {
		graph.Nodes[link.Src].Outbound = append(graph.Nodes[link.Src].Outbound, index)
		graph.Nodes[link.Dst].Inbound = append(graph.Nodes[link.Dst].Inbound, index)
	}
}

func TestCanonicalJSON(t *testing.T) {
	got, err := canonicalizeJSON(json.RawMessage(`{"z":1,"a":{"b":2,"a":1}}`))
	require.NoError(t, err)
	assert.Equal(t, `{"a":{"a":1,"b":2},"z":1}`, got)
}

func TestCanonicalBoundaryOmitsRoleIdentity(t *testing.T) {
	params, err := NewJSONObject(json.RawMessage(`{"id":1}`))
	require.NoError(t, err)
	fields, err := NewJSONObject(json.RawMessage(`{"value":2}`))
	require.NoError(t, err)
	role, err := NewRoleState("Worker", params, fields)
	require.NoError(t, err)
	got, err := canonicalizeJSON([]RoleState{role})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"params":{"id":1},"fields":{"value":2}}]`, got)
	assert.NotContains(t, got, "Worker")
}

func TestPublicStateConstructorsRejectInvalidValues(t *testing.T) {
	_, err := NewActionRef("", 0, "Run")
	assert.ErrorContains(t, err, "role is empty")

	_, err = NewJSONObject(json.RawMessage(`{`))
	assert.Error(t, err)

	_, err = NewInput(nil)
	assert.ErrorContains(t, err, "roles are empty")

	action, err := NewActionRef("Worker", 1, "Run")
	require.NoError(t, err)
	params, err := NewJSONObject(json.RawMessage(`{}`))
	require.NoError(t, err)
	fields, err := NewJSONObject(json.RawMessage(`{}`))
	require.NoError(t, err)
	role, err := NewRoleState("Worker", params, fields)
	require.NoError(t, err)
	input, err := NewInput([]RoleState{role})
	require.NoError(t, err)
	_, err = NewActionCase(action, input, nil)
	assert.ErrorContains(t, err, "outputs are empty")
}
