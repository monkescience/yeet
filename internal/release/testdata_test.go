//nolint:testpackage // This test helper supports tests of unexported release behavior.
package release

import (
	"os"
	"testing"

	"github.com/monkescience/testastic"
)

func readTestFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	testastic.NoError(t, err)

	return string(content)
}
