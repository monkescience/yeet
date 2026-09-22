package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseGitHubAutoMergeGraphQLFailure(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"refused", "null_data"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			// given: GitHub refuses scheduling or returns no GraphQL data
			repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
			providerServer := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0",
				BoundarySHA: shas[0], BranchHeadSHA: shas[1], ForbidPublication: true,
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/graphql") {
					body, err := io.ReadAll(r.Body)
					testastic.NoError(t, err)
					err = r.Body.Close()
					testastic.NoError(t, err)

					r.Body = io.NopCloser(strings.NewReader(string(body)))

					if strings.Contains(string(body), "enablePullRequestAutoMerge") {
						w.Header().Set("Content-Type", "application/json")
						_, err = w.Write([]byte(readTestFile(t,
							"testdata/release/github_auto_merge_"+scenario+"/response.json")))
						testastic.NoError(t, err)

						return
					}
				}

				providerServer.Config.Handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			configPath := providerAutoMergeConfig(t, "github")

			// when: the CLI requests provider-managed auto-merge
			result := binary.RunWithOptions(t,
				[]string{"release", "--auto-merge", "--config", configPath},
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
			)

			// then: the CLI reports a provider failure without publishing
			testastic.Equal(t, 1, result.ExitCode)
			testastic.AssertFile(t,
				"testdata/release/github_auto_merge_"+scenario+"/stderr.expected.txt",
				result.Stderr,
			)
		})
	}
}

func TestReleaseGitLabAutoMergeNullResponse(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			// given: GitLab returns null when fetching or accepting the reconciled merge request
			repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
			providerServer := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
				Project: "group/service", LatestTag: "v1.0.0",
				BoundarySHA: shas[0], BranchHeadSHA: shas[1], ForbidPublication: true,
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == method && strings.Contains(r.URL.Path, "/merge_requests/42") {
					w.Header().Set("Content-Type", "application/json")
					_, err := w.Write([]byte("null"))
					testastic.NoError(t, err)

					return
				}

				providerServer.Config.Handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			configPath := providerAutoMergeConfig(t, "gitlab")

			// when: the CLI requests provider-managed auto-merge
			result := binary.RunWithOptions(t,
				[]string{"release", "--auto-merge", "--config", configPath},
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
			)

			// then: the CLI reports the missing response without panicking or publishing
			testastic.Equal(t, 1, result.ExitCode)
			testastic.AssertFile(t,
				"testdata/release/gitlab_auto_merge_null_"+strings.ToLower(method)+"/stderr.expected.txt",
				result.Stderr,
			)
		})
	}
}

