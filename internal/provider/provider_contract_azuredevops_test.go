package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
)

const (
	azureDevOpsContractOrg     = "contoso-org"
	azureDevOpsContractProject = "contoso-project"
	azureDevOpsContractRepo    = "contoso-repo"
)

// azureDevOpsContractRepoAPI returns the per-repository API path prefix used
// by SDK-generated requests (e.g. refs, commits, items, pushes,
// annotatedTags, pullRequests/{id}/labels).
func azureDevOpsContractRepoAPI(suffix string) string {
	return fmt.Sprintf(
		"/%s/%s/_apis/git/repositories/%s/%s",
		azureDevOpsContractOrg,
		azureDevOpsContractProject,
		azureDevOpsContractRepo,
		strings.TrimLeft(suffix, "/"),
	)
}

// azureDevOpsContractPullRequestAPI returns the project-scoped PR API path used
// by GetPullRequestById, which is not repo-scoped in the SDK route table.
func azureDevOpsContractPullRequestAPI() string {
	return fmt.Sprintf(
		"/%s/%s/_apis/git/pullRequests/42",
		azureDevOpsContractOrg,
		azureDevOpsContractProject,
	)
}

func azureDevOpsContractFixture(parts ...string) string {
	return filepath.Join(append([]string{"contracts", "azuredevops"}, parts...)...)
}

func azureDevOpsContractExpectedRepoURL(serverURL string) string {
	return fmt.Sprintf(
		"%s/%s/%s/_git/%s",
		serverURL,
		azureDevOpsContractOrg,
		azureDevOpsContractProject,
		azureDevOpsContractRepo,
	)
}

func newAzureDevOpsContractProvider(t *testing.T, server *httptest.Server) provider.Provider {
	t.Helper()

	return provider.NewAzureDevOps(
		server.Client(),
		server.URL,
		"contoso-pat",
		azureDevOpsContractOrg,
		azureDevOpsContractOrg,
		azureDevOpsContractProject,
		azureDevOpsContractRepo,
	)
}

func TestAzureDevOpsFindOpenPendingReleasePRsAcceptsExactPaginationCapacity(t *testing.T) {
	t.Parallel()

	page := make([]map[string]any, 100)
	for idx := range page {
		page[idx] = map[string]any{"pullRequestId": idx + 1}
	}

	var calls atomic.Int32

	// given: one hundred full pull request pages followed by an empty page
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		if !isAzureDevOpsPullRequestsListRequest(r) {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		calls.Add(1)
		testastic.Equal(t, "100", r.URL.Query().Get("$top"))
		testastic.Equal(t, "active", r.URL.Query().Get("searchCriteria.status"))

		if r.URL.Query().Get("$skip") == "10000" {
			writeJSON(t, w, map[string]any{"count": 0, "value": []any{}})

			return
		}

		writeJSON(t, w, map[string]any{"count": len(page), "value": page})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: all pull requests at the page capacity are listed
	prs, err := p.FindOpenPendingReleasePRs(context.Background(), providerContractBaseBranch)

	// then: the empty exhaustion probe proves the complete result fits
	testastic.NoError(t, err)
	testastic.Equal(t, 0, len(prs))
	testastic.Equal(t, int32(101), calls.Load())
}

func TestAzureDevOpsUpdateFilesCreatesMissingBranchWithoutDuplicateLookups(t *testing.T) {
	t.Parallel()

	var baseLookups atomic.Int32

	var branchLookups atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case isAzureDevOpsRefsRequest(r, "heads/"+providerContractBaseBranch):
			baseLookups.Add(1)
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_branch", "base_ref.json"))
		case isAzureDevOpsRefsRequest(r, "heads/"+providerContractReleaseBranch):
			branchLookups.Add(1)
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_branch", "empty_refs.json"))
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("refs"):
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_branch", "ref_update.json"))
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("pushes"):
			writeJSONFixture(t, w, azureDevOpsContractFixture("update_files", "push.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	err := p.UpdateFiles(
		context.Background(),
		providerContractReleaseBranch,
		providerContractBaseBranch,
		map[string]provider.FileUpdate{"VERSION.txt": {Content: "version=1.2.3\n"}},
		"chore: release v1.2.3",
	)

	testastic.NoError(t, err)
	testastic.Equal(t, int32(1), baseLookups.Load())
	testastic.Equal(t, int32(1), branchLookups.Load())
}

func TestAzureDevOpsMergeReleasePRWaitsForFinalMergeCommit(t *testing.T) {
	t.Parallel()

	// given: Azure queues completion while returning preview and source commits
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			writeJSON(t, w, map[string]any{
				"pullRequestId": 42,
				"status":        "active",
				"mergeStatus":   "succeeded",
				"isDraft":       false,
				"sourceRefName": "refs/heads/yeet/release-main",
				"targetRefName": "refs/heads/main",
				"lastMergeSourceCommit": map[string]any{
					"commitId": "source-sha",
				},
			})
		case r.Method == http.MethodPatch && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42"):
			writeJSON(t, w, map[string]any{
				"pullRequestId": 42,
				"status":        "completed",
				"mergeStatus":   "queued",
				"lastMergeCommit": map[string]any{
					"commitId": "preview-sha",
				},
				"lastMergeSourceCommit": map[string]any{
					"commitId": "source-sha",
				},
			})
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: the release pull request is submitted for completion
	mergeSHA, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
		Method: provider.MergeMethodSquash,
	})

	// then: no provisional commit is returned as the final release ref
	testastic.NoError(t, err)
	testastic.Equal(t, "", mergeSHA)
}

