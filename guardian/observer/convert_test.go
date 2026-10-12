package observer

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

type testRecord struct {
	Name string  `json:"name"`
	Note *string `json:"note"`
}

func TestConvert_MapOfMessages(t *testing.T) {
	note := "memo"
	tests := []struct {
		name  string
		input any
		want  map[string]testRecord
		err   string
	}{
		{name: "nullable field", input: map[string]any{"one": map[string]any{"name": "order", "note": nil}}, want: map[string]testRecord{"one": {Name: "order"}}},
		{name: "present nullable field", input: map[string]any{"one": map[string]any{"name": "order", "note": note}}, want: map[string]testRecord{"one": {Name: "order", Note: &note}}},
		{name: "empty map", input: map[string]any{}, want: map[string]testRecord{}},
		{name: "missing field", input: map[string]any{"one": map[string]any{"name": "order"}}, err: "unset"},
		{name: "extra field", input: map[string]any{"one": map[string]any{"name": "order", "note": nil, "extra": "x"}}, err: "invalid keys: extra"},
		{name: "wrong type", input: map[string]any{"one": map[string]any{"name": 1, "note": nil}}, err: "expected type 'string'"},
		{name: "null map", input: map[string]any(nil), err: "null map is not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[map[string]testRecord](tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_NullableMap(t *testing.T) {
	empty := map[string]string{}
	present := map[string]string{"one": "value"}
	tests := []struct {
		name  string
		input any
		want  *map[string]string
		err   string
	}{
		{"nil value", nil, nil, ""}, {"typed nil map", map[string]string(nil), nil, ""},
		{"nil pointer", (*map[string]string)(nil), nil, ""},
		{"pointer to nil map", new(map[string]string(nil)), nil, "null map is not allowed"},
		{"empty", empty, &empty, ""}, {"pointer to empty", &empty, &empty, ""},
		{"present", present, &present, ""}, {"pointer to present", &present, &present, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[*map[string]string](tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_NullableMessage(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  *testRecord
		err   string
	}{
		{name: "null"},
		{name: "present", input: map[string]any{"name": "order", "note": nil}, want: &testRecord{Name: "order"}},
		{name: "missing field", input: map[string]any{"name": "order"}, err: "unset"},
		{name: "extra field", input: map[string]any{"name": "order", "note": nil, "extra": true}, err: "invalid keys: extra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[*testRecord](tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_MessageWithNullableMessageField(t *testing.T) {
	type snapshot struct {
		SequenceNo int64 `json:"sequence_no"`
	}
	type metadata struct {
		CurrentSnapshot *snapshot `json:"current_snapshot"`
	}
	tests := []struct {
		name  string
		input any
		want  []metadata
		err   string
	}{
		{
			name:  "null and present",
			input: []map[string]any{{"current_snapshot": nil}, {"current_snapshot": map[string]any{"sequence_no": int64(1)}}},
			want:  []metadata{{}, {CurrentSnapshot: &snapshot{SequenceNo: 1}}},
		},
		{name: "missing nullable field", input: []map[string]any{{}}, err: "unset"},
		{name: "missing nested field", input: []map[string]any{{"current_snapshot": map[string]any{}}}, err: "unset"},
		{name: "extra nested field", input: []map[string]any{{"current_snapshot": map[string]any{"sequence_no": int64(1), "extra": true}}}, err: "invalid keys: extra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[[]metadata](tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_NullablePrimitive(t *testing.T) {
	got, err := Convert[*int](nil)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestConvert_NonNullableStringRejectsNull(t *testing.T) {
	got, err := Convert[string](nil)
	require.ErrorContains(t, err, "null is not allowed")
	require.Empty(t, got)
}

func TestConvert_MapTargetRequiresStringKey(t *testing.T) {
	wrongKeys, err := Convert[map[int]string](map[string]string{"one": "value"})
	require.ErrorContains(t, err, "expected type 'int'")
	require.Nil(t, wrongKeys)
}

func TestConvert_MessageRejectsExtraField(t *testing.T) {
	got, err := Convert[struct {
		Name string `json:"name"`
	}](map[string]any{"name": "order", "note": nil})
	require.ErrorContains(t, err, "invalid keys: note")
	require.Equal(t, struct {
		Name string `json:"name"`
	}{}, got)
}

func TestConvert_MessageTargetRejectsUnexportedField(t *testing.T) {
	unexportedField, err := Convert[struct {
		name string
		Note *string `json:"note"`
	}](map[string]any{"name": "order", "note": nil})
	require.ErrorContains(t, err, "invalid keys: name")
	require.Equal(t, struct {
		name string
		Note *string `json:"note"`
	}{}, unexportedField)
}

func TestConvert_NullableInt64MessageField(t *testing.T) {
	type record struct {
		OrderID *int64 `json:"order_id"`
	}
	id := int64(42)
	tests := []struct {
		name  string
		value any
		want  map[string]record
		err   string
	}{
		{name: "integer", value: map[string]any{"one": map[string]any{"order_id": id}}, want: map[string]record{"one": {OrderID: &id}}},
		{name: "null", value: map[string]any{"one": map[string]any{"order_id": nil}}, want: map[string]record{"one": {}}},
		{name: "float is rejected", value: map[string]any{"one": map[string]any{"order_id": float64(42)}}, err: "want int64"},
		{name: "missing field", value: map[string]any{"one": map[string]any{}}, err: "unset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[map[string]record](tt.value)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_ListOfMessagesWithNestedRows(t *testing.T) {
	type dataFile struct {
		FilePath string     `json:"file_path"`
		Rows     [][]string `json:"rows"`
	}
	tests := []struct {
		name  string
		input any
		want  []dataFile
		err   string
	}{
		{name: "files", input: []map[string]any{{"file_path": "first", "rows": [][]string{{"a", "b"}, {"c"}}}}, want: []dataFile{{FilePath: "first", Rows: [][]string{{"a", "b"}, {"c"}}}}},
		{name: "empty list", input: []map[string]any{}, want: []dataFile{}},
		{name: "missing field", input: []map[string]any{{"file_path": "first"}}, err: "unset"},
		{name: "extra field", input: []map[string]any{{"file_path": "first", "rows": [][]string{}, "extra": true}}, err: "invalid keys: extra"},
		{name: "null row", input: []map[string]any{{"file_path": "first", "rows": [][]string{nil}}}, err: "list index 0"},
		{name: "null list", input: []map[string]any(nil), err: "null is not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[[]dataFile](tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_BasicTypesAreStrict(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		convert func(any) (any, error)
		want    any
		err     string
	}{
		{"bool", true, func(v any) (any, error) { return Convert[bool](v) }, true, ""},
		{"numeric bool", int64(1), func(v any) (any, error) { return Convert[bool](v) }, false, "want bool"},
		{"double", float64(1.5), func(v any) (any, error) { return Convert[float64](v) }, float64(1.5), ""},
		{"integer double", int64(1), func(v any) (any, error) { return Convert[float64](v) }, float64(0), "want float64"},
		{"float32 double", float32(1), func(v any) (any, error) { return Convert[float64](v) }, float64(0), "want float64"},
		{"nan", math.NaN(), func(v any) (any, error) { return Convert[float64](v) }, float64(0), "finite"},
		{"infinity", math.Inf(1), func(v any) (any, error) { return Convert[float64](v) }, float64(0), "finite"},
		{"bytes", []byte{0, 255}, func(v any) (any, error) { return Convert[[]byte](v) }, []byte{0, 255}, ""},
		{"empty bytes", []byte{}, func(v any) (any, error) { return Convert[[]byte](v) }, []byte{}, ""},
		{"string bytes", "AP8=", func(v any) (any, error) { return Convert[[]byte](v) }, []byte(nil), "want []byte"},
		{"integer list bytes", []int{0, 255}, func(v any) (any, error) { return Convert[[]byte](v) }, []byte(nil), "want []byte"},
		{"nil bytes", []byte(nil), func(v any) (any, error) { return Convert[[]byte](v) }, []byte(nil), "null is not allowed"},
		{"nullable bool", nil, func(v any) (any, error) { return Convert[*bool](v) }, (*bool)(nil), ""},
		{"nullable double", nil, func(v any) (any, error) { return Convert[*float64](v) }, (*float64)(nil), ""},
		{"nullable bytes", nil, func(v any) (any, error) { return Convert[*[]byte](v) }, (*[]byte)(nil), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.convert(tt.value)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_NestedNullableCollections(t *testing.T) {
	type record struct {
		Rows    *map[string]*[]float64 `json:"rows"`
		Payload *[]byte                `json:"payload"`
	}
	payload := []byte{0, 255}
	tests := []struct {
		name  string
		value any
		want  record
		err   string
	}{
		{"null", map[string]any{"rows": nil, "payload": nil}, record{}, ""},
		{"empty", map[string]any{"rows": map[string]any{}, "payload": []byte{}}, record{Rows: new(map[string]*[]float64{}), Payload: new([]byte{})}, ""},
		{"nested", map[string]any{"rows": map[string]any{"null": nil, "empty": []float64{}, "values": []float64{1.5}}, "payload": payload}, record{Rows: new(map[string]*[]float64{"null": nil, "empty": new([]float64{}), "values": new([]float64{1.5})}), Payload: &payload}, ""},
		{"missing nullable field", map[string]any{"payload": nil}, record{}, "unset"},
		{"extra field", map[string]any{"rows": nil, "payload": nil, "extra": true}, record{}, "invalid keys"},
		{"wrong nested number", map[string]any{"rows": map[string]any{"bad": []any{int64(1)}}, "payload": nil}, record{}, "map key \"bad\": list index 0"},
		{"wrong bytes", map[string]any{"rows": nil, "payload": "AA=="}, record{}, "want []byte"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[record](tt.value)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_NullableList(t *testing.T) {
	empty, present := []bool{}, []bool{false, true}
	tests := []struct {
		name  string
		value any
		want  *[]bool
		err   string
	}{
		{"null", nil, nil, ""}, {"typed nil list", []bool(nil), nil, ""},
		{"nil pointer", (*[]bool)(nil), nil, ""},
		{"pointer to nil list", new([]bool(nil)), nil, "null is not allowed"},
		{"empty", empty, &empty, ""}, {"pointer to empty", &empty, &empty, ""},
		{"values", present, &present, ""}, {"pointer to values", &present, &present, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[*[]bool](tt.value)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_TypedMessageRejectsImplicitNumericConversion(t *testing.T) {
	type source struct {
		Count int64 `json:"amount"`
	}
	type target struct {
		Amount float64 `json:"amount"`
	}
	got, err := Convert[target](source{Count: 1})
	require.ErrorContains(t, err, "message field \"amount\": got int64, want float64")
	require.Equal(t, target{}, got)
}

func TestConvert_TypedMessageRejectsNullCollection(t *testing.T) {
	type record struct {
		Rows []bool `json:"rows"`
	}
	got, err := Convert[record](record{})
	require.ErrorContains(t, err, "message field \"rows\": null is not allowed")
	require.Equal(t, record{}, got)
}

func TestConvert_NullableBytesPreservesEmptyAndNull(t *testing.T) {
	payload := []byte{}
	tests := []struct {
		name  string
		value any
		want  *[]byte
	}{
		{"null", nil, nil}, {"typed nil", []byte(nil), nil},
		{"empty", payload, &payload}, {"pointer to empty", &payload, &payload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[*[]byte](tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConvert_NullableBytesRejectsPointerToNilSlice(t *testing.T) {
	got, err := Convert[*[]byte](new([]byte(nil)))
	require.ErrorContains(t, err, "null is not allowed")
	require.Nil(t, got)
}

func TestConvert_NestedNullableCollectionRejectsPointerToNil(t *testing.T) {
	type record struct {
		Rows *map[string]*[]bool `json:"rows"`
	}
	tests := []struct {
		name  string
		value any
		err   string
	}{
		{"map", map[string]any{"rows": new(map[string]*[]bool(nil))}, "null map is not allowed"},
		{"list", map[string]any{"rows": map[string]any{"one": new([]bool(nil))}}, "map key \"one\": null is not allowed"},
		{"typed message", record{Rows: new(map[string]*[]bool(nil))}, "null map is not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert[record](tt.value)
			require.ErrorContains(t, err, tt.err)
			require.Equal(t, record{}, got)
		})
	}
}
