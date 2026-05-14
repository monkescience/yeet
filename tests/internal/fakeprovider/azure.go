package fakeprovider

import (
	_ "embed"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// AzureOptions configures the responses served by [NewAzure].
type AzureOptions struct {
	Organization string
	Project      string
	Repo         string
	// LatestTag is the most recent tag returned by the tags-fallback.
	LatestTag string
	// BoundarySHA is the SHA of the commit pointed at by LatestTag.
	BoundarySHA string
	// Commits are returned (newest first) from the commits listing.
	Commits []AzureCommit
}

// AzureCommit is a tiny subset of the Azure DevOps commit payload yeet reads.
type AzureCommit struct {
	SHA     string
	Message string
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
	registerAzureWrite(mux, repoAPI)
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
	mux.HandleFunc("GET "+repoAPI+"/refs", func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")

		if strings.HasPrefix(filter, "heads/") {
			writeJSON(w, map[string]any{
				azureKeyCount: 1,
				azureKeyValue: []map[string]any{
					{
						gitlabKeyName:    "refs/" + filter,
						azureKeyObjectID: fakeBaseSHA,
					},
				},
			})

			return
		}

		writeJSON(w, map[string]any{
			azureKeyCount: 1,
			azureKeyValue: []map[string]any{
				{
					gitlabKeyName:    "refs/tags/" + opts.LatestTag,
					azureKeyObjectID: opts.BoundarySHA,
				},
			},
		})
	})

	mux.HandleFunc("GET "+repoAPI+"/commits", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			azureKeyCount: len(opts.Commits),
			azureKeyValue: azureCommitsList(opts.Commits),
		})
	})

	mux.HandleFunc("GET "+repoAPI+"/annotatedTags/{id}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
}

func registerAzurePullRequests(mux *http.ServeMux, repoAPI string, opts AzureOptions) {
	mux.HandleFunc("GET "+repoAPI+"/pullRequests", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{azureKeyCount: 0, azureKeyValue: []any{}})
	})

	mux.HandleFunc("POST "+repoAPI+"/pullRequests", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, azureFakePR(opts.Organization, opts.Project, opts.Repo))
	})

	mux.HandleFunc("PATCH "+repoAPI+"/pullRequests/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, azureFakePR(opts.Organization, opts.Project, opts.Repo))
	})

	mux.HandleFunc("GET "+repoAPI+"/pullRequests/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, azureFakePR(opts.Organization, opts.Project, opts.Repo))
	})

	mux.HandleFunc("GET "+repoAPI+"/pullRequests/{id}/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{azureKeyCount: 0, azureKeyValue: []any{}})
	})

	mux.HandleFunc("POST "+repoAPI+"/pullRequests/{id}/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			gitlabKeyName: "autorelease: pending",
			gitlabKeyID:   "00000000-0000-0000-0000-000000000042",
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
			writeJSON(w, azureFakePR(opts.Organization, opts.Project, opts.Repo))
		},
	)
}

func registerAzureWrite(mux *http.ServeMux, repoAPI string) {
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
		if path == "CHANGELOG.md" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("## Changelog\n"))

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

func azureCommitsList(commits []AzureCommit) []map[string]any {
	out := make([]map[string]any, 0, len(commits))

	for _, c := range commits {
		out = append(out, map[string]any{
			"commitId": c.SHA,
			"comment":  c.Message,
		})
	}

	return out
}

const azureFakePRID = 42

func azureFakePR(org, project, repo string) map[string]any {
	prefix := "https://example.test/" + org + "/" + project + "/_git/" + repo
	pr := map[string]any{
		"pullRequestId": azureFakePRID,
		"status":        "active",
		"sourceRefName": "refs/heads/" + fakeReleaseBranch,
		"targetRefName": "refs/heads/" + fakeBaseBranch,
		"url":           prefix + "/pullrequest/42",
		"isDraft":       false,
	}

	return pr
}
