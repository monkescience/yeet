// Package fakeprovider exposes httptest-backed provider stubs that yeet
// blackbox tests point at via the GITHUB_URL / GITLAB_URL / AZURE_DEVOPS_URL
// env vars.
package fakeprovider

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

type GitHubOptions struct {
	Owner                     string
	Repo                      string
	BranchHeadSHA             string
	LatestTag                 string
	ExtraTags                 []string
	BoundarySHA               string
	TagSHAs                   map[string]string
	Commits                   []GitHubCommit
	MergedPendingRelease      bool
	Files                     map[string]string
	MultipleOpenPRs           bool
	MergeBlocked              bool
	ExistingOpenReleasePRBody string
	ExistingRelease           bool
	PaginateCommits           bool
	FailOnMutation            bool
	Collaborators             map[string]bool
}

// GitHubCommit is a tiny subset of the GitHub commit payload that yeet reads.
type GitHubCommit struct {
	SHA              string
	Message          string
	Files            []string
	AssociatedPRBody string
}

const (
	githubKeySHA     = "sha"
	githubKeyMessage = "message"
	githubKeyCommit  = "commit"
	githubKeyRef     = "ref"
	githubKeyObject  = "object"
	githubKeyName    = "name"
	githubKeyType    = "type"
	githubKeyTagName = "tag_name"
	githubKeyHTMLURL = "html_url"
	githubKeyNumber  = "number"
	githubFakePRID   = 42
	githubChangelog  = "CHANGELOG.md"
)

// NewGitHub starts the GitHub REST fake and registers its cleanup with t.
func NewGitHub(t *testing.T, opts GitHubOptions) *httptest.Server {
	t.Helper()

	prefix := "/api/v3/repos/" + opts.Owner + "/" + opts.Repo

	mux := http.NewServeMux()
	merged := &atomic.Bool{}
	reviewersRequested := &atomic.Bool{}

	registerGitHubReleases(mux, prefix, opts)
	registerGitHubHistory(mux, prefix, opts)
	registerGitHubSearch(mux, opts, merged)
	registerGitHubPullsRead(mux, prefix, opts)
	registerGitHubWritePath(mux, prefix, opts, merged, reviewersRequested)
	registerGitHubUser(mux)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fakeprovider/github: unexpected request %s %s", r.Method, r.URL.String())
		http.Error(w, "unhandled", http.StatusNotImplemented)
	})

	var handler http.Handler = mux
	if opts.FailOnMutation {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
				t.Errorf("fakeprovider/github: unexpected mutation %s %s", r.Method, r.URL.String())
				http.Error(w, "mutation rejected", http.StatusInternalServerError)

				return
			}

			mux.ServeHTTP(w, r)
		})
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return server
}

func registerGitHubSearch(mux *http.ServeMux, opts GitHubOptions, merged *atomic.Bool) {
	mux.HandleFunc("GET /api/v3/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		items := []map[string]any{}
		if opts.MergedPendingRelease || merged.Load() {
			items = append(items, map[string]any{githubKeyNumber: githubFakePRID})
		}

		writeJSON(w, map[string]any{
			"total_count":        len(items),
			"incomplete_results": false,
			"items":              items,
		})
	})
}

func registerGitHubReleases(mux *http.ServeMux, prefix string, opts GitHubOptions) {
	mux.HandleFunc("GET "+prefix+"/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no release", http.StatusNotFound)
	})

	mux.HandleFunc("GET "+prefix+"/releases/tags/{tag}", func(w http.ResponseWriter, r *http.Request) {
		if opts.ExistingRelease {
			writeJSON(w, map[string]any{
				"id":             githubFakePRID,
				githubKeyTagName: r.PathValue("tag"),
				"name":           r.PathValue("tag"),
				"body":           "existing release notes",
				githubKeyHTMLURL: "https://example.test/releases/" + r.PathValue("tag"),
			})

			return
		}

		http.Error(w, "no release", http.StatusNotFound)
	})

	mux.HandleFunc("POST "+prefix+"/releases", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"id":               githubFakePRID,
			githubKeyTagName:   fakeNextTag,
			githubKeyHTMLURL:   "https://example.test/releases/v1.1.0",
			"target_commitish": fakeBaseBranch,
		})
	})

	mux.HandleFunc("POST "+prefix+"/git/tags", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{githubKeySHA: "tag-object-sha", "tag": fakeNextTag})
	})
}

