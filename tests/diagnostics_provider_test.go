package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestDiagnosticsGitLabNotFound(t *testing.T) {
	t.Parallel()

	for _, verbosity := range []string{"default", "verbose"} {
		t.Run(verbosity, func(t *testing.T) {
			t.Parallel()

			// given: GitLab returns a not-found response while listing release tags
			repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
			fake := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
				Project: "group/service", BranchHeadSHA: shas[1], ForbidPublication: true,
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/repository/tags") {
					w.WriteHeader(http.StatusNotFound)

					return
				}

				fake.Config.Handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)

			args := []string{"release", "--dry-run", "--config", providerAutoMergeConfig(t, "gitlab")}
			if verbosity == "verbose" {
				args = append(args, "--verbose")
			}

			// when: the release encounters the GitLab SDK's not-found sentinel
			result := binary.RunWithOptions(t, args,
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
			)

			// then: the provider diagnostic preserves status 404 and provider debug logs respect verbosity
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.Equal(t, verbosity == "verbose",
				strings.Contains(ansi.Strip(result.Stderr), "DEBUG listing tags provider=gitlab"))
			testastic.AssertFile(t,
				"testdata/diagnostics/gitlab_not_found/stderr.expected.txt",
				errorDiagnostics(result.Stderr),
			)
		})
	}
}

func TestDiagnosticsMergeTimeout(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		mode    string
		verbose bool
	}{
		{"combined", false},
		{"combined", true},
		{"independent", false},
		{"independent", true},
	} {
		name := scenario.mode + "/default"
		if scenario.verbose {
			name = scenario.mode + "/verbose"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// given: GitLab accepts a merge and its next poll exhausts the wait budget
			repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
			fake := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
				Project: "group/service", LatestTag: "v1.0.0",
				BoundarySHA: shas[0], BranchHeadSHA: shas[1],
				AsynchronousMerge: true, ForbidPublication: true,
			})

			var accepted, polled atomic.Bool

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/merge_requests/42/merge") {
					accepted.Store(true)
				}

				if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/merge_requests/42") && accepted.Load() {
					polled.Store(true)
					<-r.Context().Done()

					return
				}

				fake.Config.Handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)

			fixturePath := "testdata/diagnostics/merge_timeout_" + scenario.mode

			args := []string{"release", "--config", absoluteTestFile(t, fixturePath+"/input.yaml")}
			if scenario.verbose {
				args = append(args, "--verbose")
			}

			// when: the CLI waits for the accepted direct merge to finalize
			result := binary.RunWithOptions(t,
				args,
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
			)

			// then: the timeout retains its cause and verbose logs identify the provider and pull request
			testastic.True(t, polled.Load())
			testastic.Equal(t, scenario.verbose,
				strings.Contains(ansi.Strip(result.Stderr), "DEBUG merging merge request provider=gitlab pr_number=42"))
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.AssertFile(t, fixturePath+"/stderr.expected.txt", errorDiagnostics(result.Stderr))
		})
	}
}

func errorDiagnostics(stderr string) string {
	var output strings.Builder

	for line := range strings.SplitSeq(ansi.Strip(stderr), "\n") {
		if strings.HasPrefix(line, "ERROR ") {
			output.WriteString(line)
			output.WriteByte('\n')
		}
	}

	return output.String()
}

func withoutDebugDiagnostics(stderr string) string {
	var output strings.Builder

	for line := range strings.SplitAfterSeq(ansi.Strip(stderr), "\n") {
		if !strings.HasPrefix(line, "DEBUG ") {
			output.WriteString(line)
		}
	}

	return output.String()
}

func TestDiagnosticsMergeTimeoutWhileResponsive(t *testing.T) {
	t.Parallel()

	// given: GitLab accepts a merge and then keeps reporting it as unmerged
	repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
	fake := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
		Project: "group/service", LatestTag: "v1.0.0",
		BoundarySHA: shas[0], BranchHeadSHA: shas[1],
		AsynchronousMerge: true, ForbidPublication: true,
	})

	var accepted atomic.Bool

	var polls atomic.Int32

	var responded atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/merge_requests/42/merge") {
			accepted.Store(true)
		}

		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/merge_requests/42") && accepted.Load() {
			polls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(map[string]any{
				"id":                    42,
				"iid":                   42,
				"state":                 "opened",
				"detailed_merge_status": "mergeable",
				"sha":                   shas[1],
				"source_branch":         "yeet/release-main",
				"target_branch":         "main",
				"source_project_id":     42,
				"target_project_id":     42,
			})
			testastic.NoError(t, err)
			responded.Store(true)

			return
		}

		fake.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	// when: the CLI waits for the accepted merge to finalize
	result := binary.RunWithOptions(t,
		[]string{"release", "--config", absoluteTestFile(t, "testdata/diagnostics/merge_timeout_responsive/input.yaml")},
		testastic.WithRunWorkDir(repoDir),
		testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
	)

	// then: the timeout still names the request and the configured budget
	testastic.Equal(t, int32(1), polls.Load())
	testastic.True(t, responded.Load())
	testastic.Equal(t, 1, result.ExitCode)
	testastic.Equal(t, "", result.Stdout)

	testastic.AssertFile(t, "testdata/diagnostics/merge_timeout_responsive/stderr.expected.txt",
		errorDiagnostics(result.Stderr))
}

const gitLabDiagnosticsToken = "gitlab-diagnostics-test-token"

func TestDiagnosticsGitLabMergeStatus(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name   string
		status string
	}{
		{name: "known", status: "not_approved"},
		{name: "blocked_status", status: "blocked_status"},
		{name: "broken_status", status: "broken_status"},
		{name: "external_status_checks", status: "external_status_checks"},
		{name: "policies_denied", status: "policies_denied"},
		{name: "unknown", status: "future_merge_check"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: GitLab reports a detailed merge status, including one this client predates
			repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
			fake := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
				Project: "group/service", LatestTag: "v1.0.0",
				BoundarySHA: shas[0], BranchHeadSHA: shas[1], ForbidPublication: true,
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/merge_requests/42") {
					response := httptest.NewRecorder()
					fake.Config.Handler.ServeHTTP(response, r)

					var payload map[string]any

					testastic.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
					payload["detailed_merge_status"] = scenario.status

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(response.Code)
					testastic.NoError(t, json.NewEncoder(w).Encode(payload))

					return
				}

				fake.Config.Handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)

			// when: attempting a direct merge with verbose diagnostics
			result := binary.RunWithOptions(t,
				[]string{
					"release", "--auto-merge", "--auto-merge-mode", "direct", "--verbose",
					"--config", providerAutoMergeConfig(t, "gitlab"),
				},
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(append(fixture.GitLabEnv(server, "main"),
					"GITLAB_TOKEN="+gitLabDiagnosticsToken)...))

			// then: the status reaches the user verbatim, including values introduced after this client
			testastic.Equal(t, 1, result.ExitCode)
			testastic.AssertFile(t, "testdata/diagnostics/gitlab_status_"+scenario.name+"/stderr.expected.txt",
				errorDiagnostics(result.Stderr))
		})
	}
}
