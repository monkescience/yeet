package fakeprovider

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// GitLabOptions configures the responses served by [NewGitLab].
type GitLabOptions struct {
	// Project is the full path with namespace, e.g. "group/repo". The server
	// URL-encodes this to build endpoint paths.
	Project string
	// LatestTag is the most recent tag returned by the tags-fallback. When
	// empty, the server reports no tags and no latest release.
	LatestTag string
	// BoundarySHA is the SHA of the commit pointed at by LatestTag.
	BoundarySHA string
	// Commits are returned (newest first) from the commits listing for the
	// release branch. The last entry should point at BoundarySHA so yeet can
	// terminate the walk.
	Commits []GitLabCommit
}

// GitLabCommit is a tiny subset of the GitLab commit payload that yeet reads.
type GitLabCommit struct {
	SHA     string
	Message string
}

const (
	gitlabKeyID      = "id"
	gitlabKeyMessage = "message"
	gitlabKeyName    = "name"
)

// NewGitLab starts an httptest.Server serving the minimum GitLab REST surface
// for `yeet release --dry-run`. The server is closed via t.Cleanup.
func NewGitLab(t *testing.T, opts GitLabOptions) *httptest.Server {
	t.Helper()

	pid := url.PathEscape(opts.Project)
	prefix := "/api/v4/projects/" + pid

	mux := http.NewServeMux()

	mux.HandleFunc("GET "+prefix+"/releases", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})

	mux.HandleFunc("GET "+prefix+"/repository/tags", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabTagsPayload(opts.LatestTag))
	})

	mux.HandleFunc("GET "+prefix+"/repository/commits", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabCommitsList(opts.Commits))
	})

	mux.HandleFunc("GET "+prefix+"/repository/commits/{ref}", func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("ref")
		writeJSON(w, gitlabCommitDetail(ref, opts))
	})

	mux.HandleFunc(
		"GET "+prefix+"/repository/commits/{sha}/merge_requests",
		func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, []any{})
		},
	)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fakeprovider/gitlab: unexpected request %s %s", r.Method, r.URL.String())
		http.Error(w, "unhandled", http.StatusNotImplemented)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func gitlabTagsPayload(tag string) any {
	if tag == "" {
		return []any{}
	}

	return []map[string]any{
		{gitlabKeyName: tag},
	}
}

func gitlabCommitDetail(ref string, opts GitLabOptions) map[string]any {
	if ref == opts.LatestTag {
		return map[string]any{gitlabKeyID: opts.BoundarySHA}
	}

	for _, c := range opts.Commits {
		if c.SHA == ref {
			return map[string]any{
				gitlabKeyID:      c.SHA,
				gitlabKeyMessage: c.Message,
			}
		}
	}

	return map[string]any{gitlabKeyID: ref}
}

func gitlabCommitsList(commits []GitLabCommit) []map[string]any {
	out := make([]map[string]any, 0, len(commits))

	for _, c := range commits {
		out = append(out, map[string]any{
			gitlabKeyID:      c.SHA,
			gitlabKeyMessage: c.Message,
		})
	}

	return out
}
