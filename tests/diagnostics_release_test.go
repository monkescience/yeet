package integration_test

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestDiagnosticsReleaseIndependentFailures(t *testing.T) {
	t.Parallel()

	for _, partial := range []bool{false, true} {
		for _, verbosity := range []string{"default", "verbose", "quiet"} {
			name := "all_failed/" + verbosity
			if partial {
				name = "partial_failure/" + verbosity
			}

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				// given: independent release units with either one or two provider failures
				repoDir, shas := writeIndependentMonorepoHistory(t)
				opts := independentGitHubOptions(shas)
				opts.Files = diagnosticReleaseFiles()
				opts.ExpectedCreatedPullRequests = failedIndependentPullRequests()
				units := []string{"target:api", "target:web"}
				golden := "testdata/diagnostics/release_default/stderr.expected.txt"

				if partial {
					opts.ExpectedCreatedPullRequests = partiallyFailedIndependentPullRequests()
					units = units[:1]
					golden = "testdata/diagnostics/release_partial_default/stderr.expected.txt"
				}

				server := fakeprovider.NewGitHub(t, opts)

				var flags []string
				if verbosity != "default" {
					flags = []string{"--" + verbosity}
				}

				// when: running the release at the selected verbosity
				result := runDiagnosticsIndependentRelease(t, repoDir, server, flags)

				// then: the same errors remain visible and verbosity only filters log levels
				assertFailedReleaseUnits(t, result, units...)
				stderr := ansi.Strip(result.Stderr)

				expected := readTestFile(t, golden)
				if verbosity == "quiet" {
					expected = errorDiagnostics(expected)
				}

				testastic.Equal(t, expected, withoutDebugDiagnostics(stderr))
				testastic.Equal(t, verbosity == "verbose", strings.Contains(stderr, "DEBUG "))

				if partial && verbosity != "quiet" {
					assertSuccessfulReleaseUnit(t, result)
				} else {
					testastic.NotContains(t, stderr, "INFO ")
				}
			})
		}
	}

	t.Run("group failure preserves mixed case group identifier", func(t *testing.T) {
		t.Parallel()

		// given: a BackendTeam group whose pull request cannot be created
		repoDir, shas := writeIndependentMonorepoHistory(t)
		opts := independentGitHubOptions(shas)
		opts.Files = diagnosticReleaseFiles()
		opts.ExpectedCreatedPullRequests = []fakeprovider.GitHubPullRequestExpectation{{
			Title:      "chore: release wave",
			Head:       "yeet/release-main-group-backendteam-bf08766af1",
			Base:       "main",
			StatusCode: http.StatusUnprocessableEntity,
		}}
		server := fakeprovider.NewGitHub(t, opts)
		configPath := absoluteTestFile(t, "testdata/diagnostics/release_group/input.yaml")

		// when: running the grouped independent release
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: one error retains the group identifier's configured case
		assertFailedReleaseUnits(t, result, "group:BackendTeam")
		testastic.AssertFile(
			t,
			"testdata/diagnostics/release_group/stderr.expected.txt",
			ansi.Strip(result.Stderr),
		)
	})

	t.Run("mixed finalization failures report unscoped and unit causes", func(t *testing.T) {
		t.Parallel()

		// given: a malformed api request and a web request that fails during finalization
		repoDir, shas := writeIndependentMonorepoHistory(t)
		opts := independentGitHubOptions(shas)
		opts.Files = diagnosticReleaseFiles()
		opts.FailOnMutation = true
		fake := fakeprovider.NewGitHub(t, opts)
		server := newMixedFinalizationFailureServer(t, fake)
		configPath := absoluteTestFile(t, "testdata/diagnostics/release_mixed_failures/input.yaml")

		// when: finalizing the independent merged requests
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: each cause is reported once and the malformed request is identified
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
		stderr := ansi.Strip(result.Stderr)
		testastic.Equal(t, 1, strings.Count(stderr, "ERROR release pull request has an invalid manifest"))
		testastic.Equal(t, 1, strings.Count(stderr, "pull_request=\"pull request #101\""))
		testastic.Equal(
			t,
			1,
			strings.Count(stderr, "ERROR configured release label does not exist unit=target:web"),
		)
		testastic.Equal(t, 2, strings.Count(stderr, "ERROR "))
		testastic.AssertFile(
			t,
			"testdata/diagnostics/release_mixed_failures/stderr.expected.txt",
			stderr,
		)
	})

	t.Run("verbose output renders every failure suppressed by unit grouping", func(t *testing.T) {
		t.Parallel()

		// given: a release whose units fail for several reasons at once
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})
		fake := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:        "group/service",
			LatestTag:      "v1.0.0",
			BoundarySHA:    shas[0],
			BranchHeadSHA:  shas[1],
			Users:          map[string]int64{"alice": 1, "bob": 2},
			ExistingLabels: []string{"yeet", "autorelease: pending", "autorelease: tagged"},
		})
		server := gitLabJoinedFailureServer(t, fake)
		configPath := absoluteTestFile(t, "testdata/diagnostics/release_joined_failures/input.yaml")

		// when: releasing with verbose diagnostics
		result := binary.RunWithOptions(t,
			[]string{"release", "--verbose", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: every failure the grouped summary suppressed is still rendered
		testastic.Equal(t, 1, result.ExitCode)
		assertDiagnosticFragments(
			t,
			"testdata/diagnostics/release_joined_failures/stderr.expected.txt",
			ansi.Strip(result.Stderr),
		)
		testastic.NotContains(t, result.Stderr, "private-label-response")
	})
}

