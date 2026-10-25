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
	Hooks                Hooks
	FetchedActionCrashed bool
	ManualActionCrashed  bool
	Module               string
	Role                 string
	Actions              []*action
	Roles                []roleSchema
	DeclaredTypes        []DeclaredType
	Providers            []Provider
	ProviderGroups       []ProviderGroup
	SourceImports        []SourceImport
	ShapeFunctions       []string
}

func (d *TemplateData) ObservedRoles() []roleSchema {
	return rolesWithFields(d.Roles, func(f field) bool { return !f.Unobservable })
}

func (d *TemplateData) FetchedRoles() []roleSchema {
	return rolesWithFields(d.Roles, func(f field) bool { return f.Observed })
}

func (d *TemplateData) ManualRoles() []roleSchema {
	return rolesWithFields(d.Roles, func(f field) bool { return !f.Observed && !f.Unobservable })
}

func rolesWithFields(roles []roleSchema, include func(field) bool) []roleSchema {
	var selected []roleSchema
	for _, role := range roles {
		if slices.ContainsFunc(role.Fields, include) {
			selected = append(selected, role)
		}
	}
	return selected
}

// DeclaredType is a generated Go type shared by model, observed, and comparison states.
type DeclaredType struct {
	Name       string
	Definition string
}

// Declarations contains validated field settings used during generation.
type Declarations struct {
	Hooks                 Hooks
	CompareActionCrashed  *bool
	ActionCrashedProvider *Provider
	Providers             []Provider
	TypedFields           []TypedField
	Unobservable          []UnobservableField
	ExcludedRoles         []ExcludedRole
}

// ExcludedRole excludes every field of a source role from observation and comparison.
type ExcludedRole struct {
	Name   string
	Reason string
}

// TypedField declares a field shape supplied by manual observation.
type TypedField struct {
	Field *Field
	Type  Type
}

// UnobservableField identifies a model field excluded from observation and comparison.
type UnobservableField struct {
	Field  *Field
	Reason string
}

// Provider is a validated field request for a declared provider.
type Provider struct {
	ProviderField                                           string
	Metadata                                                bool
	Result                                                  bool
	Field                                                   *Field
	ProviderID, RoleName, FieldName, GoName, ProviderGoName string
	SourceImportPath, SourceAlias                           string
	Capabilities                                            []string
	Kind, ValueKind                                         string
	ConvertType                                             string
	Type                                                    Type
	TypeLiteral                                             string
	ProviderConfig, FieldConfig                             map[string]any
}

// ProviderGroup contains all fields served by one named provider.
type ProviderGroup struct {
	Seed        bool
	GoField     string
	ID          string
	GoName      string
	SourceAlias string
	Config      map[string]any
	Fields      []Provider
	Interfaces  []string
}

// SourceImport is one provider source package used by a generated role.
type SourceImport struct {
	Alias string
	Path  string
}

type action struct {
	Role, Name           string
	CompareActionCrashed bool
	CaseFile             string
	Cases                []actionCase
	Roles                []roleSchema
}

type actionCase struct {
	Input   roleState
	Outputs []roleState
}

type roleState struct {
	Roles      []stateRole
	SgMetadata fizzbee.SgMetadata
}

type stateRole struct {
	Name           string
	Params, Fields []fieldValue
}

type roleSchema struct {
	Name           string
	SourceName     string
	Params, Fields []field
}

func (r roleSchema) matchesSource(name string) bool {
	if r.SourceName != "" {
		return r.SourceName == name
	}
	return r.Name == name
}

type field struct {
	JSONName       string
	GoName         string
	GoType         string
	ComparisonType string
	Observed       bool
	Unobservable   bool
	Shape          *declaredShape
}

type fieldValue struct {
	JSONName string
	GoName   string
	Value    any
}

