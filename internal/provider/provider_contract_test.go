package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	githubapi "github.com/google/go-github/v85/github"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go/v2"
)

type providerContractHarness struct {
	name               string
	newProvider        func(t *testing.T, server *httptest.Server) provider.Provider
	handler            func(t *testing.T, scenario providerContractScenario) http.Handler
	expectedRepoURL    func(serverURL string) string
	expectedPathPrefix string
}

type providerContractScenario string

const (
	providerContractLatestRelease      providerContractScenario = "latest release"
	providerContractLatestFallbackTags providerContractScenario = "latest fallback tags"
	providerContractListTags           providerContractScenario = "list tags"
	providerContractGetCommitsSince    providerContractScenario = "get commits since"
	providerContractGetReleaseByTag    providerContractScenario = "get release by tag"
	providerContractTagExists          providerContractScenario = "tag exists"
	providerContractCreateReleasePR    providerContractScenario = "create release pr"
	providerContractUpdateReleasePR    providerContractScenario = "update release pr"
	providerContractFindOpenPRs        providerContractScenario = "find open prs"
	providerContractFindMergedPR       providerContractScenario = "find merged pr"
	providerContractMarkReleasePR      providerContractScenario = "mark release pr"
	providerContractMergeReleasePR     providerContractScenario = "merge release pr"
	providerContractCommitPRBody       providerContractScenario = "commit pr body"
	providerContractCreateBranch       providerContractScenario = "create branch"
	providerContractCreateRelease      providerContractScenario = "create release"
	providerContractGetFile            providerContractScenario = "get file"
	providerContractUpdateFiles        providerContractScenario = "update files"
	providerContractMissingFile        providerContractScenario = "missing file"
	providerContractMissingRelease     providerContractScenario = "missing release"
	providerContractMissingPR          providerContractScenario = "missing pr"
	providerContractBlockedMerge       providerContractScenario = "blocked merge"
	providerContractUnsupportedMerge   providerContractScenario = "unsupported merge"
	providerContractReleaseTitle                                = "chore: release v1.2.3"
	providerContractReleaseBody                                 = "release body"
	providerContractReleaseBranch                               = "release-main"
	providerContractPendingBranch                               = "yeet/release-main"
	providerContractBaseBranch                                  = "main"
	providerContractTag                                         = "v1.2.3"
	providerContractHeadSHA                                     = "head-sha"
	providerContractMergeSHA                                    = "merge-sha"
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

			t.Run("gets commits since ref", func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(harness.handler(t, providerContractGetCommitsSince))
				defer server.Close()

				p := harness.newProvider(t, server)

				entries, err := p.GetCommitsSince(
					context.Background(),
					providerContractTag,
					providerContractBaseBranch,
					true,
				)

				testastic.NoError(t, err)
				testastic.Equal(t, 1, len(entries))
				testastic.Equal(t, providerContractHeadSHA, entries[0].Hash)
				testastic.Equal(t, "feat: add release flow", entries[0].Message)
				testastic.SliceEqual(t, []string{"CHANGELOG.md", "VERSION.txt"}, entries[0].Paths)
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
				testastic.Equal(t, "https://example.com/releases/v1.2.3", release.URL)
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
				testastic.Equal(t, "https://example.com/pulls/42", pr.URL)
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
				testastic.Equal(t, "https://example.com/releases/v1.2.3", release.URL)
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
			name:               "github",
			newProvider:        newGitHubContractProvider,
			handler:            newGitHubContractHandler,
			expectedRepoURL:    func(serverURL string) string { return serverURL + "/o/r" },
			expectedPathPrefix: "",
		},
		{
			name:               "gitlab",
			newProvider:        newGitLabContractProvider,
			handler:            newGitLabContractHandler,
			expectedRepoURL:    func(serverURL string) string { return serverURL + "/o/r" },
			expectedPathPrefix: "/-",
		},
	}
}

func newGitHubContractProvider(t *testing.T, server *httptest.Server) provider.Provider {
	t.Helper()

	client := githubapi.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")

	return provider.NewGitHub(client, "o", "r")
}

