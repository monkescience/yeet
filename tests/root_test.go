package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestRoot(t *testing.T) {
	t.Parallel()

	t.Run("rejects --verbose with --quiet", func(t *testing.T) {
		t.Parallel()

		// given: both --verbose and --quiet on the same invocation

		// when: running `yeet --verbose --quiet version`
		result := binary.Run(t, "--verbose", "--quiet", "version")

		// then: the binary exits 1 with a flag conflict on stderr
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
		testastic.AssertFile(t, "testdata/root/verbose_quiet_conflict/stderr.expected.txt", result.Stderr)
	})

	t.Run("rejects unknown subcommand", func(t *testing.T) {
		t.Parallel()

		// given: a name that does not match any registered subcommand

		// when: running `yeet nope-not-a-command`
		result := binary.Run(t, "nope-not-a-command")

		// then: the binary exits 1 and stderr names the unknown command
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
		testastic.AssertFile(t, "testdata/root/unknown_subcommand/stderr.expected.txt", result.Stderr)
	})

	t.Run("--no-color produces uncolored output", func(t *testing.T) {
		t.Parallel()

		// given: --no-color forwarded to a subcommand that prints to stdout

		// when: running `yeet --no-color version`
		result := binary.Run(t, "--no-color", "version")

		// then: the version banner prints cleanly without ANSI codes
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/root/no_color_version/stdout.expected.txt", result.Stdout)
	})
}
