package renderer

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
)

// TemplateData contains the values and presentation helpers consumed by source templates.
type TemplateData struct {
	Module  string
	Role    string
	Actions []*action
	Roles   []roleSchema
}

type action struct {
	Role, Name string
	CaseFile   string
	Cases      []actionCase
	Roles      []roleSchema
}

type actionCase struct {
	Input   roleState
	Outputs []roleState
}

type roleState struct{ Roles []stateRole }

type stateRole struct {
	Name           string
	Params, Fields []fieldValue
}

type roleSchema struct {
	Name           string
	Params, Fields []field
}

type field struct{ JSONName, GoName, GoType, ComparisonType string }

type fieldValue struct {
	JSONName string
	GoName   string
	Value    any
}

// BuildTemplateData builds source-template data from validated Action cases.
func BuildTemplateData(modulePath, role string, cases []fizzbee.ActionCase) (*TemplateData, error) {
	if modulePath == "" {
		return nil, fmt.Errorf("module path is empty")
	}
	actions, err := newActions(cases)
	if err != nil {
		return nil, err
	}
	return newTemplateData(modulePath, role, actions), nil
}

func newActions(cases []fizzbee.ActionCase) ([]*action, error) {
	byAction := make(map[string][]fizzbee.ActionCase)
	for _, actionCase := range cases {
		key := actionCase.Action.Role + "\x00" + actionCase.Action.Action
		byAction[key] = append(byAction[key], actionCase)
	}
	keys := make([]string, 0, len(byAction))
	for key := range byAction {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	actions := make([]*action, 0, len(keys))
	for _, key := range keys {
		action, err := newAction(byAction[key])
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	if err := validateSharedSchemas(actions); err != nil {
		return nil, err
	}
	return actions, nil
}

func validateSharedSchemas(actions []*action) error {
	firstByRole := make(map[string]*action)
	for _, action := range actions {
		first, ok := firstByRole[action.Role]
		if !ok {
			firstByRole[action.Role] = action
			continue
		}
		if len(first.Roles) != len(action.Roles) {
			return fmt.Errorf("%s.%s has a different role schema from %s.%s", action.Role, action.Name, first.Role, first.Name)
		}
		for index, role := range action.Roles {
			original := first.Roles[index]
			if role.Name != original.Name || !sameFieldNames(role.Params, original.Params) || !sameFieldNames(role.Fields, original.Fields) {
				return fmt.Errorf("%s.%s has a different role schema from %s.%s", action.Role, action.Name, first.Role, first.Name)
			}
		}
	}
	return nil
}

func sameFieldNames(first, second []field) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index].JSONName != second[index].JSONName || first[index].GoName != second[index].GoName {
			return false
		}
	}
	return true
}

// newTemplateData returns source-template data for the supplied module, role, and Actions.
func newTemplateData(module, role string, actions []*action) *TemplateData {
	data := &TemplateData{Module: module, Role: role, Actions: actions}
	if role != "" && len(actions) > 0 {
		data.Roles = mergeRoleSchemas(actions)
	}
	return data
}

func mergeRoleSchemas(actions []*action) []roleSchema {
	roles := make([]roleSchema, len(actions[0].Roles))
	for index, role := range actions[0].Roles {
		roles[index] = roleSchema{Name: role.Name, Params: slices.Clone(role.Params), Fields: slices.Clone(role.Fields)}
		for i := range roles[index].Params {
			roles[index].Params[i].ComparisonType = roles[index].Params[i].GoType
		}
		for i := range roles[index].Fields {
			roles[index].Fields[i].ComparisonType = roles[index].Fields[i].GoType
		}
	}
	for _, action := range actions[1:] {
		for roleIndex, role := range action.Roles {
			for fieldIndex, field := range role.Params {
				roles[roleIndex].Params[fieldIndex].GoType = mergedGoType(roles[roleIndex].Params[fieldIndex].GoType, field.GoType)
			}
			for fieldIndex, field := range role.Fields {
				roles[roleIndex].Fields[fieldIndex].GoType = mergedGoType(roles[roleIndex].Fields[fieldIndex].GoType, field.GoType)
			}
		}
	}
	return roles
}

func mergedGoType(first, second string) string {
	if first == second || second == "any" {
		return first
	}
	if first == "any" {
		return second
	}
	return "any"
}

// newAction returns template data for cases belonging to one Action.
func newAction(cases []fizzbee.ActionCase) (*action, error) {
	if len(cases) == 0 {
		return nil, fmt.Errorf("action cases are empty")
	}
	first := cases[0]
	for _, actionCase := range cases {
		if actionCase.Action.Role != first.Action.Role || actionCase.Action.Action != first.Action.Action {
			return nil, fmt.Errorf("action cases include both %s.%s and %s.%s", first.Action.Role, first.Action.Action, actionCase.Action.Role, actionCase.Action.Action)
		}
	}
	roles, err := schemas(cases)
	if err != nil {
		return nil, err
	}
	action := &action{
		Role:     first.Action.Role,
		Name:     first.Action.Action,
		CaseFile: strings.ToLower(first.Action.Role) + "_" + strings.ToLower(first.Action.Action) + "_cases_generated.json",
		Roles:    roles,
		Cases:    make([]actionCase, 0, len(cases)),
	}
	for _, sourceCase := range cases {
		input, err := newRoleState(sourceCase.Input.Roles, roles)
		if err != nil {
			return nil, err
		}
		outputs := make([]roleState, 0, len(sourceCase.Outputs))
		for _, output := range sourceCase.Outputs {
			state, err := newRoleState(output.Roles, roles)
			if err != nil {
				return nil, err
			}
			outputs = append(outputs, state)
		}
		action.Cases = append(action.Cases, actionCase{Input: input, Outputs: outputs})
	}
	return action, nil
}

