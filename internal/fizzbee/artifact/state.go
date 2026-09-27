package artifact

import (
	"bytes"
	"encoding/json"
)

// State contains the model-state fields selected from one FizzBee node JSON.
//
// Given a node JSON such as:
//
//	{
//	  "state":{"key":1}, "roles":[...],
//	  "channels":{}, "channel_messages":{}, "threads":[...]
//	}
//
// State retains the first four fields and excludes exploration metadata such as threads and stats.
type State struct {
	// Globals is the top-level state JSON value, such as `{"key":1}`.
	Globals json.RawMessage `json:"state"`
	// Roles is the top-level roles JSON array, such as `[ {"name":"Request","ref":2,...} ]`.
	Roles json.RawMessage `json:"roles"`
	// Channels is the top-level channels JSON value.
	Channels json.RawMessage `json:"channels"`
	// ChannelMessages is the top-level channel_messages JSON value.
	ChannelMessages json.RawMessage `json:"channel_messages"`

	// RoleStates is the decoded representation of Roles.
	RoleStates []RoleState `json:"-"`
}

func NewState(globals, roles, channels, channelMessages json.RawMessage) *State {
	return &State{
		Globals:         globals,
		Roles:           roles,
		Channels:        channels,
		ChannelMessages: channelMessages,
	}
}

func decodeState(data json.RawMessage) (*State, error) {
	rawState := &State{}
	if err := json.Unmarshal(data, rawState); err != nil {
		return nil, err
	}

	state := NewState(
		bytes.Clone(rawState.Globals),
		bytes.Clone(rawState.Roles),
		bytes.Clone(rawState.Channels),
		bytes.Clone(rawState.ChannelMessages),
	)
	if err := json.Unmarshal(state.Roles, &state.RoleStates); err != nil {
		return nil, err
	}
	return state, nil
}
