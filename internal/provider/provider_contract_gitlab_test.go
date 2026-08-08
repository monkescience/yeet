package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go/v2"
)

func newGitLabContractProvider(
	t *testing.T,
	server *httptest.Server,
	options ...provider.MergePollingOption,
) provider.Provider {
	t.Helper()

	client, err := gitlabapi.NewClient(
		"",
		gitlabapi.WithBaseURL(server.URL),
		gitlabapi.WithHTTPClient(server.Client()),
		gitlabapi.WithoutRetries(),
	)
	testastic.NoError(t, err)

	return provider.NewGitLab(client, "o/r", options...)
}

func newGitLabContractHandler(t *testing.T, scenario providerContractScenario) http.Handler {
	t.Helper()

	var mergeAccepted atomic.Bool

	var tagPages atomic.Int32

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case providerContractListTags:
			handleGitLabListTagsContract(t, w, r)
		case providerContractListTagsPaged:
			handleGitLabListTagsPagedContract(t, w, r)
		case providerContractBranchHead:
			handleGitLabBranchHeadContract(t, w, r)
		case providerContractBranchHeadMissing:
			handleGitLabBranchHeadMissingContract(t, w, r)
		case providerContractGetReleaseByTag:
			handleGitLabGetReleaseByTagContract(t, w, r)
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
		case providerContractFindOpenPRsUnlabeled:
			handleGitLabFindOpenPRsListContract(
				t, w, r,
				gitLabLabelledOpenMRResponse([]string{testReleaseLabelPending}),
			)
		case providerContractFindOpenPRsAdoptable:
			handleGitLabFindOpenPRsListContract(t, w, r, gitLabLabelledOpenMRResponse(nil))
		case providerContractFindMergedPR:
			handleGitLabFindMergedPRContract(t, w, r)
		case providerContractMergeReleasePR:
			handleGitLabMergeReleasePRContract(t, w, r)
		case providerContractAsyncMergeReleasePR:
			handleGitLabAsyncMergeReleasePRContract(t, w, r, &mergeAccepted)
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
		case providerContractTagPaginationLimit:
			handleGitLabTagPaginationLimitContract(t, w, r, &tagPages)
		case providerContractForcedMergeUntrusted:
			handleGitLabForcedMergeUntrustedContract(t, w, r)
		case providerContractForcedMergeConflicted:
			handleGitLabForcedMergeConflictedContract(t, w, r)
		default:
			t.Fatalf("unhandled GitLab contract scenario: %s", scenario)
		}
	})
}

func handleGitLabBranchHeadContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet &&
		r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches/"+providerContractBaseBranch {
		writeJSON(t, w, gitLabBranchResponse(providerContractBaseBranch, providerContractHeadSHA))

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabBranchHeadMissingContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet &&
		r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches/missing-branch" {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(t, w, gitLabNotFoundResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabListTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/tags" {
		writeJSON(t, w, gitLabTagsResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabListTagsPagedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/repository/tags" {
		fatalUnexpectedProviderRequest(t, "GitLab", r)

		return
	}

	if r.URL.Query().Get("page") == "2" {
		writeJSON(t, w, gitLabTagsPageTwoResponse())

		return
	}

	w.Header().Set("X-Next-Page", "2")
	writeJSON(t, w, gitLabTagsResponse())
}

func handleGitLabGetReleaseByTagContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/releases/"+providerContractEscapedTag() {
		writeJSON(t, w, gitLabReleaseResponse())

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

	writeJSON(t, w, gitLabReleaseMRResponse(providerContractReleaseBody))
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
		writeJSON(t, w, gitLabReleaseMRWithReviewersResponse(
			gitLabMemberResponse(gitLabContractAliceID, providerContractReviewerAlice),
			gitLabMemberResponse(gitLabContractBobID, providerContractReviewerBob),
		))
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabUnknownReviewerContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/members/all" {
		testastic.Equal(t, providerContractUnknownReviewerName, r.URL.Query().Get("query"))
		writeJSON(t, w, []map[string]any{})

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func writeGitLabMemberFixture(t *testing.T, w http.ResponseWriter, query string) {
	t.Helper()

	switch query {
	case providerContractReviewerAlice:
		writeJSON(t, w, []map[string]any{gitLabMemberResponse(gitLabContractAliceID, providerContractReviewerAlice)})
	case providerContractReviewerBob:
		writeJSON(t, w, []map[string]any{gitLabMemberResponse(gitLabContractBobID, providerContractReviewerBob)})
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
			writeJSON(t, w, gitLabReleaseMRWithReviewersResponse(
				gitLabMemberResponse(gitLabContractAliceID, providerContractReviewerAlice),
			))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
			writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
			pendingMarked = true

			writeJSON(t, w, gitLabUpdatedMRResponse())
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
		Labels:        defaultReleasePRLabels(),
	})

	// then: the run fails naming the reviewer GitLab silently dropped
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, provider.ErrReviewerNotApplied)
	testastic.Equal(
		t,
		"reviewer not applied: [bob] (multiple merge request reviewers require GitLab Premium or "+
			"Ultimate)",
		err.Error(),
	)
	testastic.True(t, pendingMarked)
}

