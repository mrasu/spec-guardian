package provider

import "reflect"

// CloneSeedRequest copies model values before handing them to a provider. It
// preserves Go types and null/empty distinctions while isolating maps, lists,
// messages, bytes, and nullable pointers from the original model input.
func CloneSeedRequest(request SeedRequest) SeedRequest {
	if request.Values == nil {
		return SeedRequest{}
	}
	values := make([]SeedValue, len(request.Values))
	for i, value := range request.Values {
		values[i] = SeedValue{Field: value.Field, Value: cloneSeedValue(reflect.ValueOf(value.Value)).Interface()}
	}
	return SeedRequest{Values: values}
}

func cloneSeedValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return reflect.Zero(reflect.TypeFor[any]())
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		if value.Kind() == reflect.Pointer {
			copy := reflect.New(value.Type().Elem())
			copy.Elem().Set(cloneSeedValue(value.Elem()))
			return copy.Convert(value.Type())
		}
		copy := reflect.New(value.Type()).Elem()
		copy.Set(cloneSeedValue(value.Elem()))
		return copy
	case reflect.Slice, reflect.Array:
		return cloneSeedList(value)
	case reflect.Map:
		return cloneSeedMap(value)
	case reflect.Struct:
		return cloneSeedMessage(value)
	default:
		return value
	}
}

func cloneSeedList(value reflect.Value) reflect.Value {
	if value.Kind() == reflect.Slice && value.IsNil() {
		return reflect.Zero(value.Type())
	}
	copy := reflect.New(value.Type()).Elem()
	if value.Kind() == reflect.Slice {
		copy = reflect.MakeSlice(value.Type(), value.Len(), value.Len())
	}
	for i := 0; i < value.Len(); i++ {
		copy.Index(i).Set(cloneSeedValue(value.Index(i)))
	}
	return copy
}

func cloneSeedMap(value reflect.Value) reflect.Value {
	if value.IsNil() {
		return reflect.Zero(value.Type())
	}
	copy := reflect.MakeMapWithSize(value.Type(), value.Len())
	for iter := value.MapRange(); iter.Next(); {
		copy.SetMapIndex(iter.Key(), cloneSeedValue(iter.Value()))
	}
	return copy
}

func cloneSeedMessage(value reflect.Value) reflect.Value {
	copy := reflect.New(value.Type()).Elem()
	copy.Set(value)
	for field := range value.Type().Fields() {
		if field.IsExported() {
			copy.FieldByIndex(field.Index).Set(cloneSeedValue(value.FieldByIndex(field.Index)))
		}
	}
	return copy
}
