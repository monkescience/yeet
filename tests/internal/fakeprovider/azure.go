package fakeprovider

import (
	_ "embed"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

type AzureOptions struct {
	Organization string
	Project      string
	Repo         string
	// LatestTag is the most recent tag returned by the tags-fallback.
	LatestTag string
	// ExtraTags are additional historical tags returned alongside LatestTag
	// from the tags listing. Used to drive the multi-ref ordering paths.
	ExtraTags []string
	// BoundarySHA is the SHA of the commit pointed at by LatestTag.
	BoundarySHA string
	// Commits are returned (newest first) from the commits listing.
	Commits []AzureCommit
	// MergedPendingRelease toggles the merged-release-PR-waiting-for-tagging
	// fixture: when true, the completed-pulls listing returns one merged PR
	// with the autorelease:pending label so yeet enters the finalization path.
	MergedPendingRelease bool
	// MultipleOpenPRs returns two pending release PRs from the active-pulls
	// listing to drive yeet down the ErrMultiplePendingReleasePRs path.
	MultipleOpenPRs bool
	// MergeBlocked makes GET /pullRequests/{id} return a draft PR, triggering
	// ErrMergeBlocked on --auto-merge.
	MergeBlocked bool
	// ExistingOpenReleasePRBody, when non-empty, makes the active-pull-requests
	// listing return a single pending release PR with this body so yeet drives
	// the update-existing-PR workflow.
	ExistingOpenReleasePRBody string
	// Files maps repository-relative paths to their raw content. The items
	// endpoint serves these for matching paths. Paths not in the map return
	// 404 except for CHANGELOG.md, which always has a default response.
	Files map[string]string
}

// AzureCommit is a tiny subset of the Azure DevOps commit payload yeet reads.
type AzureCommit struct {
	SHA     string
	Message string
	// Files are the changed file paths returned by the changes endpoint when
	// yeet asks for per-commit paths (multi-target mode).
	Files []string
}

//go:embed testdata/resource_locations.json
var azureResourceLocations []byte

//go:embed testdata/resource_areas_empty.json
var azureResourceAreasEmpty []byte

// NewAzure starts an httptest.Server serving the minimum Azure DevOps REST
// surface yeet exercises. The server is closed via t.Cleanup.
func NewAzure(t *testing.T, opts AzureOptions) *httptest.Server {
	t.Helper()

	rootAPI := "/" + opts.Organization + "/_apis"
	repoAPI := "/" + opts.Organization + "/" + opts.Project + "/_apis/git/repositories/" + opts.Repo

	mux := http.NewServeMux()

	mux.HandleFunc("OPTIONS "+rootAPI, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write(azureResourceLocations)
	})

	mux.HandleFunc("GET "+rootAPI+"/ResourceAreas", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write(azureResourceAreasEmpty)
	})

	registerAzureHistory(mux, repoAPI, opts)
	registerAzurePullRequests(mux, repoAPI, opts)
	registerAzureWrite(mux, repoAPI, opts)
	registerAzureReleases(mux, opts.Organization, opts.Project)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fakeprovider/azure: unexpected request %s %s", r.Method, r.URL.String())
		http.Error(w, "unhandled", http.StatusNotImplemented)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func registerAzureHistory(mux *http.ServeMux, repoAPI string, opts AzureOptions) {
	mux.HandleFunc("GET "+repoAPI+"/refs", azureRefsHandler(opts))

	mux.HandleFunc("GET "+repoAPI+"/commits", func(w http.ResponseWriter, r *http.Request) {
		itemVersion := r.URL.Query().Get("searchCriteria.itemVersion.version")
		if itemVersion != "" {
			writeJSON(w, azureResolveCommitPayload(itemVersion, opts))

			return
		}

		writeJSON(w, map[string]any{
			azureKeyCount: len(opts.Commits),
			azureKeyValue: azureCommitsList(opts.Commits),
		})
	})

	mux.HandleFunc("GET "+repoAPI+"/annotatedTags/{id}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	mux.HandleFunc("GET "+repoAPI+"/commits/{id}/changes", azureCommitChangesHandler(opts))
}

func azureRefsHandler(opts AzureOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")

		if strings.HasPrefix(filter, "heads/") {
			writeJSON(w, map[string]any{
				azureKeyCount: 1,
				azureKeyValue: []map[string]any{{
					gitlabKeyName:    "refs/" + filter,
					azureKeyObjectID: fakeBaseSHA,
				}},
			})

			return
		}

		if filter == "tags/"+opts.LatestTag && opts.LatestTag != "" {
			writeJSON(w, azureTagRefsPayload(opts))

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

func azureTagRefsListPayload(opts AzureOptions) map[string]any {
	tags := make([]map[string]any, 0, 1+len(opts.ExtraTags))

	if opts.LatestTag != "" {
		tags = append(tags, map[string]any{
			gitlabKeyName:    "refs/tags/" + opts.LatestTag,
			azureKeyObjectID: opts.BoundarySHA,
		})
	}

	for _, t := range opts.ExtraTags {
		tags = append(tags, map[string]any{
			gitlabKeyName:    "refs/tags/" + t,
			azureKeyObjectID: opts.BoundarySHA,
		})
	}

	return map[string]any{
		azureKeyCount: len(tags),
		azureKeyValue: tags,
	}
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

func azurePullRequestsListHandler(opts AzureOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("searchCriteria.status")

		if status == azureStatusCompleted && opts.MergedPendingRelease {
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

func registerAzurePullRequests(mux *http.ServeMux, repoAPI string, opts AzureOptions) {
	mux.HandleFunc("GET "+repoAPI+"/pullRequests", azurePullRequestsListHandler(opts))

	mux.HandleFunc("POST "+repoAPI+"/pullRequests", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, azureFakePR(opts))
	})

	mux.HandleFunc("PATCH "+repoAPI+"/pullRequests/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, azureFakePR(opts))
	})

	mux.HandleFunc("GET "+repoAPI+"/pullRequests/{id}", func(w http.ResponseWriter, _ *http.Request) {
		if opts.MergedPendingRelease {
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

	mux.HandleFunc(
		"GET /"+opts.Organization+"/"+opts.Project+"/_apis/git/pullRequests/{id}",
		func(w http.ResponseWriter, _ *http.Request) {
			if opts.MergedPendingRelease {
				writeJSON(w, azureMergedPendingPR(opts))

				return
			}

			writeJSON(w, azureFakePR(opts))
		},
	)
}

func registerAzureWrite(mux *http.ServeMux, repoAPI string, opts AzureOptions) {
	mux.HandleFunc("POST "+repoAPI+"/refs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			azureKeyCount: 1,
			azureKeyValue: []map[string]any{
				{
					gitlabKeyName:    "refs/heads/" + fakeReleaseBranch,
					azureKeyObjectID: fakeBaseSHA,
					"success":        true,
				},
			},
		})
	})

	mux.HandleFunc("POST "+repoAPI+"/pushes", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"pushId": 1, "commits": []any{}})
	})

	mux.HandleFunc("POST "+repoAPI+"/annotatedTags", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			azureKeyObjectID: "tag-object-sha",
			gitlabKeyName:    fakeNextTag,
			"taggedObject":   map[string]any{azureKeyObjectID: fakeMergeSHA},
		})
	})

	mux.HandleFunc("GET "+repoAPI+"/items", func(w http.ResponseWriter, r *http.Request) {
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
	})
}

func registerAzureReleases(mux *http.ServeMux, org, project string) {
	prefix := "/" + org + "/" + project + "/_apis"

	mux.HandleFunc("GET "+prefix+"/release/releases", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{azureKeyCount: 0, azureKeyValue: []any{}})
	})
}

// azureResolveCommitPayload mirrors what ADO returns when yeet resolves a ref
// to its commit SHA via the commits endpoint (top=1, itemVersion=ref). Unknown
// refs get an empty payload (count: 0), matching how the real API responds and
// preventing typos or stale refs from quietly resolving to BoundarySHA.
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
	pr["lastMergeCommit"] = map[string]any{azureCommitIDKey: fakeMergeSHA}
	pr["lastMergeSourceCommit"] = map[string]any{azureCommitIDKey: fakeMergeSHA}

	return pr
}

func azurePRBase(opts AzureOptions, id int, status string, draft, completed bool) map[string]any {
	prefix := "https://example.test/" + opts.Organization + "/" + opts.Project + "/_git/" + opts.Repo

	pr := map[string]any{
		"pullRequestId": id,
		"status":        status,
		"sourceRefName": "refs/heads/" + fakeReleaseBranch,
		"targetRefName": "refs/heads/" + fakeBaseBranch,
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
