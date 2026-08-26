package fakeprovider

import (
	_ "embed"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

type AzureOptions struct {
	Organization              string
	Project                   string
	Repo                      string
	BranchHeadSHA             string
	ReleaseBranchHeadSHA      string
	ReleaseBranchMissing      bool
	RefUpdateFailure          string
	LatestTag                 string
	ExtraTags                 []string
	BoundarySHA               string
	TagSHAs                   map[string]string
	ExistingReleaseTag        string
	Commits                   []AzureCommit
	MergedPendingRelease      bool
	MultipleOpenPRs           bool
	MergeBlocked              bool
	ExistingOpenReleasePRBody string
	Files                     map[string]string
	Reviewers                 map[string]string
}

// AzureCommit is a tiny subset of the Azure DevOps commit payload yeet reads.
type AzureCommit struct {
	SHA     string
	Message string
	Files   []string
}

//go:embed testdata/resource_locations.json
var azureResourceLocations []byte

//go:embed testdata/resource_areas_empty.json
var azureResourceAreasEmpty []byte

// NewAzure starts the Azure DevOps REST fake and registers its cleanup with t.
func NewAzure(t *testing.T, opts AzureOptions) *httptest.Server {
	t.Helper()

	rootAPI := "/" + opts.Organization + "/_apis"
	repoAPI := "/" + opts.Organization + "/" + opts.Project + "/_apis/git/repositories/" + opts.Repo

	mux := http.NewServeMux()
	branchCreated := &atomic.Bool{}
	merged := &atomic.Bool{}

	mux.HandleFunc("OPTIONS "+rootAPI, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write(azureResourceLocations)
	})

	mux.HandleFunc("GET "+rootAPI+"/ResourceAreas", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write(azureResourceAreasEmpty)
	})

	mux.HandleFunc("GET "+rootAPI+"/identities", func(w http.ResponseWriter, r *http.Request) {
		id, exists := opts.Reviewers[r.URL.Query().Get("filterValue")]
		if !exists {
			writeJSON(w, map[string]any{azureKeyCount: 0, azureKeyValue: []any{}})

			return
		}

		writeJSON(w, map[string]any{
			azureKeyCount: 1,
			azureKeyValue: []map[string]any{{
				gitlabKeyID:           id,
				"providerDisplayName": r.URL.Query().Get("filterValue"),
			}},
		})
	})

	registerAzureHistory(mux, repoAPI, opts, branchCreated)
	registerAzurePullRequests(mux, repoAPI, opts, merged)
	registerAzureWrite(mux, repoAPI, opts, branchCreated)
	registerAzureReleases(mux, opts.Organization, opts.Project)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fakeprovider/azure: unexpected request %s %s", r.Method, r.URL.String())
		http.Error(w, "unhandled", http.StatusNotImplemented)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func registerAzureHistory(
	mux *http.ServeMux,
	repoAPI string,
	opts AzureOptions,
	branchCreated *atomic.Bool,
) {
	mux.HandleFunc("GET "+repoAPI+"/refs", azureRefsHandler(opts, branchCreated))

	mux.HandleFunc("GET "+repoAPI+"/commits", azureCommitsHandler(opts))

	mux.HandleFunc("GET "+repoAPI+"/annotatedTags/{id}", func(w http.ResponseWriter, r *http.Request) {
		if opts.ExistingReleaseTag != "" {
			writeJSON(w, map[string]any{
				azureKeyObjectID: r.PathValue("id"),
				gitlabKeyName:    opts.ExistingReleaseTag,
				"message":        "existing release notes",
				"taggedObject": map[string]any{
					azureKeyObjectID: opts.BranchHeadSHA,
				},
			})

			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	})

	mux.HandleFunc("GET "+repoAPI+"/commits/{id}/changes", azureCommitChangesHandler(opts))
}

func azureCommitsHandler(opts AzureOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		itemVersion := query.Get("searchCriteria.itemVersion.version")
		compareVersion := query.Get("searchCriteria.compareVersion.version")

		if compareVersion == "" {
			writeJSON(w, azureResolveCommitPayload(itemVersion, opts))

			return
		}

		if itemVersion == "" {
			writeJSON(w, map[string]any{
				azureKeyCount: len(opts.Commits),
				azureKeyValue: azureCommitsList(opts.Commits),
			})

			return
		}

		boundarySHA, ok := azureResolveRefSHA(itemVersion, opts)
		if !ok {
			http.Error(w, "unknown boundary", http.StatusNotFound)

			return
		}

		since := azureCommitsSince(opts.Commits, boundarySHA)
		writeJSON(w, map[string]any{
			azureKeyCount: len(since),
			azureKeyValue: azureCommitsList(since),
		})
	}
}

