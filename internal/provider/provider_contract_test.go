package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
)

type providerContractHarness struct {
	name                 string
	newProvider          func(t *testing.T, server *httptest.Server) provider.Provider
	handler              func(t *testing.T, scenario providerContractScenario) http.Handler
	expectedRepoURL      func(serverURL string) string
	expectedReleasePRURL func(serverURL string) string
	expectedReleaseURL   func(serverURL string) string
	expectedPathPrefix   string
}

type providerContractScenario string

const (
	providerContractLatestRelease            providerContractScenario = "latest release"
	providerContractListTags                 providerContractScenario = "list tags"
	providerContractBranchHead               providerContractScenario = "branch head"
	providerContractBranchHeadMissing        providerContractScenario = "branch head missing"
	providerContractGetReleaseByTag          providerContractScenario = "get release by tag"
	providerContractCreateReleasePR          providerContractScenario = "create release pr"
	providerContractCreateReleasePRReviewers providerContractScenario = "create release pr reviewers"
	providerContractUnknownReviewer          providerContractScenario = "unknown reviewer"
	providerContractUpdateReleasePR          providerContractScenario = "update release pr"
	providerContractFindOpenPRs              providerContractScenario = "find open prs"
	providerContractFindMergedPR             providerContractScenario = "find merged pr"
	providerContractMarkReleasePR            providerContractScenario = "mark release pr"
	providerContractMergeReleasePR           providerContractScenario = "merge release pr"
	providerContractCreateBranch             providerContractScenario = "create branch"
	providerContractCreateRelease            providerContractScenario = "create release"
	providerContractGetFile                  providerContractScenario = "get file"
	providerContractUpdateFiles              providerContractScenario = "update files"
	providerContractMissingFile              providerContractScenario = "missing file"
	providerContractMissingRelease           providerContractScenario = "missing release"
	providerContractMissingPR                providerContractScenario = "missing pr"
	providerContractBlockedMerge             providerContractScenario = "blocked merge"
	providerContractUnsupportedMerge         providerContractScenario = "unsupported merge"
	providerContractReleaseTitle                                      = "chore: release v1.2.3"
	providerContractReleaseBody                                       = "release body"
	providerContractReleaseBranch                                     = "release-main"
	providerContractPendingBranch                                     = "yeet/release-main"
	providerContractBaseBranch                                        = "main"
	providerContractTag                                               = "v1.2.3"
	providerContractTagCommitSHA                                      = "tag-commit-123"
	providerContractHeadSHA                                           = "head-sha"
	providerContractMergeSHA                                          = "merge-sha"
	providerContractReviewerAlice                                     = "alice"
	providerContractReviewerBob                                       = "bob"
	providerContractUnknownReviewerName                               = "ghost"
)

