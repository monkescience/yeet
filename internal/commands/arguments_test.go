package commands_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/build"
	"github.com/monkescience/yeet/internal/commands"
	"github.com/monkescience/yeet/internal/telemetry"
)

func TestCommandsRejectPositionalArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "init"},
		{name: "release"},
		{name: "version"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: the named command from a fresh root command
			command, _, err := commands.NewRoot(telemetry.New(build.Version())).Find([]string{test.name})
			testastic.NoError(t, err)

			// when: an unexpected positional argument is validated
			err = command.ValidateArgs([]string{"unexpected"})

			// then: the command rejects it with the Cobra argument error
			testastic.Error(t, err)

			if err != nil {
				testastic.Equal(
					t,
					"unknown command \"unexpected\" for \"yeet "+test.name+"\"",
					err.Error(),
				)
			}
		})
	}
}