// azureCommitsSince returns the commits ahead of the boundary commit in the
// newest-first list. The fixtures place the boundary commit at the tail, so
// this drops it (and anything older). A boundary absent from the list is
// treated as older than every listed commit.
func azureCommitsSince(commits []AzureCommit, boundarySHA string) []AzureCommit {
	for idx, c := range commits {
		if c.SHA == boundarySHA {
			return commits[:idx]
		}
	}

	return commits
}

func azureRefsHandler(opts AzureOptions, branchCreated *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")

		if branch, found := strings.CutPrefix(filter, "heads/"); found {
			writeJSON(w, azureBranchRefsPayload(opts, branch, branchCreated.Load()))

			return
		}

		if filter == "tags/"+opts.LatestTag && opts.LatestTag != "" {
			writeJSON(w, azureTagRefsPayload(opts))

			return
		}

		if filter == "tags/"+opts.ExistingReleaseTag && opts.ExistingReleaseTag != "" {
			writeJSON(w, azureExistingReleaseTagRefsPayload(filter))

			return
		}

		if filter == "tags/" || filter == "tags" {
			writeJSON(w, azureTagRefsListPayload(opts))

			return
		}

		if strings.HasPrefix(filter, "tags/") {
			writeJSON(w, map[string]any{azureKeyCount: 0, azureKeyValue: []any{}})

			return
		}

		writeJSON(w, azureTagRefsPayload(opts))
	}
}

func azureBranchRefsPayload(opts AzureOptions, branch string, branchCreated bool) map[string]any {
	if branch == fakeReleaseBranch && opts.ReleaseBranchMissing && !branchCreated {
		return map[string]any{azureKeyCount: 0, azureKeyValue: []any{}}
	}

	headSHA := opts.BranchHeadSHA
	if branch == fakeReleaseBranch && opts.ReleaseBranchHeadSHA != "" {
		headSHA = opts.ReleaseBranchHeadSHA
	}

	if headSHA == "" {
		headSHA = fakeBaseSHA
	}

	return map[string]any{
		azureKeyCount: 1,
		azureKeyValue: []map[string]any{{
			gitlabKeyName:    "refs/heads/" + branch,
			azureKeyObjectID: headSHA,
		}},
	}
}

func azureExistingReleaseTagRefsPayload(filter string) map[string]any {
	return map[string]any{
		azureKeyCount: 1,
		azureKeyValue: []map[string]any{{
			gitlabKeyName:    "refs/" + filter,
			azureKeyObjectID: "6578697374696e677461676f626a656374736861",
		}},
	}
}

func azureTagRefsListPayload(opts AzureOptions) map[string]any {
	tags := make([]map[string]any, 0, 1+len(opts.ExtraTags))

	if opts.LatestTag != "" {
		tags = append(tags, map[string]any{
			gitlabKeyName:    "refs/tags/" + opts.LatestTag,
			azureKeyObjectID: azureTagSHA(opts, opts.LatestTag),
		})
	}

	for _, t := range opts.ExtraTags {
		tags = append(tags, map[string]any{
			gitlabKeyName:    "refs/tags/" + t,
			azureKeyObjectID: azureTagSHA(opts, t),
		})
	}

	return map[string]any{
		azureKeyCount: len(tags),
		azureKeyValue: tags,
	}
}

