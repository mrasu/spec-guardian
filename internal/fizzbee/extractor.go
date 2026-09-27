package fizzbee

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/mrasu/spec-guardian/internal/fizzbee/artifact"
)

type pendingActionCase struct {
	Action  ActionRef
	Input   Input
	Outputs []Output
}

// actionStart records one matching Action-start Link and its destination thread identity.
//
// It comes from a Link such as:
//
//	{
//	  "name":"Request#2.Process", "reqId":0,
//	  "type":"action"
//	}
type actionStart struct {
	// Action is the concrete Action parsed from the Link name.
	Action ActionRef
	// LinkIndex is the index of the Action-start Link in Graph.Links.
	LinkIndex int
	// ThreadID is the Action thread's index in the start Link destination Node.
	ThreadID int64
	// Input is the model state at the start Link source Node.
	Input *artifact.State
}

// actionExecution contains the input and all completion states for one Action start.
//
// It is derived from a start Link plus the paths of its tracked thread; it has no direct artifact JSON form.
type actionExecution struct {
	// Action identifies the concrete Action that started the tracked thread.
	Action ActionRef
	// Input is the model state before that Action started.
	Input *artifact.State
	// Ends contains model states where the tracked thread no longer exists.
	Ends []*artifact.State
}

// ReadActionCases reads dir and extracts cases for every @conformance Action.
func ReadActionCases(dir string) ([]ActionCase, error) {
	artifacts, err := artifact.ReadArtifacts(dir)
	if err != nil {
		return nil, err
	}

	var cases []ActionCase
	for _, role := range artifacts.ConformanceTargetRoles {
		for _, action := range role.Actions {
			actionCases, err := extractActionCases(artifacts.Graph, role.Name, action)
			if err != nil {
				return nil, err
			}
			validatedCases, err := NewActionCases(actionCases)
			if err != nil {
				return nil, err
			}
			cases = append(cases, validatedCases...)
		}
	}
	return cases, nil
}

// extractActionCases extracts complete action executions for every reachable input.
func extractActionCases(graph *artifact.Graph, role, action string) ([]ActionCase, error) {
	executions, err := extractActionExecutions(graph, role, action)
	if err != nil {
		return nil, err
	}

	byInput := make(map[string]*pendingActionCase, len(executions))
	for _, execution := range executions {
		roles, err := selectTestBoundaryRoleStates(graph, execution.Input, execution.Action.Role, execution.Action.Ref)
		if err != nil {
			return nil, err
		}
		input, err := NewInput(roles)
		if err != nil {
			return nil, err
		}
		inputKey, err := canonicalizeJSON(input)
		if err != nil {
			return nil, err
		}
		actionCase := byInput[inputKey]
		if actionCase == nil {
			actionCase = &pendingActionCase{Action: execution.Action, Input: input}
			byInput[inputKey] = actionCase
		}

		outputs := make(map[string]Output, len(actionCase.Outputs)+len(execution.Ends))
		for _, output := range actionCase.Outputs {
			outputKey, err := canonicalizeJSON(output)
			if err != nil {
				return nil, err
			}
			outputs[outputKey] = output
		}
		for _, end := range execution.Ends {
			roleStates, err := selectTestBoundaryRoleStates(graph, end, execution.Action.Role, execution.Action.Ref)
			if err != nil {
				return nil, err
			}
			output, err := NewOutput(roleStates)
			if err != nil {
				return nil, err
			}
			outputKey, err := canonicalizeJSON(output)
			if err != nil {
				return nil, err
			}
			outputs[outputKey] = output
		}
		outputKeys := slices.Sorted(maps.Keys(outputs))
		actionCase.Outputs = actionCase.Outputs[:0]
		for _, outputKey := range outputKeys {
			actionCase.Outputs = append(actionCase.Outputs, outputs[outputKey])
		}
	}

	inputKeys := slices.Sorted(maps.Keys(byInput))
	cases := make([]ActionCase, 0, len(inputKeys))
	for _, inputKey := range inputKeys {
		actionCase, err := NewActionCase(byInput[inputKey].Action, byInput[inputKey].Input, byInput[inputKey].Outputs)
		if err != nil {
			return nil, err
		}
		cases = append(cases, actionCase)
	}
	return cases, nil
}

func extractActionExecutions(graph *artifact.Graph, role, action string) ([]actionExecution, error) {
	starts, err := findActionStarts(graph, role, action)
	if err != nil {
		return nil, err
	}
	if len(starts) == 0 {
		return nil, fmt.Errorf("no start link found for %s.%s", role, action)
	}

	executions := make([]actionExecution, 0, len(starts))
	for _, start := range starts {
		ends, skippedInterleaving, err := findActionEndStates(graph, start)
		if err != nil {
			return nil, err
		}
		if len(ends) == 0 {
			if !skippedInterleaving {
				slog.Warn("cannot extract completed action execution", "role", start.Action.Role, "ref", start.Action.Ref, "action", start.Action.Action, "start_link", start.LinkIndex)
			}
			continue
		}
		executions = append(executions, actionExecution{Action: start.Action, Input: start.Input, Ends: ends})
	}
	if len(executions) == 0 {
		return nil, fmt.Errorf("no completed action executions found for %s.%s from %d start candidates", role, action, len(starts))
	}
	return executions, nil
}