// BuildTemplateData builds source-template data from validated Action cases.
func BuildTemplateData(modulePath, role string, model fizzbee.Model, declarations Declarations) (*TemplateData, error) {
	if modulePath == "" {
		return nil, fmt.Errorf("module path is empty")
	}
	actions, err := newActions(model.Cases)
	if err != nil {
		return nil, err
	}
	data := newTemplateData(modulePath, role, actions)
	data.Hooks = declarations.Hooks
	for _, action := range actions {
		action.CompareActionCrashed = model.Options.CrashOnYield
		if declarations.CompareActionCrashed != nil {
			action.CompareActionCrashed = *declarations.CompareActionCrashed
		}
		if action.CompareActionCrashed {
			if declarations.ActionCrashedProvider != nil {
				data.FetchedActionCrashed = true
			} else {
				data.ManualActionCrashed = true
			}
		}
	}
	providers := append([]Provider(nil), declarations.Providers...)
	if data.FetchedActionCrashed {
		providers = append(providers, *declarations.ActionCrashedProvider)
	}
	if len(providers) > 0 {
		applyProviders(data, providers)
	}
	if len(declarations.TypedFields) > 0 {
		applyTypedFields(data, declarations.TypedFields)
	}
	if len(declarations.Unobservable) > 0 {
		applyUnobservable(data, declarations.Unobservable)
	}
	if role != "" && len(actions) > 0 {
		data.Roles = mergeRoleSchemas(actions)
	}
	data.DeclaredTypes = collectDeclaredTypes(actions)
	data.ShapeFunctions = collectShapeFunctions(actions)
	return data, nil
}

func collectDeclaredTypes(actions []*action) []DeclaredType {
	byName := make(map[string]string)
	for _, action := range actions {
		for _, role := range action.Roles {
			for _, field := range role.Fields {
				if field.Shape != nil {
					for _, definition := range field.Shape.Definitions {
						byName[definition.Name] = definition.Definition
					}
				}
			}
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	types := make([]DeclaredType, 0, len(names))
	for _, name := range names {
		types = append(types, DeclaredType{Name: name, Definition: byName[name]})
	}
	return types
}

func applyTypedFields(data *TemplateData, declarations []TypedField) {
	for _, declaration := range declarations {
		for _, action := range data.Actions {
			for roleIndex := range action.Roles {
				role := &action.Roles[roleIndex]
				if !role.matchesSource(declaration.Field.RoleName()) {
					continue
				}
				for fieldIndex := range role.Fields {
					field := &role.Fields[fieldIndex]
					if field.JSONName != declaration.Field.FieldName() {
						continue
					}
					shape := toDeclaredShape(declaration.Type, action.Role+role.Name+field.GoName, true)
					applyDeclaredShape(field, shape)
				}
			}
		}
	}
}

func applyUnobservable(data *TemplateData, declarations []UnobservableField) {
	for _, declaration := range declarations {
		for _, action := range data.Actions {
			markUnobservableField(action.Roles, declaration.Field.RoleName(), declaration.Field.FieldName())
		}
	}
}

func markUnobservableField(roles []roleSchema, roleName, fieldName string) {
	for roleIndex := range roles {
		if !roles[roleIndex].matchesSource(roleName) {
			continue
		}
		for fieldIndex := range roles[roleIndex].Fields {
			if roles[roleIndex].Fields[fieldIndex].JSONName == fieldName {
				roles[roleIndex].Fields[fieldIndex].Unobservable = true
			}
		}
	}
}

func applyProviders(data *TemplateData, providers []Provider) {
	for _, provider := range providers {
		provider.RoleName = provider.Field.RoleName()
		provider.FieldName = provider.Field.FieldName()
		provider.GoName = goName(provider.FieldName)
		provider.ProviderGoName = goName(provider.ProviderID)
		if provider.ProviderConfig == nil {
			provider.ProviderConfig = map[string]any{}
		}
		if provider.FieldConfig == nil {
			provider.FieldConfig = map[string]any{}
		}
		if provider.Metadata {
			provider.RoleName = "SgMetadata"
		}
		provider.ConvertType = "map[string]" + provider.ValueKind
		if provider.Type.Kind != "" {
			provider.ConvertType = provider.Type.GoType()
			provider.TypeLiteral = typeLiteral(provider.Type)
			if needsDeclaredShape(provider.Type) {
				provider.ConvertType = toDeclaredShape(provider.Type, provider.RoleName+provider.GoName, true).RawType
			}
		}
		data.Providers = append(data.Providers, provider)
		for _, action := range data.Actions {
			if provider.Metadata {
				continue
			}
			applyProviderType(action, provider)
		}
	}
	sort.Slice(data.Providers, func(i, j int) bool {
		return data.Providers[i].Field.String() < data.Providers[j].Field.String()
	})
	data.ProviderGroups = groupProviders(data.Providers)
	for i := range data.Providers {
		for _, group := range data.ProviderGroups {
			if data.Providers[i].ProviderID == group.ID {
				data.Providers[i].ProviderField = group.GoField
			}
		}
	}
	data.SourceImports = sourceImports(data.ProviderGroups)
}

func applyProviderType(action *action, provider Provider) {
	for roleIndex := range action.Roles {
		role := &action.Roles[roleIndex]
		if !role.matchesSource(provider.Field.RoleName()) {
			continue
		}
		for fieldIndex := range role.Fields {
			field := &role.Fields[fieldIndex]
			if field.JSONName != provider.FieldName {
				continue
			}
			field.GoType = provider.ConvertType
			if needsDeclaredShape(provider.Type) {
				applyDeclaredShape(field, toDeclaredShape(provider.Type, provider.RoleName+provider.GoName, true))
			} else {
				field.ComparisonType = field.GoType
			}
			field.Observed = true
		}
	}
}

func typeLiteral(spec Type) string {
	parts := []string{"Kind: provider.Kind" + goName(string(spec.Kind))}
	if spec.Nullable {
		parts = append(parts, "Nullable: true")
	}
	if spec.Values != nil {
		parts = append(parts, "Values: &"+typeLiteral(*spec.Values))
	}
	if len(spec.Fields) != 0 {
		keys := make([]string, 0, len(spec.Fields))
		for key := range spec.Fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fields := make([]string, 0, len(keys))
		for _, key := range keys {
			fields = append(fields, strconv.Quote(key)+": "+strings.TrimPrefix(typeLiteral(spec.Fields[key]), "provider.Type"))
		}
		parts = append(parts, "Fields: "+compositeLiteral("map[string]provider.Type", fields))
	}
	return compositeLiteral("provider.Type", parts)
}

func groupProviders(providers []Provider) []ProviderGroup {
	byID := make(map[string]*ProviderGroup)
	for _, provider := range providers {
		group := byID[provider.ProviderID]
		if group == nil {
			group = &ProviderGroup{
				Seed:        slices.Contains(provider.Capabilities, "seed"),
				GoField:     "provider" + provider.ProviderGoName,
				ID:          provider.ProviderID,
				GoName:      provider.ProviderGoName,
				SourceAlias: provider.SourceAlias,
				Config:      provider.ProviderConfig,
				Interfaces:  providerInterfaces(provider.Capabilities),
			}
			byID[provider.ProviderID] = group
		}
		group.Fields = append(group.Fields, provider)
	}
	groups := make([]ProviderGroup, 0, len(byID))
	for _, group := range byID {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].ID < groups[j].ID
	})
	return groups
}

