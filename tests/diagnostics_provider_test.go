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

			// then: one provider diagnostic preserves status 404 at either verbosity
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
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

			// then: one timeout diagnostic retains the cause and any independent unit
			testastic.True(t, polled.Load())
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

	var accepted, polled atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/merge_requests/42/merge") {
			accepted.Store(true)
		}

		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/merge_requests/42") && accepted.Load() {
			polled.Store(true)
			writeUnmergedMergeRequest(t, w, r, fake.Config.Handler)

			return
		}

		fake.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	// when: the CLI waits for the accepted merge to finalize
	result := binary.RunWithOptions(t,
		[]string{"release", "--config", absoluteTestFile(t, "testdata/diagnostics/merge_timeout_combined/input.yaml")},
		testastic.WithRunWorkDir(repoDir),
		testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
	)

	// then: the timeout still names the request and the configured budget
	testastic.True(t, polled.Load())
	testastic.Equal(t, 1, result.ExitCode)
	testastic.Equal(t, "", result.Stdout)

	diagnostics := errorDiagnostics(result.Stderr)
	testastic.Contains(t, diagnostics, "merge finalization timed out")
	testastic.Contains(t, diagnostics, `pull_request="merge request !42"`)
	testastic.Contains(t, diagnostics, "timeout=250ms")
	testastic.Contains(t, diagnostics, `cause="operation timed out"`)
}

func writeUnmergedMergeRequest(t *testing.T, w http.ResponseWriter, r *http.Request, next http.Handler) {
	t.Helper()

	recorder := httptest.NewRecorder()
	next.ServeHTTP(recorder, r)

	var body map[string]any

	err := json.Unmarshal(recorder.Body.Bytes(), &body)
	testastic.NoError(t, err)

	body["state"] = "opened"
	body["merged_at"] = nil
	body["merge_commit_sha"] = nil
	body["squash_commit_sha"] = nil

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(recorder.Code)

	err = json.NewEncoder(w).Encode(body)
	testastic.NoError(t, err)
}
