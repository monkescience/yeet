package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	t.Run("prints version banner", func(t *testing.T) {
		t.Parallel()

		// given: a freshly built yeet binary

		// when: invoking `yeet version`
		result := binary.Run(t, "version")

		// then: the binary exits 0 and stdout matches the golden banner
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stderr)
		testastic.AssertFile(t, "testdata/version/success/stdout.expected.txt", result.Stdout)
	})
}
