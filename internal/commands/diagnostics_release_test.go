package commands //nolint:testpackage // exercises internal diagnostic rendering

import (
	"fmt"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/versionfile"
)

func TestVersionFileDiagnosticsDescribeEverySentinel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cause   error
		message string
		hint    string
	}{
		{
			name:    "invalid next version",
			cause:   versionfile.ErrInvalidNextVersion,
			message: "could not update version file: next version is invalid for the configured versioning scheme",
			hint:    "check target versioning and the version_files configuration",
		},
		{
			name:    "invalid versioning scheme",
			cause:   versionfile.ErrInvalidScheme,
			message: "could not update version file: configured versioning scheme is invalid",
			hint:    "check the target versioning configuration",
		},
		{
			name:    "invalid JSON pointer",
			cause:   versionfile.ErrInvalidJSONPointer,
			message: "could not update version file: json pointer is invalid",
			hint:    "use a valid JSON pointer in the version_files configuration",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a version file failure carrying one sentinel
			diagnostic := diagnostic{message: "could not update version file"}

			// when: describing the problem
			describeVersionFileProblem(&diagnostic, fmt.Errorf("update version file: %w", test.cause))

			// then: the sentinel gets its own explanation and remedy
			testastic.Equal(t, test.message, diagnostic.message)
			testastic.Equal(t, test.hint, diagnostic.hint)
		})
	}
}
