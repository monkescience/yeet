package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		golden string
	}{
		{
			name:   "root",
			args:   []string{"--help"},
			golden: "testdata/help/root/stdout.expected.txt",
		},
		{
			name:   "release",
			args:   []string{"release", "--help"},
			golden: "testdata/help/release/stdout.expected.txt",
		},
		{
			name:   "init",
			args:   []string{"init", "--help"},
			golden: "testdata/help/init/stdout.expected.txt",
		},
		{
			name:   "version",
			args:   []string{"version", "--help"},
			golden: "testdata/help/version/stdout.expected.txt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := binary.Run(t, tc.args...)

			testastic.Equal(t, 0, result.ExitCode)
			testastic.Equal(t, "", result.Stderr)
			testastic.AssertFile(t, tc.golden, result.Stdout)
		})
	}
}
