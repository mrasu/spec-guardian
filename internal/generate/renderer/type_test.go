package renderer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewType_ValidShape(t *testing.T) {
	tests := []struct {
		name string
		spec Type
	}{
		{name: "int64 values", spec: Type{Kind: KindMap, Values: &Type{Kind: KindInt64}}},
		{name: "nullable int64 values", spec: Type{Kind: KindMap, Values: &Type{Kind: KindInt64, Nullable: true}}},
		{name: "map values", spec: Type{Kind: KindMap, Values: &Type{Kind: KindMap, Values: &Type{Kind: KindInt64}}}},
		{name: "list values", spec: Type{Kind: KindMap, Values: &Type{Kind: KindList, Values: &Type{Kind: KindInt64}}}},
		{
			name: "string",
			spec: Type{Kind: KindString},
		},
		{
			name: "nullable string",
			spec: Type{Kind: KindString, Nullable: true},
		},
		{
			name: "nullable int64 message field",
			spec: Type{Kind: KindMap, Values: &Type{Kind: KindMessage, Fields: map[string]Type{"order_id": {Kind: KindInt64, Nullable: true}}}},
		},
		{
			name: "string values",
			spec: Type{Kind: KindMap, Values: &Type{Kind: KindString}},
		},
		{
			name: "message values with nullable string field",
			spec: Type{
				Kind: KindMap,
				Values: &Type{
					Kind:   KindMessage,
					Fields: map[string]Type{"name": {Kind: KindString, Nullable: true}},
				},
			},
		},
		{
			name: "nullable message value",
			spec: Type{Kind: KindList, Values: &Type{
				Kind: KindMessage,
				Fields: map[string]Type{"current_snapshot": {
					Kind: KindMessage, Nullable: true,
					Fields: map[string]Type{"sequence_no": {Kind: KindInt64}},
				}},
			}},
		},
		{
			name: "nested list in message",
			spec: Type{Kind: KindList, Values: &Type{Kind: KindMessage, Fields: map[string]Type{
				"file_path": {Kind: KindString},
				"rows":      {Kind: KindList, Values: &Type{Kind: KindList, Values: &Type{Kind: KindString}}},
			}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := NewType(tt.spec.Kind, tt.spec.Nullable, tt.spec.Values, tt.spec.Fields)
			require.NoError(t, err)
			require.Equal(t, &tt.spec, spec)
		})
	}
}

func TestNewType_InvalidShape(t *testing.T) {
	tests := []struct {
		name string
		spec Type
		want string
	}{
		{
			name: "string has fields",
			spec: Type{Kind: KindString, Fields: map[string]Type{"name": {Kind: KindString}}},
			want: "string type cannot have values or fields",
		},
		{
			name: "map has no values",
			spec: Type{Kind: KindMap},
			want: "map type requires values",
		},
		{
			name: "unsupported value type",
			spec: Type{Kind: KindMap, Values: &Type{Kind: "unknown"}},
			want: "unsupported kind",
		},
		{name: "list has no values", spec: Type{Kind: KindList}, want: "list type requires values"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := NewType(tt.spec.Kind, tt.spec.Nullable, tt.spec.Values, tt.spec.Fields)
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, spec)
		})
	}
}

func TestTypeGoTypeNestedList(t *testing.T) {
	spec := Type{Kind: KindList, Values: &Type{Kind: KindMessage, Fields: map[string]Type{
		"file_path": {Kind: KindString},
		"rows":      {Kind: KindList, Values: &Type{Kind: KindList, Values: &Type{Kind: KindString}}},
	}}}
	require.Equal(t, "[]struct { FilePath string `json:\"file_path\"`; Rows [][]string `json:\"rows\"` }", spec.GoType())
}

func TestTypeGoTypeNullableMessage(t *testing.T) {
	spec := Type{Kind: KindMessage, Nullable: true, Fields: map[string]Type{
		"sequence_no": {Kind: KindInt64},
	}}
	require.Equal(t, "*struct { SequenceNo int64 `json:\"sequence_no\"` }", spec.GoType())
}

func TestType_GoRepresentationOfBasicAndNullableTypes(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{KindBool, "bool"}, {KindDouble, "float64"}, {KindBytes, "[]byte"},
		{KindMap, "map[string]bool"}, {KindList, "[]bool"},
	}
	for _, tt := range tests {
		for _, nullable := range []bool{false, true} {
			t.Run(string(tt.kind)+fmt.Sprint(nullable), func(t *testing.T) {
				var values *Type
				if tt.kind == KindMap || tt.kind == KindList {
					values = &Type{Kind: KindBool}
				}
				spec, err := NewType(tt.kind, nullable, values, nil)
				require.NoError(t, err)
				want := tt.want
				if nullable {
					want = "*" + want
				}
				require.Equal(t, want, spec.GoType())
			})
		}
	}
}
