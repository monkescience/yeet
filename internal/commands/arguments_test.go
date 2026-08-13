package commands

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestCommandsRejectPositionalArguments(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "init"},
		{name: "release"},
		{name: "version"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command, _, err := NewRoot().Find([]string{test.name})
			testastic.NoError(t, err)

			err = command.ValidateArgs([]string{"unexpected"})
			if err == nil {
				t.Fatal("expected positional argument to be rejected")
			}

			testastic.Equal(
				t,
				"unknown command \"unexpected\" for \"yeet "+test.name+"\"",
				err.Error(),
			)
		})
	}
}