func registerGitHubHistory(mux *http.ServeMux, prefix string, opts GitHubOptions) {
	mux.HandleFunc("GET "+prefix+"/tags", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubTagsPayload(opts))
	})

	mux.HandleFunc("GET "+prefix+"/commits", func(w http.ResponseWriter, r *http.Request) {
		if opts.PaginateCommits && len(opts.Commits) > 1 {
			if r.URL.Query().Get("page") != "2" {
				w.Header().Set("Link", `<https://api.github.com/?page=2>; rel="next"`)
				writeJSON(w, githubCommitsList(opts.Commits[:1]))

				return
			}

			writeJSON(w, githubCommitsList(opts.Commits[1:]))

			return
		}

		writeJSON(w, githubCommitsList(opts.Commits))
	})

	mux.HandleFunc("GET "+prefix+"/compare/{spec...}", githubCompareHandler(opts))

	// The wildcard also serves GetBranchHead's two-segment "heads/{branch}"
	// ref, which resolves to the newest fake commit. The more specific
	// "/commits/{sha}/pulls" route still wins for PR-body lookups.
	mux.HandleFunc("GET "+prefix+"/commits/{ref...}", func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("ref")

		if strings.HasPrefix(ref, "heads/") {
			writeJSON(w, map[string]any{githubKeySHA: githubHeadSHA(opts)})

			return
		}

		writeJSON(w, githubCommitDetail(ref, opts))
	})
}

// githubCompareHandler returns commits ahead of the boundary, oldest first.
func githubCompareHandler(opts GitHubOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base, _, _ := strings.Cut(r.PathValue("spec"), "...")

		boundarySHA, ok := githubResolveRefSHA(base, opts)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)

			return
		}

		ahead, reachable := githubCommitsAhead(opts.Commits, boundarySHA)
		if !reachable {
			// Boundary exists but is not an ancestor of the branch.
			writeJSON(w, githubComparisonPayload(nil, 0, "diverged"))

			return
		}

		slices.Reverse(ahead) // compare returns oldest-first

		if opts.PaginateCommits && r.URL.Query().Get("page") != "2" {
			w.Header().Set("Link", `<https://api.github.com/?page=2>; rel="next"`)
			writeJSON(w, githubComparisonPayload(nil, len(ahead), "ahead"))

			return
		}

		writeJSON(w, githubComparisonPayload(ahead, len(ahead), "ahead"))
	}
}

func githubResolveRefSHA(ref string, opts GitHubOptions) (string, bool) {
	if ref == opts.LatestTag || slices.Contains(opts.ExtraTags, ref) {
		return opts.BoundarySHA, true
	}

	for _, c := range opts.Commits {
		if c.SHA == ref {
			return c.SHA, true
		}
	}

	return "", false
}

// githubCommitsAhead returns the commits ahead of the boundary in the
// newest-first list (the boundary commit and anything older are dropped). A
// boundary absent from the list models an off-branch ref: not reachable.
func githubCommitsAhead(commits []GitHubCommit, boundarySHA string) ([]GitHubCommit, bool) {
	for idx, c := range commits {
		if c.SHA == boundarySHA {
			return slices.Clone(commits[:idx]), true
		}
	}

	return nil, false
}

func githubComparisonPayload(commits []GitHubCommit, total int, status string) map[string]any {
	return map[string]any{
		"status":        status,
		"ahead_by":      total,
		"total_commits": total,
		keyCommits:      githubCommitsList(commits),
	}
}

func registerGitHubPullsRead(mux *http.ServeMux, prefix string, opts GitHubOptions) {
	mux.HandleFunc("GET "+prefix+"/commits/{sha}/pulls", func(w http.ResponseWriter, r *http.Request) {
		sha := r.PathValue("sha")
		for _, c := range opts.Commits {
			if c.SHA == sha && c.AssociatedPRBody != "" {
				writeJSON(w, []map[string]any{{
					"number":           fakeAssociatedPRID,
					"merged_at":        fakeMergedAtTimestamp,
					"body":             c.AssociatedPRBody,
					"head":             map[string]any{githubKeySHA: sha},
					"merge_commit_sha": sha,
					fakeStateMerged:    true,
				}})

				return
			}
		}

		writeJSON(w, []any{})
	})

	mux.HandleFunc("GET "+prefix+"/pulls", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")

		if state == fakeStateOpen && opts.MultipleOpenPRs {
			const (
				firstID  = 43
				secondID = 44
			)

			writeJSON(w, []map[string]any{githubPendingPR(opts, firstID), githubPendingPR(opts, secondID)})

			return
		}

		if state == fakeStateOpen && opts.ExistingOpenReleasePRBody != "" {
			pr := githubPendingPR(opts, githubFakePRID)
			pr["body"] = opts.ExistingOpenReleasePRBody

			writeJSON(w, []map[string]any{pr})

			return
		}

		writeJSON(w, []any{})
	})
}