func azureTagSHA(opts AzureOptions, tag string) string {
	if commitHash := opts.TagSHAs[tag]; commitHash != "" {
		return commitHash
	}

	return opts.BoundarySHA
}

func azureTagRefsPayload(opts AzureOptions) map[string]any {
	return map[string]any{
		azureKeyCount: 1,
		azureKeyValue: []map[string]any{{
			gitlabKeyName:    "refs/tags/" + opts.LatestTag,
			azureKeyObjectID: opts.BoundarySHA,
		}},
	}
}

func azurePullRequestsListHandler(opts AzureOptions, merged *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("searchCriteria.status")

		if status == azureStatusCompleted && (opts.MergedPendingRelease || merged.Load()) {
			writeJSON(w, map[string]any{
				azureKeyCount: 1,
				azureKeyValue: []map[string]any{azureMergedPendingPR(opts)},
			})

			return
		}

		if status == azureStatusActive && opts.MultipleOpenPRs {
			writeJSON(w, map[string]any{
				azureKeyCount: azureMultipleOpenPRCount,
				azureKeyValue: []map[string]any{
					azurePendingPR(opts, azureFakePRID),
					azurePendingPR(opts, azureFakeSecondPRID),
				},
			})

			return
		}

		if status == azureStatusActive && opts.ExistingOpenReleasePRBody != "" {
			pr := azurePendingPR(opts, azureFakePRID)
			pr["description"] = opts.ExistingOpenReleasePRBody

			writeJSON(w, map[string]any{
				azureKeyCount: 1,
				azureKeyValue: []map[string]any{pr},
			})

			return
		}

		writeJSON(w, map[string]any{azureKeyCount: 0, azureKeyValue: []any{}})
	}
}

func azurePullRequestQueryHandler(opts AzureOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Queries []struct {
				Items []string `json:"items"`
			} `json:"queries"`
		}

		_ = json.UnmarshalRead(r.Body, &request)

		matches := map[string][]map[string]any{}

		if opts.MergedPendingRelease {
			for _, query := range request.Queries {
				for _, item := range query.Items {
					if item == fakeMergeSHA {
						matches[item] = []map[string]any{azureMergedPendingPR(opts)}
					}
				}
			}
		}

		writeJSON(w, map[string]any{
			azureKeyResults: []map[string][]map[string]any{matches},
		})
	}
}

func azureCommitChangesHandler(opts AzureOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sha := r.PathValue("id")

		for _, c := range opts.Commits {
			if c.SHA != sha {
				continue
			}

			changes := make([]map[string]any, 0, len(c.Files))
			for _, path := range c.Files {
				changes = append(changes, map[string]any{
					"item":       map[string]any{"path": "/" + strings.TrimPrefix(path, "/")},
					"changeType": "edit",
				})
			}

			writeJSON(w, map[string]any{
				azureKeyCount: len(changes),
				"changes":     changes,
			})

			return
		}

		writeJSON(w, map[string]any{azureKeyCount: 0, "changes": []any{}})
	}
}

func azureUpdatePullRequestHandler(opts AzureOptions, merged *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Status string `json:"status"`
		}

		_ = json.UnmarshalRead(r.Body, &request)
		if request.Status == azureStatusCompleted {
			merged.Store(true)
			writeJSON(w, azureMergedPendingPR(opts))

			return
		}

		writeJSON(w, azureFakePR(opts))
	}
}