func TestProviderContract(t *testing.T) {
	t.Parallel()

	for _, harness := range providerContractHarnesses() {
		t.Run(harness.name, func(t *testing.T) {
			t.Parallel()

			// given: the current provider harness defining server fixtures and provider construction
			// when: each contract scenario subtest exercises a provider method
			// then: every scenario satisfies the shared provider contract for this harness

			t.Run("exposes repository metadata", func(t *testing.T) {
				t.Parallel()

				// given: a provider server serving the latest release scenario
				server := httptest.NewServer(harness.handler(t, providerContractLatestRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: the repository URL and path prefix are read from the provider
				// then: the values match the harness expectations
				testastic.Equal(t, harness.expectedRepoURL(server.URL), p.RepoURL())
				testastic.Equal(t, harness.expectedPathPrefix, p.PathPrefix())
			})

			t.Run("lists tag commit targets", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning tags with their peeled commit targets
				server := httptest.NewServer(harness.handler(t, providerContractListTags))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: tag refs are requested
				refs, err := p.ListTagRefs(context.Background())

				// then: names and commit targets are preserved in provider order
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, refs[0].Name)
				testastic.Equal(t, providerContractTagCommitSHA, refs[0].CommitSHA)
			})

			t.Run("resolves branch head commit", func(t *testing.T) {
				t.Parallel()

				// given: a provider server exposing the base branch head
				server := httptest.NewServer(harness.handler(t, providerContractBranchHead))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetBranchHead is invoked for the base branch
				head, err := p.GetBranchHead(context.Background(), providerContractBaseBranch)

				// then: the branch head commit SHA is returned
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractHeadSHA, head)
			})

			t.Run("reports missing branch as ref not found", func(t *testing.T) {
				t.Parallel()

				// given: a provider server without the requested branch
				server := httptest.NewServer(harness.handler(t, providerContractBranchHeadMissing))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetBranchHead is invoked for a branch that does not exist
				_, err := p.GetBranchHead(context.Background(), "missing-branch")

				// then: the sentinel ref-not-found error is surfaced
				testastic.Error(t, err)
				testastic.True(t, errors.Is(err, provider.ErrRefNotFound))
			})

			t.Run("gets release by tag", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a release for the contract tag
				server := httptest.NewServer(harness.handler(t, providerContractGetReleaseByTag))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetReleaseByTag is invoked for the contract tag
				release, err := p.GetReleaseByTag(context.Background(), providerContractTag)

				// then: the release metadata matches the harness expectations
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, release.TagName)
				testastic.Equal(t, "release notes", release.Body)
				testastic.Equal(t, harness.expectedReleaseURL(server.URL), release.URL)
			})

			t.Run("creates release pull request", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting a new release pull request for the release branch
				server := httptest.NewServer(harness.handler(t, providerContractCreateReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateReleasePR is invoked with the contract title, body, and branches
				pr, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
					Title:         providerContractReleaseTitle,
					Body:          providerContractReleaseBody,
					BaseBranch:    providerContractBaseBranch,
					ReleaseBranch: providerContractReleaseBranch,
				})

				// then: the created pull request reflects the supplied options and the harness URL
				testastic.NoError(t, err)
				testastic.Equal(t, 42, pr.Number)
				testastic.Equal(t, providerContractReleaseTitle, pr.Title)
				testastic.Equal(t, providerContractReleaseBody, pr.Body)
				testastic.Equal(t, providerContractReleaseBranch, pr.Branch)
				testastic.Equal(t, harness.expectedReleasePRURL(server.URL), pr.URL)
			})

			t.Run("creates release pull request with reviewers", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting a new release pull request and reviewer assignment
				server := httptest.NewServer(harness.handler(t, providerContractCreateReleasePRReviewers))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateReleasePR is invoked with two reviewers
				pr, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
					Title:         providerContractReleaseTitle,
					Body:          providerContractReleaseBody,
					BaseBranch:    providerContractBaseBranch,
					ReleaseBranch: providerContractReleaseBranch,
					Reviewers:     []string{providerContractReviewerAlice, providerContractReviewerBob},
				})

				// then: the pull request is created and the reviewer requests reach the server
				testastic.NoError(t, err)
				testastic.Equal(t, 42, pr.Number)
			})

			t.Run("fails when a reviewer cannot be assigned", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that cannot resolve or assign the requested reviewer
				server := httptest.NewServer(harness.handler(t, providerContractUnknownReviewer))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateReleasePR is invoked with an unknown reviewer
				_, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
					Title:         providerContractReleaseTitle,
					Body:          providerContractReleaseBody,
					BaseBranch:    providerContractBaseBranch,
					ReleaseBranch: providerContractReleaseBranch,
					Reviewers:     []string{providerContractUnknownReviewerName},
				})

				// then: the error names the reviewer and wraps the not-found sentinel
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrReviewerNotFound)
				testastic.ErrorContains(t, err, providerContractUnknownReviewerName)
			})

			t.Run("updates release pull request", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting updates to an existing release pull request
				server := httptest.NewServer(harness.handler(t, providerContractUpdateReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: UpdateReleasePR is invoked with a new body and reviewers for PR 42
				err := p.UpdateReleasePR(context.Background(), 42, provider.ReleasePROptions{
					Title:     providerContractReleaseTitle,
					Body:      "updated release body",
					Reviewers: []string{providerContractReviewerAlice},
				})

				// then: the update completes without touching reviewers
				testastic.NoError(t, err)
			})

			t.Run("finds open pending release pull requests", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a single open pending release PR targeting the base branch
				server := httptest.NewServer(harness.handler(t, providerContractFindOpenPRs))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: FindOpenPendingReleasePRs is invoked for the base branch
				prs, err := p.FindOpenPendingReleasePRs(context.Background(), providerContractBaseBranch)

				// then: a single PR is returned with the expected number and pending branch
				testastic.NoError(t, err)
				testastic.Equal(t, 1, len(prs))
				testastic.Equal(t, 42, prs[0].Number)
				testastic.Equal(t, providerContractPendingBranch, prs[0].Branch)
			})

			t.Run("finds merged release pull request", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a recently merged release PR for the base branch
				server := httptest.NewServer(harness.handler(t, providerContractFindMergedPR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: FindMergedReleasePR is invoked for the base branch
				pr, err := p.FindMergedReleasePR(context.Background(), providerContractBaseBranch)

				// then: the merged PR is returned with the expected number, branch, merge SHA, and body
				testastic.NoError(t, err)
				testastic.Equal(t, 42, pr.Number)
				testastic.Equal(t, providerContractPendingBranch, pr.Branch)
				testastic.Equal(t, providerContractMergeSHA, pr.MergeCommitSHA)
				testastic.Equal(t, providerContractReleaseBody, pr.Body)
			})

			t.Run("marks release pull request state", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting label transitions on PR 42
				server := httptest.NewServer(harness.handler(t, providerContractMarkReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: MarkReleasePRPending and MarkReleasePRTagged are invoked in sequence on PR 42
				err := p.MarkReleasePRPending(context.Background(), 42)
				testastic.NoError(t, err)

				err = p.MarkReleasePRTagged(context.Background(), 42)

				// then: both label transitions succeed
				testastic.NoError(t, err)
			})

			t.Run("merges release pull request", func(t *testing.T) {
				t.Parallel()

				// given: a provider server reporting PR 42 as ready to merge
				server := httptest.NewServer(harness.handler(t, providerContractMergeReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: MergeReleasePR is invoked with the auto merge method on PR 42
				err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
					Method: provider.MergeMethodAuto,
				})

				// then: the merge completes without error
				testastic.NoError(t, err)
			})

			t.Run("creates branch", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting a new branch off the base branch
				server := httptest.NewServer(harness.handler(t, providerContractCreateBranch))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateBranch is invoked for the release branch with the base branch as source
				err := p.CreateBranch(context.Background(), providerContractReleaseBranch, providerContractBaseBranch)

				// then: the branch is created without error
				testastic.NoError(t, err)
			})

			t.Run("creates release", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting a new prerelease against the base branch
				server := httptest.NewServer(harness.handler(t, providerContractCreateRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateRelease is invoked with the contract tag and release notes
				release, err := p.CreateRelease(context.Background(), provider.ReleaseOptions{
					TagName:    providerContractTag,
					Ref:        providerContractBaseBranch,
					Name:       providerContractTag,
					Body:       "release notes",
					Prerelease: true,
				})

				// then: the returned release matches the requested tag, body, and harness URL
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, release.TagName)
				testastic.Equal(t, "release notes", release.Body)
				testastic.Equal(t, harness.expectedReleaseURL(server.URL), release.URL)
			})

			t.Run("reads file content", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a CHANGELOG.md file on the base branch
				server := httptest.NewServer(harness.handler(t, providerContractGetFile))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetFile is invoked for CHANGELOG.md on the base branch
				content, err := p.GetFile(context.Background(), providerContractBaseBranch, "CHANGELOG.md")

				// then: the file content matches the fixture
				testastic.NoError(t, err)
				testastic.Equal(t, "# Changelog\n", content)
			})

			t.Run("updates release files", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting file updates on the release branch
				server := httptest.NewServer(harness.handler(t, providerContractUpdateFiles))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: UpdateFiles is invoked with CHANGELOG.md and VERSION.txt against the release branch
				err := p.UpdateFiles(
					context.Background(),
					providerContractReleaseBranch,
					providerContractBaseBranch,
					map[string]provider.FileUpdate{
						"CHANGELOG.md": {Content: "# Changelog\n", Exists: true},
						"VERSION.txt":  {Content: "version=1.2.3\n"},
					},
					"chore: release v1.2.3",
				)

				// then: the file updates are committed without error
				testastic.NoError(t, err)
			})

			t.Run("returns file not found error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that reports MISSING.md as not found on the base branch
				server := httptest.NewServer(harness.handler(t, providerContractMissingFile))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetFile is invoked for MISSING.md on the base branch
				_, err := p.GetFile(context.Background(), providerContractBaseBranch, "MISSING.md")

				// then: ErrFileNotFound is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrFileNotFound)
			})

			t.Run("returns release not found error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that reports no release for the contract tag
				server := httptest.NewServer(harness.handler(t, providerContractMissingRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetReleaseByTag is invoked for the contract tag
				_, err := p.GetReleaseByTag(context.Background(), providerContractTag)

				// then: ErrNoRelease is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrNoRelease)
			})

			t.Run("returns release pull request not found error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that reports no merged release PR on the base branch
				server := httptest.NewServer(harness.handler(t, providerContractMissingPR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: FindMergedReleasePR is invoked for the base branch
				_, err := p.FindMergedReleasePR(context.Background(), providerContractBaseBranch)

				// then: ErrNoPR is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrNoPR)
			})

			t.Run("returns blocked merge error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server reporting PR 42 as not ready to merge
				server := httptest.NewServer(harness.handler(t, providerContractBlockedMerge))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: MergeReleasePR is invoked without the force option on PR 42
				err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{})

				// then: ErrMergeBlocked is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
			})

			t.Run("returns unsupported merge method error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server prepared for a merge attempt with an unsupported method
				server := httptest.NewServer(harness.handler(t, providerContractUnsupportedMerge))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: MergeReleasePR is invoked with the unsupported "octopus" merge method
				err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
					Method: provider.MergeMethod("octopus"),
				})

				// then: ErrMergeMethodUnsupported is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrMergeMethodUnsupported)
			})
		})
	}
}

