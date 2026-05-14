package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestRoot(t *testing.T) {
	t.Run("verbose and quiet conflict", func(t *testing.T) {
		result := binary.Run(t, "--verbose", "--quiet", "version")

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
		testastic.Contains(t, result.Stderr, "--verbose and --quiet cannot be used together")
	})

	t.Run("unknown subcommand", func(t *testing.T) {
		result := binary.Run(t, "nope-not-a-command")

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
		testastic.Contains(t, result.Stderr, `unknown command "nope-not-a-command"`)
	})

	t.Run("no color smoke", func(t *testing.T) {
		result := binary.Run(t, "--no-color", "version")

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.Contains(t, result.Stdout, "version:")
	})
}
