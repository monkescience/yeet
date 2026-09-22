package integration_test

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/monkescience/testastic"
)

func TestDiagnosticsEarlyFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown_command", args: []string{"--no-color", "NotACommand"}},
		{name: "unknown_flag", args: []string{"version", "--no-color", "--UnknownFlag"}},
		{name: "missing_flag_value", args: []string{"release", "--no-color", "--config"}},
		{name: "missing_provider_value", args: []string{"release", "--no-color", "--provider"}},
		{name: "invalid_flag_value", args: []string{"version", "--no-color", "--verbose=Perhaps"}},
		{name: "quiet_argument", args: []string{"--quiet", "version", "Unexpected"}},
		{name: "short_conflict", args: []string{"-v", "--quiet", "version"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a command that fails before its normal execution

			// when: invoking the compiled command with invalid arguments
			result := binary.Run(t, test.args...)

			// then: one structured error carries actionable context on stderr
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.AssertFile(t, "testdata/diagnostics/"+test.name+"/stderr.expected.txt", ansi.Strip(result.Stderr))
		})
	}
}

func TestDiagnosticsErrorsDoNotDependOnVerbosity(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name  string
		args  []string
		cause string
	}{
		{name: "unknown flag", args: []string{"version", "--UnknownFlag"}},
		{
			name: "invalid configuration",
			args: []string{
				"release", "--config",
				absoluteTestFile(t, "testdata/release/rejects_removed_auto_merge_force_config/input.yaml"),
			},
			cause: "yaml constructor at 4:3",
		},
		{
			name: "missing configuration", args: []string{"release", "--config", "missing/config.yaml"},
			cause: "no such file or directory",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a command that fails with the same inputs at every verbosity
			workDir := t.TempDir()

			var expected string

			// when: running the command normally, verbosely, and quietly
			for _, flags := range [][]string{nil, {"--verbose"}, {"--quiet"}} {
				args := append(append([]string{"--no-color"}, scenario.args...), flags...)
				result := binary.RunWithOptions(t, args, testastic.WithRunWorkDir(workDir))

				// then: error records stay identical and actionable without debug logging
				testastic.Equal(t, 1, result.ExitCode)
				testastic.Equal(t, "", result.Stdout)

				actual := errorDiagnostics(result.Stderr)
				if expected == "" {
					expected = actual
				}

				testastic.Equal(t, expected, actual)

				if scenario.cause != "" {
					testastic.Contains(t, actual, scenario.cause)
				}
			}
		})
	}
}

func TestDiagnosticsUnknownCommandSuggestion(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name     string
		args     []string
		argument string
	}{
		{name: "direct", args: []string{"--no-color", "releas"}, argument: "releas"},
		{name: "after long flag", args: []string{"--no-color", "--config", "config.yaml", "relase"}, argument: "relase"},
		{name: "after short flag", args: []string{"--no-color", "-c", "config.yaml", "relase"}, argument: "relase"},
		{name: "after target flag", args: []string{"--no-color", "--target", "app", "relase"}, argument: "relase"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			result := binary.Run(t, scenario.args...)

			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.Equal(t,
				"ERROR unknown command command=yeet argument="+scenario.argument+" hint=\"did you mean release?\"\n",
				ansi.Strip(result.Stderr),
			)
		})
	}
}
