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
	providerContractCreateReleasePR    providerContractScenario = "create release pr"
	providerContractMarkReleasePR      providerContractScenario = "mark release pr"
	providerContractCreateRelease      providerContractScenario = "create release"
	providerContractGetFile            providerContractScenario = "get file"
	providerContractUpdateFiles        providerContractScenario = "update files"
	providerContractReleaseTitle                                = "chore: release v1.2.3"
	providerContractReleaseBody                                 = "release body"
	providerContractReleaseBranch                               = "release-main"
	providerContractBaseBranch                                  = "main"
	providerContractTag                                         = "v1.2.3"
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
		case providerContractCreateReleasePR:
			handleGitHubCreateReleasePRContract(t, w, r)
		case providerContractMarkReleasePR:
			handleGitHubMarkReleasePRContract(t, w, r)
		case providerContractCreateRelease:
			handleGitHubCreateReleaseContract(t, w, r)
		case providerContractGetFile:
			handleGitHubGetFileContract(t, w, r)
		case providerContractUpdateFiles:
			handleGitHubUpdateFilesContract(t, w, r)
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
		case providerContractCreateReleasePR:
			handleGitLabCreateReleasePRContract(t, w, r)
		case providerContractMarkReleasePR:
			handleGitLabMarkReleasePRContract(t, w, r)
		case providerContractCreateRelease:
			handleGitLabCreateReleaseContract(t, w, r)
		case providerContractGetFile:
			handleGitLabGetFileContract(t, w, r)
		case providerContractUpdateFiles:
			handleGitLabUpdateFilesContract(t, w, r)
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

func isGitLabReleaseListRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/releases"
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
