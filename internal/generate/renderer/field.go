package renderer

import (
	"fmt"
	"strings"
)

// Field is a validated <Role>.<field> reference.
type Field struct {
	roleName  string
	fieldName string
}

// NewField parses a role field reference.
func NewField(value string) (*Field, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid field path %q", value)
	}
	return &Field{roleName: parts[0], fieldName: parts[1]}, nil
}

// NewRoleField constructs a role field reference.
func NewRoleField(role, name string) (*Field, error) {
	if strings.TrimSpace(role) == "" || strings.Contains(role, ".") || strings.TrimSpace(name) == "" || strings.Contains(name, ".") {
		return nil, fmt.Errorf("invalid field path %q", role+"."+name)
	}
	return &Field{roleName: role, fieldName: name}, nil
}

// RoleName returns the FizzBee role name.
func (field *Field) RoleName() string {
	return field.roleName
}

// FieldName returns the FizzBee field name.
func (field *Field) FieldName() string {
	return field.fieldName
}

// String returns the reference as <Role>.<field>.
func (field *Field) String() string {
	return field.roleName + "." + field.fieldName
}
