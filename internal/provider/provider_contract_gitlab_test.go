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
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go/v2"
)

const gitLabSourceTipSHA = "736f757263657469707368610000000000000000"

func newGitLabContractProvider(
	t *testing.T,
	server *httptest.Server,
	options ...provider.MergePollingOption,
) forge.Provider {
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
			handleGitLabFindOpenPRsFixtureContract(t, w, r, "find_open_prs_unlabeled")
		case providerContractFindOpenPRsAdoptable:
			handleGitLabFindOpenPRsFixtureContract(t, w, r, "find_open_prs_adoptable")
		case providerContractFindMergedPR:
			handleGitLabFindMergedPRContract(t, w, r)
		case providerContractMergeReleasePR:
			handleGitLabMergeReleasePRContract(t, w, r)
		case providerContractAsyncMergeReleasePR:
			handleGitLabAsyncMergeReleasePRContract(t, w, r, &mergeAccepted)
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
			failProviderContractHandler(t, fmt.Sprintf("unhandled GitLab contract scenario: %s", scenario))
		}
	})
}

func handleGitLabBranchHeadContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet &&
		r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches/"+providerContractBaseBranch {
		writeJSONFixture(t, w, "contracts/gitlab/branch_head/branch.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabBranchHeadMissingContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet &&
		r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches/missing-branch" {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/gitlab/_shared/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabListTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/tags" {
		writeJSONFixture(t, w, "contracts/gitlab/list_tags/tags.json")

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
		writeJSONFixture(t, w, "contracts/gitlab/list_tags/page_two.json")

		return
	}

	w.Header().Set("X-Next-Page", "2")
	writeJSONFixture(t, w, "contracts/gitlab/list_tags/tags.json")
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
		failProviderContractHandler(t, "unexpected GitLab member lookup: "+query)
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
			writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
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
	_, err := p.CreateReleasePR(context.Background(), forge.ReleasePROptions{
		Title:         providerContractReleaseTitle,
		Body:          providerContractReleaseBody,
		BaseBranch:    providerContractBaseBranch,
		ReleaseBranch: providerContractReleaseBranch,
		Reviewers:     []string{providerContractReviewerAlice, providerContractReviewerBob},
		Labels:        defaultReleasePRLabels(),
	})

	// then: the run fails naming the reviewer GitLab silently dropped
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, forge.ErrReviewerNotApplied)
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
	pr, err := p.CreateReleasePR(context.Background(), forge.ReleasePROptions{
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

func handleGitLabFindOpenPRsFixtureContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	dir string,
) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		testastic.Equal(t, "", r.URL.Query().Get("labels"))
		testastic.Equal(t, providerContractPendingBranch, r.URL.Query().Get("source_branch"))
		writeJSONFixture(t, w, "contracts/gitlab/"+dir+"/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabFindMergedPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests" {
		testastic.Equal(t, "merged", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("target_branch"))
		testastic.Equal(t, providerContractPendingBranch, r.URL.Query().Get("source_branch"))
		writeJSONFixture(t, w, "contracts/gitlab/find_merged_pr/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

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
				writeJSONFixture(t, w, "contracts/gitlab/_shared/not_found.json")

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
			writeJSONFixture(t, w, "contracts/gitlab/mark_release_pr/update.json")
		default:
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		}
	})
}

