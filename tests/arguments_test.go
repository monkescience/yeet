package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestCommandsRejectPositionalArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		args           []string
		expectedStderr string
	}{
		{
			name:           "init",
			args:           []string{"init", "unexpected"},
			expectedStderr: "testdata/init/unexpected_argument/stderr.expected.txt",
		},
		{
			name:           "release",
			args:           []string{"release", "unexpected"},
			expectedStderr: "testdata/release/unexpected_argument/stderr.expected.txt",
		},
		{
			name:           "version",
			args:           []string{"version", "unexpected"},
			expectedStderr: "testdata/version/unexpected_argument/stderr.expected.txt",
		},
		{
			name:           "completion shell",
			args:           []string{"completion", "zsh", "unexpected"},
			expectedStderr: "testdata/completion/unexpected_argument/stderr.expected.txt",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result := binary.Run(t, test.args...)

			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.AssertFile(t, test.expectedStderr, result.Stderr)
		})
	}
}
