package fakeprovider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

type GitLabOptions struct {
	Project                   string
	BranchHeadSHA             string
	LatestTag                 string
	BoundarySHA               string
	Commits                   []GitLabCommit
	Files                     map[string]string
	MergedPendingRelease      bool
	AsynchronousMerge         bool
	FastForwardMerge          bool
	MultipleOpenPRs           bool
	MergeBlocked              bool
	ExistingOpenReleasePRBody string
	Users                     map[string]int64
	ExistingLabels            []string
	UnlabeledOpenReleaseMR    bool
	ForeignLabelOpenReleaseMR bool
}

// GitLabCommit is a tiny subset of the GitLab commit payload that yeet reads.
type GitLabCommit struct {
	SHA              string
	Message          string
	Files            []string
	AssociatedPRBody string
}

const (
	gitlabKeyID       = "id"
	gitlabKeyCommit   = "commit"
	gitlabKeyMessage  = "message"
	gitlabKeyName     = "name"
	gitlabKeyTagName  = "tag_name"
	gitlabStateOpened = "opened"
)

// NewGitLab starts the GitLab REST fake and registers its cleanup with t.
func NewGitLab(t *testing.T, opts GitLabOptions) *httptest.Server {
	t.Helper()

	pid := url.PathEscape(opts.Project)
	prefix := "/api/v4/projects/" + pid

	mux := http.NewServeMux()
	mergeAccepted := &atomic.Bool{}
	merged := &atomic.Bool{}

	registerGitLabHistory(mux, prefix, opts)
	registerGitLabMergeBase(mux, prefix, opts)
	registerGitLabMerge(mux, prefix, opts, mergeAccepted, merged)
	registerGitLabMembers(mux, prefix, opts)
	registerGitLabContent(mux, prefix, opts)
	registerGitLabLabels(t, mux, prefix, opts)
	registerGitLabReleases(mux, prefix, opts, merged)
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
		writeJSON(w, gitlabTagsPayload(opts.LatestTag, opts.BoundarySHA))
	})

	mux.HandleFunc("GET "+prefix+"/repository/commits", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabCommitsList(opts.Commits))
	})

	mux.HandleFunc("GET "+prefix+"/repository/branches/{branch}", gitlabBranchHeadHandler(opts))
	mux.HandleFunc("GET "+prefix+"/repository/compare", gitlabCompareHandler(opts))
	mux.HandleFunc("GET "+prefix+"/repository/commits/{ref}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, gitlabCommitDetail(r.PathValue("ref"), opts))
	})

	mux.HandleFunc(
		"GET "+prefix+"/repository/commits/{sha}/merge_requests",
		func(w http.ResponseWriter, r *http.Request) {
			sha := r.PathValue("sha")
			for _, c := range opts.Commits {
				if c.SHA != sha || c.AssociatedPRBody == "" {
					continue
				}

				mr := gitlabFakeMR()
				mr["state"] = "merged"
				mr["merge_commit_sha"] = sha
				mr["description"] = c.AssociatedPRBody

				writeJSON(w, []map[string]any{mr})

				return
			}

			writeJSON(w, []any{})
		},
	)

	mux.HandleFunc("GET "+prefix+"/repository/commits/{sha}/diff", func(w http.ResponseWriter, r *http.Request) {
		sha := r.PathValue("sha")

		for _, c := range opts.Commits {
			if c.SHA != sha {
				continue
			}

			diffs := make([]map[string]any, 0, len(c.Files))
			for _, path := range c.Files {
				diffs = append(diffs, map[string]any{
					"old_path": path,
					"new_path": path,
				})
			}

			writeJSON(w, diffs)

			return
		}

		writeJSON(w, []any{})
	})
}

// gitlabBranchHeadHandler falls back to the newest commit, then the base SHA.
func gitlabBranchHeadHandler(opts GitLabOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		headSHA := opts.BranchHeadSHA
		if headSHA == "" && len(opts.Commits) > 0 {
			headSHA = opts.Commits[0].SHA
		}

		if headSHA == "" {
			headSHA = fakeBaseSHA
		}

		writeJSON(w, map[string]any{
			gitlabKeyName:   r.PathValue("branch"),
			gitlabKeyCommit: map[string]any{gitlabKeyID: headSHA},
		})
	}
}

func registerGitLabMergeBase(mux *http.ServeMux, prefix string, opts GitLabOptions) {
	mux.HandleFunc("GET "+prefix+"/repository/merge_base", gitlabMergeBaseHandler(opts))
}