func gitLabJoinedFailureServer(t *testing.T, fake *httptest.Server) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/merge_requests") {
			response := httptest.NewRecorder()
			fake.Config.Handler.ServeHTTP(response, r)

			var payload map[string]any

			err := json.Unmarshal(response.Body.Bytes(), &payload)
			testastic.NoError(t, err)
			delete(payload, "reviewers")

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(response.Code)
			err = json.MarshalWrite(w, payload)
			testastic.NoError(t, err)

			return
		}

		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/merge_requests/42") {
			http.Error(w, "private-label-response", http.StatusForbidden)

			return
		}

		fake.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	return server
}

func newMixedFinalizationFailureServer(t *testing.T, fake *httptest.Server) *httptest.Server {
	t.Helper()

	apiPullRequest := readTestFile(t, "testdata/diagnostics/release_mixed_failures/api_pull_request.json")
	webPullRequest := readTestFile(t, "testdata/diagnostics/release_mixed_failures/web_pull_request.json")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls") &&
			r.URL.Query().Get("state") == "closed" {
			head := r.URL.Query().Get("head")
			if strings.HasSuffix(head, "yeet/release-main-target-api-21f150df25") {
				writeMixedFinalizationResponse(t, w, "["+apiPullRequest+"]")

				return
			}

			if strings.HasSuffix(head, "yeet/release-main-target-web-1355f0b5d0") {
				writeMixedFinalizationResponse(t, w, "["+webPullRequest+"]")

				return
			}
		}

		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls/101") {
			writeMixedFinalizationResponse(t, w, apiPullRequest)

			return
		}

		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls/102") {
			writeMixedFinalizationResponse(t, w, webPullRequest)

			return
		}

		fake.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	return server
}

func writeMixedFinalizationResponse(t *testing.T, w http.ResponseWriter, response string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(response))
	testastic.NoError(t, err)
}

func diagnosticReleaseFiles() map[string]string {
	return map[string]string{
		"api/CHANGELOG.md": "api release\n",
		"web/CHANGELOG.md": "web release\n",
	}
}

func failedIndependentPullRequests() []fakeprovider.GitHubPullRequestExpectation {
	return []fakeprovider.GitHubPullRequestExpectation{
		failedIndependentPullRequest("api", "1.1.0"),
		failedIndependentPullRequest("web", "2.0.1"),
	}
}

func partiallyFailedIndependentPullRequests() []fakeprovider.GitHubPullRequestExpectation {
	return []fakeprovider.GitHubPullRequestExpectation{
		failedIndependentPullRequest("api", "1.1.0"),
		{
			Title: "chore: release 2.0.1",
			Head:  "yeet/release-main-target-web-1355f0b5d0",
			Base:  "main",
		},
	}
}

func failedIndependentPullRequest(target, version string) fakeprovider.GitHubPullRequestExpectation {
	branches := map[string]string{
		"api": "yeet/release-main-target-api-21f150df25",
		"web": "yeet/release-main-target-web-1355f0b5d0",
	}

	return fakeprovider.GitHubPullRequestExpectation{
		Title:      "chore: release " + version,
		Head:       branches[target],
		Base:       "main",
		StatusCode: http.StatusUnprocessableEntity,
	}
}

func runDiagnosticsIndependentRelease(
	t *testing.T,
	repoDir string,
	server *httptest.Server,
	flags []string,
) *testastic.RunResult {
	t.Helper()

	args := append([]string{"release"}, flags...)
	args = append(args, "--config", absoluteTestFile(t, "testdata/diagnostics/release_independent/input.yaml"))

	return binary.RunWithOptions(t,
		args,
		testastic.WithRunWorkDir(repoDir),
		testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
	)
}

func assertFailedReleaseUnits(t *testing.T, result *testastic.RunResult, units ...string) {
	t.Helper()

	testastic.Equal(t, 1, result.ExitCode)
	testastic.Equal(t, "", result.Stdout)
	stderr := ansi.Strip(result.Stderr)

	for _, unit := range units {
		testastic.Equal(
			t,
			1,
			strings.Count(stderr, "ERROR provider request failed unit="+unit+" phase=reconciliation status=422"),
		)
	}

	testastic.Equal(t, len(units), strings.Count(stderr, "ERROR "))
	testastic.NotContains(t, stderr, "release failed:")
	testastic.NotContains(t, stderr, "injected pull request failure")
}

func assertSuccessfulReleaseUnit(t *testing.T, result *testastic.RunResult) {
	t.Helper()

	stderr := ansi.Strip(result.Stderr)
	testastic.Contains(t, stderr, "INFO  created release pull request")
	testastic.Equal(t, 1, strings.Count(stderr, "INFO  created release pull request"))
}
