package provider_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	case providerContractLatestRelease:
		return azureDevOpsLatestReleaseHandler(t)
	case providerContractLatestFallbackTags:
		return azureDevOpsLatestFallbackTagsHandler(t)
	case providerContractListTags:
		return azureDevOpsListTagsHandler(t)
	case providerContractGetCommitsSince:
		return azureDevOpsGetCommitsSinceHandler(t)
	case providerContractGetReleaseByTag:
		return azureDevOpsGetReleaseByTagHandler(t)
	case providerContractTagExists:
		return azureDevOpsTagExistsHandler(t)
	case providerContractCreateReleasePR:
		return azureDevOpsCreateReleasePRHandler(t)
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
	case providerContractCommitPRBody:
		return azureDevOpsCommitPRBodyHandler(t)
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

func azureDevOpsLatestReleaseHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if isAzureDevOpsRefsRequest(r, "tags/") {
			writeJSONFixture(t, w, azureDevOpsContractFixture("latest_release", "tags.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func azureDevOpsLatestFallbackTagsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if isAzureDevOpsRefsRequest(r, "tags/") {
			writeJSONFixture(t, w, azureDevOpsContractFixture("latest_fallback_tags", "tags.json"))

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

func azureDevOpsGetCommitsSinceHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isAzureDevOpsCommitsListRequest(r):
			testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("searchCriteria.itemVersion.version"))
			testastic.Equal(t, providerContractTag, r.URL.Query().Get("searchCriteria.compareVersion.version"))
			testastic.Equal(t, "tag", r.URL.Query().Get("searchCriteria.compareVersion.versionType"))
			writeJSONFixture(t, w, azureDevOpsContractFixture("get_commits_since", "commits.json"))
		case r.Method == http.MethodGet &&
			r.URL.Path == azureDevOpsContractRepoAPI("commits/"+providerContractHeadSHA+"/changes"):
			writeJSONFixture(t, w, azureDevOpsContractFixture("get_commits_since", "changes.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

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

func azureDevOpsTagExistsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if isAzureDevOpsRefsRequest(r, "tags/"+providerContractTag) {
			writeJSONFixture(t, w, azureDevOpsContractFixture("tag_exists", "tag_refs.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
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

func azureDevOpsCommitPRBodyHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if !isAzureDevOpsPullRequestsListRequest(r) {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		writeJSONFixture(t, w, azureDevOpsContractFixture("commit_pr_body", "pull_requests.json"))
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
		case isAzureDevOpsCommitsListRequest(r):
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
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("items"):
			path := r.URL.Query().Get("path")
			switch path {
			case "CHANGELOG.md":
				writeTextFixture(t, w, azureDevOpsContractFixture("update_files", "changelog.txt"))
			case "VERSION.txt":
				http.NotFound(w, r)
			default:
				fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
			}
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