func TestGitLabCachesSuccessfulLabelDefinitionsAcrossReleasePhases(t *testing.T) {
	t.Parallel()

	// given: a GitLab label registry containing every managed label
	lookups := make(map[string]int)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
			name := decodedPathTail(t, r)
			lookups[name]++
			writeJSON(t, w, map[string]any{"name": name})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
			writeJSON(t, w, map[string]any{"iid": 42})
		default:
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		}
	}))
	defer server.Close()

	p := newGitLabContractProvider(t, server)
	labels := providerContractManagedLabels()

	// when: validation, pending application, preflight, and tagged application share one provider
	err := p.SetReleasePRLabels(context.Background(), 42, labels, forge.ReleasePRPhasePending)
	testastic.NoError(t, err)
	err = p.PreflightReleasePRTagging(context.Background(), labels.Tagged)
	testastic.NoError(t, err)
	err = p.SetReleasePRLabels(context.Background(), 42, labels, forge.ReleasePRPhaseTagged)
	testastic.NoError(t, err)

	// then: each distinct successful definition is fetched once
	testastic.Len(t, lookups, 5)

	for _, count := range lookups {
		testastic.Equal(t, 1, count)
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

		writeJSONFixture(t, w, "contracts/gitlab/merge_release_pr/pr.json")
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
		writeJSONFixture(t, w, "contracts/gitlab/merge_release_pr/project.json")
	case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42/merge":
		mergeAccepted.Store(true)
		writeJSON(t, w, map[string]any{"iid": 42, "state": "opened"})
	default:
		fatalUnexpectedProviderRequest(t, "GitLab", r)
	}
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
	testastic.Equal(t, providerContractHeadSHA, request.Ref)
	testastic.Equal(t, providerContractTag, request.Name)
	testastic.Equal(t, "release notes", request.Description)

	writeJSONFixture(t, w, "contracts/gitlab/create_release/release.json")
}

func TestGitLabCreateReleaseRejectsConflictingCommit(t *testing.T) {
	t.Parallel()

	// given: GitLab creates a release for a pre-existing tag at another commit
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isGitLabCreateReleaseRequest(r) {
			fatalUnexpectedProviderRequest(t, "GitLab", r)

			return
		}

		writeJSON(t, w, map[string]any{
			"tag_name": providerContractTag,
			"commit":   map[string]any{"id": providerContractTagCommitSHA},
		})
	}))
	defer server.Close()

	p := newGitLabContractProvider(t, server)

	// when: creating the release for the expected branch-head commit
	release, err := p.CreateRelease(context.Background(), forge.ReleaseOptions{
		TagName: providerContractTag,
		Ref:     providerContractHeadSHA,
		Name:    providerContractTag,
		Body:    "release notes",
	})

	// then: the conflicting tag target is rejected
	testastic.ErrorIs(t, err, forge.ErrReleaseTagMismatch)
	testastic.True(t, release == nil)
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
		writeJSONFixture(t, w, "contracts/gitlab/forced_merge_untrusted/pr.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func handleGitLabForcedMergeConflictedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42" {
		writeJSONFixture(t, w, "contracts/gitlab/forced_merge_conflicted/pr.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitLab", r)
}

func TestGitLabCreateReleaseRejectsRefsThatAreNotCommitSHAs(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name string
		ref  string
	}{
		{name: "rejects a branch name", ref: providerContractBaseBranch},
		{name: "rejects an abbreviated SHA", ref: "6865616473"},
		{name: "rejects a blank ref", ref: "  "},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a GitLab provider whose server fails any request it receives
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}))
			defer server.Close()

			p := newGitLabContractProvider(t, server)

			// when: creating a release for a ref that is not a commit SHA
			_, err := p.CreateRelease(context.Background(), forge.ReleaseOptions{
				TagName: providerContractTag,
				Ref:     scenario.ref,
				Name:    providerContractTag,
				Body:    providerContractReleaseNotes,
			})

			// then: the sentinel for an unusable ref is returned before any request
			testastic.ErrorIs(t, err, forge.ErrInvalidCommitSHA)
		})
	}
}

func TestGitLabFindMergedReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("uses source tip for fast-forward merged MR", func(t *testing.T) {
		t.Parallel()

		// given: GitLab returns a fast-forward merged MR without merge or squash commit SHAs
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests" {
				fatalUnexpectedProviderRequest(t, "GitLab", r)

				return
			}

			assertJSONRequest(t, r, "contracts/gitlab/find_merged_pr_fast_forward/request.json")
			writeJSONFixture(t, w, "contracts/gitlab/find_merged_pr_fast_forward/prs.json")
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: finding the fast-forward merged release MR
		pr, err := p.FindMergedReleasePR(context.Background(), providerContractBaseBranch, testReleaseLabelPending)

		// then: the source tip identifies the commit now on the target branch
		testastic.NoError(t, err)
		testastic.Equal(t, 6, pr.Number)
		testastic.Equal(t, gitLabSourceTipSHA, pr.MergeCommitSHA)
	})

	t.Run("reports the withheld merge time before re-reading the next candidate", func(t *testing.T) {
		t.Parallel()

		// given: two competing MRs listed without merge times, where re-reading the
		// first still withholds one and re-reading the second fails
		var rereads []string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.EscapedPath() {
			case "/api/v4/projects/o%2Fr/merge_requests":
				writeJSONFixture(t, w, "contracts/gitlab/find_merged_pr_undated/prs.json")
			case "/api/v4/projects/o%2Fr/merge_requests/5":
				rereads = append(rereads, "5")

				writeJSONFixture(t, w, "contracts/gitlab/find_merged_pr_undated/mr_5.json")
			case "/api/v4/projects/o%2Fr/merge_requests/9":
				rereads = append(rereads, "9")

				w.WriteHeader(http.StatusInternalServerError)
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: finding the merged release MR
		_, err := p.FindMergedReleasePR(context.Background(), providerContractBaseBranch, testReleaseLabelPending)

		// then: the ambiguity names the MR that caused it, and no later failure masks it
		testastic.Contains(t, err.Error(), "merged release PR completion time is unavailable: merge request !5")
		testastic.SliceEqual(t, []string{"5"}, rereads)
	})

	t.Run("selects the most recently merged release MR", func(t *testing.T) {
		t.Parallel()

		// given: two merged release MRs where the most recently updated one was
		// merged earlier than the other
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests" {
				fatalUnexpectedProviderRequest(t, "GitLab", r)

				return
			}

			assertJSONRequest(t, r, "contracts/gitlab/find_merged_pr_most_recent/request.json")
			writeJSONFixture(t, w, "contracts/gitlab/find_merged_pr_most_recent/prs.json")
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: finding merged release MR
		pr, err := p.FindMergedReleasePR(context.Background(), providerContractBaseBranch, testReleaseLabelPending)

		// then: the most recently merged MR is returned, not the most recently updated
		testastic.NoError(t, err)
		testastic.Equal(t, 8, pr.Number)
		testastic.Equal(t, "6672657368736861000000000000000000000000", pr.MergeCommitSHA)
	})
}

func TestGitLabEnsureLabel(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		yeet     bool
		phase    forge.ReleasePRPhase
		expected []string
	}{
		{
			name:     "creates managed and lifecycle labels when not found",
			yeet:     true,
			phase:    forge.ReleasePRPhasePending,
			expected: []string{provider.ReleaseLabelYeet, testReleaseLabelPending, testReleaseLabelTagged},
		},
		{
			name:     "does not create managed label when disabled",
			yeet:     false,
			phase:    forge.ReleasePRPhasePending,
			expected: []string{testReleaseLabelPending, testReleaseLabelTagged},
		},
		{
			name:     "tagged transition recreates only the tagged label",
			yeet:     true,
			phase:    forge.ReleasePRPhaseTagged,
			expected: []string{testReleaseLabelTagged},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a GitLab API where the labels do not exist
			var created []string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && strings.Contains(r.URL.EscapedPath(), "/labels/"):
					w.WriteHeader(http.StatusNotFound)
					writeJSONFixture(t, w, "contracts/gitlab/_shared/not_found.json")
				case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/labels":
					var request struct {
						Name string `json:"name"`
					}
					decodeJSONRequest(t, r, &request)
					created = append(created, request.Name)

					w.WriteHeader(http.StatusCreated)
					writeJSON(t, w, map[string]any{"name": request.Name})
				case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
					writeJSONFixture(t, w, "contracts/gitlab/ensure_label/update.json")
				default:
					fatalUnexpectedProviderRequest(t, "GitLab", r)
				}
			}))
			defer server.Close()

			p := newGitLabContractProvider(t, server)
			labels := defaultReleasePRLabels()
			labels.Yeet = testCase.yeet

			// when: the requested phase is applied
			err := p.SetReleasePRLabels(context.Background(), 42, labels, testCase.phase)

			// then: only definitions owned by that phase are created
			testastic.NoError(t, err)
			testastic.SliceEqual(t, testCase.expected, created)
		})
	}
}

