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
	// Files maps repository-relative paths to raw content for /files/{path}/raw.
	Files map[string]string
	// MergedPendingRelease toggles the merged-release-MR fixture.
	MergedPendingRelease bool
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
// for `yeet release` (dry-run and non-dry-run). The server is closed via
// t.Cleanup.
func NewGitLab(t *testing.T, opts GitLabOptions) *httptest.Server {
	t.Helper()

	pid := url.PathEscape(opts.Project)
	prefix := "/api/v4/projects/" + pid

	mux := http.NewServeMux()

	registerGitLabHistory(mux, prefix, opts)
	registerGitLabMerge(mux, prefix, opts)
	registerGitLabContent(mux, prefix, opts)
	registerGitLabLabels(mux, prefix)
	registerGitLabReleases(mux, prefix)
	registerGitLabProject(mux, prefix)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fakeprovider/gitlab: unexpected request %s %s", r.Method, r.URL.String())
		http.Error(w, "unhandled", http.StatusNotImplemented)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func registerGitLabHistory(mux *http.ServeMux, prefix string, opts GitLabOptions) {
	mux.HandleFunc("GET "+prefix+"/repository/tags", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabTagsPayload(opts.LatestTag))
	})

	mux.HandleFunc("GET "+prefix+"/repository/commits", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabCommitsList(opts.Commits))
	})

	mux.HandleFunc("GET "+prefix+"/repository/commits/{ref}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, gitlabCommitDetail(r.PathValue("ref"), opts))
	})

	mux.HandleFunc(
		"GET "+prefix+"/repository/commits/{sha}/merge_requests",
		func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, []any{})
		},
	)
}

func registerGitLabMerge(mux *http.ServeMux, prefix string, opts GitLabOptions) {
	mux.HandleFunc("GET "+prefix+"/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") == fakeStateMerged && opts.MergedPendingRelease {
			writeJSON(w, []map[string]any{gitlabMergedPendingMR()})

			return
		}

		writeJSON(w, []any{})
	})

	mux.HandleFunc("POST "+prefix+"/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabFakeMR())
	})

	mux.HandleFunc("GET "+prefix+"/merge_requests/{iid}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabFakeMR())
	})

	mux.HandleFunc("PUT "+prefix+"/merge_requests/{iid}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabFakeMR())
	})

	mux.HandleFunc("PUT "+prefix+"/merge_requests/{iid}/merge", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabFakeMR())
	})

	mux.HandleFunc("POST "+prefix+"/repository/branches", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{gitlabKeyName: fakeReleaseBranch, "commit": map[string]any{gitlabKeyID: fakeBaseSHA}})
	})

	mux.HandleFunc("POST "+prefix+"/repository/commits", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{gitlabKeyID: "new-commit-sha"})
	})
}

func registerGitLabContent(mux *http.ServeMux, prefix string, opts GitLabOptions) {
	mux.HandleFunc(
		"GET "+prefix+"/repository/files/{path...}",
		func(w http.ResponseWriter, r *http.Request) {
			// Strip a trailing "/raw" suffix from the path captured by the wildcard.
			path := r.PathValue("path")
			if len(path) > len("/raw") && path[len(path)-len("/raw"):] == "/raw" {
				path = path[:len(path)-len("/raw")]
			}

			if content, ok := opts.Files[path]; ok {
				_, _ = w.Write([]byte(content))

				return
			}

			if path == "CHANGELOG.md" {
				_, _ = w.Write([]byte("## Changelog\n\n## [v1.1.0]\n\n* feat: add a thing\n"))

				return
			}

			http.Error(w, "not found", http.StatusNotFound)
		},
	)
}

func registerGitLabLabels(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/labels/{name}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{gitlabKeyName: r.PathValue(gitlabKeyName)})
	})

	mux.HandleFunc("POST "+prefix+"/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{gitlabKeyName: "label"})
	})
}

func registerGitLabReleases(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/releases", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})

	mux.HandleFunc("GET "+prefix+"/releases/{tag}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	mux.HandleFunc("POST "+prefix+"/releases", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"tag_name": fakeNextTag,
			"_links":   map[string]any{"self": "https://example.test/releases/v1.1.0"},
		})
	})

	mux.HandleFunc("GET "+prefix+"/repository/tags/{tag}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
}

func registerGitLabProject(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			gitlabKeyID:                             gitlabFakeMRID,
			"merge_method":                          "merge",
			"only_allow_merge_if_pipeline_succeeds": false,
		})
	})
}

const gitlabFakeMRID = 42

const gitlabReleaseManifest = "<!-- yeet-release-manifest\n" +
	`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}` +
	"\n-->"

func gitlabMergedPendingMR() map[string]any {
	mr := gitlabFakeMR()
	mr["state"] = "merged"
	mr["merged_at"] = "2026-01-01T00:00:00Z"
	mr["description"] = "## ٩(^ᴗ^)۶ release created\n\n" + gitlabReleaseManifest + "\n"
	mr["labels"] = []string{fakePendingReleaseTag}

	return mr
}

func gitlabFakeMR() map[string]any {
	return map[string]any{
		"iid":              gitlabFakeMRID,
		gitlabKeyID:        gitlabFakeMRID,
		"state":            "opened",
		"merge_status":     "can_be_merged",
		"web_url":          "https://example.test/mr/42",
		"source_branch":    fakeReleaseBranch,
		"target_branch":    fakeBaseBranch,
		"draft":            false,
		"work_in_progress": false,
		"sha":              "head-sha",
		"merge_commit_sha": fakeMergeSHA,
	}
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