func registerGitLabMerge(
	mux *http.ServeMux,
	prefix string,
	opts GitLabOptions,
	mergeAccepted, merged *atomic.Bool,
) {
	mux.HandleFunc(
		"GET "+prefix+"/merge_requests",
		gitlabMergeRequestListHandler(opts, mergeAccepted, merged),
	)

	mux.HandleFunc("POST "+prefix+"/merge_requests", handleGitLabCreateMR)

	mux.HandleFunc("GET "+prefix+"/merge_requests/{iid}", func(w http.ResponseWriter, _ *http.Request) {
		if opts.AsynchronousMerge && mergeAccepted.Load() {
			merged.Store(true)

			writeJSON(w, gitlabMergedPendingMR(opts))

			return
		}

		mr := gitlabFakeMR()
		if opts.MergeBlocked {
			mr["draft"] = true
		}

		writeJSON(w, mr)
	})

	mux.HandleFunc("PUT "+prefix+"/merge_requests/{iid}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabFakeMR())
	})

	mux.HandleFunc(
		"PUT "+prefix+"/merge_requests/{iid}/merge",
		gitlabMergeAcceptHandler(opts, mergeAccepted, merged),
	)

	mux.HandleFunc("POST "+prefix+"/repository/branches", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, gitlabCreatedBranch())
	})

	mux.HandleFunc("POST "+prefix+"/repository/commits", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{gitlabKeyID: "new-commit-sha"})
	})
}

func gitlabMergeRequestListHandler(
	opts GitLabOptions,
	mergeAccepted, merged *atomic.Bool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")

		if state == fakeStateMerged && opts.AsynchronousMerge && mergeAccepted.Load() {
			merged.Store(true)
		}

		if state == fakeStateMerged && (opts.MergedPendingRelease || merged.Load()) {
			writeJSON(w, []map[string]any{gitlabMergedPendingMR(opts)})

			return
		}

		if state == gitlabStateOpened && opts.MultipleOpenPRs {
			const (
				firstIID  = 43
				secondIID = 44
			)

			writeJSON(w, []map[string]any{gitlabPendingMR(firstIID), gitlabPendingMR(secondIID)})

			return
		}

		if state == gitlabStateOpened && opts.ExistingOpenReleasePRBody != "" {
			mr := gitlabFakeMR()
			mr["description"] = opts.ExistingOpenReleasePRBody

			if opts.UnlabeledOpenReleaseMR {
				mr["labels"] = []string{}
			}

			if opts.ForeignLabelOpenReleaseMR {
				mr["labels"] = []string{"needs-triage"}
			}

			if !gitlabMRMatchesLabelFilter(mr, r.URL.Query().Get("labels")) {
				writeJSON(w, []any{})

				return
			}

			writeJSON(w, []map[string]any{mr})

			return
		}

		writeJSON(w, []any{})
	}
}

func gitlabMRMatchesLabelFilter(mr map[string]any, filter string) bool {
	if filter == "" {
		return true
	}

	labels, _ := mr["labels"].([]string)

	for wanted := range strings.SplitSeq(filter, ",") {
		if !slices.Contains(labels, wanted) {
			return false
		}
	}

	return true
}

func gitlabMergeAcceptHandler(
	opts GitLabOptions,
	mergeAccepted, merged *atomic.Bool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		mergeAccepted.Store(true)

		if opts.AsynchronousMerge {
			mr := gitlabFakeMR()
			delete(mr, "merge_commit_sha")
			writeJSON(w, mr)

			return
		}

		merged.Store(true)
		writeJSON(w, gitlabMergedPendingMR(opts))
	}
}

func gitlabCreatedBranch() map[string]any {
	return map[string]any{
		gitlabKeyName:   fakeReleaseBranch,
		gitlabKeyCommit: map[string]any{gitlabKeyID: fakeBaseSHA},
	}
}

// handleGitLabCreateMR echoes every requested reviewer as applied.
func handleGitLabCreateMR(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ReviewerIDs []int64 `json:"reviewer_ids"`
	}

	_ = json.NewDecoder(r.Body).Decode(&request)

	mr := gitlabFakeMR()

	if len(request.ReviewerIDs) > 0 {
		reviewers := make([]map[string]any, 0, len(request.ReviewerIDs))
		for _, id := range request.ReviewerIDs {
			reviewers = append(reviewers, map[string]any{gitlabKeyID: id})
		}

		mr["reviewers"] = reviewers
	}

	writeJSON(w, mr)
}

