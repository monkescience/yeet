package provider_test

import (
	"context"
	"encoding/json"
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
	providerContractLatestRelease                    providerContractScenario = "latest release"
	providerContractLatestFallbackTags               providerContractScenario = "latest fallback tags"
	providerContractListTags                         providerContractScenario = "list tags"
	providerContractGetCommitsSinceRefs              providerContractScenario = "get commits since refs"
	providerContractGetCommitsSinceRefsMissing       providerContractScenario = "get commits since refs missing"
	providerContractGetCommitsSinceRefsUnresolved    providerContractScenario = "get commits since refs unresolved"
	providerContractGetCommitsSinceRefsMultiBoundary providerContractScenario = "get commits since refs multi boundary"
	providerContractGetReleaseByTag                  providerContractScenario = "get release by tag"
	providerContractTagExists                        providerContractScenario = "tag exists"
	providerContractCreateReleasePR                  providerContractScenario = "create release pr"
	providerContractUpdateReleasePR                  providerContractScenario = "update release pr"
	providerContractFindOpenPRs                      providerContractScenario = "find open prs"
	providerContractFindMergedPR                     providerContractScenario = "find merged pr"
	providerContractMarkReleasePR                    providerContractScenario = "mark release pr"
	providerContractMergeReleasePR                   providerContractScenario = "merge release pr"
	providerContractCommitPRBody                     providerContractScenario = "commit pr body"
	providerContractCreateBranch                     providerContractScenario = "create branch"
	providerContractCreateRelease                    providerContractScenario = "create release"
	providerContractGetFile                          providerContractScenario = "get file"
	providerContractUpdateFiles                      providerContractScenario = "update files"
	providerContractMissingFile                      providerContractScenario = "missing file"
	providerContractMissingRelease                   providerContractScenario = "missing release"
	providerContractMissingPR                        providerContractScenario = "missing pr"
	providerContractBlockedMerge                     providerContractScenario = "blocked merge"
	providerContractUnsupportedMerge                 providerContractScenario = "unsupported merge"
	providerContractReleaseTitle                                              = "chore: release v1.2.3"
	providerContractReleaseBody                                               = "release body"
	providerContractReleaseBranch                                             = "release-main"
	providerContractPendingBranch                                             = "yeet/release-main"
	providerContractBaseBranch                                                = "main"
	providerContractTag                                                       = "v1.2.3"
	providerContractHeadSHA                                                   = "head-sha"
	providerContractMergeSHA                                                  = "merge-sha"
	providerContractMissingTag                                                = "v0.5.0"
	providerContractIntermediateTag                                           = "v1.4.0"
	providerContractMidSHA                                                    = "mid-sha"
	providerContractIntermediateSHA                                           = "inter-sha"
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

			t.Run("prefers latest release as version ref", func(t *testing.T) {
				t.Parallel()

				// given: a provider server with a latest release of v1.2.4 available
				server := httptest.NewServer(harness.handler(t, providerContractLatestRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetLatestVersionRef is invoked
				ref, err := p.GetLatestVersionRef(context.Background())

				// then: the release tag v1.2.4 is returned as the version ref
				testastic.NoError(t, err)
				testastic.Equal(t, "v1.2.4", ref)
			})

			t.Run("falls back to tags for version ref", func(t *testing.T) {
				t.Parallel()

				// given: a provider server with no releases but a tag list available
				server := httptest.NewServer(harness.handler(t, providerContractLatestFallbackTags))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetLatestVersionRef is invoked
				ref, err := p.GetLatestVersionRef(context.Background())

				// then: the latest tag is returned as the version ref
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, ref)
			})

			t.Run("lists tags", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a fixture of two tags
				server := httptest.NewServer(harness.handler(t, providerContractListTags))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: ListTags is invoked
				tags, err := p.ListTags(context.Background())

				// then: the tags are returned in the expected order
				testastic.NoError(t, err)
				testastic.SliceEqual(t, []string{providerContractTag, "v1.2.2"}, tags)
			})

			t.Run("gets commits since refs", func(t *testing.T) {
				t.Parallel()

				// given: a provider server with a commit history reachable from the contract tag
				server := httptest.NewServer(harness.handler(t, providerContractGetCommitsSinceRefs))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetCommitsSinceRefs is invoked for the contract tag against the base branch
				history, err := p.GetCommitsSinceRefs(
					context.Background(),
					[]string{providerContractTag},
					providerContractBaseBranch,
					true,
				)

				// then: the returned history contains the expected commit metadata and no missing refs
				testastic.NoError(t, err)
				testastic.Equal(t, 0, len(history.MissingRefs))

				entries := history.EntriesByRef[providerContractTag]
				testastic.Equal(t, 1, len(entries))
				testastic.Equal(t, providerContractHeadSHA, entries[0].Hash)
				testastic.Equal(t, "feat: add release flow", entries[0].Message)
				testastic.SliceEqual(t, []string{"CHANGELOG.md", "VERSION.txt"}, entries[0].Paths)
			})

			t.Run("reports missing refs alongside reachable ones in a multi-ref call", func(t *testing.T) {
				t.Parallel()

				// given: a provider server where the contract tag exists but the missing tag does not
				server := httptest.NewServer(harness.handler(t, providerContractGetCommitsSinceRefsMissing))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetCommitsSinceRefs is invoked for both the reachable and missing tags
				history, err := p.GetCommitsSinceRefs(
					context.Background(),
					[]string{providerContractTag, providerContractMissingTag},
					providerContractBaseBranch,
					true,
				)

				// then: the missing tag is reported in MissingRefs while the reachable tag yields its commit history
				testastic.NoError(t, err)
				testastic.SliceEqual(t, []string{providerContractMissingTag}, history.MissingRefs)

				entries := history.EntriesByRef[providerContractTag]
				testastic.Equal(t, 1, len(entries))
				testastic.Equal(t, providerContractHeadSHA, entries[0].Hash)

				_, missingEntriesPresent := history.EntriesByRef[providerContractMissingTag]
				testastic.False(t, missingEntriesPresent)
			})

			t.Run("includes intermediate boundary commits in older refs' histories", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a multi-boundary history with intermediate and older tags
				server := httptest.NewServer(harness.handler(t, providerContractGetCommitsSinceRefsMultiBoundary))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetCommitsSinceRefs is invoked for the intermediate and older tags
				history, err := p.GetCommitsSinceRefs(
					context.Background(),
					[]string{providerContractIntermediateTag, providerContractTag},
					providerContractBaseBranch,
					false,
				)

				// then: each ref receives the commits visible up to its respective boundary
				testastic.NoError(t, err)
				testastic.Equal(t, 0, len(history.MissingRefs))

				intermediateEntries := history.EntriesByRef[providerContractIntermediateTag]
				testastic.SliceEqual(
					t,
					[]string{providerContractHeadSHA},
					commitEntryHashes(intermediateEntries),
				)

				olderEntries := history.EntriesByRef[providerContractTag]
				testastic.SliceEqual(
					t,
					[]string{
						providerContractHeadSHA,
						providerContractMidSHA,
						providerContractIntermediateSHA,
					},
					commitEntryHashes(olderEntries),
				)
			})

			t.Run("reports unresolvable ref as missing without failing the batch", func(t *testing.T) {
				t.Parallel()

				// given: a provider server where the missing tag cannot be resolved by the API
				server := httptest.NewServer(harness.handler(t, providerContractGetCommitsSinceRefsUnresolved))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetCommitsSinceRefs is invoked for the resolvable and unresolvable tags together
				history, err := p.GetCommitsSinceRefs(
					context.Background(),
					[]string{providerContractTag, providerContractMissingTag},
					providerContractBaseBranch,
					true,
				)

				// then: the unresolvable tag is reported as missing while the batch still succeeds for the other tag
				testastic.NoError(t, err)
				testastic.SliceEqual(t, []string{providerContractMissingTag}, history.MissingRefs)

				entries := history.EntriesByRef[providerContractTag]
				testastic.Equal(t, 1, len(entries))
				testastic.Equal(t, providerContractHeadSHA, entries[0].Hash)
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

			t.Run("reports tag existence", func(t *testing.T) {
				t.Parallel()

				// given: a provider server confirming the contract tag exists
				server := httptest.NewServer(harness.handler(t, providerContractTagExists))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: TagExists is invoked for the contract tag
				exists, err := p.TagExists(context.Background(), providerContractTag)

				// then: the provider reports the tag as existing
				testastic.NoError(t, err)
				testastic.True(t, exists)
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

			t.Run("updates release pull request", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting updates to an existing release pull request
				server := httptest.NewServer(harness.handler(t, providerContractUpdateReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: UpdateReleasePR is invoked with a new body for PR 42
				err := p.UpdateReleasePR(context.Background(), 42, provider.ReleasePROptions{
					Title: providerContractReleaseTitle,
					Body:  "updated release body",
				})

				// then: the update completes without error
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

			t.Run("finds commit pull request body", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning an overridden PR body for the merge commit SHA
				server := httptest.NewServer(harness.handler(t, providerContractCommitPRBody))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CommitPullRequestBody is invoked for the merge commit SHA
				body, found, err := p.CommitPullRequestBody(context.Background(), providerContractMergeSHA)

				// then: the overridden body is returned and the found flag is true
				testastic.NoError(t, err)
				testastic.True(t, found)
				testastic.Equal(t, "override body", body)
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
					map[string]string{
						"CHANGELOG.md": "# Changelog\n",
						"VERSION.txt":  "version=1.2.3\n",
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

func commitEntryHashes(entries []provider.CommitEntry) []string {
	hashes := make([]string, 0, len(entries))

	for _, entry := range entries {
		hashes = append(hashes, entry.Hash)
	}

	return hashes
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