func registerGitHubUser(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v3/user", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"login":       "yeet-bot",
			githubKeyName: "yeet-bot",
			"email":       "yeet-bot@example.test",
		})
	})
}

func githubTagsPayload(opts GitHubOptions) any {
	tags := make([]map[string]any, 0, 1+len(opts.ExtraTags))

	if opts.LatestTag != "" {
		tags = append(tags, map[string]any{
			githubKeyName:   opts.LatestTag,
			githubKeyCommit: map[string]any{githubKeySHA: githubTagSHA(opts, opts.LatestTag)},
		})
	}

	for _, tag := range opts.ExtraTags {
		tags = append(tags, map[string]any{
			githubKeyName:   tag,
			githubKeyCommit: map[string]any{githubKeySHA: githubTagSHA(opts, tag)},
		})
	}

	if len(tags) == 0 {
		return []any{}
	}

	return tags
}

func githubTagSHA(opts GitHubOptions, tag string) string {
	if commitHash := opts.TagSHAs[tag]; commitHash != "" {
		return commitHash
	}

	return opts.BoundarySHA
}

// githubHeadSHA falls back from BranchHeadSHA to the newest commit, then the base SHA.
func githubHeadSHA(opts GitHubOptions) string {
	if opts.BranchHeadSHA != "" {
		return opts.BranchHeadSHA
	}

	if len(opts.Commits) > 0 {
		return opts.Commits[0].SHA
	}

	return fakeBaseSHA
}

func githubCommitDetail(ref string, opts GitHubOptions) map[string]any {
	if ref == opts.LatestTag {
		return map[string]any{githubKeySHA: opts.BoundarySHA}
	}

	if slices.Contains(opts.ExtraTags, ref) {
		return map[string]any{githubKeySHA: opts.BoundarySHA}
	}

	for _, c := range opts.Commits {
		if c.SHA == ref {
			return map[string]any{
				githubKeySHA:    c.SHA,
				githubKeyCommit: map[string]any{githubKeyMessage: c.Message},
				"files":         githubFilesPayload(c.Files),
			}
		}
	}

	return map[string]any{githubKeySHA: ref}
}

func githubFilesPayload(paths []string) []map[string]any {
	out := make([]map[string]any, 0, len(paths))

	for _, path := range paths {
		out = append(out, map[string]any{"filename": path})
	}

	return out
}

// registerGitHubWritePath attaches the handlers used by non-dry-run releases.
func registerGitHubWritePath(
	mux *http.ServeMux,
	prefix string,
	opts GitHubOptions,
	merged, reviewersRequested *atomic.Bool,
) {
	registerGitHubGitData(mux, prefix)
	registerGitHubContent(mux, prefix, opts)
	registerGitHubPullsWrite(mux, prefix, opts, merged, reviewersRequested)
	registerGitHubLabels(mux, prefix, opts, reviewersRequested)
	registerGitHubCollaborators(mux, prefix, opts)

	mux.HandleFunc("GET "+prefix, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"allow_squash_merge": true,
			"allow_rebase_merge": true,
			"allow_merge_commit": true,
		})
	})
}

