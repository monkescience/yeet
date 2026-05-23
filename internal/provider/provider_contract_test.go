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

			t.Run("exposes repository metadata", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractLatestRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				testastic.Equal(t, harness.expectedRepoURL(server.URL), p.RepoURL())
				testastic.Equal(t, harness.expectedPathPrefix, p.PathPrefix())
			})

			t.Run("prefers latest release as version ref", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractLatestRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				ref, err := p.GetLatestVersionRef(context.Background())

				testastic.NoError(t, err)
				testastic.Equal(t, "v1.2.4", ref)
			})

			t.Run("falls back to tags for version ref", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractLatestFallbackTags))
				defer server.Close()

				p := harness.newProvider(t, server)

				ref, err := p.GetLatestVersionRef(context.Background())

				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, ref)
			})

			t.Run("lists tags", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractListTags))
				defer server.Close()

				p := harness.newProvider(t, server)

				tags, err := p.ListTags(context.Background())

				testastic.NoError(t, err)
				testastic.SliceEqual(t, []string{providerContractTag, "v1.2.2"}, tags)
			})

			t.Run("gets commits since refs", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractGetCommitsSinceRefs))
				defer server.Close()

				p := harness.newProvider(t, server)

				history, err := p.GetCommitsSinceRefs(
					context.Background(),
					[]string{providerContractTag},
					providerContractBaseBranch,
					true,
				)

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

				server := httptest.NewServer(harness.handler(t, providerContractGetCommitsSinceRefsMissing))
				defer server.Close()

				p := harness.newProvider(t, server)

				history, err := p.GetCommitsSinceRefs(
					context.Background(),
					[]string{providerContractTag, providerContractMissingTag},
					providerContractBaseBranch,
					true,
				)

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

				server := httptest.NewServer(harness.handler(t, providerContractGetCommitsSinceRefsMultiBoundary))
				defer server.Close()

				p := harness.newProvider(t, server)

				history, err := p.GetCommitsSinceRefs(
					context.Background(),
					[]string{providerContractIntermediateTag, providerContractTag},
					providerContractBaseBranch,
					false,
				)

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

				server := httptest.NewServer(harness.handler(t, providerContractGetCommitsSinceRefsUnresolved))
				defer server.Close()

				p := harness.newProvider(t, server)

				history, err := p.GetCommitsSinceRefs(
					context.Background(),
					[]string{providerContractTag, providerContractMissingTag},
					providerContractBaseBranch,
					true,
				)

				testastic.NoError(t, err)
				testastic.SliceEqual(t, []string{providerContractMissingTag}, history.MissingRefs)

				entries := history.EntriesByRef[providerContractTag]
				testastic.Equal(t, 1, len(entries))
				testastic.Equal(t, providerContractHeadSHA, entries[0].Hash)
			})

			t.Run("gets release by tag", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractGetReleaseByTag))
				defer server.Close()

				p := harness.newProvider(t, server)

				release, err := p.GetReleaseByTag(context.Background(), providerContractTag)

				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, release.TagName)
				testastic.Equal(t, "release notes", release.Body)
				testastic.Equal(t, harness.expectedReleaseURL(server.URL), release.URL)
			})

			t.Run("reports tag existence", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractTagExists))
				defer server.Close()

				p := harness.newProvider(t, server)

				exists, err := p.TagExists(context.Background(), providerContractTag)

				testastic.NoError(t, err)
				testastic.True(t, exists)
			})

			t.Run("creates release pull request", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractCreateReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				pr, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
					Title:         providerContractReleaseTitle,
					Body:          providerContractReleaseBody,
					BaseBranch:    providerContractBaseBranch,
					ReleaseBranch: providerContractReleaseBranch,
				})

				testastic.NoError(t, err)
				testastic.Equal(t, 42, pr.Number)
				testastic.Equal(t, providerContractReleaseTitle, pr.Title)
				testastic.Equal(t, providerContractReleaseBody, pr.Body)
				testastic.Equal(t, providerContractReleaseBranch, pr.Branch)
				testastic.Equal(t, harness.expectedReleasePRURL(server.URL), pr.URL)
			})

			t.Run("updates release pull request", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractUpdateReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.UpdateReleasePR(context.Background(), 42, provider.ReleasePROptions{
					Title: providerContractReleaseTitle,
					Body:  "updated release body",
				})

				testastic.NoError(t, err)
			})

			t.Run("finds open pending release pull requests", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractFindOpenPRs))
				defer server.Close()

				p := harness.newProvider(t, server)

				prs, err := p.FindOpenPendingReleasePRs(context.Background(), providerContractBaseBranch)

				testastic.NoError(t, err)
				testastic.Equal(t, 1, len(prs))
				testastic.Equal(t, 42, prs[0].Number)
				testastic.Equal(t, providerContractPendingBranch, prs[0].Branch)
			})

			t.Run("finds merged release pull request", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractFindMergedPR))
				defer server.Close()

				p := harness.newProvider(t, server)

				pr, err := p.FindMergedReleasePR(context.Background(), providerContractBaseBranch)

				testastic.NoError(t, err)
				testastic.Equal(t, 42, pr.Number)
				testastic.Equal(t, providerContractPendingBranch, pr.Branch)
				testastic.Equal(t, providerContractMergeSHA, pr.MergeCommitSHA)
				testastic.Equal(t, providerContractReleaseBody, pr.Body)
			})

			t.Run("marks release pull request state", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractMarkReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.MarkReleasePRPending(context.Background(), 42)
				testastic.NoError(t, err)

				err = p.MarkReleasePRTagged(context.Background(), 42)

				testastic.NoError(t, err)
			})

			t.Run("merges release pull request", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractMergeReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
					Method: provider.MergeMethodAuto,
				})

				testastic.NoError(t, err)
			})

			t.Run("finds commit pull request body", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractCommitPRBody))
				defer server.Close()

				p := harness.newProvider(t, server)

				body, found, err := p.CommitPullRequestBody(context.Background(), providerContractMergeSHA)

				testastic.NoError(t, err)
				testastic.True(t, found)
				testastic.Equal(t, "override body", body)
			})

			t.Run("creates branch", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractCreateBranch))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.CreateBranch(context.Background(), providerContractReleaseBranch, providerContractBaseBranch)

				testastic.NoError(t, err)
			})

			t.Run("creates release", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractCreateRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				release, err := p.CreateRelease(context.Background(), provider.ReleaseOptions{
					TagName:    providerContractTag,
					Ref:        providerContractBaseBranch,
					Name:       providerContractTag,
					Body:       "release notes",
					Prerelease: true,
				})

				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, release.TagName)
				testastic.Equal(t, "release notes", release.Body)
				testastic.Equal(t, harness.expectedReleaseURL(server.URL), release.URL)
			})

			t.Run("reads file content", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractGetFile))
				defer server.Close()

				p := harness.newProvider(t, server)

				content, err := p.GetFile(context.Background(), providerContractBaseBranch, "CHANGELOG.md")

				testastic.NoError(t, err)
				testastic.Equal(t, "# Changelog\n", content)
			})

			t.Run("updates release files", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractUpdateFiles))
				defer server.Close()

				p := harness.newProvider(t, server)

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

				testastic.NoError(t, err)
			})

			t.Run("returns file not found error", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractMissingFile))
				defer server.Close()

				p := harness.newProvider(t, server)

				_, err := p.GetFile(context.Background(), providerContractBaseBranch, "MISSING.md")

				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrFileNotFound)
			})

			t.Run("returns release not found error", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractMissingRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				_, err := p.GetReleaseByTag(context.Background(), providerContractTag)

				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrNoRelease)
			})

			t.Run("returns release pull request not found error", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractMissingPR))
				defer server.Close()

				p := harness.newProvider(t, server)

				_, err := p.FindMergedReleasePR(context.Background(), providerContractBaseBranch)

				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrNoPR)
			})

			t.Run("returns blocked merge error", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractBlockedMerge))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{})

				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
			})

			t.Run("returns unsupported merge method error", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractUnsupportedMerge))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
					Method: provider.MergeMethod("octopus"),
				})

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
