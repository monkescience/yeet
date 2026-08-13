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
	"sync"
	"sync/atomic"
	"testing"

	"github.com/monkescience/testastic"
)

type GitHubOptions struct {
	Owner                     string
	Repo                      string
	BranchHeadSHA             string
	ReleaseBranchMissing      bool
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
	ExistingLabels            []string
	ExpectPRTitle             string
	ExpectPRBodyFile          string
	ExpectCommitSubject       string
}

type labelRegistry struct {
	mu     sync.Mutex
	labels map[string]struct{}
}

func newLabelRegistry(names []string) *labelRegistry {
	labels := make(map[string]struct{}, len(names))
	for _, name := range names {
		labels[name] = struct{}{}
	}

	return &labelRegistry{labels: labels}
}

func (r *labelRegistry) exists(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.labels[name]

	return exists
}

func (r *labelRegistry) create(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.labels[name]; exists {
		return false
	}

	r.labels[name] = struct{}{}

	return true
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
	registerGitHubPullsRead(mux, prefix, opts, merged)
	registerGitHubWritePath(t, mux, prefix, opts, merged, reviewersRequested)
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
		writeJSON(w, map[string]any{githubKeySHA: "7461676f626a6563747368610000000000000000", "tag": fakeNextTag})
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

		if strings.HasPrefix(ref, "tags/") {
			if opts.ExistingRelease {
				writeJSON(w, map[string]any{githubKeySHA: opts.BranchHeadSHA})

				return
			}

			http.Error(w, "not found", http.StatusNotFound)

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

func registerGitHubPullsRead(
	mux *http.ServeMux,
	prefix string,
	opts GitHubOptions,
	merged *atomic.Bool,
) {
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
		if state == "closed" && (opts.MergedPendingRelease || merged.Load()) {
			writeJSON(w, []map[string]any{githubMergedPendingPR(opts)})

			return
		}

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

func registerGitHubWritePath(
	t *testing.T,
	mux *http.ServeMux,
	prefix string,
	opts GitHubOptions,
	merged, reviewersRequested *atomic.Bool,
) {
	t.Helper()

	registerGitHubGitData(t, mux, prefix, opts)
	registerGitHubContent(mux, prefix, opts)
	registerGitHubPullsWrite(t, mux, prefix, opts, merged, reviewersRequested)
	registerGitHubLabels(t, mux, prefix, opts, reviewersRequested)
	registerGitHubCollaborators(mux, prefix, opts)

	mux.HandleFunc("GET "+prefix, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"allow_squash_merge": true,
			"allow_rebase_merge": true,
			"allow_merge_commit": true,
		})
	})
}

func registerGitHubGitData(t *testing.T, mux *http.ServeMux, prefix string, opts GitHubOptions) {
	t.Helper()

	const fakeCommitSHA = "6e6577636f6d6d69747368610000000000000000"

	const fakeTreeSHA = "7472656573686100000000000000000000000000"

	mux.HandleFunc("GET "+prefix+"/git/ref/heads/{branch...}", githubBranchRefHandler(opts))

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
			githubKeySHA:   r.PathValue(githubKeySHA),
			contentKeyTree: map[string]any{githubKeySHA: fakeTreeSHA},
			"parents":      []any{},
		})
	})

	mux.HandleFunc("GET "+prefix+"/git/trees/{sha}", githubTreeHandler(t, opts))

	mux.HandleFunc("POST "+prefix+"/git/trees", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{githubKeySHA: fakeTreeSHA})
	})

	mux.HandleFunc("POST "+prefix+"/git/commits", func(w http.ResponseWriter, r *http.Request) {
		if !expectGitHubFields(t, r, map[string]string{githubKeyMessage: opts.ExpectCommitSubject}) {
			http.Error(w, "unexpected commit message", http.StatusUnprocessableEntity)

			return
		}

		writeJSON(w, map[string]any{
			githubKeySHA:   fakeCommitSHA,
			contentKeyTree: map[string]any{githubKeySHA: fakeTreeSHA},
		})
	})
}

func githubTreeHandler(t *testing.T, opts GitHubOptions) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		testastic.Equal(t, "1", r.URL.Query().Get("recursive"))

		paths := []string{githubChangelog}
		for path := range opts.Files {
			if path != githubChangelog {
				paths = append(paths, path)
			}
		}

		slices.Sort(paths)

		entries := make([]map[string]any, 0, len(paths))
		for _, path := range paths {
			entries = append(entries, map[string]any{
				contentKeyPath: path,
				"mode":         "100644",
				githubKeyType:  "blob",
				githubKeySHA:   "626c6f6273686100000000000000000000000000",
			})
		}

		writeJSON(w, map[string]any{
			githubKeySHA:   r.PathValue(githubKeySHA),
			contentKeyTree: entries,
			"truncated":    false,
		})
	}
}

