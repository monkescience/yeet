// Package fakeprovider exposes httptest-backed provider stubs that yeet
// blackbox tests point at via the GITHUB_URL / GITLAB_URL / AZURE_DEVOPS_URL
// env vars.
package fakeprovider

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// GitHubOptions configures the responses served by [NewGitHub].
type GitHubOptions struct {
	Owner string
	Repo  string
	// LatestTag is the most recent tag returned by the tags-fallback. When
	// empty, the server reports no tags and no latest release.
	LatestTag string
	// BoundarySHA is the SHA of the commit pointed at by LatestTag.
	BoundarySHA string
	// Commits are returned (newest first) from the commits listing for the
	// release branch. The last entry should point at BoundarySHA so yeet can
	// terminate the walk.
	Commits []GitHubCommit
}

// GitHubCommit is a tiny subset of the GitHub commit payload that yeet reads.
type GitHubCommit struct {
	SHA     string
	Message string
}

const (
	githubKeySHA     = "sha"
	githubKeyMessage = "message"
)

// NewGitHub starts an httptest.Server serving the minimum set of GitHub REST
// endpoints exercised by `yeet release --dry-run` against a configured
// owner/repo. The server is stopped via t.Cleanup.
func NewGitHub(t *testing.T, opts GitHubOptions) *httptest.Server {
	t.Helper()

	owner := opts.Owner
	repo := opts.Repo
	prefix := "/api/v3/repos/" + owner + "/" + repo

	mux := http.NewServeMux()

	mux.HandleFunc("GET "+prefix+"/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no release", http.StatusNotFound)
	})

	mux.HandleFunc("GET "+prefix+"/tags", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubTagsPayload(opts.LatestTag))
	})

	mux.HandleFunc("GET "+prefix+"/commits", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubCommitsList(opts.Commits))
	})

	mux.HandleFunc("GET "+prefix+"/commits/{ref}", func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("ref")
		writeJSON(w, githubCommitDetail(ref, opts))
	})

	mux.HandleFunc("GET "+prefix+"/commits/{sha}/pulls", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fakeprovider/github: unexpected request %s %s", r.Method, r.URL.String())
		http.Error(w, "unhandled", http.StatusNotImplemented)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func githubTagsPayload(tag string) any {
	if tag == "" {
		return []any{}
	}

	return []map[string]any{
		{"name": tag},
	}
}

func githubCommitDetail(ref string, opts GitHubOptions) map[string]any {
	if ref == opts.LatestTag {
		return map[string]any{githubKeySHA: opts.BoundarySHA}
	}

	for _, c := range opts.Commits {
		if c.SHA == ref {
			return map[string]any{
				githubKeySHA: c.SHA,
				"commit":     map[string]any{"message": c.Message},
			}
		}
	}

	return map[string]any{githubKeySHA: ref}
}

func githubCommitsList(commits []GitHubCommit) []map[string]any {
	out := make([]map[string]any, 0, len(commits))

	for _, c := range commits {
		out = append(out, map[string]any{
			githubKeySHA: c.SHA,
			"commit": map[string]any{
				githubKeyMessage: c.Message,
			},
		})
	}

	return out
}
