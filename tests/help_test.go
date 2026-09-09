package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		args           []string
		expectedStdout string
	}{
		{
			name:           "root --help shows the top-level usage",
			args:           []string{"--help"},
			expectedStdout: "testdata/help/root/stdout.expected.txt",
		},
		{
			name:           "release --help shows the release usage",
			args:           []string{"release", "--help"},
			expectedStdout: "testdata/help/release/stdout.expected.txt",
		},
		{
			name:           "init --help shows the init usage",
			args:           []string{"init", "--help"},
			expectedStdout: "testdata/help/init/stdout.expected.txt",
		},
		{
			name:           "version --help shows the version usage",
			args:           []string{"version", "--help"},
			expectedStdout: "testdata/help/version/stdout.expected.txt",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: the yeet binary and a help command scope

			// when: requesting help for that scope
			result := binary.Run(t, test.args...)

			// then: the command succeeds with the complete usage on stdout and no stderr
			testastic.Equal(t, 0, result.ExitCode)
			testastic.Equal(t, "", result.Stderr)
			testastic.AssertFile(t, test.expectedStdout, result.Stdout)
		})
	}
}
