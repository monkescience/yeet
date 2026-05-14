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
	githubKeyCommit  = "commit"
	githubKeyRef     = "ref"
	githubKeyObject  = "object"
	githubKeyName    = "name"
	githubKeyType    = "type"
	githubFakePRID   = 42
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

	mux.HandleFunc("GET "+prefix+"/pulls", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})

	registerGitHubWritePath(mux, prefix)

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
		{githubKeyName: tag},
	}
}

func githubCommitDetail(ref string, opts GitHubOptions) map[string]any {
	if ref == opts.LatestTag {
		return map[string]any{githubKeySHA: opts.BoundarySHA}
	}

	for _, c := range opts.Commits {
		if c.SHA == ref {
			return map[string]any{
				githubKeySHA:    c.SHA,
				githubKeyCommit: map[string]any{"message": c.Message},
			}
		}
	}

	return map[string]any{githubKeySHA: ref}
}

// registerGitHubWritePath attaches the handlers exercised by a non-dry-run
// release (creating a release branch, updating files, opening the PR, and
// managing labels). All responses are canned — the test asserts side effects
// via exit code / stdout rather than payload bodies.
func registerGitHubWritePath(mux *http.ServeMux, prefix string) {
	registerGitHubGitData(mux, prefix)
	registerGitHubContent(mux, prefix)
	registerGitHubPullsWrite(mux, prefix)
	registerGitHubLabels(mux, prefix)

	mux.HandleFunc("GET "+prefix, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"allow_squash_merge": true,
			"allow_rebase_merge": true,
			"allow_merge_commit": true,
		})
	})
}

func registerGitHubGitData(mux *http.ServeMux, prefix string) {
	const fakeBaseSHA = "base-sha"

	const fakeCommitSHA = "new-commit-sha"

	const fakeTreeSHA = "tree-sha"

	mux.HandleFunc("GET "+prefix+"/git/ref/heads/{branch...}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			githubKeyRef:    "refs/heads/" + r.PathValue("branch"),
			githubKeyObject: map[string]any{githubKeySHA: fakeBaseSHA, githubKeyType: githubKeyCommit},
		})
	})

	mux.HandleFunc("GET "+prefix+"/git/ref/tags/{name}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	mux.HandleFunc("POST "+prefix+"/git/refs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			githubKeyRef:    "refs/heads/yeet/release-main",
			githubKeyObject: map[string]any{githubKeySHA: fakeBaseSHA, githubKeyType: githubKeyCommit},
		})
	})

	mux.HandleFunc("PATCH "+prefix+"/git/refs/heads/{branch...}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			githubKeyRef:    "refs/heads/" + r.PathValue("branch"),
			githubKeyObject: map[string]any{githubKeySHA: fakeCommitSHA, githubKeyType: githubKeyCommit},
		})
	})

	mux.HandleFunc("GET "+prefix+"/git/commits/{sha}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			githubKeySHA: r.PathValue(githubKeySHA),
			"tree":       map[string]any{githubKeySHA: fakeTreeSHA},
			"parents":    []any{},
		})
	})

	mux.HandleFunc("POST "+prefix+"/git/trees", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{githubKeySHA: fakeTreeSHA})
	})

	mux.HandleFunc("POST "+prefix+"/git/commits", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			githubKeySHA: fakeCommitSHA,
			"tree":       map[string]any{githubKeySHA: fakeTreeSHA},
		})
	})
}

func registerGitHubContent(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/contents/{path...}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
}

func registerGitHubPullsWrite(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("POST "+prefix+"/pulls", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubFakePR())
	})

	mux.HandleFunc("PATCH "+prefix+"/pulls/{number}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubFakePR())
	})

	mux.HandleFunc("GET "+prefix+"/pulls/{number}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubFakePR())
	})
}

func registerGitHubLabels(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/labels/{name}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{githubKeyName: r.PathValue("name")})
	})

	mux.HandleFunc("POST "+prefix+"/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{githubKeyName: "label"})
	})

	mux.HandleFunc("POST "+prefix+"/issues/{number}/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})

	mux.HandleFunc("DELETE "+prefix+"/issues/{number}/labels/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func githubFakePR() map[string]any {
	return map[string]any{
		"number":          githubFakePRID,
		"state":           "open",
		"draft":           false,
		"merged":          false,
		"mergeable_state": "clean",
		"html_url":        "https://example.test/pulls/42",
		"head": map[string]any{
			githubKeyRef: "yeet/release-main",
			githubKeySHA: "head-sha",
		},
		"base": map[string]any{githubKeyRef: "main"},
	}
}

func githubCommitsList(commits []GitHubCommit) []map[string]any {
	out := make([]map[string]any, 0, len(commits))

	for _, c := range commits {
		out = append(out, map[string]any{
			githubKeySHA: c.SHA,
			githubKeyCommit: map[string]any{
				githubKeyMessage: c.Message,
			},
		})
	}

	return out
}
