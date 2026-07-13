package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go/v2"
)

func newGitLabContractProvider(t *testing.T, server *httptest.Server) provider.Provider {
	t.Helper()

	client, err := gitlabapi.NewClient(
		"",
		gitlabapi.WithBaseURL(server.URL),
		gitlabapi.WithHTTPClient(server.Client()),
		gitlabapi.WithoutRetries(),
	)
	testastic.NoError(t, err)

	return provider.NewGitLab(client, "o/r")
}

func newGitLabContractHandler(t *testing.T, scenario providerContractScenario) http.Handler {
	t.Helper()
	sharedPathsRecorder := newCommitPathsRecorder(t)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleGitLabBoundaryRequest(t, w, r, map[string]string{
			providerContractTag:             "boundary-sha",
			providerContractIntermediateTag: "intermediate-boundary-sha",
			providerContractMissingTag:      "",
		}) {
			return
		}

		switch scenario {
		case providerContractLatestRelease:
			handleGitLabLatestReleaseContract(t, w, r)
		case providerContractLatestFallbackTags:
			handleGitLabLatestFallbackTagsContract(t, w, r)
		case providerContractListTags:
			handleGitLabListTagsContract(t, w, r)
		case providerContractGetCommitsSinceRefs:
			handleGitLabGetCommitsSinceContract(t, w, r)
		case providerContractGetCommitsSinceRefsMissing:
			handleGitLabGetCommitsSinceMissingContract(t, w, r)
		case providerContractGetCommitsSinceRefsUnresolved:
			handleGitLabGetCommitsSinceUnresolvedContract(t, w, r)
		case providerContractGetCommitsSinceRefsMultiBoundary:
			handleGitLabGetCommitsSinceMultiBoundaryContract(t, w, r)
		case providerContractGetCommitsSinceRefsSharedPaths:
			handleGitLabSharedPathsContract(t, w, r, sharedPathsRecorder)
		case providerContractGetReleaseByTag:
			handleGitLabGetReleaseByTagContract(t, w, r)
		case providerContractTagExists:
			handleGitLabTagExistsContract(t, w, r)
		case providerContractCreateReleasePR:
			handleGitLabCreateReleasePRContract(t, w, r)
		case providerContractCreateReleasePRReviewers:
			handleGitLabCreateReleasePRReviewersContract(t, w, r)
		case providerContractUnknownReviewer:
			handleGitLabUnknownReviewerContract(t, w, r)
		case providerContractUpdateReleasePR:
			handleGitLabUpdateReleasePRContract(t, w, r)
		case providerContractFindOpenPRs:
			handleGitLabFindOpenPRsContract(t, w, r)
		case providerContractFindMergedPR:
			handleGitLabFindMergedPRContract(t, w, r)
		case providerContractMarkReleasePR:
			handleGitLabMarkReleasePRContract(t, w, r)
		case providerContractMergeReleasePR:
			handleGitLabMergeReleasePRContract(t, w, r)
		case providerContractCommitPRBody:
			handleGitLabCommitPRBodyContract(t, w, r)
		case providerContractCreateBranch:
			handleGitLabCreateBranchContract(t, w, r)
		case providerContractCreateRelease:
			handleGitLabCreateReleaseContract(t, w, r)
		case providerContractGetFile:
			handleGitLabGetFileContract(t, w, r)
		case providerContractUpdateFiles:
			handleGitLabUpdateFilesContract(t, w, r)
		case providerContractMissingFile:
			handleGitLabMissingFileContract(t, w, r)
		case providerContractMissingRelease:
			handleGitLabMissingReleaseContract(t, w, r)
		case providerContractMissingPR:
			handleGitLabMissingPRContract(t, w, r)
		case providerContractBlockedMerge:
			handleGitLabBlockedMergeContract(t, w, r)
		case providerContractUnsupportedMerge:
			handleGitLabUnsupportedMergeContract(t, w, r)
		default:
			t.Fatalf("unhandled GitLab contract scenario: %s", scenario)
		}
	})
}

func handleGitLabLatestReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if isGitLabReleaseListRequest(r) {
		writeJSONFixture(t, w, "contracts/gitlab/latest_release/releases.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabLatestFallbackTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case isGitLabReleaseListRequest(r):
		writeJSONFixture(t, w, "contracts/gitlab/latest_fallback_tags/empty_releases.json")
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/tags":
		writeJSONFixture(t, w, "contracts/gitlab/latest_fallback_tags/tags.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabListTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/tags" {
		writeJSONFixture(t, w, "contracts/gitlab/list_tags/tags.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabGetCommitsSinceContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case isGitLabCompareRequest(r, providerContractTag):
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("to"))
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since/compare.json")
	case isGitLabCommitDiffRequest(r, providerContractHeadSHA):
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since/diff.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabGetCommitsSinceMissingContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case isGitLabCompareRequest(r, providerContractTag):
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since/compare.json")
	case isGitLabCompareRequest(r, providerContractMissingTag):
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/gitlab/_shared/not_found.json")
	case isGitLabCommitDiffRequest(r, providerContractHeadSHA):
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since/diff.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabSharedPathsContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	recorder *commitPathsRecorder,
) {
	t.Helper()

	switch {
	case isGitLabCompareRequest(r, providerContractIntermediateTag):
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since_multi_boundary/intermediate_compare.json")
	case isGitLabCompareRequest(r, providerContractTag):
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since_multi_boundary/older_compare.json")
	case isGitLabCommitDiffRequest(r, providerContractHeadSHA):
		recorder.record(providerContractHeadSHA)
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since/diff.json")
	case isGitLabCommitDiffRequest(r, providerContractMidSHA):
		recorder.record(providerContractMidSHA)
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since/diff.json")
	case isGitLabCommitDiffRequest(r, providerContractIntermediateSHA):
		recorder.record(providerContractIntermediateSHA)
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since/diff.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabGetCommitsSinceMultiBoundaryContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case isGitLabCompareRequest(r, providerContractIntermediateTag):
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since_multi_boundary/intermediate_compare.json")
	case isGitLabCompareRequest(r, providerContractTag):
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since_multi_boundary/older_compare.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabGetCommitsSinceUnresolvedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case isGitLabCompareRequest(r, providerContractTag):
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since/compare.json")
	case isGitLabCompareRequest(r, providerContractMissingTag):
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/gitlab/_shared/not_found.json")
	case isGitLabCommitDiffRequest(r, providerContractHeadSHA):
		writeJSONFixture(t, w, "contracts/gitlab/get_commits_since/diff.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabGetReleaseByTagContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/releases/"+providerContractEscapedTag() {
		writeJSONFixture(t, w, "contracts/gitlab/get_release_by_tag/release.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabTagExistsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/repository/tags/"+providerContractEscapedTag() {
		writeJSONFixture(t, w, "contracts/gitlab/tag_exists/tag.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabCreateReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPost || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests" {
		fatalUnexpectedProviderRequest(t, "GitLab", r)

		return
	}

	var request struct {
		Title        string `json:"title"`
		Description  string `json:"description"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractReleaseTitle, request.Title)
	testastic.Equal(t, providerContractReleaseBody, request.Description)
	testastic.Equal(t, providerContractReleaseBranch, request.SourceBranch)
	testastic.Equal(t, providerContractBaseBranch, request.TargetBranch)

	writeJSONFixture(t, w, "contracts/gitlab/create_release_pr/response.json")
}

func handleGitLabCreateReleasePRReviewersContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/members/all":
		writeGitLabMemberFixture(t, w, r.URL.Query().Get("query"))
	case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests":
		var request struct {
			Title       string  `json:"title"`
			ReviewerIDs []int64 `json:"reviewer_ids"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, providerContractReleaseTitle, request.Title)
		testastic.SliceEqual(t, []int64{101, 102}, request.ReviewerIDs)
		writeJSONFixture(t, w, "contracts/gitlab/create_release_pr_reviewers/response.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabUnknownReviewerContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/members/all" {
		testastic.Equal(t, providerContractUnknownReviewerName, r.URL.Query().Get("query"))
		writeJSONFixture(t, w, "contracts/gitlab/unknown_reviewer/members_empty.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func writeGitLabMemberFixture(t *testing.T, w http.ResponseWriter, query string) {
	t.Helper()

	switch query {
	case providerContractReviewerAlice:
		writeJSONFixture(t, w, "contracts/gitlab/create_release_pr_reviewers/members_alice.json")
	case providerContractReviewerBob:
		writeJSONFixture(t, w, "contracts/gitlab/create_release_pr_reviewers/members_bob.json")
	default:
		t.Fatalf("unexpected GitLab member lookup: %s", query)
	}
}

func TestGitLabFailsWhenReviewerIsDropped(t *testing.T) {
	t.Parallel()

	// given: a GitLab server that resolves both reviewers but applies only the
	// first one on the created MR (Free-tier truncation behavior)
	pendingMarked := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/members/all":
			writeGitLabMemberFixture(t, w, r.URL.Query().Get("query"))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests":
			writeJSONFixture(t, w, "contracts/gitlab/create_release_pr_reviewers/response_dropped.json")
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
			writeGitLabLabelFixture(t, w, decodedPathTail(t, r))
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
			pendingMarked = true

			writeJSONFixture(t, w, "contracts/gitlab/mark_release_pr/update.json")
		default:
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		}
	}))
	defer server.Close()

	p := newGitLabContractProvider(t, server)

	// when: creating a release MR with two reviewers
	_, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
		Title:         providerContractReleaseTitle,
		Body:          providerContractReleaseBody,
		BaseBranch:    providerContractBaseBranch,
		ReleaseBranch: providerContractReleaseBranch,
		Reviewers:     []string{providerContractReviewerAlice, providerContractReviewerBob},
	})

	// then: the run fails naming the reviewer GitLab silently dropped
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, provider.ErrReviewerNotApplied)
	testastic.ErrorContains(t, err, providerContractReviewerBob)
	testastic.True(t, pendingMarked)
}

func handleGitLabUpdateReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPut || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests/42" {
		fatalUnexpectedProviderRequest(t, "GitLab", r)

		return
	}

	var request struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractReleaseTitle, request.Title)
	testastic.Equal(t, "updated release body", request.Description)

	writeJSONFixture(t, w, "contracts/gitlab/update_release_pr/response.json")
}

func handleGitLabFindOpenPRsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		testastic.Equal(t, "opened", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("target_branch"))
		writeJSONFixture(t, w, "contracts/gitlab/find_open_prs/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabFindMergedPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		testastic.Equal(t, "merged", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("target_branch"))
		writeJSONFixture(t, w, "contracts/gitlab/find_merged_pr/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabMarkReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
		writeGitLabLabelFixture(t, w, decodedPathTail(t, r))
	case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
		var request struct {
			AddLabels    string `json:"add_labels"`
			RemoveLabels string `json:"remove_labels"`
		}
		decodeJSONRequest(t, r, &request)
		writeJSONFixture(t, w, "contracts/gitlab/mark_release_pr/update.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabMergeReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
		writeJSONFixture(t, w, "contracts/gitlab/merge_release_pr/pr.json")
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
		writeJSONFixture(t, w, "contracts/gitlab/merge_release_pr/project.json")
	case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42/merge":
		var request struct {
			SHA string `json:"sha"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, providerContractHeadSHA, request.SHA)
		writeJSONFixture(t, w, "contracts/gitlab/merge_release_pr/result.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabCommitPRBodyContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/repository/commits/"+providerContractMergeSHA+"/merge_requests" {
		writeJSONFixture(t, w, "contracts/gitlab/commit_pr_body/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabCreateBranchContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches" {
		writeJSONFixture(t, w, "contracts/gitlab/create_branch/branch.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabCreateReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if !isGitLabCreateReleaseRequest(r) {
		fatalUnexpectedProviderRequest(t, "GitLab", r)

		return
	}

	var request struct {
		TagName     string `json:"tag_name"`
		Ref         string `json:"ref"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractTag, request.TagName)
	testastic.Equal(t, providerContractBaseBranch, request.Ref)
	testastic.Equal(t, providerContractTag, request.Name)
	testastic.Equal(t, "release notes", request.Description)

	writeJSONFixture(t, w, "contracts/gitlab/create_release/release.json")
}

func handleGitLabGetFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && isGitLabRawFilePath(r, "CHANGELOG.md") {
		writeTextFixture(t, w, "contracts/gitlab/get_file/file.txt")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabUpdateFilesContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && isGitLabRawFilePath(r, "CHANGELOG.md"):
		writeTextFixture(t, w, "contracts/gitlab/update_files/file.txt")
	case r.Method == http.MethodGet && isGitLabRawFilePath(r, "VERSION.txt"):
		http.NotFound(w, r)
	case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/commits":
		writeJSONFixture(t, w, "contracts/gitlab/update_files/push.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabMissingFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && isGitLabRawFilePath(r, "MISSING.md") {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/gitlab/_shared/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabMissingReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/releases/"+providerContractEscapedTag() {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/gitlab/_shared/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabMissingPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		writeJSONFixture(t, w, "contracts/gitlab/missing_pr/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabBlockedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42" {
		writeJSONFixture(t, w, "contracts/gitlab/blocked_merge/pr.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabUnsupportedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
		writeJSONFixture(t, w, "contracts/gitlab/unsupported_merge/pr.json")
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
		writeJSONFixture(t, w, "contracts/gitlab/unsupported_merge/project.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func isGitLabReleaseListRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/releases"
}

func providerContractEscapedTag() string {
	return strings.ReplaceAll(providerContractTag, ".", "%2E")
}

func writeGitLabLabelFixture(t *testing.T, w http.ResponseWriter, label string) {
	t.Helper()

	switch label {
	case provider.ReleaseLabelPending:
		writeJSONFixture(t, w, "contracts/gitlab/mark_release_pr/label_pending.json")
	case provider.ReleaseLabelTagged:
		writeJSONFixture(t, w, "contracts/gitlab/mark_release_pr/label_tagged.json")
	default:
		t.Fatalf("unexpected GitLab label: %s", label)
	}
}