func registerAzurePullRequests(mux *http.ServeMux, repoAPI string, opts AzureOptions, merged *atomic.Bool) {
	mux.HandleFunc("GET "+repoAPI+"/pullRequests", azurePullRequestsListHandler(opts, merged))

	mux.HandleFunc("POST "+repoAPI+"/pullRequestQuery", azurePullRequestQueryHandler(opts))

	mux.HandleFunc("POST "+repoAPI+"/pullRequests", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, azureFakePR(opts))
	})

	mux.HandleFunc("PATCH "+repoAPI+"/pullRequests/{id}", azureUpdatePullRequestHandler(opts, merged))

	mux.HandleFunc("GET "+repoAPI+"/pullRequests/{id}", func(w http.ResponseWriter, _ *http.Request) {
		if opts.MergedPendingRelease || merged.Load() {
			writeJSON(w, azureMergedPendingPR(opts))

			return
		}

		writeJSON(w, azureFakePR(opts))
	})

	mux.HandleFunc("GET "+repoAPI+"/pullRequests/{id}/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			azureKeyCount: 1,
			azureKeyValue: []map[string]any{
				{
					gitlabKeyName: fakePendingReleaseTag,
					gitlabKeyID:   azureFakeLabelID,
				},
			},
		})
	})

	mux.HandleFunc("POST "+repoAPI+"/pullRequests/{id}/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			gitlabKeyName: fakePendingReleaseTag,
			gitlabKeyID:   azureFakeLabelID,
		})
	})

	mux.HandleFunc(
		"DELETE "+repoAPI+"/pullRequests/{id}/labels/{labelID}",
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	)
}

func registerAzureWrite(
	mux *http.ServeMux,
	repoAPI string,
	opts AzureOptions,
	branchCreated *atomic.Bool,
) {
	mux.HandleFunc("POST "+repoAPI+"/refs", azureUpdateRefsHandler(opts, branchCreated))

	mux.HandleFunc("POST "+repoAPI+"/pushes", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"pushId": 1, keyCommits: []any{}})
	})

	mux.HandleFunc("POST "+repoAPI+"/annotatedTags", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			azureKeyObjectID: "7461676f626a6563747368610000000000000000",
			gitlabKeyName:    fakeNextTag,
			"taggedObject":   map[string]any{azureKeyObjectID: fakeMergeSHA},
		})
	})

	mux.HandleFunc("GET "+repoAPI+"/items", azureItemsHandler(opts))
}

const azureKeySuccess = "success"

func azureUpdateRefsHandler(opts AzureOptions, branchCreated *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if opts.RefUpdateFailure != "" {
			writeJSON(w, map[string]any{
				azureKeyCount: 1,
				azureKeyValue: []map[string]any{{
					gitlabKeyName:    "refs/heads/" + fakeReleaseBranch,
					azureKeyObjectID: fakeBaseSHA,
					azureKeySuccess:  false,
					"customMessage":  opts.RefUpdateFailure,
				}},
			})

			return
		}

		branchCreated.Store(true)

		writeJSON(w, map[string]any{
			azureKeyCount: 1,
			azureKeyValue: []map[string]any{
				{
					gitlabKeyName:    "refs/heads/" + fakeReleaseBranch,
					azureKeyObjectID: fakeBaseSHA,
					azureKeySuccess:  true,
				},
			},
		})
	}
}

func azureItemsHandler(opts AzureOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Query().Get("path"), "/")

		if content, ok := opts.Files[path]; ok {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(content))

			return
		}

		if path == "CHANGELOG.md" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("## Changelog\n\n## [v1.1.0]\n\n* feat: add a thing\n"))

			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}
}

func registerAzureReleases(mux *http.ServeMux, org, project string) {
	prefix := "/" + org + "/" + project + "/_apis"

	mux.HandleFunc("GET "+prefix+"/release/releases", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{azureKeyCount: 0, azureKeyValue: []any{}})
	})
}