func newGitLabContractProvider(t *testing.T, server *httptest.Server) provider.Provider {
	t.Helper()

	client, err := gitlabapi.NewClient(
		"",
		gitlabapi.WithBaseURL(server.URL),
		gitlabapi.WithHTTPClient(server.Client()),
		gitlabapi.WithoutRetries(),
	)
	testastic.NoError(t, err)

	return provider.NewGitLab(client, "o/r")
}

func newGitHubContractHandler(t *testing.T, scenario providerContractScenario) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case providerContractLatestRelease:
			handleGitHubLatestReleaseContract(t, w, r)
		case providerContractLatestFallbackTags:
			handleGitHubLatestFallbackTagsContract(t, w, r)
		case providerContractListTags:
			handleGitHubListTagsContract(t, w, r)
		case providerContractGetCommitsSince:
			handleGitHubGetCommitsSinceContract(t, w, r)
		case providerContractGetReleaseByTag:
			handleGitHubGetReleaseByTagContract(t, w, r)
		case providerContractTagExists:
			handleGitHubTagExistsContract(t, w, r)
		case providerContractCreateReleasePR:
			handleGitHubCreateReleasePRContract(t, w, r)
		case providerContractUpdateReleasePR:
			handleGitHubUpdateReleasePRContract(t, w, r)
		case providerContractFindOpenPRs:
			handleGitHubFindOpenPRsContract(t, w, r)
		case providerContractFindMergedPR:
			handleGitHubFindMergedPRContract(t, w, r)
		case providerContractMarkReleasePR:
			handleGitHubMarkReleasePRContract(t, w, r)
		case providerContractMergeReleasePR:
			handleGitHubMergeReleasePRContract(t, w, r)
		case providerContractCommitPRBody:
			handleGitHubCommitPRBodyContract(t, w, r)
		case providerContractCreateBranch:
			handleGitHubCreateBranchContract(t, w, r)
		case providerContractCreateRelease:
			handleGitHubCreateReleaseContract(t, w, r)
		case providerContractGetFile:
			handleGitHubGetFileContract(t, w, r)
		case providerContractUpdateFiles:
			handleGitHubUpdateFilesContract(t, w, r)
		case providerContractMissingFile:
			handleGitHubMissingFileContract(t, w, r)
		case providerContractMissingRelease:
			handleGitHubMissingReleaseContract(t, w, r)
		case providerContractMissingPR:
			handleGitHubMissingPRContract(t, w, r)
		case providerContractBlockedMerge:
			handleGitHubBlockedMergeContract(t, w, r)
		case providerContractUnsupportedMerge:
			handleGitHubUnsupportedMergeContract(t, w, r)
		default:
			t.Fatalf("unhandled GitHub contract scenario: %s", scenario)
		}
	})
}

func newGitLabContractHandler(t *testing.T, scenario providerContractScenario) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case providerContractLatestRelease:
			handleGitLabLatestReleaseContract(t, w, r)
		case providerContractLatestFallbackTags:
			handleGitLabLatestFallbackTagsContract(t, w, r)
		case providerContractListTags:
			handleGitLabListTagsContract(t, w, r)
		case providerContractGetCommitsSince:
			handleGitLabGetCommitsSinceContract(t, w, r)
		case providerContractGetReleaseByTag:
			handleGitLabGetReleaseByTagContract(t, w, r)
		case providerContractTagExists:
			handleGitLabTagExistsContract(t, w, r)
		case providerContractCreateReleasePR:
			handleGitLabCreateReleasePRContract(t, w, r)
		case providerContractUpdateReleasePR:
			handleGitLabUpdateReleasePRContract(t, w, r)
		case providerContractFindOpenPRs:
			handleGitLabFindOpenPRsContract(t, w, r)
		case providerContractFindMergedPR:
			handleGitLabFindMergedPRContract(t, w, r)
		case providerContractMarkReleasePR:
			handleGitLabMarkReleasePRContract(t, w, r)
		case providerContractMergeReleasePR:
			handleGitLabMergeReleasePRContract(t, w, r)
		case providerContractCommitPRBody:
			handleGitLabCommitPRBodyContract(t, w, r)
		case providerContractCreateBranch:
			handleGitLabCreateBranchContract(t, w, r)
		case providerContractCreateRelease:
			handleGitLabCreateReleaseContract(t, w, r)
		case providerContractGetFile:
			handleGitLabGetFileContract(t, w, r)
		case providerContractUpdateFiles:
			handleGitLabUpdateFilesContract(t, w, r)
		case providerContractMissingFile:
			handleGitLabMissingFileContract(t, w, r)
		case providerContractMissingRelease:
			handleGitLabMissingReleaseContract(t, w, r)
		case providerContractMissingPR:
			handleGitLabMissingPRContract(t, w, r)
		case providerContractBlockedMerge:
			handleGitLabBlockedMergeContract(t, w, r)
		case providerContractUnsupportedMerge:
			handleGitLabUnsupportedMergeContract(t, w, r)
		default:
			t.Fatalf("unhandled GitLab contract scenario: %s", scenario)
		}
	})
}

func handleGitHubLatestReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/latest" {
		writeJSONFixture(t, w, "contracts/github/latest_release.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubLatestFallbackTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/latest":
		http.NotFound(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/tags":
		writeJSONFixture(t, w, "contracts/github/tags.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubListTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/tags" {
		writeJSONFixture(t, w, "contracts/github/tags.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubGetCommitsSinceContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+providerContractTag:
		writeJSONFixture(t, w, "contracts/github/commits/ref.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+providerContractHeadSHA:
		writeJSONFixture(t, w, "contracts/github/commits/detail.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits":
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("sha"))
		writeJSONFixture(t, w, "contracts/github/commits/list.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubGetReleaseByTagContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/"+providerContractTag {
		writeJSONFixture(t, w, "contracts/github/release_by_tag.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubTagExistsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/"+providerContractTag {
		writeJSONFixture(t, w, "contracts/github/tag_ref.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubCreateReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPost || r.URL.Path != "/repos/o/r/pulls" {
		fatalUnexpectedProviderRequest(t, "GitHub", r)

		return
	}

	var request struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractReleaseTitle, request.Title)
	testastic.Equal(t, providerContractReleaseBody, request.Body)
	testastic.Equal(t, providerContractReleaseBranch, request.Head)
	testastic.Equal(t, providerContractBaseBranch, request.Base)

	writeJSONFixture(t, w, "contracts/github/create_release_pr.json")
}

func handleGitHubUpdateReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPatch || r.URL.Path != "/repos/o/r/pulls/42" {
		fatalUnexpectedProviderRequest(t, "GitHub", r)

		return
	}

	var request struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractReleaseTitle, request.Title)
	testastic.Equal(t, "updated release body", request.Body)

	writeJSONFixture(t, w, "contracts/github/update_release_pr.json")
}

func handleGitHubFindOpenPRsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls" {
		testastic.Equal(t, "open", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("base"))
		writeJSONFixture(t, w, "contracts/github/open_prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubFindMergedPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls":
		testastic.Equal(t, "closed", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("base"))
		writeJSONFixture(t, w, "contracts/github/merged_prs.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSONFixture(t, w, "contracts/github/merged_pr.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubMarkReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
		writeGitHubLabelFixture(t, w, pathLabel(t, r))
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
		var labels []string
		decodeJSONRequest(t, r, &labels)
		writeGitHubLabelsFixture(t, w, strings.Join(labels, ","))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
		w.WriteHeader(http.StatusNoContent)
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubMergeReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSONFixture(t, w, "contracts/github/merge/pr.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
		writeJSONFixture(t, w, "contracts/github/merge/repo.json")
	case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/42/merge":
		var request struct {
			MergeMethod string `json:"merge_method"`
			SHA         string `json:"sha"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, string(provider.MergeMethodSquash), request.MergeMethod)
		testastic.Equal(t, providerContractHeadSHA, request.SHA)
		writeJSONFixture(t, w, "contracts/github/merge/result.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubCommitPRBodyContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+providerContractMergeSHA+"/pulls" {
		writeJSONFixture(t, w, "contracts/github/commit_prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubCreateBranchContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
		writeJSONFixture(t, w, "contracts/github/create_branch/base_ref.json")
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
		writeJSONFixture(t, w, "contracts/github/create_branch/ref.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubCreateReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if !isGitHubCreateReleaseRequest(r) {
		fatalUnexpectedProviderRequest(t, "GitHub", r)

		return
	}

	var request struct {
		TagName         string `json:"tag_name"`
		TargetCommitish string `json:"target_commitish"`
		Name            string `json:"name"`
		Body            string `json:"body"`
		Prerelease      bool   `json:"prerelease"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractTag, request.TagName)
	testastic.Equal(t, providerContractBaseBranch, request.TargetCommitish)
	testastic.Equal(t, providerContractTag, request.Name)
	testastic.Equal(t, "release notes", request.Body)
	testastic.True(t, request.Prerelease)

	writeJSONFixture(t, w, "contracts/github/create_release.json")
}

func handleGitHubGetFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/contents/CHANGELOG.md" {
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("ref"))
		writeJSONFixture(t, w, "contracts/github/get_file.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubUpdateFilesContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
		writeJSONFixture(t, w, "contracts/github/update_files/base_ref.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/commits/base-ref-sha":
		writeJSONFixture(t, w, "contracts/github/update_files/base_commit.json")
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/trees":
		writeJSONFixture(t, w, "contracts/github/update_files/tree.json")
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/commits":
		writeJSONFixture(t, w, "contracts/github/update_files/commit.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/release-main":
		http.NotFound(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
		writeJSONFixture(t, w, "contracts/github/update_files/create_ref.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubMissingFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/contents/MISSING.md" {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/github/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubMissingReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/"+providerContractTag {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/github/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubMissingPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls" {
		writeJSONFixture(t, w, "contracts/github/empty_prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubBlockedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42" {
		writeJSONFixture(t, w, "contracts/github/merge/blocked_pr.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubUnsupportedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSONFixture(t, w, "contracts/github/merge/pr.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
		writeJSONFixture(t, w, "contracts/github/merge/repo.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitLabLatestReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if isGitLabReleaseListRequest(r) {
		writeJSONFixture(t, w, "contracts/gitlab/latest_releases.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabLatestFallbackTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case isGitLabReleaseListRequest(r):
		writeJSONFixture(t, w, "contracts/gitlab/empty_releases.json")
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/tags":
		writeJSONFixture(t, w, "contracts/gitlab/tags.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabListTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/tags" {
		writeJSONFixture(t, w, "contracts/gitlab/tags.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabGetCommitsSinceContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/repository/commits/"+providerContractEscapedTag():
		writeJSONFixture(t, w, "contracts/gitlab/commits/ref.json")
	case isGitLabCommitDiffRequest(r, providerContractHeadSHA):
		writeJSONFixture(t, w, "contracts/gitlab/commits/diff.json")
	case isGitLabCommitsListRequest(r):
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("ref_name"))
		writeJSONFixture(t, w, "contracts/gitlab/commits/list.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabGetReleaseByTagContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/releases/"+providerContractEscapedTag() {
		writeJSONFixture(t, w, "contracts/gitlab/release_by_tag.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabTagExistsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/repository/tags/"+providerContractEscapedTag() {
		writeJSONFixture(t, w, "contracts/gitlab/tag.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabCreateReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPost || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests" {
		fatalUnexpectedProviderRequest(t, "GitLab", r)

		return
	}

	var request struct {
		Title        string `json:"title"`
		Description  string `json:"description"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractReleaseTitle, request.Title)
	testastic.Equal(t, providerContractReleaseBody, request.Description)
	testastic.Equal(t, providerContractReleaseBranch, request.SourceBranch)
	testastic.Equal(t, providerContractBaseBranch, request.TargetBranch)

	writeJSONFixture(t, w, "contracts/gitlab/create_release_pr.json")
}

func handleGitLabUpdateReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPut || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests/42" {
		fatalUnexpectedProviderRequest(t, "GitLab", r)

		return
	}

	var request struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractReleaseTitle, request.Title)
	testastic.Equal(t, "updated release body", request.Description)

	writeJSONFixture(t, w, "contracts/gitlab/update_release_pr.json")
}

func handleGitLabFindOpenPRsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		testastic.Equal(t, "opened", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("target_branch"))
		writeJSONFixture(t, w, "contracts/gitlab/open_prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabFindMergedPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		testastic.Equal(t, "merged", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("target_branch"))
		writeJSONFixture(t, w, "contracts/gitlab/merged_prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabMarkReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
		writeGitLabLabelFixture(t, w, pathLabel(t, r))
	case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
		var request struct {
			AddLabels    string `json:"add_labels"`
			RemoveLabels string `json:"remove_labels"`
		}
		decodeJSONRequest(t, r, &request)
		writeJSONFixture(t, w, "contracts/gitlab/update_merge_request.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabMergeReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
		writeJSONFixture(t, w, "contracts/gitlab/merge/pr.json")
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
		writeJSONFixture(t, w, "contracts/gitlab/merge/project.json")
	case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42/merge":
		var request struct {
			SHA string `json:"sha"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, providerContractHeadSHA, request.SHA)
		writeJSONFixture(t, w, "contracts/gitlab/merge/result.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabCommitPRBodyContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/repository/commits/"+providerContractMergeSHA+"/merge_requests" {
		writeJSONFixture(t, w, "contracts/gitlab/commit_prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabCreateBranchContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches" {
		writeJSONFixture(t, w, "contracts/gitlab/create_branch.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabCreateReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if !isGitLabCreateReleaseRequest(r) {
		fatalUnexpectedProviderRequest(t, "GitLab", r)

		return
	}

	var request struct {
		TagName     string `json:"tag_name"`
		Ref         string `json:"ref"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractTag, request.TagName)
	testastic.Equal(t, providerContractBaseBranch, request.Ref)
	testastic.Equal(t, providerContractTag, request.Name)
	testastic.Equal(t, "release notes", request.Description)

	writeJSONFixture(t, w, "contracts/gitlab/create_release.json")
}

func handleGitLabGetFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && isGitLabRawFilePath(r, "CHANGELOG.md") {
		writeTextFixture(t, w, "contracts/gitlab/get_file.txt")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabUpdateFilesContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && isGitLabRawFilePath(r, "CHANGELOG.md"):
		writeTextFixture(t, w, "contracts/gitlab/get_file.txt")
	case r.Method == http.MethodGet && isGitLabRawFilePath(r, "VERSION.txt"):
		http.NotFound(w, r)
	case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/commits":
		writeJSONFixture(t, w, "contracts/gitlab/update_files/commit.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabMissingFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && isGitLabRawFilePath(r, "MISSING.md") {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/gitlab/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabMissingReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/releases/"+providerContractEscapedTag() {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/gitlab/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabMissingPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		writeJSONFixture(t, w, "contracts/gitlab/empty_prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabBlockedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42" {
		writeJSONFixture(t, w, "contracts/gitlab/merge/blocked_pr.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabUnsupportedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
		writeJSONFixture(t, w, "contracts/gitlab/merge/pr.json")
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
		writeJSONFixture(t, w, "contracts/gitlab/merge/project.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func isGitLabReleaseListRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/releases"
}

func providerContractEscapedTag() string {
	return strings.ReplaceAll(providerContractTag, ".", "%2E")
}

func writeGitHubLabelFixture(t *testing.T, w http.ResponseWriter, label string) {
	t.Helper()

	switch label {
	case provider.ReleaseLabelPending:
		writeJSONFixture(t, w, "contracts/github/label_pending.json")
	case provider.ReleaseLabelTagged:
		writeJSONFixture(t, w, "contracts/github/label_tagged.json")
	default:
		t.Fatalf("unexpected GitHub label: %s", label)
	}
}

func writeGitHubLabelsFixture(t *testing.T, w http.ResponseWriter, labels string) {
	t.Helper()

	switch labels {
	case provider.ReleaseLabelPending:
		writeJSONFixture(t, w, "contracts/github/add_pending_labels.json")
	case provider.ReleaseLabelTagged:
		writeJSONFixture(t, w, "contracts/github/add_tagged_labels.json")
	default:
		t.Fatalf("unexpected GitHub labels: %s", labels)
	}
}

func writeGitLabLabelFixture(t *testing.T, w http.ResponseWriter, label string) {
	t.Helper()

	switch label {
	case provider.ReleaseLabelPending:
		writeJSONFixture(t, w, "contracts/gitlab/label_pending.json")
	case provider.ReleaseLabelTagged:
		writeJSONFixture(t, w, "contracts/gitlab/label_tagged.json")
	default:
		t.Fatalf("unexpected GitLab label: %s", label)
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
