package integration_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestDiagnosticsColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		env        []string
		color      bool
		decoration bool
	}{
		{name: "pipe"},
		{name: "force", env: []string{"CLICOLOR_FORCE=1"}, color: true, decoration: true},
		{name: "no_color", env: []string{"NO_COLOR=1"}},
		{name: "forced_color_in_pipe", env: []string{"CLICOLOR_FORCE=1", "NO_COLOR=1"}, color: true, decoration: true},
		{name: "no_color_flag", args: []string{"--no-color"}, env: []string{"CLICOLOR_FORCE=1"}, decoration: true},
		{name: "clicolor_zero", env: []string{"CLICOLOR=0"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: captured output and explicit color preferences
			env := append([]string{
				"TERM=xterm-256color", "COLORTERM=", "NO_COLOR=", "CLICOLOR=", "CLICOLOR_FORCE=",
			}, test.env...)
			args := append(append([]string(nil), test.args...), "NotACommand")

			// when: a command fails before its logging pre-run hook
			result := binary.RunWithOptions(t, args, testastic.WithRunEnv(env...))

			// then: color and decoration follow existing selection rules and text stays readable
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.Equal(t, test.color, regexp.MustCompile(`\x1b\[[^m]*[349][0-9][^m]*m`).MatchString(result.Stderr))
			testastic.Equal(t, test.decoration, strings.Contains(result.Stderr, "\x1b["))
			testastic.AssertFile(t, "testdata/diagnostics/unknown_command/stderr.expected.txt", ansi.Strip(result.Stderr))
		})
	}
}

func TestDiagnosticsLoggingOptionOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		args  []string
		color bool
	}{
		{name: "no_color_before", args: []string{"--no-color", "--UnknownFlag"}},
		{name: "no_color_after", args: []string{"--UnknownFlag", "--no-color"}, color: true},
		{name: "verbose_before", args: []string{"--verbose", "--UnknownFlag"}, color: true},
		{name: "verbose_after", args: []string{"--UnknownFlag", "--verbose"}, color: true},
		{name: "short_verbose_after", args: []string{"--UnknownFlag", "-v"}, color: true},
		{name: "no_color_after_invalid_value", args: []string{"--verbose=Perhaps", "--no-color"}, color: true},
		{name: "no_color_after_invalid_syntax", args: []string{"---Invalid", "--no-color"}, color: true},
		{name: "last_value_wins", args: []string{"--verbose", "--UnknownFlag", "--verbose=false"}, color: true},
		{name: "flag_value", args: []string{"release", "--UnknownFlag", "--config", "--no-color"}, color: true},
		{name: "after_separator", args: []string{"version", "--UnknownFlag", "--", "--no-color", "-v"}, color: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: forced color and logging options on either side of an invalid flag
			env := []string{"CLICOLOR_FORCE=1", "NO_COLOR=", "TERM=xterm-256color"}

			// when: command parsing fails
			result := binary.RunWithOptions(t, test.args, testastic.WithRunEnv(env...))

			// then: one error is reported, honoring the logging options parsed before the invalid flag
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.Equal(t, test.color, regexp.MustCompile(`\x1b\[[^m]*[349][0-9][^m]*m`).MatchString(result.Stderr))
			plain := ansi.Strip(result.Stderr)
			testastic.Equal(t, 1, strings.Count(plain, "ERROR "))
			testastic.NotContains(t, plain, " cause=")
		})
	}
}

func TestDiagnosticsProviderCause(t *testing.T) {
	t.Parallel()

	for _, verbose := range []bool{false, true} {
		name := "default"
		if verbose {
			name = "verbose"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// given: a provider failure whose body and request metadata contain synthetic secrets
			repoDir, shas := writeIndependentMonorepoHistory(t)
			opts := independentGitHubOptions(shas)
			opts.FailOnMutation = true
			fake := fakeprovider.NewGitHub(t, opts)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/commits/heads/main") {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-GitHub-Request-Id", "diagnostic-test-credential")
					w.WriteHeader(http.StatusForbidden)
					_, err := w.Write([]byte(readTestFile(t, "testdata/diagnostics/provider_cause/response.json")))
					testastic.NoError(t, err)

					return
				}

				fake.Config.Handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			configPath := absoluteTestFile(t, "testdata/release/independent_create/input.yaml")

			args := []string{"release", "--config", configPath}
			if verbose {
				args = append(args, "--verbose")
			}

			env := append(fixture.GitHubEnv(server, "main"), "GITHUB_TOKEN=diagnostic-test-credential")

			// when: the release fails while checking the provider's branch head
			result := binary.RunWithOptions(t, args, testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(env...))

			// then: verbosity only adds debug records and errors retain the same safe details
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.False(t, strings.Contains(result.Stderr, "diagnostic-test-credential"))
			testastic.NotContains(t, withoutDebugDiagnostics(result.Stderr), "private-sdk-response")
			testastic.Equal(t, verbose, strings.Contains(result.Stderr, "private-sdk-response"))
			testastic.Equal(t, verbose, strings.Contains(result.Stderr, "DEBUG http request completed"))
			testastic.NotContains(t, result.Stderr, " cause=")

			var terminal []string

			for line := range strings.SplitSeq(result.Stderr, "\n") {
				if strings.HasPrefix(line, "ERROR ") {
					terminal = append(terminal, line)
				}
			}

			testastic.AssertFile(t, "testdata/diagnostics/provider_cause/stderr.expected.txt",
				strings.Join(terminal, "\n")+"\n")
		})
	}
}

func TestDiagnosticsTokenRedaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		provider string
	}{
		{name: "placeholder_token", token: "github", provider: "provider=[redacted]"},
		{name: "real_token", token: "ghp_0123456789abcdefghijklmnopqrstuvwxyzAB", provider: "provider=github"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a custom host configuration outside a git repository
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider: "github",
				Branch:   "main",
				Host:     "github.example",
				Owner:    "acme",
				Repo:     "repo",
			})

			// when: planning a release with the token in the environment
			result := binary.RunWithOptions(
				t,
				[]string{"release", "--dry-run", "--no-color", "--config", configPath},
				testastic.WithRunWorkDir(t.TempDir()),
				testastic.WithRunEnv("GITHUB_TOKEN="+test.token),
			)

			// then: no token value reaches the diagnostic, whatever its length
			testastic.Equal(t, 1, result.ExitCode)

			stderr := ansi.Strip(result.Stderr)
			testastic.NotContains(t, stderr, test.token)
			testastic.Contains(t, stderr, test.provider)
		})
	}
}

func TestDiagnosticsUnclassifiedCause(t *testing.T) {
	t.Parallel()

	// given: a provider that answers the branch head with a body it cannot parse
	repoDir, shas := writeIndependentMonorepoHistory(t)
	fake := fakeprovider.NewGitHub(t, independentGitHubOptions(shas))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/commits/heads/main") {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte("{not json"))
			testastic.NoError(t, err)

			return
		}

		fake.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	// when: the release runs against it
	result := binary.RunWithOptions(t,
		[]string{"release", "--no-color", "--config", absoluteTestFile(t, "testdata/release/independent_create/input.yaml")},
		testastic.WithRunWorkDir(repoDir),
		testastic.WithRunEnv(append(fixture.GitHubEnv(server, "main"), "GITHUB_TOKEN=test-token")...),
	)

	// then: the bare category line still names why the run failed
	testastic.Equal(t, 1, result.ExitCode)
	testastic.Equal(t, "", result.Stdout)

	stderr := ansi.Strip(result.Stderr)
	testastic.Contains(t, stderr, "ERROR release could not be completed")
	testastic.Contains(t, stderr, " cause=")
}
