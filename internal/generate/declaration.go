package generate

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/mrasu/spec-guardian/internal/fizzbee"
	"github.com/mrasu/spec-guardian/internal/generate/renderer"
)

func resolveDeclarations(config *Config, cases []fizzbee.ActionCase) (renderer.Declarations, error) {
	declarations := config.declarations
	exclusions, err := resolveExcludedRoles(declarations.Unobservable, declarations.ExcludedRoles, cases)
	if err != nil {
		return renderer.Declarations{}, err
	}
	declarations.Unobservable = exclusions
	for _, field := range declarations.Unobservable {
		if err := validateFieldInCases(field.Field, cases); err != nil {
			return renderer.Declarations{}, err
		}
	}
	for _, field := range config.declarations.Providers {
		if err := validateFieldInCases(field.Field, cases); err != nil {
			return renderer.Declarations{}, err
		}
		if err := validateFizzBeeType(cases, field.Field.RoleName(), field.Field.FieldName(), field.Type); err != nil {
			return renderer.Declarations{}, err
		}
	}
	for _, field := range config.declarations.TypedFields {
		if err := validateFieldInCases(field.Field, cases); err != nil {
			return renderer.Declarations{}, err
		}
		if err := validateFizzBeeType(cases, field.Field.RoleName(), field.Field.FieldName(), field.Type); err != nil {
			return renderer.Declarations{}, err
		}
	}
	return declarations, nil
}

func resolveExcludedRoles(fields []renderer.UnobservableField, roles []renderer.ExcludedRole, cases []fizzbee.ActionCase) ([]renderer.UnobservableField, error) {
	result := append([]renderer.UnobservableField(nil), fields...)
	for _, excluded := range roles {
		found := false
		names := make(map[string]bool)
		for _, actionCase := range cases {
			for _, role := range actionCase.Input.Roles {
				if role.Name != excluded.Name {
					continue
				}
				found = true
				for name := range role.Fields {
					names[name] = true
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown excluded role %q", excluded.Name)
		}
		sorted := make([]string, 0, len(names))
		for name := range names {
			sorted = append(sorted, name)
		}
		sort.Strings(sorted)
		for _, name := range sorted {
			field, err := renderer.NewRoleField(excluded.Name, name)
			if err != nil {
				return nil, err
			}
			result = append(result, renderer.UnobservableField{Field: field, Reason: excluded.Reason})
		}
	}
	return result, nil
}

func validateFizzBeeType(cases []fizzbee.ActionCase, roleName, fieldName string, spec renderer.Type) error {
	field := roleName + "." + fieldName
	for _, actionCase := range cases {
		states := append([]fizzbee.RoleState{}, actionCase.Input.Roles...)
		for _, state := range actionCase.Outputs {
			states = append(states, state.Roles...)
		}
		for _, role := range states {
			if role.Name != roleName {
				continue
			}
			value, ok := role.Fields[fieldName]
			if !ok {
				return fmt.Errorf("field %s: missing FizzBee field", field)
			}
			if err := validateFizzBeeValue(value, &spec); err != nil {
				return fmt.Errorf("field %s: %w", field, err)
			}
		}
	}
	return nil
}

func validateFizzBeeValue(value any, spec *renderer.Type) error {
	if value == nil {
		if spec.Nullable {
			return nil
		}
		return fmt.Errorf("null is not allowed")
	}
	switch spec.Kind {
	case renderer.KindString:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("got %T, want string", value)
		}
	case renderer.KindInt64:
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("got %T, want int64", value)
		}
		if _, err := number.Int64(); err != nil {
			return fmt.Errorf("invalid int64 %q: %w", number, err)
		}
	case renderer.KindBool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("got %T, want bool", value)
		}
	case renderer.KindDouble:
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("got %T, want double", value)
		}
		floating, err := number.Float64()
		if err != nil || math.IsNaN(floating) || math.IsInf(floating, 0) {
			return fmt.Errorf("invalid double %q", number)
		}
	case renderer.KindBytes:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("got %T, want base64 string", value)
		}
		if _, err := base64.StdEncoding.Strict().DecodeString(text); err != nil {
			return fmt.Errorf("invalid base64 bytes: %w", err)
		}
	case renderer.KindMap:
		mapping, ok := value.(map[string]any)
		if !ok || spec.Values == nil {
			return fmt.Errorf("got %T, want map", value)
		}
		for key, item := range mapping {
			if err := validateFizzBeeValue(item, spec.Values); err != nil {
				return fmt.Errorf("map key %q: %w", key, err)
			}
		}
	case renderer.KindList:
		items, ok := value.([]any)
		if !ok || spec.Values == nil {
			return fmt.Errorf("got %T, want list", value)
		}
		for index, item := range items {
			if err := validateFizzBeeValue(item, spec.Values); err != nil {
				return fmt.Errorf("list index %d: %w", index, err)
			}
		}
	case renderer.KindMessage:
		mapping, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("got %T, want message", value)
		}
		for name, spec := range spec.Fields {
			item, ok := mapping[name]
			if !ok {
				return fmt.Errorf("missing message field %q", name)
			}
			if err := validateFizzBeeValue(item, &spec); err != nil {
				return fmt.Errorf("message field %q: %w", name, err)
			}
		}
		for name := range mapping {
			if _, known := spec.Fields[name]; !known {
				return fmt.Errorf("extra message field %q", name)
			}
		}
	default:
		return fmt.Errorf("unsupported kind %q", spec.Kind)
	}
	return nil
}

func validateFieldInCases(field *renderer.Field, cases []fizzbee.ActionCase) error {
	if !hasFizzBeeField(cases, field.RoleName(), field.FieldName()) {
		return fmt.Errorf("invalid field path %q", field.String())
	}
	return nil
}

func hasFizzBeeField(cases []fizzbee.ActionCase, roleName, fieldName string) bool {
	for _, actionCase := range cases {
		for _, role := range actionCase.Input.Roles {
			if role.Name == roleName {
				_, ok := role.Fields[fieldName]
				return ok
			}
		}
	}
	return false
}