func TestAzureDevOpsMergeReleasePRRejectsQueuedCommitFromCompletedPullRequest(t *testing.T) {
	t.Parallel()

	// given: Azure reports a completed pull request whose final merge is still queued
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		if r.Method != http.MethodGet || r.URL.Path != azureDevOpsContractPullRequestAPI() {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		writeJSON(t, w, map[string]any{
			"pullRequestId": 42,
			"status":        "completed",
			"mergeStatus":   "queued",
			"lastMergeCommit": map[string]any{
				"commitId": "preview-sha",
			},
		})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: completion is retried for the already completed pull request
	mergeSHA, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{})

	// then: the queued preview commit is not exposed as the final merge commit
	testastic.NoError(t, err)
	testastic.Equal(t, "", mergeSHA)
}

func TestAzureDevOpsFindMergedReleasePRRejectsQueuedCommit(t *testing.T) {
	t.Parallel()

	pullRequest := map[string]any{
		"pullRequestId": 42,
		"status":        "completed",
		"mergeStatus":   "queued",
		"sourceRefName": "refs/heads/yeet/release-main",
		"targetRefName": "refs/heads/main",
		"lastMergeCommit": map[string]any{
			"commitId": "preview-sha",
		},
		"labels": []map[string]any{{
			"name":   provider.ReleaseLabelPending,
			"active": true,
		}},
	}

	// given: Azure lists a completed release pull request whose final merge is still queued
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case isAzureDevOpsPullRequestsListRequest(r):
			writeJSON(t, w, map[string]any{
				"count": 1,
				"value": []map[string]any{pullRequest},
			})
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			writeJSON(t, w, pullRequest)
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: the completed release pull request is found during the merge polling window
	pr, err := p.FindMergedReleasePR(context.Background(), providerContractBaseBranch)

	// then: the queued preview commit is not exposed as the final merge commit
	testastic.NoError(t, err)
	testastic.Equal(t, "", pr.MergeCommitSHA)
}

func TestAzureDevOpsFindMergedReleasePRDoesNotUseSourceCommit(t *testing.T) {
	t.Parallel()

	pullRequest := map[string]any{
		"pullRequestId": 42,
		"status":        "completed",
		"sourceRefName": "refs/heads/yeet/release-main",
		"targetRefName": "refs/heads/main",
		"lastMergeSourceCommit": map[string]any{
			"commitId": "source-sha",
		},
		"labels": []map[string]any{{
			"name":   provider.ReleaseLabelPending,
			"active": true,
		}},
	}

	// given: a completed pull request whose final merge commit is not populated
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case isAzureDevOpsPullRequestsListRequest(r):
			writeJSON(t, w, map[string]any{
				"count": 1,
				"value": []map[string]any{pullRequest},
			})
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			writeJSON(t, w, pullRequest)
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: the completed release pull request is found
	pr, err := p.FindMergedReleasePR(context.Background(), providerContractBaseBranch)

	// then: its source commit is not exposed as the final merge commit
	testastic.NoError(t, err)
	testastic.Equal(t, "", pr.MergeCommitSHA)
}