func TestReleaseProviderAutoMerge(t *testing.T) {
	t.Parallel()

	t.Run("github schedules while checks are pending without publishing", func(t *testing.T) {
		t.Parallel()

		// given: a releasable GitHub project whose release PR is waiting on checks
		repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                     "testorg",
			Repo:                      "testrepo",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			PendingChecks:             true,
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 1,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfigEnabled(t, "github", "direct")

		// when: the CLI overrides configured direct mode with provider mode
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge-mode", "provider", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: GitHub accepts scheduling and yeet reports no completed publication
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
		testastic.AssertFile(
			t,
			"testdata/release/github_provider_auto_merge_pending/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github enrolls a ready pull request in the required merge queue", func(t *testing.T) {
		t.Parallel()

		// given: a releasable GitHub project with a required merge queue
		repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                      "testorg",
			Repo:                       "testrepo",
			LatestTag:                  "v1.0.0",
			BoundarySHA:                shas[0],
			BranchHeadSHA:              shas[1],
			MergeQueue:                 true,
			ExpectedAutoMergeOperation: "enqueuePullRequest",
			AssertAutoMergeRequests:    true,
			ExpectedAutoMergeRequests:  1,
			ForbidPublication:          true,
		})
		configPath := providerAutoMergeConfig(t, "github")

		// when: invoking provider auto-merge for the ready pull request
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: GitHub accepts queue enrollment without publishing
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_provider_auto_merge_pending/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github rejects a method that conflicts with the required queue", func(t *testing.T) {
		t.Parallel()

		// given: a required squash queue and an explicit rebase request
		repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                      "testorg",
			Repo:                       "testrepo",
			LatestTag:                  "v1.0.0",
			BoundarySHA:                shas[0],
			BranchHeadSHA:              shas[1],
			MergeQueue:                 true,
			ExpectedAutoMergeOperation: "enqueuePullRequest",
			AssertAutoMergeRequests:    true,
			ExpectedAutoMergeRequests:  0,
			ForbidPublication:          true,
		})
		configPath := providerAutoMergeConfig(t, "github")

		// when: invoking provider auto-merge with an incompatible method for the ready pull request
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--auto-merge-method", "rebase", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the CLI reports the conflict without scheduling or publishing
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_provider_auto_merge_queue_conflict/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github schedules pending checks for the required merge queue", func(t *testing.T) {
		t.Parallel()

		// given: a required GitHub merge queue whose pull request is waiting on checks
		repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                      "testorg",
			Repo:                       "testrepo",
			LatestTag:                  "v1.0.0",
			BoundarySHA:                shas[0],
			BranchHeadSHA:              shas[1],
			PendingChecks:              true,
			MergeQueue:                 true,
			ExpectedAutoMergeOperation: "enablePullRequestAutoMerge",
			ExpectedAutoMergeMethod:    "SQUASH",
			AssertAutoMergeRequests:    true,
			ExpectedAutoMergeRequests:  1,
			ForbidPublication:          true,
		})
		configPath := providerAutoMergeConfig(t, "github")

		// when: invoking provider auto-merge before the queue accepts enrollment
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: GitHub accepts native scheduling without publishing
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_provider_auto_merge_pending/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab schedules while checks are pending without publishing", func(t *testing.T) {
		t.Parallel()

		// given: a releasable GitLab project whose release MR is waiting on checks
		repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			PendingChecks:             true,
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 1,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "gitlab")

		// when: invoking auto-merge with its default provider mode
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: GitLab accepts scheduling and yeet reports no completed publication
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_provider_auto_merge_pending/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab immediate merge response does not publish", func(t *testing.T) {
		t.Parallel()

		// given: GitLab immediately merges an accepted auto-merge request
		repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			AutoMergeImmediate:        true,
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 1,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "gitlab")

		// when: invoking provider auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet reports scheduling without creating tags or releases
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_provider_auto_merge_pending/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab enrolls the merge request in the required train", func(t *testing.T) {
		t.Parallel()

		// given: a releasable GitLab project with merge trains enabled
		repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			MergeTrain:                true,
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 1,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "gitlab")

		// when: invoking provider auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: GitLab accepts train enrollment without publishing
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_provider_auto_merge_pending/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("immediate provider merge is published by a later run", func(t *testing.T) {
		t.Parallel()

		// given: GitHub immediately merges an accepted scheduling request
		repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
		schedulingServer := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                     "testorg",
			Repo:                      "testrepo",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			AutoMergeImmediate:        true,
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 1,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "github")

		// when: the scheduling run asks GitHub to auto-merge
		schedulingResult := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(schedulingServer, "main")...),
		)

		// then: the scheduling run exits without publishing
		testastic.Equal(t, 0, schedulingResult.ExitCode)

		// given: the next base-branch run sees that merged pending release
		finalizationServer := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                     "testorg",
			Repo:                      "testrepo",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			MergedPendingRelease:      true,
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 0,
			AssertPublication:         true,
		})

		// when: the later run invokes the normal release flow
		finalizationResult := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(finalizationServer, "main")...),
		)

		// then: the later run finalizes the provider-merged release
		testastic.Equal(t, 0, finalizationResult.ExitCode)
	})

	t.Run("false override leaves existing scheduling unchanged", func(t *testing.T) {
		t.Parallel()

		// given: an open GitHub release PR with provider auto-merge already enabled
		repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                     "testorg",
			Repo:                      "testrepo",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			ExistingOpenReleasePRBody: providerAutoMergeManifest(),
			AutoMergeAlreadyEnabled:   true,
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 0,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfigEnabled(t, "github", "provider")

		// when: the invocation explicitly disables auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge=false", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: reconciliation succeeds without a scheduling or cancellation request
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("selecting provider mode does not enable auto-merge", func(t *testing.T) {
		t.Parallel()

		// given: a releasable GitHub project with auto-merge disabled
		repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                     "testorg",
			Repo:                      "testrepo",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 0,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "github")

		// when: selecting provider mode without enabling auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge-mode", "provider", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet reconciles the release PR without scheduling it
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("repeated github run validates existing scheduling", func(t *testing.T) {
		t.Parallel()

		for _, scenario := range []struct {
			name         string
			queue        bool
			method       string
			wantExitCode int
		}{
			{name: "without a queue", method: "auto"},
			{name: "matching queue method", queue: true, method: "rebase"},
			{name: "conflicting queue method", queue: true, method: "merge", wantExitCode: 1},
		} {
			t.Run(scenario.name, func(t *testing.T) {
				t.Parallel()

				// given: an open GitHub release PR with auto-merge enabled and an optional rebase queue
				repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
				server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
					Owner:                     "testorg",
					Repo:                      "testrepo",
					LatestTag:                 "v1.0.0",
					BoundarySHA:               shas[0],
					BranchHeadSHA:             shas[1],
					ExistingOpenReleasePRBody: providerAutoMergeManifest(),
					AutoMergeAlreadyEnabled:   true,
					MergeQueue:                scenario.queue,
					MergeQueueMethod:          "REBASE",
					AssertAutoMergeRequests:   true,
					ExpectedAutoMergeRequests: 0,
					ForbidPublication:         true,
				})
				configPath := providerAutoMergeConfig(t, "github")

				// when: provider auto-merge runs again with the requested method
				result := binary.RunWithOptions(t,
					[]string{"release", "--auto-merge", "--auto-merge-method", scenario.method, "--config", configPath},
					testastic.WithRunWorkDir(repoDir),
					testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
				)

				// then: existing scheduling is preserved and incompatible queue methods are refused
				testastic.Equal(t, scenario.wantExitCode, result.ExitCode)

				if scenario.wantExitCode != 0 {
					testastic.AssertFile(
						t,
						"testdata/release/github_provider_auto_merge_existing_queue_conflict/stderr.expected.txt",
						result.Stderr,
					)
				}
			})
		}
	})

	t.Run("repeated gitlab run accepts existing automatic scheduling", func(t *testing.T) {
		t.Parallel()

		// given: an open GitLab release MR with auto-merge already enabled
		repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			ExistingOpenReleasePRBody: providerAutoMergeManifest(),
			AutoMergeAlreadyEnabled:   true,
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 0,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "gitlab")

		// when: provider auto-merge runs again without choosing a method
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: the existing request is accepted without duplicate scheduling
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("repeated gitlab run validates existing explicit scheduling", func(t *testing.T) {
		t.Parallel()

		for _, scenario := range []struct {
			name           string
			squash         bool
			method         string
			wantExitCode   int
			stderrExpected string
		}{
			{name: "matching squash", squash: true, method: "squash"},
			{name: "matching merge", method: "merge"},
			{
				name:           "stored method conflict",
				squash:         true,
				method:         "merge",
				wantExitCode:   1,
				stderrExpected: "gitlab_provider_auto_merge_existing_method_conflict",
			},
			{
				name:           "project strategy conflict",
				method:         "rebase",
				wantExitCode:   1,
				stderrExpected: "gitlab_provider_auto_merge_existing_strategy_conflict",
			},
		} {
			t.Run(scenario.name, func(t *testing.T) {
				t.Parallel()

				// given: an open GitLab release MR with an existing auto-merge method
				repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
				server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
					Project:                   "group/service",
					LatestTag:                 "v1.0.0",
					BoundarySHA:               shas[0],
					BranchHeadSHA:             shas[1],
					ExistingOpenReleasePRBody: providerAutoMergeManifest(),
					AutoMergeAlreadyEnabled:   true,
					AutoMergeSquash:           scenario.squash,
					AssertAutoMergeRequests:   true,
					ExpectedAutoMergeRequests: 0,
					ForbidPublication:         true,
				})
				configPath := providerAutoMergeConfig(t, "gitlab")

				// when: provider auto-merge requests the selected explicit method
				result := binary.RunWithOptions(t,
					[]string{"release", "--auto-merge", "--auto-merge-method", scenario.method, "--config", configPath},
					testastic.WithRunWorkDir(repoDir),
					testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
				)

				// then: yeet preserves the request or reports its specific incompatibility
				testastic.Equal(t, scenario.wantExitCode, result.ExitCode)

				if scenario.stderrExpected != "" {
					testastic.AssertFile(
						t,
						"testdata/release/"+scenario.stderrExpected+"/stderr.expected.txt",
						result.Stderr,
					)
				}
			})
		}
	})

	t.Run("github re-enables scheduling canceled by refresh", func(t *testing.T) {
		t.Parallel()

		// given: GitHub cancels existing auto-merge while yeet refreshes the release PR
		repoDir, shas := providerAutoMergeRepo(t, "https://github.com/testorg/testrepo.git")
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                         "testorg",
			Repo:                          "testrepo",
			LatestTag:                     "v1.0.0",
			BoundarySHA:                   shas[0],
			BranchHeadSHA:                 shas[1],
			ExistingOpenReleasePRBody:     providerAutoMergeManifest(),
			AutoMergeAlreadyEnabled:       true,
			AutoMergeCanceledAfterRefresh: true,
			AssertAutoMergeRequests:       true,
			ExpectedAutoMergeRequests:     1,
			ForbidPublication:             true,
		})
		configPath := providerAutoMergeConfig(t, "github")

		// when: provider auto-merge reconciles the existing pull request
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet asks GitHub to enable auto-merge again
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab success response without acceptance is refused", func(t *testing.T) {
		t.Parallel()

		// given: GitLab returns HTTP 200 with a merge error and no enabled auto-merge
		repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			AutoMergeResponseError:    "project rules refused auto-merge",
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 1,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "gitlab")

		// when: invoking provider auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet reports the provider refusal without publishing
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_provider_auto_merge_refused/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab refusal reason reaches the debug record", func(t *testing.T) {
		t.Parallel()

		// given: GitLab refuses auto-merge with a reason only its merge_error field carries
		repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			AutoMergeResponseError:    "project rules refused auto-merge",
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 1,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "gitlab")

		// when: raising the log level to debug
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--no-color", "--verbose", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: the provider reason appears in the debug detail, never in the error line
		testastic.Equal(t, 1, result.ExitCode)

		stderr := ansi.Strip(result.Stderr)
		testastic.Contains(t, stderr, `provider_message="project rules refused auto-merge"`)
		testastic.NotContains(t, withoutDebugDiagnostics(stderr), "project rules refused auto-merge")
	})

	t.Run("gitlab rejects versions without native auto-merge", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab version older than the native auto-merge API
		repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			Version:                   "17.10.9",
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 0,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "gitlab")

		// when: invoking provider auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet requires explicit direct mode without attempting scheduling
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_provider_auto_merge_unsupported_version/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab rejects an unreadable server version", func(t *testing.T) {
		t.Parallel()

		// given: GitLab returns an unreadable server version
		repoDir, shas := providerAutoMergeRepo(t, "https://gitlab.com/group/service.git")
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			Version:                   "unknown",
			AssertAutoMergeRequests:   true,
			ExpectedAutoMergeRequests: 0,
			ForbidPublication:         true,
		})
		configPath := providerAutoMergeConfig(t, "gitlab")

		// when: invoking provider auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet reports unavailable version information without attempting scheduling
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_provider_auto_merge_invalid_version/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("azuredevops requires explicit direct mode", func(t *testing.T) {
		t.Parallel()

		// given: a releasable Azure DevOps project with auto-merge enabled
		repoDir, shas := providerAutoMergeRepo(t, "https://dev.azure.com/contoso/platform/_git/yeet")
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})
		configPath := providerAutoMergeConfig(t, "azuredevops")

		// when: invoking auto-merge without selecting direct mode
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet rejects unsupported provider scheduling instead of merging directly
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/azuredevops_provider_auto_merge_unsupported/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("independent units preserve accepted scheduling after a refusal", func(t *testing.T) {
		t.Parallel()

		// given: a short token and two independent releases where the first scheduling request is refused
		repoDir, shas := writeIndependentMonorepoHistory(t)
		opts := independentGitHubOptions(shas)
		opts.Files = map[string]string{
			"api/CHANGELOG.md": "api release\n",
			"web/CHANGELOG.md": "web release\n",
		}
		opts.ExpectedCreatedPullRequests = []fakeprovider.GitHubPullRequestExpectation{
			{Title: "chore: release 1.1.0", Head: "yeet/release-main-target-api-21f150df25", Base: "main"},
			{Title: "chore: release 2.0.1", Head: "yeet/release-main-target-web-1355f0b5d0", Base: "main"},
		}
		opts.AutoMergeStatusCodes = []int{http.StatusUnprocessableEntity, 0}
		opts.AssertAutoMergeRequests = true
		opts.ExpectedAutoMergeRequests = 2
		opts.ForbidPublication = true
		server := fakeprovider.NewGitHub(t, opts)
		configPath := absoluteTestFile(t, "testdata/release/independent_create/input.yaml")

		// when: invoking provider auto-merge for both independent units
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(append(fixture.GitHubEnv(server, "main"), "GITHUB_TOKEN=e")...),
		)

		// then: the later unit remains scheduled while the command reports the first refusal
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/independent_provider_auto_merge_partial_failure/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func providerAutoMergeRepo(t *testing.T, remoteURL string) (string, []string) {
	t.Helper()

	return fixture.WriteRepoWithHistory(t, remoteURL, "main", []fixture.RepoCommit{
		{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
		{
			Message: "feat: add a thing",
			Files: map[string]string{
				"CHANGELOG.md": "## Changelog\n\n## [v1.1.0]\n\n* feat: add a thing\n",
			},
		},
	})
}

func providerAutoMergeConfig(t *testing.T, providerName string) string {
	t.Helper()

	opts := fixture.ConfigOptions{Provider: providerName, Branch: "main"}
	switch providerName {
	case "github":
		opts.Host = "github.com"
		opts.Owner = "testorg"
		opts.Repo = "testrepo"
	case "gitlab":
		opts.Host = "gitlab.com"
		opts.Project = "group/service"
	case "azuredevops":
		opts.Host = "dev.azure.com"
		opts.Organization = "contoso"
		opts.Project = "platform"
		opts.Repo = "yeet"
	}

	return fixture.WriteConfig(t, opts)
}

func providerAutoMergeConfigEnabled(t *testing.T, providerName, mode string) string {
	t.Helper()

	opts := fixture.ConfigOptions{
		Provider: providerName, Branch: "main", AutoMerge: new(true), AutoMergeMode: mode,
	}
	if providerName == "github" {
		opts.Host = "github.com"
		opts.Owner = "testorg"
		opts.Repo = "testrepo"
	}

	return fixture.WriteConfig(t, opts)
}

func providerAutoMergeManifest() string {
	return "## ٩(^ᴗ^)۶ release created\n\n" +
		"<!-- yeet-release-manifest\n" +
		`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}` +
		"\n-->\n"
}
