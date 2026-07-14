package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/monkescience/testastic"
)

// TestAzureDevOpsGetCommitsSinceRefsNonLinear reproduces the v0.10.x regression
// where a non-linear history over-includes commits. The boundary tag's
// graph-aware range (commits reachable from the branch but not from the tag) is
// a single commit, but an unbounded date-ordered walk buries the boundary
// commit behind already-released commits that carry newer commit dates. A
// client-side "stop at the boundary SHA" scan therefore returns every commit
// ahead of the boundary in date order, not the graph range.
func TestAzureDevOpsGetCommitsSinceRefsNonLinear(t *testing.T) {
	t.Parallel()

	const (
		branch      = "main"
		boundaryTag = "v1.0.0"
		boundarySHA = "boundarysha"
	)

	// given: a repository whose branch reaches one new commit since v1.0.0, while
	// an unbounded walk also surfaces two already-released commits dated after
	// the boundary commit.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		query := r.URL.Query()
		itemVersion := query.Get("searchCriteria.itemVersion.version")
		compareVersion := query.Get("searchCriteria.compareVersion.version")

		switch {
		case isAzureDevOpsCommitsListRequest(r) && itemVersion == boundaryTag && compareVersion == "":
			// Liveness resolve of the boundary tag (top=1, no compare version).
			writeAzureCommits(t, w, []azureTestCommit{{SHA: boundarySHA}})
		case isAzureDevOpsCommitsListRequest(r) && itemVersion == boundaryTag && compareVersion == branch:
			// Graph-aware range: only the commit since v1.0.0.
			writeAzureCommits(t, w, []azureTestCommit{{SHA: "head-sha", Comment: "feat: new"}})
		case isAzureDevOpsCommitsListRequest(r) && itemVersion == "" && compareVersion == branch:
			// Unbounded date-ordered walk: boundary buried behind released commits.
			writeAzureCommits(t, w, []azureTestCommit{
				{SHA: "head-sha", Comment: "feat: new"},
				{SHA: "released-a", Comment: "fix: already released a"},
				{SHA: "released-b", Comment: "fix: already released b"},
				{SHA: boundarySHA, Comment: "chore: release v1.0.0"},
				{SHA: "older-sha", Comment: "feat: older"},
			})
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: requesting commits since v1.0.0 on main
	history, err := p.GetCommitsSinceRefs(context.Background(), []string{boundaryTag}, branch, false)

	// then: only the single graph-range commit is returned, not the released ones
	testastic.NoError(t, err)
	testastic.Equal(t, 0, len(history.MissingRefs))
	testastic.SliceEqual(t, []string{"head-sha"}, commitEntryHashes(history.EntriesByRef[boundaryTag]))
}

func TestAzureDevOpsGetCommitsSinceRefsPaginatesChanges(t *testing.T) {
	t.Parallel()

	const branch = "main"

	firstPage := make([]map[string]any, 100)
	wantPaths := make([]string, 0, 101)

	for i := range firstPage {
		path := fmt.Sprintf("services/api/file-%03d.go", i)
		firstPage[i] = map[string]any{"item": map[string]any{"path": "/" + path}}
		wantPaths = append(wantPaths, path)
	}

	wantPaths = append(wantPaths, "services/api/tail.go")

	// given: a commit whose changed paths fill one page and continue onto a second
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case isAzureDevOpsCommitsListRequest(r):
			writeAzureCommits(t, w, []azureTestCommit{{SHA: "head-sha", Comment: "feat: update API"}})
		case isAzureDevOpsCommitChangesRequest(r, "head-sha"):
			testastic.Equal(t, "100", r.URL.Query().Get("top"))

			switch r.URL.Query().Get("skip") {
			case "", "0":
				writeJSON(t, w, map[string]any{"changes": firstPage})
			case "100":
				writeJSON(t, w, map[string]any{"changes": []map[string]any{{
					"item": map[string]any{"path": "/services/api/tail.go"},
				}}})
			default:
				fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
			}
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: changed paths are requested for the commit
	history, err := p.GetCommitsSinceRefs(context.Background(), []string{""}, branch, true)

	// then: paths from both change pages are returned in order
	testastic.NoError(t, err)

	entries := history.EntriesByRef[""]
	testastic.Equal(t, 1, len(entries))
	testastic.SliceEqual(t, wantPaths, entries[0].Paths)
}

type azureTestCommit struct {
	SHA     string
	Comment string
}

func writeAzureCommits(t *testing.T, w http.ResponseWriter, commits []azureTestCommit) {
	t.Helper()

	values := make([]map[string]any, 0, len(commits))
	for _, c := range commits {
		values = append(values, map[string]any{"commitId": c.SHA, "comment": c.Comment})
	}

	writeJSON(t, w, map[string]any{"count": len(commits), "value": values})
}
