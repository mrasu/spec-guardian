package renderer

import (
	"fmt"
	"sort"
	"strings"
)

// Kind identifies a supported declared value shape.
type Kind string

const (
	KindMap     Kind = "map"
	KindList    Kind = "list"
	KindMessage Kind = "message"
	KindString  Kind = "string"
	KindInt64   Kind = "int64"
	KindBool    Kind = "bool"
	KindDouble  Kind = "double"
	KindBytes   Kind = "bytes"
)

// Type describes the declared shape of a field during generation.
type Type struct {
	Kind           Kind
	Nullable       bool
	SkipComparison bool
	IgnoreOrder    bool
	Values         *Type
	Fields         map[string]Type
}

// NewType constructs a validated type shape supported by generation.
func NewType(kind Kind, nullable bool, values *Type, fields map[string]Type) (*Type, error) {
	spec := Type{Kind: kind, Nullable: nullable, Values: values, Fields: fields}
	if err := spec.validate(); err != nil {
		return nil, err
	}
	return &spec, nil
}

func (s *Type) validate() error {
	if s.IgnoreOrder && s.Kind != KindList {
		return fmt.Errorf("compare_order requires list type")
	}
	switch s.Kind {
	case KindString, KindInt64, KindBool, KindDouble, KindBytes:
		if s.Values != nil || len(s.Fields) != 0 {
			return fmt.Errorf("%s type cannot have values or fields", s.Kind)
		}
	case KindList:
		if s.Values == nil || len(s.Fields) != 0 {
			return fmt.Errorf("list type requires values and no fields")
		}
		return s.Values.validate()
	case KindMap:
		if s.Values == nil || len(s.Fields) != 0 {
			return fmt.Errorf("map type requires values and no fields")
		}
		if err := s.Values.validate(); err != nil {
			return err
		}
	case KindMessage:
		if s.Values != nil || len(s.Fields) == 0 {
			return fmt.Errorf("message type requires fields and no values")
		}
		for name, field := range s.Fields {
			if name == "" {
				return fmt.Errorf("message field name is empty")
			}
			if err := field.validate(); err != nil {
				return fmt.Errorf("message field %s: %w", name, err)
			}
		}
	default:
		return fmt.Errorf("unsupported kind %q", s.Kind)
	}
	return nil
}

// GoType returns the Go representation of a declared field shape.
func (s Type) GoType() string {
	var result string
	switch s.Kind {
	case KindString:
		result = "string"
	case KindInt64:
		result = "int64"
	case KindBool:
		result = "bool"
	case KindDouble:
		result = "float64"
	case KindBytes:
		result = "[]byte"
	case KindList:
		result = "[]" + s.Values.GoType()
	case KindMap:
		result = "map[string]" + s.Values.GoType()
	case KindMessage:
		names := make([]string, 0, len(s.Fields))
		for name := range s.Fields {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		used := make(map[string]struct{}, len(names))
		for _, name := range names {
			parts = append(parts, fmt.Sprintf("%s %s `json:%q`", uniqueGoName(name, used), s.Fields[name].GoType(), name))
		}
		result = "struct { " + strings.Join(parts, "; ") + " }"
	}
	if s.Nullable {
		return "*" + result
	}
	return result
}
