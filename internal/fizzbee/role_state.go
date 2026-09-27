package fizzbee

import "fmt"

// RoleState contains the test-boundary state of one concrete FizzBee role.
//
// It is selected from node JSON such as:
//
//	{
//	  "name":"Request", "ref":2,
//	  "params":{"key":1}, "fields":{"status":"DONE"}
//	}
type RoleState struct {
	// Name is the source role name, retained in memory to identify the target role.
	// It is intentionally omitted from generated boundary JSON.
	Name string `json:"-"`
	// Params is the validated JSON object from the role's params field.
	Params JSONObject `json:"params"`
	// Fields is the validated JSON object from the role's fields field.
	Fields JSONObject `json:"fields"`
}

// NewRoleState returns a validated test-boundary role state.
func NewRoleState(name string, params, fields JSONObject) (RoleState, error) {
	if name == "" {
		return RoleState{}, fmt.Errorf("role state name is empty")
	}
	if params == nil {
		return RoleState{}, fmt.Errorf("role state %s params is nil", name)
	}
	if fields == nil {
		return RoleState{}, fmt.Errorf("role state %s fields is nil", name)
	}
	return RoleState{Name: name, Params: params, Fields: fields}, nil
}
