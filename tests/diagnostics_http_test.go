package integration_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestDiagnosticsHTTPFailures(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name      string
		status    int
		rateLimit bool
		hint      string
	}{
		{"unauthorized", http.StatusUnauthorized, false, "check that the provider token is valid and has not expired"},
		{"forbidden", http.StatusForbidden, false, "check token permissions, repository access, and provider policy"},
		{"rate_limited", http.StatusForbidden, true, "wait for the provider rate limit to reset before retrying"},
		{"conflict", http.StatusConflict, false, "check the requested resource state and provider validation rules"},
		{
			"validation", http.StatusUnprocessableEntity, false,
			"check the requested resource state and provider validation rules",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a GitHub branch lookup fails with a known HTTP status
			repoDir, shas := writeIndependentMonorepoHistory(t)
			fake := fakeprovider.NewGitHub(t, independentGitHubOptions(shas))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/commits/heads/main") {
					fake.Config.Handler.ServeHTTP(w, r)

					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-GitHub-Request-Id", "request-123")

				if scenario.rateLimit {
					w.Header().Set("X-RateLimit-Remaining", "0")
				}

				w.WriteHeader(scenario.status)
				_, err := w.Write([]byte(`{"message":"private response body"}`))
				testastic.NoError(t, err)
			}))
			t.Cleanup(server.Close)

			// when: a release fails without verbose logging
			result := binary.RunWithOptions(t,
				[]string{"release", "--config", absoluteTestFile(t, "testdata/release/independent_create/input.yaml")},
				testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
			)

			// then: the terminal error identifies the request and offers status-specific guidance
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			output := errorDiagnostics(result.Stderr)
			testastic.Equal(t, 1, strings.Count(output, "ERROR "))
			testastic.Contains(t, output, "status="+strconv.Itoa(scenario.status))
			testastic.Contains(t, output, "provider=github method=GET path=/api/v3/repos/testorg/testrepo/commits/heads/main")
			testastic.Contains(t, output, "request_id=request-123")
			testastic.Contains(t, output, scenario.hint)
			testastic.NotContains(t, result.Stderr, "private response body")
		})
	}
}