func registerGitLabMembers(mux *http.ServeMux, prefix string, opts GitLabOptions) {
	mux.HandleFunc("GET "+prefix+"/members/all", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")

		id, exists := opts.Users[query]
		if !exists {
			writeJSON(w, []any{})

			return
		}

		writeJSON(w, []map[string]any{{gitlabKeyID: id, "username": query}})
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

func registerGitLabLabels(t *testing.T, mux *http.ServeMux, prefix string, opts GitLabOptions) {
	t.Helper()

	mux.HandleFunc("GET "+prefix+"/labels/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue(gitlabKeyName)

		if !slices.Contains(opts.ExistingLabels, name) {
			http.Error(w, "not found", http.StatusNotFound)

			return
		}

		writeJSON(w, map[string]any{gitlabKeyName: name})
	})

	mux.HandleFunc("POST "+prefix+"/labels", func(w http.ResponseWriter, r *http.Request) {
		if name := readJSONString(t, r, gitlabKeyName); slices.Contains(opts.ExistingLabels, name) {
			t.Errorf("fakeprovider/gitlab: recreated existing label %q", name)
			http.Error(w, "label already exists", http.StatusUnprocessableEntity)

			return
		}

		writeJSON(w, map[string]any{gitlabKeyName: "label"})
	})
}

func registerGitLabReleases(
	mux *http.ServeMux,
	prefix string,
	opts GitLabOptions,
	merged *atomic.Bool,
) {
	mux.HandleFunc("GET "+prefix+"/releases", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})

	mux.HandleFunc("GET "+prefix+"/releases/{tag}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	mux.HandleFunc("POST "+prefix+"/releases", func(w http.ResponseWriter, r *http.Request) {
		if opts.AsynchronousMerge && !merged.Load() {
			http.Error(w, "merge not completed", http.StatusConflict)

			return
		}

		if opts.FastForwardMerge {
			var request struct {
				Ref string `json:"ref"`
			}

			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, "invalid release request", http.StatusBadRequest)

				return
			}

			if request.Ref != opts.BranchHeadSHA {
				http.Error(w, "release ref is not the fast-forward commit", http.StatusConflict)

				return
			}
		}

		writeJSON(w, map[string]any{
			gitlabKeyTagName: fakeNextTag,
			"_links":         map[string]any{"self": "https://example.test/releases/v1.1.0"},
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

func gitlabMergedPendingMR(opts GitLabOptions) map[string]any {
	mr := gitlabFakeMR()
	mr["state"] = "merged"
	mr["merged_at"] = fakeMergedAtTimestamp
	mr["description"] = "## ٩(^ᴗ^)۶ release created\n\n" + gitlabReleaseManifest + "\n"
	mr["labels"] = []string{fakePendingReleaseTag}

	if opts.FastForwardMerge {
		delete(mr, "merge_commit_sha")
		mr["sha"] = opts.BranchHeadSHA
	}

	return mr
}

func gitlabFakeMR() map[string]any {
	return gitlabPendingMR(gitlabFakeMRID)
}

func gitlabPendingMR(iid int) map[string]any {
	return map[string]any{
		"iid":               iid,
		gitlabKeyID:         iid,
		"state":             gitlabStateOpened,
		"merge_status":      "can_be_merged",
		"web_url":           "https://example.test/mr/42",
		"source_branch":     fakeReleaseBranch,
		"target_branch":     fakeBaseBranch,
		"source_project_id": gitlabFakeMRID,
		"target_project_id": gitlabFakeMRID,
		"draft":             false,
		"work_in_progress":  false,
		"sha":               "head-sha",
		"merge_commit_sha":  fakeMergeSHA,
		"labels":            []string{fakePendingReleaseTag},
	}
}

func gitlabTagsPayload(tag, commitHash string) any {
	if tag == "" {
		return []any{}
	}

	return []map[string]any{
		{
			gitlabKeyName:   tag,
			gitlabKeyCommit: map[string]any{gitlabKeyID: commitHash},
		},
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

// gitlabCompareHandler returns commits ahead of the boundary.
func gitlabCompareHandler(opts GitLabOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boundarySHA, ok := gitlabResolveRefSHA(r.URL.Query().Get("from"), opts)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)

			return
		}

		writeJSON(w, map[string]any{
			keyCommits:         gitlabCommitsList(gitlabCommitsSince(opts.Commits, boundarySHA)),
			"compare_timeout":  false,
			"compare_same_ref": false,
		})
	}
}

func gitlabMergeBaseHandler(opts GitLabOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		refs := r.URL.Query()["refs[]"]
		if len(refs) == 0 {
			http.Error(w, "refs are required", http.StatusBadRequest)

			return
		}

		boundarySHA, ok := gitlabResolveRefSHA(refs[0], opts)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)

			return
		}

		writeJSON(w, map[string]any{gitlabKeyID: boundarySHA})
	}
}

func gitlabResolveRefSHA(ref string, opts GitLabOptions) (string, bool) {
	if ref != "" && ref == opts.LatestTag {
		return opts.BoundarySHA, true
	}

	for _, c := range opts.Commits {
		if c.SHA == ref {
			return c.SHA, true
		}
	}

	return "", false
}

// gitlabCommitsSince returns the commits ahead of the boundary in the
// newest-first list (the boundary commit and anything older are dropped). A
// boundary absent from the list is treated as older than every listed commit.
func gitlabCommitsSince(commits []GitLabCommit, boundarySHA string) []GitLabCommit {
	for idx, c := range commits {
		if c.SHA == boundarySHA {
			return commits[:idx]
		}
	}

	return commits
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
