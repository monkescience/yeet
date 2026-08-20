package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestCompletion(t *testing.T) {
	t.Parallel()

	t.Run("prints zsh completion", func(t *testing.T) {
		t.Parallel()

		result := binary.Run(t, "completion", "zsh")

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/completion/zsh/stdout.expected.txt", result.Stdout)
	})

	t.Run("shows help for an unknown shell", func(t *testing.T) {
		t.Parallel()

		result := binary.Run(t, "completion", "invalid-shell")

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/completion/unknown_shell/stdout.expected.txt", result.Stdout)
	})
}