func azureResolveCommitPayload(ref string, opts AzureOptions) map[string]any {
	sha, ok := azureResolveRefSHA(ref, opts)
	if !ok {
		return map[string]any{
			azureKeyCount: 0,
			azureKeyValue: []map[string]any{},
		}
	}

	return map[string]any{
		azureKeyCount: 1,
		azureKeyValue: []map[string]any{{azureCommitIDKey: sha}},
	}
}

func azureResolveRefSHA(ref string, opts AzureOptions) (string, bool) {
	if ref == opts.LatestTag || slices.Contains(opts.ExtraTags, ref) {
		return opts.BoundarySHA, true
	}

	if ref == fakeBaseBranch {
		return fakeBaseSHA, true
	}

	// fakeMergeSHA stands in for the merge-commit SHA returned by the merged-PR
	// fixtures. Real Azure DevOps would short-circuit a hex SHA via
	// isAzureDevOpsCommitSHA. The fake recognises it here so finalize flows
	// can resolve their tag target.
	if ref == fakeMergeSHA {
		return fakeMergeSHA, true
	}

	for _, c := range opts.Commits {
		if c.SHA == ref {
			return c.SHA, true
		}
	}

	return "", false
}

func azureCommitsList(commits []AzureCommit) []map[string]any {
	out := make([]map[string]any, 0, len(commits))

	for _, c := range commits {
		out = append(out, map[string]any{
			azureCommitIDKey: c.SHA,
			"comment":        c.Message,
		})
	}

	return out
}

const (
	azureFakePRID            = 42
	azureFakeSecondPRID      = 43
	azureFakeLabelID         = "00000000-0000-0000-0000-000000000042"
	azureStatusActive        = "active"
	azureStatusCompleted     = "completed"
	azureMultipleOpenPRCount = 2
	azureCommitIDKey         = "commitId"
)

const azureReleaseManifest = "<!-- yeet-release-manifest\n" +
	`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}` +
	"\n-->"

func azureFakePR(opts AzureOptions) map[string]any {
	return azurePRBase(opts, azureFakePRID, azureStatusActive, opts.MergeBlocked, false)
}

func azurePendingPR(opts AzureOptions, id int) map[string]any {
	pr := azurePRBase(opts, id, azureStatusActive, false, false)
	pr["labels"] = []map[string]any{
		{gitlabKeyName: fakePendingReleaseTag, gitlabKeyID: azureFakeLabelID},
	}

	return pr
}

func azureMergedPendingPR(opts AzureOptions) map[string]any {
	pr := azurePRBase(opts, azureFakePRID, azureStatusCompleted, false, true)
	pr["labels"] = []map[string]any{
		{gitlabKeyName: fakePendingReleaseTag, gitlabKeyID: azureFakeLabelID},
	}
	pr["description"] = "## release created\n\n" + azureReleaseManifest + "\n"

	pr["lastMergeCommit"] = map[string]any{azureCommitIDKey: opts.BranchHeadSHA}
	pr["lastMergeSourceCommit"] = map[string]any{azureCommitIDKey: opts.BranchHeadSHA}

	return pr
}

func azurePRBase(opts AzureOptions, id int, status string, draft, completed bool) map[string]any {
	prefix := "https://example.test/" + opts.Organization + "/" + opts.Project + "/_git/" + opts.Repo

	pr := map[string]any{
		"pullRequestId": id,
		"status":        status,
		"sourceRefName": "refs/heads/" + fakeReleaseBranch,
		"targetRefName": "refs/heads/" + fakeBaseBranch,
		"repository":    map[string]any{gitlabKeyName: opts.Repo},
		"url":           fmt.Sprintf("%s/pullrequest/%d", prefix, id),
		"isDraft":       draft,
		"mergeStatus":   "succeeded",
		"lastMergeSourceCommit": map[string]any{
			azureCommitIDKey: fakeMergeSHA,
		},
	}

	if completed {
		pr["lastMergeCommit"] = map[string]any{azureCommitIDKey: fakeMergeSHA}
	}

	return pr
}