func TestGitLabMergeReleasePRMethods(t *testing.T) {
	t.Parallel()

	t.Run("auto method prefers squash when the project permits it", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab project that allows squashing but does not force it
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_auto_squash/pr.json")
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_auto_squash/project.json")
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1/merge":
				assertJSONRequest(t, r, "contracts/gitlab/merge_methods_auto_squash/merge_request.json")
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_auto_squash/result.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: merging with auto method
		_, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			Method: forge.MergeMethodAuto,
		})

		// then: squash is requested
		testastic.NoError(t, err)
	})

	t.Run("auto method does not squash when the project forbids it", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab project that forbids squashing
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_auto_no_squash/pr.json")
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_auto_no_squash/project.json")
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1/merge":
				assertJSONRequest(t, r, "contracts/gitlab/merge_methods_auto_no_squash/merge_request.json")
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_auto_no_squash/result.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: merging with auto method
		_, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			Method: forge.MergeMethodAuto,
		})

		// then: the project's own merge method is left untouched
		testastic.NoError(t, err)
	})

	t.Run("auto method returns source tip for fast-forward merge", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab project using fast-forward merges without squashing
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_fast_forward/pr.json")
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_fast_forward/project.json")
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1/merge":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_fast_forward/result.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: merging with the project's fast-forward method
		mergeSHA, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			Method: forge.MergeMethodAuto,
		})

		// then: the source tip is returned as the final commit on the target branch
		testastic.NoError(t, err)
		testastic.Equal(t, gitLabSourceTipSHA, mergeSHA)
	})

	t.Run("auto method waits for the asynchronous accept to finalize", func(t *testing.T) {
		t.Parallel()

		// given: GitLab accepts the MR asynchronously and reports it merged shortly after
		var accepted atomic.Bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				if accepted.Load() {
					writeJSONFixture(t, w, "contracts/gitlab/merge_methods_async_finalize/pr_merged.json")

					return
				}

				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_async_finalize/pr_opened.json")
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_async_finalize/project.json")
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1/merge":
				accepted.Store(true)
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_async_finalize/accept.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		}))
		defer server.Close()

		p := newGitLabContractProvider(
			t,
			server,
			provider.WithMergePolling(time.Millisecond, time.Millisecond, 5*time.Second),
		)

		// when: merging with the project's asynchronous fast-forward flow
		mergeSHA, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			Method: forge.MergeMethodAuto,
		})

		// then: the source tip is returned once GitLab reports the MR merged
		testastic.NoError(t, err)
		testastic.Equal(t, gitLabSourceTipSHA, mergeSHA)
	})

	t.Run("auto method reports an accept that never finalizes", func(t *testing.T) {
		t.Parallel()

		// given: GitLab accepts the MR but never reports it merged
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_never_finalizes/pr.json")
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_never_finalizes/project.json")
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1/merge":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_never_finalizes/accept.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		}))
		defer server.Close()

		p := newGitLabContractProvider(
			t,
			server,
			provider.WithMergePolling(time.Millisecond, time.Millisecond, 50*time.Millisecond),
		)

		// when: merging with the project's asynchronous fast-forward flow
		_, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			Method: forge.MergeMethodAuto,
		})

		// then: the unfinalized merge is reported instead of an empty commit
		testastic.ErrorIs(t, err, forge.ErrMergeNotFinalized)
	})

	t.Run("squash blocked by project settings", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab project with squash disabled
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_squash_forbidden/pr.json")
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSONFixture(t, w, "contracts/gitlab/merge_methods_squash_forbidden/project.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: merging with squash method
		_, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            forge.MergeMethodSquash,
		})

		// then: merge is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
	})
}

