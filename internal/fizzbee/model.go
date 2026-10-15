package fizzbee

// Model contains model-wide options and extracted conformance cases.
type Model struct {
	Options ModelOptions
	Cases   []ActionCase
}

// ModelOptions contains effective options read from FizzBee artifacts.
type ModelOptions struct {
	CrashOnYield bool
}
