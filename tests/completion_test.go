package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestCompletion(t *testing.T) {
	t.Parallel()

	t.Run("prints zsh completion", func(t *testing.T) {
		t.Parallel()

		// given: zsh is a supported completion shell

		// when: requesting completion for zsh
		result := binary.Run(t, "completion", "zsh")

		// then: the command succeeds with the zsh script on stdout and no stderr
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/completion/zsh/stdout.expected.txt", result.Stdout)
	})

	t.Run("shows help for an unknown shell", func(t *testing.T) {
		t.Parallel()

		// given: the requested completion shell is unsupported

		// when: requesting completion for that shell
		result := binary.Run(t, "completion", "invalid-shell")

		// then: the command succeeds with help on stdout and no stderr
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/completion/unknown_shell/stdout.expected.txt", result.Stdout)
	})
}
