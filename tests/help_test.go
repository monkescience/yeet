package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestHelp(t *testing.T) {
	t.Parallel()

	t.Run("root --help shows the top-level usage", func(t *testing.T) {
		t.Parallel()

		// given: the yeet binary with no subcommand context

		// when: running `yeet --help`
		result := binary.Run(t, "--help")

		// then: the root usage matches the golden file
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/help/root/stdout.expected.txt", result.Stdout)
	})

	t.Run("release --help shows the release usage", func(t *testing.T) {
		t.Parallel()

		// given: the yeet binary

		// when: running `yeet release --help`
		result := binary.Run(t, "release", "--help")

		// then: the release usage matches the golden file
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/help/release/stdout.expected.txt", result.Stdout)
	})

	t.Run("init --help shows the init usage", func(t *testing.T) {
		t.Parallel()

		// given: the yeet binary

		// when: running `yeet init --help`
		result := binary.Run(t, "init", "--help")

		// then: the init usage matches the golden file
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/help/init/stdout.expected.txt", result.Stdout)
	})

	t.Run("version --help shows the version usage", func(t *testing.T) {
		t.Parallel()

		// given: the yeet binary

		// when: running `yeet version --help`
		result := binary.Run(t, "version", "--help")

		// then: the version usage matches the golden file
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/help/version/stdout.expected.txt", result.Stdout)
	})
}