func TestGitLabFindsReviewerOnLaterMemberPage(t *testing.T) {
	t.Parallel()

	// given: a GitLab server whose member list spans two pages, with the
	// requested reviewer only on the second
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/members/all":
			testastic.Equal(t, providerContractReviewerAlice, r.URL.Query().Get("query"))

			if r.URL.Query().Get("page") == "2" {
				writeJSON(t, w, []map[string]any{{"id": 101, "username": providerContractReviewerAlice}})

				return
			}

			w.Header().Set("X-Next-Page", "2")
			writeJSON(t, w, gitLabOtherMembers(100))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests":
			var request struct {
				ReviewerIDs []int64 `json:"reviewer_ids"`
			}
			decodeJSONRequest(t, r, &request)
			testastic.SliceEqual(t, []int64{101}, request.ReviewerIDs)
			writeJSON(t, w, map[string]any{
				"iid":           42,
				"title":         providerContractReleaseTitle,
				"description":   providerContractReleaseBody,
				"web_url":       "https://example.com/pulls/42",
				"source_branch": providerContractReleaseBranch,
				"reviewers":     []map[string]any{{"id": 101, "username": providerContractReviewerAlice}},
			})
		default:
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		}
	}))
	defer server.Close()

	p := newGitLabContractProvider(t, server)

	// when: creating a release MR with that reviewer
	pr, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
		Title:         providerContractReleaseTitle,
		Body:          providerContractReleaseBody,
		BaseBranch:    providerContractBaseBranch,
		ReleaseBranch: providerContractReleaseBranch,
		Reviewers:     []string{providerContractReviewerAlice},
		Labels:        defaultReleasePRLabels(),
	})

	// then: the reviewer resolves and the merge request is created
	testastic.NoError(t, err)
	testastic.Equal(t, 42, pr.Number)
}

func gitLabOtherMembers(count int) []map[string]any {
	members := make([]map[string]any, 0, count)

	for i := range count {
		members = append(members, map[string]any{
			"id":       200 + i,
			"username": fmt.Sprintf("%s-%d", providerContractReviewerAlice, i),
		})
	}

	return members
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

	writeJSON(t, w, gitLabReleaseMRResponse(providerContractUpdatedReleaseBody))
}

func handleGitLabFindOpenPRsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		testastic.Equal(t, "opened", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("target_branch"))
		writeJSON(t, w, gitLabOpenMRsResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabFindOpenPRsListContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	prs []map[string]any,
) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		testastic.Equal(t, "", r.URL.Query().Get("labels"))
		testastic.Equal(t, providerContractPendingBranch, r.URL.Query().Get("source_branch"))
		writeJSON(t, w, prs)

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabFindMergedPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		testastic.Equal(t, "merged", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("target_branch"))
		writeJSON(t, w, gitLabMergedMRsResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

// newGitLabContractLabelHandler tracks the labels on MR 42 so a scenario can
// assert the set a phase leaves behind.
func newGitLabContractLabelHandler(
	t *testing.T,
	store *providerContractLabelStore,
	registry providerContractLabelRegistry,
) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
			name := decodedPathTail(t, r)
			if status, answered := registry.status(name); answered {
				w.WriteHeader(status)
				writeJSON(t, w, gitLabNotFoundResponse())

				return
			}

			writeJSON(t, w, map[string]any{"name": name})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
			var request struct {
				AddLabels    string `json:"add_labels"`
				RemoveLabels string `json:"remove_labels"`
			}
			decodeJSONRequest(t, r, &request)
			store.attach(splitProviderContractLabels(request.AddLabels)...)
			store.detach(splitProviderContractLabels(request.RemoveLabels)...)
			writeJSON(t, w, gitLabUpdatedMRResponse())
		default:
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		}
	})
}

func handleGitLabMergeReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
		writeJSON(t, w, gitLabMergeStateMRResponse("mergeable"))
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
		writeJSON(t, w, gitLabMergeCommitProjectResponse())
	case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42/merge":
		var request struct {
			SHA string `json:"sha"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, providerContractHeadSHA, request.SHA)
		writeJSON(t, w, gitLabMergeResultResponse())
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

// handleGitLabAsyncMergeReleasePRContract models an accept GitLab queues, leaving
// the merge request open until the merge is applied.
func handleGitLabAsyncMergeReleasePRContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	mergeAccepted *atomic.Bool,
) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
		if mergeAccepted.Load() {
			writeJSON(t, w, map[string]any{
				"iid":              42,
				"state":            "merged",
				"merge_commit_sha": providerContractMergeSHA,
			})

			return
		}

		writeJSON(t, w, gitLabMergeStateMRResponse("mergeable"))
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
		writeJSON(t, w, gitLabMergeCommitProjectResponse())
	case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42/merge":
		mergeAccepted.Store(true)
		writeJSON(t, w, map[string]any{"iid": 42, "state": "opened"})
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabCreateBranchContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches" {
		writeJSON(t, w, gitLabBranchResponse(providerContractReleaseBranch, gitLabContractBaseRefSHA))

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

	writeJSON(t, w, gitLabReleaseResponse())
}

func handleGitLabGetFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && isGitLabRawFilePath(r, "CHANGELOG.md") {
		writeText(t, w, providerContractChangelogContent)

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabUpdateFilesContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/commits":
		var request struct {
			Branch        string `json:"branch"`
			CommitMessage string `json:"commit_message"`
			StartBranch   string `json:"start_branch"`
			Force         bool   `json:"force"`
			Actions       []struct {
				Action   string `json:"action"`
				FilePath string `json:"file_path"`
			} `json:"actions"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, providerContractReleaseBranch, request.Branch)
		testastic.Equal(t, providerContractBaseBranch, request.StartBranch)
		testastic.Equal(t, "chore: release v1.2.3", request.CommitMessage)
		testastic.True(t, request.Force)
		testastic.Equal(t, 2, len(request.Actions))
		testastic.Equal(t, "CHANGELOG.md", request.Actions[0].FilePath)
		testastic.Equal(t, "update", request.Actions[0].Action)
		testastic.Equal(t, "VERSION.txt", request.Actions[1].FilePath)
		testastic.Equal(t, "create", request.Actions[1].Action)
		writeJSON(t, w, gitLabPushResponse())
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func handleGitLabMissingFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && isGitLabRawFilePath(r, "MISSING.md") {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(t, w, gitLabNotFoundResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabMissingReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() ==
		"/api/v4/projects/o%2Fr/releases/"+providerContractEscapedTag() {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(t, w, gitLabNotFoundResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabMissingPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		writeJSON(t, w, []map[string]any{})

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabBlockedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42" {
		writeJSON(t, w, gitLabMergeStateMRResponse("not_approved"))

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabUnsupportedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
		writeJSON(t, w, gitLabMergeStateMRResponse("mergeable"))
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
		writeJSON(t, w, gitLabMergeCommitProjectResponse())
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
}

func providerContractEscapedTag() string {
	return strings.ReplaceAll(providerContractTag, ".", "%2E")
}

func handleGitLabTagPaginationLimitContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	pages *atomic.Int32,
) {
	t.Helper()

	if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/repository/tags" {
		fatalUnexpectedProviderRequest(t, "GitLab", r)

		return
	}

	page := pages.Add(1)
	w.Header().Set("X-Next-Page", strconv.Itoa(int(page)+1))
	writeJSON(t, w, []map[string]any{{
		"name":   fmt.Sprintf("v0.0.%d", page),
		"commit": map[string]any{"id": fmt.Sprintf("sha-%d", page)},
	}})
}

func handleGitLabForcedMergeUntrustedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42" {
		mr := gitLabMergeStateMRResponse("mergeable")
		mr["source_project_id"] = gitLabContractForkProjectID
		writeJSON(t, w, mr)

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabForcedMergeConflictedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42" {
		writeJSON(t, w, gitLabConflictedMRResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}