func findActionStarts(graph *artifact.Graph, role, action string) ([]actionStart, error) {
	var starts []actionStart
	for linkIndex, link := range graph.Links {
		if link.Type != "action" {
			continue
		}
		actionRef, ok := parseActionRef(link.Name)
		if !ok || actionRef.Role != role || actionRef.Action != action {
			continue
		}
		threadID, ok := mapThreadID(link.ReqID, link)
		if !ok {
			return nil, fmt.Errorf("start link %d for %s.%s does not preserve its thread identity", linkIndex, role, action)
		}
		starts = append(starts, actionStart{Action: actionRef, LinkIndex: linkIndex, ThreadID: threadID, Input: graph.Nodes[link.Src].State})
	}
	return starts, nil
}

func findActionEndStates(graph *artifact.Graph, start actionStart) ([]*artifact.State, bool, error) {
	startLink := graph.Links[start.LinkIndex]
	queue := []threadNode{{NodeIndex: int(startLink.Dst), ThreadID: start.ThreadID}}
	visited := make(map[threadNode]bool)
	var ends []*artifact.State
	skippedInterleaving := false
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		running := isThreadRunning(graph, current.NodeIndex, current.ThreadID)
		if !running {
			ends = append(ends, graph.Nodes[current.NodeIndex].State)
			continue
		}
		for _, linkIndex := range graph.Nodes[current.NodeIndex].Outbound {
			link := graph.Links[linkIndex]
			if link.Type == "action" || link.ReqID != current.ThreadID {
				skippedInterleaving = true
				continue
			}
			nextThreadID, ok := mapThreadID(current.ThreadID, link)
			if !ok {
				continue
			}
			queue = append(queue, threadNode{NodeIndex: int(link.Dst), ThreadID: nextThreadID})
		}
	}
	return ends, skippedInterleaving, nil
}

type threadNode struct {
	// NodeIndex is an index into Graph.Nodes for the current state of the tracked thread.
	NodeIndex int
	// ThreadID is the thread's index in that Node's JSON threads array.
	ThreadID int64
}

// mapThreadID returns the destination thread identity and whether it can be followed.
func mapThreadID(oldThreadID int64, link *artifact.Link) (int64, bool) {
	// An empty NewToOldThreads means FizzBee did not need to record a thread-index remapping,
	// so the source index is preserved as an implicit identity mapping. In that case the bool
	// is true even though the map has no explicit entry. A non-empty map is destination-to-source,
	// so this function finds the destination key whose value is oldThreadID.
	if len(link.NewToOldThreads) == 0 {
		return oldThreadID, true
	}
	for newThreadID, mappedOldThreadID := range link.NewToOldThreads {
		if mappedOldThreadID != oldThreadID {
			continue
		}
		return newThreadID, true
	}
	return 0, false
}

func isThreadRunning(graph *artifact.Graph, nodeIndex int, threadID int64) bool {
	node := graph.Nodes[nodeIndex]
	return int(threadID) < len(node.Threads) && node.Threads[threadID] != nil
}

func selectTestBoundaryRoleStates(graph *artifact.Graph, state *artifact.State, targetName string, targetRef uint64) ([]RoleState, error) {
	selected := make([]RoleState, 0, len(state.RoleStates))
	found := false
	for _, role := range state.RoleStates {
		isTarget := role.Name == targetName && role.Ref == targetRef
		if !isTarget && graph.IsConformanceTargetRole(role.Name) {
			continue
		}
		params, err := NewJSONObject(role.Params)
		if err != nil {
			return nil, err
		}
		fields, err := NewJSONObject(role.Fields)
		if err != nil {
			return nil, err
		}
		roleState, err := NewRoleState(role.Name, params, fields)
		if err != nil {
			return nil, err
		}
		selected = append(selected, roleState)
		found = found || isTarget
	}
	if !found {
		return nil, fmt.Errorf("role instance %s#%d for test state not found", targetName, targetRef)
	}
	return selected, nil
}

func parseActionRef(name string) (ActionRef, bool) {
	roleInstance, action, ok := strings.Cut(name, ".")
	if !ok {
		return ActionRef{}, false
	}
	role, ref, ok := strings.Cut(roleInstance, "#")
	if !ok {
		return ActionRef{}, false
	}
	reference, err := strconv.ParseUint(ref, 10, 64)
	if err != nil {
		return ActionRef{}, false
	}
	actionRef, err := NewActionRef(role, reference, action)
	if err != nil {
		return ActionRef{}, false
	}
	return actionRef, true
}

func canonicalizeJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}