func TestGitLabFindsUnlabelledOpenReleaseMRInOneListing(t *testing.T) {
	t.Parallel()

	// given: a GitLab project whose only release MR was left unlabelled
	var listings atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests" {
			fatalUnexpectedProviderRequest(t, "GitLab", r)

			return
		}

		listings.Add(1)

		assertJSONRequest(t, r, "contracts/gitlab/find_open_prs_unlabelled_single_listing/request.json")
		writeJSONFixture(t, w, "contracts/gitlab/find_open_prs_unlabelled_single_listing/prs.json")
	}))
	defer server.Close()

	p := newGitLabContractProvider(t, server)

	// when: finding open pending release MRs
	prs, err := p.FindOpenPendingReleasePRs(context.Background(), providerContractBaseBranch, testReleaseLabelPending)

	// then: one source-branch listing finds the MR and offers it for adoption
	testastic.NoError(t, err)
	testastic.Equal(t, int32(1), listings.Load())
	testastic.Equal(t, 1, len(prs))
	testastic.Equal(t, 10, prs[0].Number)
	testastic.True(t, prs[0].NeedsPendingLabel)
}

func TestGitLabMatchesThePendingLabelExactly(t *testing.T) {
	t.Parallel()

	// given: an open release MR labelled in a different case than the configured
	// pending label, which GitLab treats as a distinct label
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests" {
			fatalUnexpectedProviderRequest(t, "GitLab", r)

			return
		}

		assertJSONRequest(t, r, "contracts/gitlab/find_open_prs_case_sensitive/request.json")
		writeJSONFixture(t, w, "contracts/gitlab/find_open_prs_case_sensitive/prs.json")
	}))
	defer server.Close()

	p := newGitLabContractProvider(t, server)

	// when: finding open pending release MRs
	prs, err := p.FindOpenPendingReleasePRs(context.Background(), providerContractBaseBranch, testReleaseLabelPending)

	// then: the case variant is a different label, so the MR is a mismatch naming
	// the merge request, its branch and the label yeet expected
	testastic.ErrorIs(t, err, forge.ErrReleasePRLabelMismatch)
	testastic.Equal(t, 0, len(prs))
	testastic.Contains(
		t,
		err.Error(),
		`trusted merge request !10 on branch "yeet/release-main" is missing configured pending label "autorelease: pending"`,
	)
}

func TestGitLabReleasePRLabelPreflight(t *testing.T) {
	t.Parallel()

	t.Run("allows lifecycle labels sharing a scope", func(t *testing.T) {
		t.Parallel()

		// given: sequential lifecycle labels in one GitLab exclusive scope
		var calls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: the pending phase is applied
		err := p.SetReleasePRLabels(context.Background(), 42, forge.ReleasePRLabels{
			Pending: "release::pending",
			Tagged:  "release::tagged",
		}, forge.ReleasePRPhasePending)

		// then: both sequential lifecycle states are accepted, prepared and applied
		testastic.NoError(t, err)
		testastic.Equal(t, int32(3), calls.Load())
	})

	t.Run("rejects an extra label sharing a lifecycle scope", func(t *testing.T) {
		t.Parallel()

		// given: a permanent extra label sharing the pending lifecycle scope
		var calls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)
		labels := forge.ReleasePRLabels{
			Pending: "workflow::backend::pending",
			Tagged:  "release::tagged",
			Extra:   []string{"workflow::backend::automated"},
		}

		// when: the pending phase is applied
		err := p.SetReleasePRLabels(context.Background(), 42, labels, forge.ReleasePRPhasePending)

		// then: the permanent label conflict is rejected before a provider request
		testastic.ErrorContains(t, err, "share GitLab scope workflow::backend")
		testastic.Equal(t, int32(0), calls.Load())
	})

	t.Run("rejects reserved lifecycle labels", func(t *testing.T) {
		t.Parallel()

		for _, reserved := range []string{"Any", "nOnE"} {
			t.Run(reserved, func(t *testing.T) {
				t.Parallel()

				// given: a GitLab server and a reserved pending label name
				var calls atomic.Int32

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
				}))
				defer server.Close()

				p := newGitLabContractProvider(t, server)

				// when: the pending phase is applied
				err := p.SetReleasePRLabels(context.Background(), 42, forge.ReleasePRLabels{
					Pending: reserved,
					Tagged:  "release::tagged",
				}, forge.ReleasePRPhasePending)

				// then: the reserved filter value is rejected before a provider request
				testastic.ErrorContains(t, err, "reserved GitLab label filter value")

				_, err = p.FindOpenPendingReleasePRs(context.Background(), providerContractBaseBranch, reserved)
				testastic.ErrorContains(t, err, "reserved GitLab label filter value")

				_, err = p.FindMergedReleasePR(context.Background(), providerContractBaseBranch, reserved)
				testastic.ErrorContains(t, err, "reserved GitLab label filter value")
				testastic.Equal(t, int32(0), calls.Load())
			})
		}
	})
}