// newAzureDevOpsContractHandler wraps every scenario with the bootstrap
// (OPTIONS /_apis + resourceAreas) so the SDK's lazy lookups succeed.
func newAzureDevOpsContractHandler(t *testing.T, scenario providerContractScenario) http.Handler {
	t.Helper()

	scenarioHandler := newAzureDevOpsScenarioHandler(t, scenario)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		scenarioHandler(w, r)
	})
}

func handleAzureDevOpsBootstrap(t *testing.T, w http.ResponseWriter, r *http.Request) bool {
	t.Helper()

	apisPath := fmt.Sprintf("/%s/_apis", azureDevOpsContractOrg)

	switch {
	case r.Method == http.MethodOptions && r.URL.Path == apisPath:
		writeJSONFixture(t, w, azureDevOpsContractFixture("_shared", "resource_locations.json"))

		return true
	case r.Method == http.MethodGet && strings.EqualFold(r.URL.Path, apisPath+"/ResourceAreas"):
		writeJSONFixture(t, w, azureDevOpsContractFixture("_shared", "resource_areas_empty.json"))

		return true
	}

	return false
}

//nolint:cyclop // Scenario dispatch is intentionally exhaustive.
func newAzureDevOpsScenarioHandler(
	t *testing.T,
	scenario providerContractScenario,
) http.HandlerFunc {
	t.Helper()

	switch scenario {
	case providerContractListTags:
		return azureDevOpsListTagsHandler(t)
	case providerContractBranchHead:
		return azureDevOpsBranchHeadHandler(t)
	case providerContractBranchHeadMissing:
		return azureDevOpsBranchHeadMissingHandler(t)
	case providerContractGetReleaseByTag:
		return azureDevOpsGetReleaseByTagHandler(t)
	case providerContractCreateReleasePR:
		return azureDevOpsCreateReleasePRHandler(t)
	case providerContractCreateReleasePRReviewers:
		return azureDevOpsCreateReleasePRReviewersHandler(t)
	case providerContractUnknownReviewer:
		return azureDevOpsUnknownReviewerHandler(t)
	case providerContractUpdateReleasePR:
		return azureDevOpsUpdateReleasePRHandler(t)
	case providerContractFindOpenPRs:
		return azureDevOpsFindOpenPRsHandler(t)
	case providerContractFindMergedPR:
		return azureDevOpsFindMergedPRHandler(t)
	case providerContractMarkReleasePR:
		return azureDevOpsMarkReleasePRHandler(t)
	case providerContractMergeReleasePR:
		return azureDevOpsMergeReleasePRHandler(t)
	case providerContractCreateBranch:
		return azureDevOpsCreateBranchHandler(t)
	case providerContractCreateRelease:
		return azureDevOpsCreateReleaseHandler(t)
	case providerContractGetFile:
		return azureDevOpsGetFileHandler(t)
	case providerContractUpdateFiles:
		return azureDevOpsUpdateFilesHandler(t)
	case providerContractMissingFile:
		return azureDevOpsMissingFileHandler(t)
	case providerContractMissingRelease:
		return azureDevOpsMissingReleaseHandler(t)
	case providerContractMissingPR:
		return azureDevOpsMissingPRHandler(t)
	case providerContractBlockedMerge:
		return azureDevOpsBlockedMergeHandler(t)
	case providerContractUnsupportedMerge:
		return azureDevOpsUnsupportedMergeHandler(t)
	default:
		return func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("unhandled Azure DevOps contract scenario: %s (request %s %s)", scenario, r.Method, r.URL.String())
		}
	}
}

func isAzureDevOpsRefsRequest(r *http.Request, filter string) bool {
	return r.Method == http.MethodGet &&
		r.URL.Path == azureDevOpsContractRepoAPI("refs") &&
		r.URL.Query().Get("filter") == filter
}

func isAzureDevOpsCommitsListRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("commits")
}

func isAzureDevOpsPullRequestsListRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests")
}

func azureDevOpsBranchHeadHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if isAzureDevOpsRefsRequest(r, "heads/"+providerContractBaseBranch) {
			writeJSONFixture(t, w, azureDevOpsContractFixture("branch_head", "refs.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func azureDevOpsBranchHeadMissingHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if isAzureDevOpsRefsRequest(r, "heads/missing-branch") {
			writeJSONFixture(t, w, azureDevOpsContractFixture("branch_head", "empty_refs.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func azureDevOpsListTagsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if isAzureDevOpsRefsRequest(r, "tags/") {
			writeJSONFixture(t, w, azureDevOpsContractFixture("list_tags", "tags.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

// azureDevOpsBoundedCommitsRequest matches the graph-aware range query the
// provider now issues per ref: ItemVersion is the boundary tag and
// CompareVersion is the branch head, so Azure computes "commits reachable from
// the branch but not from the tag" itself.
func azureDevOpsGetReleaseByTagHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isAzureDevOpsRefsRequest(r, "tags/"+providerContractTag):
			writeJSONFixture(t, w, azureDevOpsContractFixture("get_release_by_tag", "tag_refs.json"))
		case r.Method == http.MethodGet &&
			r.URL.Path == azureDevOpsContractRepoAPI("annotatedTags/tag-object-123"):
			writeJSONFixture(t, w, azureDevOpsContractFixture("get_release_by_tag", "annotated_tag.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

func azureDevOpsCreateReleasePRHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != azureDevOpsContractRepoAPI("pullRequests") {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		var request struct {
			SourceRefName string `json:"sourceRefName"`
			TargetRefName string `json:"targetRefName"`
			Title         string `json:"title"`
			Description   string `json:"description"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, "refs/heads/"+providerContractReleaseBranch, request.SourceRefName)
		testastic.Equal(t, "refs/heads/"+providerContractBaseBranch, request.TargetRefName)
		testastic.Equal(t, providerContractReleaseTitle, request.Title)
		testastic.Equal(t, providerContractReleaseBody, request.Description)

		writeJSONFixture(t, w, azureDevOpsContractFixture("create_release_pr", "pull_request.json"))
	}
}

const (
	azureDevOpsContractReviewerAliceID = "11111111-1111-1111-1111-111111111111"
	azureDevOpsContractReviewerBobID   = "22222222-2222-2222-2222-222222222222"
)

func isAzureDevOpsIdentitiesRequest(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		r.URL.Path == fmt.Sprintf("/%s/_apis/identities", azureDevOpsContractOrg)
}

func azureDevOpsCreateReleasePRReviewersHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isAzureDevOpsIdentitiesRequest(r):
			writeAzureDevOpsIdentityFixture(t, w, r)
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests"):
			var request struct {
				Title     string `json:"title"`
				Reviewers []struct {
					ID string `json:"id"`
				} `json:"reviewers"`
			}
			decodeJSONRequest(t, r, &request)
			testastic.Equal(t, providerContractReleaseTitle, request.Title)
			testastic.Equal(t, 2, len(request.Reviewers))
			testastic.Equal(t, azureDevOpsContractReviewerAliceID, request.Reviewers[0].ID)
			testastic.Equal(t, azureDevOpsContractReviewerBobID, request.Reviewers[1].ID)
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_release_pr", "pull_request.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

func azureDevOpsUnknownReviewerHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if isAzureDevOpsIdentitiesRequest(r) {
			testastic.Equal(t, providerContractUnknownReviewerName, r.URL.Query().Get("filterValue"))
			writeJSONFixture(t, w, azureDevOpsContractFixture("unknown_reviewer", "identities_empty.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func writeAzureDevOpsIdentityFixture(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	testastic.Equal(t, "General", r.URL.Query().Get("searchFilter"))

	switch filterValue := r.URL.Query().Get("filterValue"); filterValue {
	case providerContractReviewerAlice:
		writeJSONFixture(t, w, azureDevOpsContractFixture("create_release_pr_reviewers", "identities_alice.json"))
	case providerContractReviewerBob:
		writeJSONFixture(t, w, azureDevOpsContractFixture("create_release_pr_reviewers", "identities_bob.json"))
	default:
		t.Fatalf("unexpected Azure DevOps identity lookup: %s", filterValue)
	}
}

func TestAzureDevOpsRejectsAmbiguousReviewer(t *testing.T) {
	t.Parallel()

	// given: an Azure DevOps server whose identity search returns two matches
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		if isAzureDevOpsIdentitiesRequest(r) {
			testastic.Equal(t, "alex", r.URL.Query().Get("filterValue"))
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_release_pr_reviewers", "identities_ambiguous.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: creating a release PR with a reviewer name matching two identities
	_, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
		Title:         providerContractReleaseTitle,
		Body:          providerContractReleaseBody,
		BaseBranch:    providerContractBaseBranch,
		ReleaseBranch: providerContractReleaseBranch,
		Reviewers:     []string{"alex"},
	})

	// then: the run fails before any PR is created, flagging the ambiguity
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, provider.ErrReviewerAmbiguous)
	testastic.Equal(t, "reviewer is ambiguous: \"alex\" matches 2 identities", err.Error())
}

func azureDevOpsUpdateReleasePRHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != azureDevOpsContractRepoAPI("pullRequests/42") {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		var request struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, providerContractReleaseTitle, request.Title)
		testastic.Equal(t, "updated release body", request.Description)

		writeJSONFixture(t, w, azureDevOpsContractFixture("update_release_pr", "pull_request.json"))
	}
}

func azureDevOpsFindOpenPRsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if !isAzureDevOpsPullRequestsListRequest(r) {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		testastic.Equal(t, "active", r.URL.Query().Get("searchCriteria.status"))
		testastic.Equal(t, "refs/heads/"+providerContractBaseBranch, r.URL.Query().Get("searchCriteria.targetRefName"))
		writeJSONFixture(t, w, azureDevOpsContractFixture("find_open_prs", "pull_requests.json"))
	}
}

func azureDevOpsFindMergedPRHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isAzureDevOpsPullRequestsListRequest(r):
			testastic.Equal(t, "completed", r.URL.Query().Get("searchCriteria.status"))
			writeJSONFixture(t, w, azureDevOpsContractFixture("find_merged_pr", "pull_requests.json"))
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			writeJSONFixture(t, w, azureDevOpsContractFixture("find_merged_pr", "pull_request.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

func azureDevOpsMarkReleasePRHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels"):
			var request struct {
				Name string `json:"name"`
			}
			decodeJSONRequest(t, r, &request)
			testastic.NotEqual(t, "", request.Name)
			writeJSONFixture(t, w, azureDevOpsContractFixture("mark_release_pr", "label.json"))
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels"):
			writeJSONFixture(t, w, azureDevOpsContractFixture("mark_release_pr", "labels.json"))
		case r.Method == http.MethodDelete &&
			r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels/00000000-0000-0000-0000-000000000043"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete &&
			r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels/00000000-0000-0000-0000-000000000044"):
			w.WriteHeader(http.StatusNoContent)
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

func azureDevOpsMergeReleasePRHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			writeJSONFixture(t, w, azureDevOpsContractFixture("merge_release_pr", "pull_request.json"))
		case r.Method == http.MethodPatch && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42"):
			var request struct {
				Status            string `json:"status"`
				CompletionOptions struct {
					MergeStrategy string `json:"mergeStrategy"`
				} `json:"completionOptions"`
			}
			decodeJSONRequest(t, r, &request)
			testastic.Equal(t, "completed", request.Status)
			testastic.Equal(t, "squash", request.CompletionOptions.MergeStrategy)
			writeJSONFixture(t, w, azureDevOpsContractFixture("merge_release_pr", "completed.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

func azureDevOpsCreateBranchHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isAzureDevOpsRefsRequest(r, "heads/"+providerContractBaseBranch):
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_branch", "base_ref.json"))
		case isAzureDevOpsRefsRequest(r, "heads/"+providerContractReleaseBranch):
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_branch", "empty_refs.json"))
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("refs"):
			var request []struct {
				Name        string `json:"name"`
				OldObjectID string `json:"oldObjectId"`
				NewObjectID string `json:"newObjectId"`
			}
			decodeJSONRequest(t, r, &request)
			testastic.Equal(t, 1, len(request))
			testastic.Equal(t, "refs/heads/"+providerContractReleaseBranch, request[0].Name)
			testastic.Equal(t, "0000000000000000000000000000000000000000", request[0].OldObjectID)
			testastic.Equal(t, "base-sha", request[0].NewObjectID)
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_branch", "ref_update.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

func azureDevOpsCreateReleaseHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isAzureDevOpsCommitsListRequest(r) &&
			r.URL.Query().Get("searchCriteria.itemVersion.versionType") == "tag":
			// Branch and tag share the name "main" in this scenario, and the tag
			// points at a stale commit. CreateRelease must resolve to the
			// branch's HEAD, not the tag's commit.
			testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("searchCriteria.itemVersion.version"))
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_release", "tag_collision.json"))
		case isAzureDevOpsCommitsListRequest(r) &&
			r.URL.Query().Get("searchCriteria.itemVersion.versionType") == "branch":
			testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("searchCriteria.itemVersion.version"))
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_release", "commits.json"))
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("annotatedTags"):
			var request struct {
				Name         string `json:"name"`
				Message      string `json:"message"`
				TaggedObject struct {
					ObjectID string `json:"objectId"`
				} `json:"taggedObject"`
			}
			decodeJSONRequest(t, r, &request)
			testastic.Equal(t, providerContractTag, request.Name)
			testastic.Equal(t, "release notes", request.Message)
			testastic.Equal(t, providerContractHeadSHA, request.TaggedObject.ObjectID)
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_release", "annotated_tag.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

func azureDevOpsGetFileHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("items") {
			testastic.Equal(t, "CHANGELOG.md", r.URL.Query().Get("path"))
			testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("versionDescriptor.version"))
			writeTextFixture(t, w, azureDevOpsContractFixture("get_file", "changelog.txt"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func azureDevOpsUpdateFilesHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	var resetCalled bool

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isAzureDevOpsRefsRequest(r, "heads/"+providerContractReleaseBranch):
			writeJSONFixture(t, w, azureDevOpsContractFixture("update_files", "branch_ref.json"))
		case isAzureDevOpsRefsRequest(r, "heads/"+providerContractBaseBranch):
			writeJSONFixture(t, w, azureDevOpsContractFixture("update_files", "base_ref.json"))
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("refs"):
			var request []struct {
				Name        string `json:"name"`
				OldObjectID string `json:"oldObjectId"`
				NewObjectID string `json:"newObjectId"`
			}
			decodeJSONRequest(t, r, &request)
			testastic.Equal(t, 1, len(request))
			testastic.Equal(t, "refs/heads/"+providerContractReleaseBranch, request[0].Name)
			testastic.Equal(t, "release-tip", request[0].OldObjectID)
			testastic.Equal(t, "base-sha", request[0].NewObjectID)

			resetCalled = true

			writeJSONFixture(t, w, azureDevOpsContractFixture("update_files", "ref_reset.json"))
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("pushes"):
			testastic.True(t, resetCalled)

			var push struct {
				RefUpdates []struct {
					Name        string `json:"name"`
					OldObjectID string `json:"oldObjectId"`
				} `json:"refUpdates"`
			}
			decodeJSONRequest(t, r, &push)
			testastic.Equal(t, 1, len(push.RefUpdates))
			testastic.Equal(t, "refs/heads/"+providerContractReleaseBranch, push.RefUpdates[0].Name)
			testastic.Equal(t, "base-sha", push.RefUpdates[0].OldObjectID)
			writeJSONFixture(t, w, azureDevOpsContractFixture("update_files", "push.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

func azureDevOpsMissingFileHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("items") {
			http.NotFound(w, r)

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func azureDevOpsMissingReleaseHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if isAzureDevOpsRefsRequest(r, "tags/"+providerContractTag) {
			writeJSONFixture(t, w, azureDevOpsContractFixture("missing_release", "empty_refs.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func azureDevOpsMissingPRHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if isAzureDevOpsPullRequestsListRequest(r) {
			writeJSONFixture(t, w, azureDevOpsContractFixture("missing_pr", "empty_prs.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func azureDevOpsBlockedMergeHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI() {
			writeJSONFixture(t, w, azureDevOpsContractFixture("blocked_merge", "pull_request.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func azureDevOpsUnsupportedMergeHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI() {
			writeJSONFixture(t, w, azureDevOpsContractFixture("unsupported_merge", "pull_request.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}