func providerInterfaces(capabilities []string) []string {
	contracts := []struct {
		capability  string
		goInterface string
	}{
		{capability: "seed", goInterface: "Seeder"},
		{capability: "observe", goInterface: "Observer"},
		{capability: "observe_result", goInterface: "ResultObserver"},
	}
	interfaces := []string{"Lifecycle"}
	for _, contract := range contracts {
		if slices.Contains(capabilities, contract.capability) {
			interfaces = append(interfaces, contract.goInterface)
		}
	}
	return interfaces
}

func sourceImports(groups []ProviderGroup) []SourceImport {
	byPath := make(map[string]SourceImport)
	for _, group := range groups {
		field := group.Fields[0]
		byPath[field.SourceImportPath] = SourceImport{Alias: field.SourceAlias, Path: field.SourceImportPath}
	}
	imports := make([]SourceImport, 0, len(byPath))
	for _, source := range byPath {
		imports = append(imports, source)
	}
	sort.Slice(imports, func(i, j int) bool {
		return imports[i].Path < imports[j].Path
	})
	return imports
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
		roles[index] = roleSchema{Name: role.Name, SourceName: role.SourceName, Params: slices.Clone(role.Params), Fields: slices.Clone(role.Fields)}
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
			state.SgMetadata = output.SgMetadata
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
		schemas = append(schemas, roleSchema{Name: name, SourceName: role.Name, Params: params, Fields: fields})
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

func applyDeclaredShape(field *field, shape *declaredShape) {
	field.Shape = shape
	field.GoType = shape.RawType
	field.ComparisonType = shape.RawType
}

func collectShapeFunctions(actions []*action) []string {
	byName := make(map[string]*declaredShape)
	for _, action := range actions {
		for _, role := range action.Roles {
			for _, field := range role.Fields {
				if field.Shape != nil {
					byName[field.Shape.ObservedConvert] = field.Shape
				}
			}
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	var functions []string
	for _, name := range names {
		functions = append(functions, byName[name].Functions...)
	}
	return functions
}

func needsDeclaredShape(spec Type) bool {
	return spec.Kind == KindList || spec.Kind == KindMessage || (spec.Kind == KindMap && spec.Values != nil && (spec.Values.Kind == KindMessage || spec.Values.Kind == KindMap || spec.Values.Kind == KindList || spec.Values.Nullable || spec.Values.SkipComparison || spec.SkipComparison))
}
