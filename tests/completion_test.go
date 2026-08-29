package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestCompletion(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"bash", "fish", "powershell", "zsh"} {
		t.Run("prints "+shell+" completion", func(t *testing.T) {
			t.Parallel()

			// given: the requested shell is supported

			// when: requesting completion for that shell
			result := binary.Run(t, "completion", shell)

			// then: the command succeeds with the shell script on stdout and no stderr
			testastic.Equal(t, 0, result.ExitCode)
			testastic.Equal(t, "", result.Stderr)
			testastic.AssertFile(
				t,
				"testdata/completion/"+shell+"/stdout.expected.txt",
				result.Stdout,
			)
		})
	}

	t.Run("shows help when no shell is selected", func(t *testing.T) {
		t.Parallel()

		// given: no completion shell argument

		// when: invoking the completion command
		result := binary.Run(t, "completion")

		// then: the command succeeds with completion help on stdout and no stderr
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/completion/no_shell/stdout.expected.txt", result.Stdout)
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
