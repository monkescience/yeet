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

	for _, scenario := range []struct {
		name     string
		collides bool
		token    string
	}{
		{name: "long", token: "diagnostic-test-credential"},
		{name: "short", token: "e", collides: true},
		{name: "request_id_collision", token: "E123:ABC:456", collides: true},
		{name: "numeric_collision", token: "0", collides: true},
	} {
		for _, verbose := range []bool{false, true} {
			name := "default"
			if verbose {
				name = "verbose"
			}

			t.Run(scenario.name+"/"+name, func(t *testing.T) {
				t.Parallel()

				// given: a provider failure with private body content and documented diagnostic headers
				repoDir, shas := writeIndependentMonorepoHistory(t)
				opts := independentGitHubOptions(shas)
				opts.FailOnMutation = true
				fake := fakeprovider.NewGitHub(t, opts)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/commits/heads/main") {
						w.Header().Set("Content-Type", "application/json")

						w.Header().Set("X-GitHub-Request-Id", "E123:ABC:456")
						w.Header().Set("X-RateLimit-Remaining", "100")
						w.Header().Set("X-RateLimit-Reset", "1700000000")
						w.Header().Set("Retry-After", "60")
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

				env := append(fixture.GitHubEnv(server, "main"), "GITHUB_TOKEN="+scenario.token)

				// when: the release fails while checking the provider's branch head
				result := binary.RunWithOptions(t, args, testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(env...))

				// then: headers remain intact while response bodies are excluded at either verbosity
				testastic.Equal(t, 1, result.ExitCode)
				testastic.Equal(t, "", result.Stdout)

				if !scenario.collides {
					testastic.NotContains(t, result.Stderr, scenario.token)
				}

				testastic.NotContains(t, withoutDebugDiagnostics(result.Stderr), "private-sdk-response")
				testastic.NotContains(t, result.Stderr, "private-sdk-response")
				testastic.Equal(t, verbose, strings.Contains(result.Stderr, "DEBUG http request completed"))
				testastic.NotContains(t, result.Stderr, " cause=")

				if verbose {
					testastic.Contains(t, result.Stderr, "request_id=E123:ABC:456")
					testastic.Contains(t, result.Stderr, "rate_limit_remaining=100")
					testastic.Contains(t, result.Stderr, "rate_limit_reset=1700000000")
					testastic.Contains(t, result.Stderr, "retry_after=60")
				}

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
}

func TestDiagnosticsTokenRedaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  string
	}{
		{name: "single_character", env: "GITHUB_TOKEN=e"},
		{name: "attribute_key", env: "GITLAB_TOKEN=t"},
		{name: "provider_name", env: "GITHUB_TOKEN=github"},
		{name: "remote_name", env: "GITLAB_TOKEN=origin"},
		{name: "real_token", env: "GITHUB_TOKEN=ghp_0123456789abcdefghijklmnopqrstuvwxyzAB"},
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
				testastic.WithRunEnv(test.env),
			)

			// then: incidental matches do not corrupt application messages or repository coordinates
			testastic.Equal(t, 1, result.ExitCode)

			stderr := ansi.Strip(result.Stderr)
			testastic.AssertFile(t, "testdata/diagnostics/token_collision/stderr.expected.txt", stderr)
		})
	}
}

func TestDiagnosticsRedactsTokensInArguments(t *testing.T) {
	t.Parallel()

	const token = "ghp_0123456789abcdefghijklmnopqrstuvwxyzAB"

	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "unknown_command", args: []string{token}},
		{name: "unknown_flag", args: []string{"--" + token}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a token long enough to be unambiguous, echoed back through a CLI argument
			args := append([]string{}, test.args...)
			args = append(args, "--no-color")

			// when: yeet reports the resulting argument failure
			result := binary.RunWithOptions(t, args,
				testastic.WithRunWorkDir(t.TempDir()),
				testastic.WithRunEnv("GITHUB_TOKEN="+token))

			// then: the token value never reaches the diagnostic
			stderr := ansi.Strip(result.Stderr)
			testastic.NotContains(t, stderr, token)
			testastic.Contains(t, stderr, "[redacted]")
		})
	}
}