func providerContractHarnesses() []providerContractHarness {
	return []providerContractHarness{
		{
			name:                 "github",
			newProvider:          newGitHubContractProvider,
			handler:              newGitHubContractHandler,
			expectedRepoURL:      func(serverURL string) string { return serverURL + "/o/r" },
			expectedReleasePRURL: func(_ string) string { return "https://example.com/pulls/42" },
			expectedReleaseURL:   func(_ string) string { return "https://example.com/releases/v1.2.3" },
			expectedPathPrefix:   "",
		},
		{
			name:                 "gitlab",
			newProvider:          newGitLabContractProvider,
			handler:              newGitLabContractHandler,
			expectedRepoURL:      func(serverURL string) string { return serverURL + "/o/r" },
			expectedReleasePRURL: func(_ string) string { return "https://example.com/pulls/42" },
			expectedReleaseURL:   func(_ string) string { return "https://example.com/releases/v1.2.3" },
			expectedPathPrefix:   "/-",
		},
		{
			name:                 "azuredevops",
			newProvider:          newAzureDevOpsContractProvider,
			handler:              newAzureDevOpsContractHandler,
			expectedRepoURL:      azureDevOpsContractExpectedRepoURL,
			expectedReleasePRURL: func(s string) string { return azureDevOpsContractExpectedRepoURL(s) + "/pullrequest/42" },
			expectedReleaseURL: func(s string) string {
				return azureDevOpsContractExpectedRepoURL(s) + "?version=GT" + providerContractTag
			},
			expectedPathPrefix: "",
		},
	}
}

func decodeJSONRequest(t *testing.T, r *http.Request, value any) {
	t.Helper()

	err := json.NewDecoder(r.Body).Decode(value)
	testastic.NoError(t, err)
}

func writeJSONFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	writeFixture(t, w, name)
}

func writeTextFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writeFixture(t, w, name)
}

func writeFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	testastic.NoError(t, err)

	_, err = w.Write(data)
	testastic.NoError(t, err)
}

func fatalUnexpectedProviderRequest(t *testing.T, providerName string, r *http.Request) {
	t.Helper()

	t.Fatalf("unexpected %s request: %s %s", providerName, r.Method, r.URL.String())
}
