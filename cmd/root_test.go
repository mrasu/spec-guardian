package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecuteRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown command", args: []string{"unknown"}, want: "unknown command"},
		{name: "missing artifact directory", args: []string{"generate"}, want: "--fizz-output-dir is required"},
		{name: "positional argument", args: []string{"generate", "extra", "--fizz-output-dir", "artifacts"}, want: "unknown command"},
		{name: "unknown flag", args: []string{"generate", "--unknown"}, want: "unknown flag"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Execute(t.Context(), test.args)
			assert.ErrorContains(t, err, test.want)
		})
	}
}