func TestDiagnosticsRedactsTokensInAttributes(t *testing.T) {
	t.Parallel()

	const token = "ghp_0123456789abcdefghijklmnopqrstuvwxyzAB"

	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "provider", args: []string{"--provider", token}},
		{name: "remote", args: []string{"--remote", token}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a token echoed back through a flag value that reaches a log attribute
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider: "github",
				Branch:   "main",
				Host:     "github.example",
				Owner:    "acme",
				Repo:     "repo",
			})
			args := append([]string{"release", "--no-color", "--config", configPath}, test.args...)

			// when: yeet reports the resulting failure
			result := binary.RunWithOptions(t, args,
				testastic.WithRunWorkDir(t.TempDir()),
				testastic.WithRunEnv("GITHUB_TOKEN="+token))

			// then: the token value never reaches the diagnostic, in the message or in any attribute
			testastic.Equal(t, 1, result.ExitCode)

			stderr := ansi.Strip(result.Stderr)
			testastic.NotContains(t, stderr, token)
			testastic.Contains(t, stderr, "[redacted]")
		})
	}
}

func TestDiagnosticsRedactionLengthBoundary(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		token    string
		redacted bool
	}{
		{name: "below_threshold", token: "abcdefghijklmno"},
		{name: "at_threshold", token: "abcdefghijklmnop", redacted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a token one byte either side of the redaction threshold, echoed through a flag
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider: "github",
				Branch:   "main",
				Host:     "github.example",
				Owner:    "acme",
				Repo:     "repo",
			})

			// when: yeet reports the resulting failure
			result := binary.RunWithOptions(t,
				[]string{"release", "--no-color", "--config", configPath, "--provider", test.token},
				testastic.WithRunWorkDir(t.TempDir()),
				testastic.WithRunEnv("GITHUB_TOKEN="+test.token))

			// then: only tokens long enough to be unambiguous are replaced
			testastic.Equal(t, 1, result.ExitCode)

			stderr := ansi.Strip(result.Stderr)
			testastic.Equal(t, test.redacted, strings.Contains(stderr, "[redacted]"))
			testastic.Equal(t, !test.redacted, strings.Contains(stderr, "provider="+test.token))
		})
	}
}

func TestDiagnosticsConfigurationTokenCollision(t *testing.T) {
	t.Parallel()

	// given: malformed configuration and a token matching ordinary diagnostic prose
	configPath := absoluteTestFile(t, "testdata/release/malformed_yaml/input.yaml")

	// when: validating that configuration
	result := binary.RunWithOptions(t,
		[]string{"release", "--config", configPath, "--no-color"},
		testastic.WithRunEnv("GITHUB_TOKEN=e"))

	// then: the parser location and explanation remain readable
	testastic.Equal(t, 1, result.ExitCode)
	testastic.AssertFile(t, "testdata/release/malformed_yaml/stderr.expected.txt", ansi.Strip(result.Stderr))
}

func TestDiagnosticsUnclassifiedCause(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name  string
		body  string
		cause string
	}{
		{name: "syntax", body: "{not json", cause: "response is not valid JSON"},
		{name: "unknown_shape", body: "[]", cause: "unclassified error"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a provider that answers the branch head with a body it cannot parse
			repoDir, shas := writeIndependentMonorepoHistory(t)
			fake := fakeprovider.NewGitHub(t, independentGitHubOptions(shas))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/commits/heads/main") {
					w.Header().Set("Content-Type", "application/json")
					_, err := w.Write([]byte(scenario.body))
					testastic.NoError(t, err)

					return
				}

				fake.Config.Handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)

			configPath := absoluteTestFile(t, "testdata/release/independent_create/input.yaml")

			// when: the release runs against it
			result := binary.RunWithOptions(t,
				[]string{"release", "--no-color", "--config", configPath},
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(append(fixture.GitHubEnv(server, "main"), "GITHUB_TOKEN=test-token")...),
			)

			// then: the bare category line still names why the run failed
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)

			stderr := ansi.Strip(result.Stderr)
			testastic.Contains(t, stderr, "ERROR release could not be completed")
			testastic.Contains(t, stderr, `cause="`+scenario.cause+`"`)
		})
	}
}