func registerGitHubGitData(mux *http.ServeMux, prefix string) {
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

func registerGitHubContent(mux *http.ServeMux, prefix string, opts GitHubOptions) {
	mux.HandleFunc("GET "+prefix+"/contents/{path...}", func(w http.ResponseWriter, r *http.Request) {
		path := r.PathValue("path")

		if content, ok := opts.Files[path]; ok {
			writeJSON(w, githubFileContent(path, content))

			return
		}

		if path == githubChangelog {
			writeJSON(w, githubFileContent(githubChangelog, "## Changelog\n\n## [v1.1.0]\n\n* feat: add a thing\n"))

			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	})
}

func githubFileContent(path, raw string) map[string]any {
	return map[string]any{
		githubKeyName: path,
		"path":        path,
		githubKeyType: "file",
		"encoding":    "base64",
		"content":     base64.StdEncoding.EncodeToString([]byte(raw)),
		githubKeySHA:  "blob-sha",
	}
}

func registerGitHubPullsWrite(
	mux *http.ServeMux,
	prefix string,
	opts GitHubOptions,
	merged, reviewersRequested *atomic.Bool,
) {
	mux.HandleFunc("POST "+prefix+"/pulls", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubFakePR(opts))
	})

	mux.HandleFunc("PATCH "+prefix+"/pulls/{number}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubFakePR(opts))
	})

	mux.HandleFunc("GET "+prefix+"/pulls/{number}", func(w http.ResponseWriter, _ *http.Request) {
		if opts.MergedPendingRelease || merged.Load() {
			writeJSON(w, githubMergedPendingPR(opts))

			return
		}

		pr := githubFakePR(opts)
		if opts.MergeBlocked {
			pr["draft"] = true
		}

		writeJSON(w, pr)
	})

	mux.HandleFunc("GET "+prefix+"/pulls/{number}/files", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})

	mux.HandleFunc("PUT "+prefix+"/pulls/{number}/merge", func(w http.ResponseWriter, _ *http.Request) {
		merged.Store(true)
		writeJSON(w, map[string]any{fakeStateMerged: true, githubKeySHA: fakeMergeSHA})
	})

	mux.HandleFunc(
		"POST "+prefix+"/pulls/{number}/requested_reviewers",
		githubRequestReviewersHandler(opts, reviewersRequested),
	)
}

func githubRequestReviewersHandler(opts GitHubOptions, reviewersRequested *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Reviewers []string `json:"reviewers"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid reviewer request", http.StatusBadRequest)

			return
		}

		for _, reviewer := range request.Reviewers {
			if !opts.Collaborators[reviewer] {
				http.Error(w, "reviewer is not a collaborator", http.StatusUnprocessableEntity)

				return
			}
		}

		reviewersRequested.Store(true)
		writeJSON(w, githubFakePR(opts))
	}
}

func registerGitHubCollaborators(mux *http.ServeMux, prefix string, opts GitHubOptions) {
	mux.HandleFunc("GET "+prefix+"/collaborators/{username}", func(w http.ResponseWriter, r *http.Request) {
		if !opts.Collaborators[r.PathValue("username")] {
			http.Error(w, "not found", http.StatusNotFound)

			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

func registerGitHubLabels(
	mux *http.ServeMux,
	prefix string,
	opts GitHubOptions,
	reviewersRequested *atomic.Bool,
) {
	mux.HandleFunc("GET "+prefix+"/labels/{name}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	mux.HandleFunc("POST "+prefix+"/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{githubKeyName: "label"})
	})

	mux.HandleFunc("POST "+prefix+"/issues/{number}/labels", func(w http.ResponseWriter, _ *http.Request) {
		if len(opts.Collaborators) > 0 && !reviewersRequested.Load() {
			http.Error(w, "reviewers were not requested", http.StatusConflict)

			return
		}

		writeJSON(w, []any{})
	})

	mux.HandleFunc("DELETE "+prefix+"/issues/{number}/labels/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// githubReleaseManifest returns the manifest embedded in a merged release PR.
const githubReleaseManifest = "<!-- yeet-release-manifest\n" +
	`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}` +
	"\n-->"

func githubMergedPendingPR(opts GitHubOptions) map[string]any {
	pr := githubFakePR(opts)
	pr["state"] = "closed"
	pr["merged"] = true
	pr["merged_at"] = fakeMergedAtTimestamp
	pr["merge_commit_sha"] = fakeMergeSHA
	pr["body"] = "## ٩(^ᴗ^)۶ release created\n\n" + githubReleaseManifest + "\n\n* feat: add a thing\n"
	pr["labels"] = []map[string]any{
		{githubKeyName: fakePendingReleaseTag},
	}

	return pr
}

func githubFakePR(opts GitHubOptions) map[string]any {
	return githubPendingPR(opts, githubFakePRID)
}

func githubPendingPR(opts GitHubOptions, number int) map[string]any {
	return map[string]any{
		"number":          number,
		"state":           fakeStateOpen,
		"draft":           false,
		fakeStateMerged:   false,
		"mergeable_state": "clean",
		githubKeyHTMLURL:  "https://example.test/pulls/42",
		"head": map[string]any{
			githubKeyRef: fakeReleaseBranch,
			githubKeySHA: "head-sha",
			"repo": map[string]any{
				"full_name": opts.Owner + "/" + opts.Repo,
			},
		},
		"base":   map[string]any{githubKeyRef: fakeBaseBranch},
		"labels": []map[string]any{{githubKeyName: fakePendingReleaseTag}},
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
