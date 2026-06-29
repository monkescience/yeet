// Package fakeprovider exposes httptest-backed provider stubs that yeet
// blackbox tests point at via the GITHUB_URL / GITLAB_URL / AZURE_DEVOPS_URL
// env vars.
package fakeprovider

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

type GitHubOptions struct {
	Owner string
	Repo  string
	// LatestTag is the most recent tag returned by the tags-fallback. When
	// empty, the server reports no tags and no latest release.
	LatestTag string
	// ExtraTags are additional historical tags returned alongside LatestTag
	// from the tags-fallback. Used to drive the multi-ref ordering paths.
	ExtraTags []string
	// BoundarySHA is the SHA of the commit pointed at by LatestTag.
	BoundarySHA string
	// Commits are returned (newest first) from the commits listing for the
	// release branch. The last entry should point at BoundarySHA so yeet can
	// terminate the walk.
	Commits []GitHubCommit
	// MergedPendingRelease toggles the merged-release-PR-waiting-for-tagging
	// fixture: when true, the closed-pulls listing returns one merged PR with
	// the autorelease:pending label so yeet enters the finalization path.
	MergedPendingRelease bool
	// Files maps repository-relative paths to their raw content. The contents
	// endpoint serves these (base64-encoded) for matching paths. Paths not in
	// the map return 404 except for CHANGELOG.md, which always has a default
	// response.
	Files map[string]string
	// MultipleOpenPRs returns two pending release PRs from the open-pulls
	// listing to drive yeet down the ErrMultiplePendingReleasePRs path.
	MultipleOpenPRs bool
	// MergeBlocked makes /pulls/{number} return a draft PR, triggering
	// ErrMergeBlocked on --auto-merge.
	MergeBlocked bool
	// ExistingOpenReleasePRBody, when non-empty, makes the open-pulls listing
	// return a single pending release PR with this body. The body should
	// include the yeet release-manifest marker so yeet recognizes the PR and
	// drives the update-existing-PR workflow.
	ExistingOpenReleasePRBody string
	// ExistingRelease makes release-by-tag lookups return an already-created
	// release so finalization can prove it is idempotent.
	ExistingRelease bool
	// PaginateCommits splits the commit list across two pages so tests can prove
	// release analysis follows provider pagination before finding the boundary.
	PaginateCommits bool
}

// GitHubCommit is a tiny subset of the GitHub commit payload that yeet reads.
type GitHubCommit struct {
	SHA     string
	Message string
	// Files are the changed file paths returned by the commit-detail
	// endpoint when yeet asks for per-commit paths (multi-target mode).
	Files []string
	// AssociatedPRBody, when non-empty, is returned as the body of the merged
	// pull request associated with this commit by /commits/{sha}/pulls. Used
	// to drive the commit-override path (BEGIN_COMMIT_OVERRIDE markers).
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
	githubFakePRID   = 42
	githubChangelog  = "CHANGELOG.md"
)

// NewGitHub starts an httptest.Server serving the minimum set of GitHub REST
// endpoints exercised by `yeet release` against a configured owner/repo.
// The server is stopped via t.Cleanup.
func NewGitHub(t *testing.T, opts GitHubOptions) *httptest.Server {
	t.Helper()

	prefix := "/api/v3/repos/" + opts.Owner + "/" + opts.Repo

	mux := http.NewServeMux()

	registerGitHubReleases(mux, prefix, opts)
	registerGitHubHistory(mux, prefix, opts)
	registerGitHubPullsRead(mux, prefix, opts)
	registerGitHubWritePath(mux, prefix, opts)
	registerGitHubUser(mux)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fakeprovider/github: unexpected request %s %s", r.Method, r.URL.String())
		http.Error(w, "unhandled", http.StatusNotImplemented)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
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
		writeJSON(w, githubTagsPayload(opts.LatestTag, opts.ExtraTags))
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

	mux.HandleFunc("GET "+prefix+"/commits/{ref}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, githubCommitDetail(r.PathValue("ref"), opts))
	})
}

// githubCompareHandler serves base...head as GitHub's compare endpoint does:
// the commits ahead of the boundary, oldest-first. An unknown base answers 404.
// Under PaginateCommits the range is delivered on page 2 behind a next-page
// link, so the test proves yeet follows compare pagination.
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

		if state == "closed" && opts.MergedPendingRelease {
			writeJSON(w, []map[string]any{githubMergedPendingPR()})

			return
		}

		if state == fakeStateOpen && opts.MultipleOpenPRs {
			const (
				firstID  = 43
				secondID = 44
			)

			writeJSON(w, []map[string]any{githubPendingPR(firstID), githubPendingPR(secondID)})

			return
		}

		if state == fakeStateOpen && opts.ExistingOpenReleasePRBody != "" {
			pr := githubPendingPR(githubFakePRID)
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

func githubTagsPayload(tag string, extra []string) any {
	tags := make([]map[string]any, 0, 1+len(extra))

	if tag != "" {
		tags = append(tags, map[string]any{githubKeyName: tag})
	}

	for _, t := range extra {
		tags = append(tags, map[string]any{githubKeyName: t})
	}

	if len(tags) == 0 {
		return []any{}
	}

	return tags
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

// registerGitHubWritePath attaches the handlers exercised by a non-dry-run
// release (creating a release branch, updating files, opening the PR, and
// managing labels). All responses are canned. The test asserts side effects
// via exit code / stdout rather than payload bodies.
func registerGitHubWritePath(mux *http.ServeMux, prefix string, opts GitHubOptions) {
	registerGitHubGitData(mux, prefix)
	registerGitHubContent(mux, prefix, opts)
	registerGitHubPullsWrite(mux, prefix, opts)
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

func registerGitHubPullsWrite(mux *http.ServeMux, prefix string, opts GitHubOptions) {
	mux.HandleFunc("POST "+prefix+"/pulls", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubFakePR())
	})

	mux.HandleFunc("PATCH "+prefix+"/pulls/{number}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, githubFakePR())
	})

	mux.HandleFunc("GET "+prefix+"/pulls/{number}", func(w http.ResponseWriter, _ *http.Request) {
		pr := githubFakePR()
		if opts.MergeBlocked {
			pr["draft"] = true
		}

		writeJSON(w, pr)
	})

	mux.HandleFunc("GET "+prefix+"/pulls/{number}/files", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{})
	})

	mux.HandleFunc("PUT "+prefix+"/pulls/{number}/merge", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{fakeStateMerged: true, githubKeySHA: fakeMergeSHA})
	})
}

func registerGitHubLabels(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/labels/{name}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
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

// githubReleaseManifest is the JSON payload yeet expects embedded inside the
// merged release PR body so it can determine which tag/target to finalize.
const githubReleaseManifest = "<!-- yeet-release-manifest\n" +
	`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}` +
	"\n-->"

func githubMergedPendingPR() map[string]any {
	pr := githubFakePR()
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

func githubFakePR() map[string]any {
	return githubPendingPR(githubFakePRID)
}

func githubPendingPR(number int) map[string]any {
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
