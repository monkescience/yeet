package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		result := binary.Run(t, "version")

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/version/success/stdout.expected.txt", result.Stdout)
	})
}