func schemas(cases []fizzbee.ActionCase) ([]roleSchema, error) {
	roles := cases[0].Input.Roles
	if len(roles) == 0 {
		return nil, fmt.Errorf("action roles are empty")
	}
	nameCounts := make(map[string]int)
	for _, role := range roles {
		nameCounts[role.Name]++
	}
	nameIndexes := make(map[string]int)
	schemas := make([]roleSchema, 0, len(roles))
	for roleIndex, role := range roles {
		nameIndexes[role.Name]++
		name := role.Name
		if nameCounts[role.Name] > 1 {
			name = fmt.Sprintf("%s%d", name, nameIndexes[role.Name])
		}
		params, err := objectSchema(cases, roleIndex, true, role.Params)
		if err != nil {
			return nil, err
		}
		fields, err := objectSchema(cases, roleIndex, false, role.Fields)
		if err != nil {
			return nil, err
		}
		schemas = append(schemas, roleSchema{Name: name, Params: params, Fields: fields})
	}
	return schemas, nil
}

func objectSchema(cases []fizzbee.ActionCase, roleIndex int, params bool, object fizzbee.JSONObject) ([]field, error) {
	keys := slices.Sorted(maps.Keys(object))
	fields := make([]field, 0, len(keys))
	usedNames := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		values, err := fieldValues(cases, roleIndex, params, key)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field{JSONName: key, GoName: uniqueGoName(key, usedNames), GoType: goType(values)})
	}
	return fields, nil
}

func fieldValues(cases []fizzbee.ActionCase, roleIndex int, params bool, key string) ([]any, error) {
	values := make([]any, 0, len(cases)*2)
	for _, actionCase := range cases {
		value, err := roleFieldValue(actionCase.Input.Roles, roleIndex, params, key)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		for _, output := range actionCase.Outputs {
			value, err := roleFieldValue(output.Roles, roleIndex, params, key)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
	}
	return values, nil
}

func roleFieldValue(roles []fizzbee.RoleState, roleIndex int, params bool, key string) (any, error) {
	if roleIndex >= len(roles) {
		return nil, fmt.Errorf("got %d roles, want role %d", len(roles), roleIndex)
	}
	object := roles[roleIndex].Fields
	if params {
		object = roles[roleIndex].Params
	}
	value, ok := object[key]
	if !ok {
		return nil, fmt.Errorf("role %d (%s) has no key %q", roleIndex, roles[roleIndex].Name, key)
	}
	return value, nil
}

func newRoleState(roles []fizzbee.RoleState, schemas []roleSchema) (roleState, error) {
	if len(roles) != len(schemas) {
		return roleState{}, fmt.Errorf("got %d roles, want %d", len(roles), len(schemas))
	}
	state := roleState{Roles: make([]stateRole, 0, len(roles))}
	for index, role := range roles {
		params, err := valuesForSchema(role.Params, schemas[index].Params)
		if err != nil {
			return roleState{}, err
		}
		fields, err := valuesForSchema(role.Fields, schemas[index].Fields)
		if err != nil {
			return roleState{}, err
		}
		state.Roles = append(state.Roles, stateRole{Name: schemas[index].Name, Params: params, Fields: fields})
	}
	return state, nil
}

func valuesForSchema(object fizzbee.JSONObject, schema []field) ([]fieldValue, error) {
	values := make([]fieldValue, 0, len(schema))
	for _, field := range schema {
		value, ok := object[field.JSONName]
		if !ok {
			return nil, fmt.Errorf("missing key %q", field.JSONName)
		}
		values = append(values, fieldValue{JSONName: field.JSONName, GoName: field.GoName, Value: value})
	}
	return values, nil
}

func goType(values []any) string {
	if len(values) == 0 {
		return "any"
	}
	nullable := false
	var kind fizzbee.ValueKind
	for _, value := range values {
		valueKind := fizzbee.ValueKindOf(value)
		if valueKind == fizzbee.ValueKindNull {
			nullable = true
			continue
		}
		if kind == fizzbee.ValueKindNull {
			kind = valueKind
			continue
		}
		if valueKind == kind {
			continue
		}
		if (kind == fizzbee.ValueKindInteger && valueKind == fizzbee.ValueKindFloat) || (kind == fizzbee.ValueKindFloat && valueKind == fizzbee.ValueKindInteger) {
			kind = fizzbee.ValueKindFloat
			continue
		}
		return "any"
	}
	var goType string
	switch kind {
	case fizzbee.ValueKindBool:
		goType = "bool"
	case fizzbee.ValueKindInteger:
		goType = "int"
	case fizzbee.ValueKindFloat:
		goType = "float64"
	case fizzbee.ValueKindString:
		goType = "string"
	case fizzbee.ValueKindArray:
		goType = "[]any"
	case fizzbee.ValueKindObject:
		goType = "map[string]any"
	default:
		return "any"
	}
	if nullable {
		return "*" + goType
	}
	return goType
}

func uniqueGoName(name string, used map[string]struct{}) string {
	base := goName(name)
	if base == "" || !unicode.IsUpper([]rune(base)[0]) {
		base = "Field_" + base
	}
	candidate := base
	for suffix := 2; ; suffix++ {
		if _, found := used[candidate]; !found {
			used[candidate] = struct{}{}
			return candidate
		}
		candidate = base + strconv.Itoa(suffix)
	}
}

func goName(name string) string {
	var builder strings.Builder
	for _, runeValue := range name {
		if unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue) || runeValue == '_' {
			builder.WriteRune(runeValue)
		} else {
			builder.WriteByte('_')
		}
	}
	parts := strings.Split(builder.String(), "_")
	for index, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		parts[index] = string(runes)
	}
	return strings.Join(parts, "")
}
