package artifact

import (
	"bytes"
	"encoding/json"
)

type nodeExecutionState struct {
	Threads []json.RawMessage `json:"threads"`
}

// Node contains one JSON state from nodes_*.pb and indexes of adjacent Links.
//
// A node JSON value resembles:
//
//	{
//	  "roles":[{"name":"Request", "ref":2, "params":{}, "fields":{}}],
//	  "state":{}, "threads":[...]
//	}
type Node struct {
	// JSON is the original node state JSON from protobuf field Nodes.json.
	JSON json.RawMessage
	// State is the validated model-state portion of JSON.
	State *State
	// Threads is the validated JSON threads array used for thread identity tracking.
	Threads []json.RawMessage
	// Inbound contains indexes into Graph.Links whose Dst is this Node.
	Inbound []int
	// Outbound contains indexes into Graph.Links whose Src is this Node.
	Outbound []int
}

// NewNode validates and converts one JSON node from a Nodes protobuf message.
func NewNode(message json.RawMessage) (*Node, error) {
	state, err := decodeState(message)
	if err != nil {
		return nil, err
	}
	var execution nodeExecutionState
	if err := json.Unmarshal(message, &execution); err != nil {
		return nil, err
	}
	return &Node{JSON: bytes.Clone(message), State: state, Threads: execution.Threads}, nil
}