func TestGitLabReleasePRStateTransitions(t *testing.T) {
	t.Parallel()

	t.Run("marks merge request pending", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab server that records label add and remove fields on MR 12 updates
		var updateRequest struct {
			AddLabels    string `json:"add_labels"`
			RemoveLabels string `json:"remove_labels"`
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
				writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/12":
				decodeJSONRequest(t, r, &updateRequest)
				writeJSONFixture(t, w, "contracts/gitlab/release_pr_state_transitions/update.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: MR 12 is put in the pending phase
		err := p.SetReleasePRLabels(context.Background(), 12, defaultReleasePRLabels(), forge.ReleasePRPhasePending)

		// then: the managed and pending labels are added and the tagged label is removed
		testastic.NoError(t, err)
		testastic.Equal(t, testReleaseLabelPending+",yeet", updateRequest.AddLabels)
		testastic.Equal(t, testReleaseLabelTagged, updateRequest.RemoveLabels)

		// when: marking the same merge request pending with the managed label disabled
		labels := defaultReleasePRLabels()
		labels.Yeet = false
		err = p.SetReleasePRLabels(context.Background(), 12, labels, forge.ReleasePRPhasePending)

		// then: only the pending label is added
		testastic.NoError(t, err)
		testastic.Equal(t, testReleaseLabelPending, updateRequest.AddLabels)
	})
}

func TestGitLabMergeReleasePR(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"checking", "unchecked", "preparing"} {
		t.Run("allows transient status "+status, func(t *testing.T) {
			t.Parallel()

			// given: a GitLab server reporting a transient merge status while recomputing readiness
			merged := false

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8":
					writeJSONFixture(t, w, "contracts/gitlab/merge_transient_status/pr_"+status+".json")
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
					writeJSONFixture(t, w, "contracts/gitlab/merge_transient_status/project.json")
				case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8/merge":
					merged = true

					writeJSONFixture(t, w, "contracts/gitlab/merge_transient_status/result.json")
				default:
					fatalUnexpectedProviderRequest(t, "GitLab", r)
				}
			}))
			defer server.Close()

			p := newGitLabContractProvider(t, server)

			// when: MergeReleasePR is invoked without force while readiness is recomputed
			_, err := p.MergeReleasePR(context.Background(), 8, forge.MergeReleasePROptions{})

			// then: the transient status does not prevent the merge request
			testastic.NoError(t, err)
			testastic.True(t, merged)
		})
	}

	t.Run("blocks readiness checks unless force is enabled", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab server reporting MR 8 as opened with a not_approved merge status
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests/8" {
				fatalUnexpectedProviderRequest(t, "GitLab", r)

				return
			}

			writeJSONFixture(t, w, "contracts/gitlab/merge_blocked_not_approved/pr.json")
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: MergeReleasePR is invoked without the force option
		_, err := p.MergeReleasePR(context.Background(), 8, forge.MergeReleasePROptions{})

		// then: forge.ErrMergeBlocked is returned with the detailed merge status in the message
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
		testastic.Equal(t, "release PR merge blocked: merge request !8 detailed_merge_status=not_approved", err.Error())
	})

	t.Run("forces merge and forwards squash option", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab server reporting MR 8 as blocked with squash always enabled at the project level
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8":
				writeJSONFixture(t, w, "contracts/gitlab/forced_merge_squash/pr.json")
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSONFixture(t, w, "contracts/gitlab/forced_merge_squash/project.json")
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8/merge":
				assertJSONRequest(t, r, "contracts/gitlab/forced_merge_squash/merge_request.json")
				writeJSONFixture(t, w, "contracts/gitlab/forced_merge_squash/result.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		}))
		defer server.Close()

		p := newGitLabContractProvider(t, server)

		// when: MergeReleasePR is invoked with merge checks bypassed and the squash merge method
		_, err := p.MergeReleasePR(context.Background(), 8, forge.MergeReleasePROptions{
			BypassMergeChecks: true,
			Method:            forge.MergeMethodSquash,
		})

		// then: the head SHA is forwarded and the squash flag is set on the merge request
		testastic.NoError(t, err)
	})
}