func githubBranchRefHandler(opts GitHubOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opts.ReleaseBranchMissing && r.PathValue("branch") == fakeReleaseBranch {
			http.Error(w, "not found", http.StatusNotFound)

			return
		}

		writeJSON(w, map[string]any{
			githubKeyRef:    "refs/heads/" + r.PathValue("branch"),
			githubKeyObject: map[string]any{githubKeySHA: fakeBaseSHA, githubKeyType: githubKeyCommit},
		})
	}
}

func registerGitHubContent(mux *http.ServeMux, prefix string, opts GitHubOptions) {
	mux.HandleFunc("GET "+prefix+"/contents/{path...}", func(w http.ResponseWriter, r *http.Request) {
		path := r.PathValue(contentKeyPath)

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
		githubKeyName:  path,
		contentKeyPath: path,
		githubKeyType:  "file",
		"encoding":     "base64",
		"content":      base64.StdEncoding.EncodeToString([]byte(raw)),
		githubKeySHA:   "626c6f6273686100000000000000000000000000",
	}
}

func registerGitHubPullsWrite(
	t *testing.T,
	mux *http.ServeMux,
	prefix string,
	opts GitHubOptions,
	merged, reviewersRequested *atomic.Bool,
) {
	t.Helper()

	mux.HandleFunc("POST "+prefix+"/pulls", func(w http.ResponseWriter, r *http.Request) {
		if !expectGitHubPullRequest(t, r, opts) {
			http.Error(w, "unexpected pull request", http.StatusUnprocessableEntity)

			return
		}

		writeJSON(w, githubFakePR(opts))
	})

	mux.HandleFunc("PATCH "+prefix+"/pulls/{number}", func(w http.ResponseWriter, r *http.Request) {
		if !expectGitHubPullRequest(t, r, opts) {
			http.Error(w, "unexpected pull request", http.StatusUnprocessableEntity)

			return
		}

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

func expectGitHubPullRequest(t *testing.T, r *http.Request, opts GitHubOptions) bool {
	t.Helper()

	if opts.ExpectPRTitle == "" && opts.ExpectPRBodyFile == "" {
		return true
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Errorf("fakeprovider/github: decode %s %s: %v", r.Method, r.URL.Path, err)

		return false
	}

	matched := true

	if opts.ExpectPRTitle != "" {
		if title, _ := payload["title"].(string); title != opts.ExpectPRTitle {
			t.Errorf("fakeprovider/github: title = %q, want %q", title, opts.ExpectPRTitle)

			matched = false
		}
	}

	if opts.ExpectPRBodyFile != "" {
		body, _ := payload["body"].(string)
		testastic.AssertFile(t, opts.ExpectPRBodyFile, body)
	}

	return matched
}

func expectGitHubFields(t *testing.T, r *http.Request, expected map[string]string) bool {
	t.Helper()

	wanted := map[string]string{}

	for field, value := range expected {
		if value != "" {
			wanted[field] = value
		}
	}

	if len(wanted) == 0 {
		return true
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Errorf("fakeprovider/github: decode %s %s: %v", r.Method, r.URL.Path, err)

		return false
	}

	matched := true

	for field, want := range wanted {
		if actual, _ := payload[field].(string); actual != want {
			t.Errorf("fakeprovider/github: %s = %q, want %q", field, actual, want)

			matched = false
		}
	}

	return matched
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
	t *testing.T,
	mux *http.ServeMux,
	prefix string,
	opts GitHubOptions,
	reviewersRequested *atomic.Bool,
) {
	t.Helper()

	labels := newLabelRegistry(opts.ExistingLabels)

	if opts.MergedPendingRelease || opts.ExistingOpenReleasePRBody != "" {
		_ = labels.create(fakePendingReleaseTag)
		_ = labels.create("autorelease: tagged")
	}

	mux.HandleFunc("GET "+prefix+"/labels/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue(githubKeyName)

		if !labels.exists(name) {
			http.Error(w, "not found", http.StatusNotFound)

			return
		}

		writeJSON(w, map[string]any{githubKeyName: name})
	})

	mux.HandleFunc("POST "+prefix+"/labels", func(w http.ResponseWriter, r *http.Request) {
		name := readJSONString(t, r, githubKeyName)
		if !labels.create(name) {
			t.Errorf("fakeprovider/github: recreated existing label %q", name)
			http.Error(w, "label already exists", http.StatusUnprocessableEntity)

			return
		}

		writeJSON(w, map[string]any{githubKeyName: name})
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

const githubReleaseManifest = "<!-- yeet-release-manifest\n" +
	`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}` +
	"\n-->"

func githubMergedPendingPR(opts GitHubOptions) map[string]any {
	pr := githubFakePR(opts)
	pr["state"] = "closed"
	pr["merged"] = true
	pr["merged_at"] = fakeMergedAtTimestamp
	pr["merge_commit_sha"] = opts.BranchHeadSHA
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
			githubKeySHA: "6865616473686100000000000000000000000000",
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