func TestGitLabMergeReleasePRFastRefusal(t *testing.T) {
	t.Parallel()

	t.Run("reports an accept rejected with method not allowed", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab server that rejects the accept the way it rejects a
		// conflicting or already closed merge request
		polls := gitLabRefusedAcceptServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			writeJSONFixture(t, w, "contracts/gitlab/refused_accept/method_not_allowed.json")
		})

		// then: the refusal is reported as a blocked merge without waiting for the forge
		testastic.Equal(t, int32(0), polls.Load())
	})

	t.Run("reports an accept that answers with a merge error", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab server that accepts the request but reports why it did not merge
		polls := gitLabRefusedAcceptServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSONFixture(t, w, "contracts/gitlab/refused_accept/merge_error.json")
		})

		// then: the refusal is reported as a blocked merge without waiting for the forge
		testastic.Equal(t, int32(0), polls.Load())
	})
}

func gitLabRefusedAcceptServer(t *testing.T, refuse http.HandlerFunc) *atomic.Int32 {
	t.Helper()

	var accepted atomic.Bool

	polls := new(atomic.Int32)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8":
			if accepted.Load() {
				polls.Add(1)
			}

			writeJSONFixture(t, w, "contracts/gitlab/refused_accept/pr.json")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
			writeJSONFixture(t, w, "contracts/gitlab/refused_accept/project.json")
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8/merge":
			accepted.Store(true)
			refuse(w, r)
		default:
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		}
	}))
	defer server.Close()

	p := newGitLabContractProvider(
		t,
		server,
		provider.WithMergePolling(time.Millisecond, time.Millisecond, 50*time.Millisecond),
	)

	// when: MergeReleasePR is invoked on the refused merge request
	mergeSHA, err := p.MergeReleasePR(context.Background(), 8, forge.MergeReleasePROptions{})

	testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
	testastic.Equal(t, "", mergeSHA)

	return polls
}

func TestGitLabUpdateFiles(t *testing.T) {
	t.Parallel()

	// given: a GitLab server accepting one commit that carries every file action
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/repository/commits" {
			fatalUnexpectedProviderRequest(t, "GitLab", r)

			return
		}

		assertJSONRequest(t, r, "contracts/gitlab/update_files_commit/request.json")
		writeJSONFixture(t, w, "contracts/gitlab/update_files_commit/commit.json")
	}))
	defer server.Close()

	p := newGitLabContractProvider(t, server)

	// when: two files are written onto the release branch off the base branch
	err := p.UpdateFiles(
		context.Background(),
		providerContractReleaseBranch,
		providerContractBaseBranch,
		map[string]forge.FileUpdate{
			"VERSION.txt":  {Content: "version=1.2.3"},
			"CHANGELOG.md": {Content: "# Changelog", Exists: true},
		},
		"chore: release 1.2.3",
	)

	// then: the branch, start branch, message, force flag, and file actions match the recorded contract
	testastic.NoError(t, err)
}
