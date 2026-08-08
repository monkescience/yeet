package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
)

func newGitHubContractProvider(
	t *testing.T,
	server *httptest.Server,
	options ...provider.MergePollingOption,
) provider.Provider {
	t.Helper()

	client := newGitHubTestClient(t, server)

	return provider.NewGitHub(client, "o", "r", options...)
}

func newGitHubContractHandler(t *testing.T, scenario providerContractScenario) http.Handler {
	t.Helper()

	var reviewersRequested atomic.Bool

	var mergeAccepted atomic.Bool

	var tagPages atomic.Int32

	if scenario == providerContractCreateReleasePRReviewers {
		t.Cleanup(func() {
			if !reviewersRequested.Load() {
				t.Error("GitHub requested_reviewers endpoint was never called")
			}
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case providerContractListTags:
			handleGitHubListTagsContract(t, w, r)
		case providerContractListTagsPaged:
			handleGitHubListTagsPagedContract(t, w, r)
		case providerContractBranchHead:
			handleGitHubBranchHeadContract(t, w, r)
		case providerContractBranchHeadMissing:
			handleGitHubBranchHeadMissingContract(t, w, r)
		case providerContractGetReleaseByTag:
			handleGitHubGetReleaseByTagContract(t, w, r)
		case providerContractCreateReleasePR:
			handleGitHubCreateReleasePRContract(t, w, r)
		case providerContractCreateReleasePRReviewers:
			handleGitHubCreateReleasePRReviewersContract(t, w, r, &reviewersRequested)
		case providerContractUnknownReviewer:
			handleGitHubUnknownReviewerContract(t, w, r)
		case providerContractUpdateReleasePR:
			handleGitHubUpdateReleasePRContract(t, w, r)
		case providerContractFindOpenPRs:
			handleGitHubFindOpenPRsContract(t, w, r)
		case providerContractFindOpenPRsUnlabeled:
			handleGitHubFindOpenPRsFixtureContract(t, w, r, "find_open_prs_unlabeled")
		case providerContractFindOpenPRsAdoptable:
			handleGitHubFindOpenPRsFixtureContract(t, w, r, "find_open_prs_adoptable")
		case providerContractFindMergedPR:
			handleGitHubFindMergedPRContract(t, w, r)
		case providerContractMergeReleasePR:
			handleGitHubMergeReleasePRContract(t, w, r)
		case providerContractAsyncMergeReleasePR:
			handleGitHubAsyncMergeReleasePRContract(t, w, r, &mergeAccepted)
		case providerContractCreateBranch:
			handleGitHubCreateBranchContract(t, w, r)
		case providerContractCreateRelease:
			handleGitHubCreateReleaseContract(t, w, r)
		case providerContractGetFile:
			handleGitHubGetFileContract(t, w, r)
		case providerContractUpdateFiles:
			handleGitHubUpdateFilesContract(t, w, r)
		case providerContractMissingFile:
			handleGitHubMissingFileContract(t, w, r)
		case providerContractMissingRelease:
			handleGitHubMissingReleaseContract(t, w, r)
		case providerContractMissingPR:
			handleGitHubMissingPRContract(t, w, r)
		case providerContractBlockedMerge:
			handleGitHubBlockedMergeContract(t, w, r)
		case providerContractUnsupportedMerge:
			handleGitHubUnsupportedMergeContract(t, w, r)
		case providerContractTagPaginationLimit:
			handleGitHubTagPaginationLimitContract(t, w, r, &tagPages)
		case providerContractForcedMergeUntrusted:
			handleGitHubForcedMergeUntrustedContract(t, w, r)
		case providerContractForcedMergeConflicted:
			handleGitHubForcedMergeConflictedContract(t, w, r)
		default:
			t.Fatalf("unhandled GitHub contract scenario: %s", scenario)
		}
	})
}

func handleGitHubListTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/tags" {
		writeJSONFixture(t, w, "contracts/github/list_tags/tags.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubListTagsPagedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodGet || r.URL.Path != "/repos/o/r/tags" {
		fatalUnexpectedProviderRequest(t, "GitHub", r)

		return
	}

	if r.URL.Query().Get("page") == "2" {
		writeJSONFixture(t, w, "contracts/github/list_tags/page_two.json")

		return
	}

	w.Header().Set("Link", fmt.Sprintf(`<http://%s/repos/o/r/tags?per_page=100&page=2>; rel="next"`, r.Host))
	writeJSONFixture(t, w, "contracts/github/list_tags/tags.json")
}

func handleGitHubBranchHeadContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/heads/"+providerContractBaseBranch {
		writeJSONFixture(t, w, "contracts/github/branch_head/commit.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubBranchHeadMissingContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/heads/missing-branch" {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubGetReleaseByTagContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/"+providerContractTag {
		writeJSONFixture(t, w, "contracts/github/get_release_by_tag/release.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubCreateReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPost || r.URL.Path != "/repos/o/r/pulls" {
		fatalUnexpectedProviderRequest(t, "GitHub", r)

		return
	}

	var request struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractReleaseTitle, request.Title)
	testastic.Equal(t, providerContractReleaseBody, request.Body)
	testastic.Equal(t, providerContractReleaseBranch, request.Head)
	testastic.Equal(t, providerContractBaseBranch, request.Base)

	writeJSONFixture(t, w, "contracts/github/create_release_pr/response.json")
}

func handleGitHubCreateReleasePRReviewersContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	reviewersRequested *atomic.Bool,
) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/o/r/collaborators/"):
		login := strings.TrimPrefix(r.URL.Path, "/repos/o/r/collaborators/")
		if login != providerContractReviewerAlice && login != providerContractReviewerBob {
			t.Errorf("unexpected GitHub collaborator check: %s", login)
		}

		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/pulls":
		writeJSONFixture(t, w, "contracts/github/create_release_pr/response.json")
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/pulls/42/requested_reviewers":
		var request struct {
			Reviewers []string `json:"reviewers"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.SliceEqual(
			t,
			[]string{providerContractReviewerAlice, providerContractReviewerBob},
			request.Reviewers,
		)
		reviewersRequested.Store(true)
		writeJSONFixture(t, w, "contracts/github/create_release_pr/response.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubUnknownReviewerContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/collaborators/"+providerContractUnknownReviewerName:
		w.WriteHeader(http.StatusNotFound)
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func TestGitHubFailsWhenReviewerRequestIsRejectedAfterCreate(t *testing.T) {
	t.Parallel()

	// given: a GitHub server where pre-validation passes but the reviewer
	// request itself is rejected after the PR exists
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/collaborators/"+providerContractReviewerAlice:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/pulls":
			writeJSONFixture(t, w, "contracts/github/create_release_pr/response.json")
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/pulls/42/requested_reviewers":
			w.WriteHeader(http.StatusUnprocessableEntity)
			writeJSONFixture(t, w, "contracts/github/unknown_reviewer/error.json")
		default:
			fatalUnexpectedProviderRequest(t, "GitHub", r)
		}
	}))
	defer server.Close()

	p := newGitHubContractProvider(t, server)

	// when: creating a release PR with a reviewer GitHub ends up rejecting
	_, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
		Title:         providerContractReleaseTitle,
		Body:          providerContractReleaseBody,
		BaseBranch:    providerContractBaseBranch,
		ReleaseBranch: providerContractReleaseBranch,
		Reviewers:     []string{providerContractReviewerAlice},
	})

	// then: the run fails naming the reviewer
	testastic.Error(t, err)
	testastic.Equal(
		t,
		"request reviewers [alice] for pull request #42: POST "+server.URL+
			"/repos/o/r/pulls/42/requested_reviewers: 422 Reviews may only be requested from collaborators. []",
		err.Error(),
	)
}

func handleGitHubUpdateReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPatch || r.URL.Path != "/repos/o/r/pulls/42" {
		fatalUnexpectedProviderRequest(t, "GitHub", r)

		return
	}

	var request struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.Equal(t, providerContractReleaseTitle, request.Title)
	testastic.Equal(t, "updated release body", request.Body)

	writeJSONFixture(t, w, "contracts/github/update_release_pr/response.json")
}

func handleGitHubFindOpenPRsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls" {
		testastic.Equal(t, "open", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("base"))
		writeJSONFixture(t, w, "contracts/github/find_open_prs/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubFindOpenPRsFixtureContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	dir string,
) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls" {
		writeJSONFixture(t, w, "contracts/github/"+dir+"/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubFindMergedPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/search/issues":
		testastic.Equal(t,
			`repo:o/r is:pr is:merged base:main label:"release: waiting"`,
			r.URL.Query().Get("q"),
		)
		writeJSONFixture(t, w, "contracts/github/find_merged_pr/prs.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSONFixture(t, w, "contracts/github/find_merged_pr/pr.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

// newGitHubContractLabelHandler tracks the labels on PR 42 so a scenario can
// assert the set a phase leaves behind.
func newGitHubContractLabelHandler(
	t *testing.T,
	store *providerContractLabelStore,
	registry providerContractLabelRegistry,
) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
			name := decodedPathTail(t, r)
			if status, answered := registry.status(name); answered {
				w.WriteHeader(status)
				writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")

				return
			}

			writeJSON(t, w, map[string]any{"name": name})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
			var names []string
			decodeJSONRequest(t, r, &names)
			store.attach(names...)
			writeJSON(t, w, []map[string]any{{"name": names[0]}})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
			store.detach(decodedPathTail(t, r))
			w.WriteHeader(http.StatusNoContent)
		default:
			fatalUnexpectedProviderRequest(t, "GitHub", r)
		}
	})
}

func handleGitHubMergeReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSONFixture(t, w, "contracts/github/merge_release_pr/pr.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
		writeJSONFixture(t, w, "contracts/github/merge_release_pr/repo.json")
	case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/42/merge":
		var request struct {
			MergeMethod string `json:"merge_method"`
			SHA         string `json:"sha"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, string(provider.MergeMethodSquash), request.MergeMethod)
		testastic.Equal(t, providerContractHeadSHA, request.SHA)
		writeJSONFixture(t, w, "contracts/github/merge_release_pr/result.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

// handleGitHubAsyncMergeReleasePRContract models a merge GitHub accepts without
// reporting a merge commit SHA on the merge response itself.
func handleGitHubAsyncMergeReleasePRContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	mergeAccepted *atomic.Bool,
) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		if mergeAccepted.Load() {
			writeJSON(t, w, map[string]any{
				"number":           42,
				"state":            "closed",
				"merged":           true,
				"merge_commit_sha": providerContractMergeSHA,
			})

			return
		}

		writeJSONFixture(t, w, "contracts/github/merge_release_pr/pr.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
		writeJSONFixture(t, w, "contracts/github/merge_release_pr/repo.json")
	case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/42/merge":
		mergeAccepted.Store(true)
		writeJSON(t, w, map[string]any{"merged": true, "sha": ""})
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubCreateBranchContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
		writeJSONFixture(t, w, "contracts/github/create_branch/base_ref.json")
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
		writeJSONFixture(t, w, "contracts/github/create_branch/created_ref.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubCreateReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/"+providerContractTag:
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+providerContractBaseBranch:
		writeJSONFixture(t, w, "contracts/github/create_release/commit_ref.json")
	case r.Method == http.MethodGet && r.URL.Path == "/user":
		writeJSONFixture(t, w, "contracts/github/create_release/user.json")
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/tags":
		var request struct {
			Tag     string `json:"tag"`
			Message string `json:"message"`
			Object  string `json:"object"`
			Type    string `json:"type"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, providerContractTag, request.Tag)
		testastic.Equal(t, "release notes", request.Message)
		testastic.Equal(t, providerContractHeadSHA, request.Object)
		testastic.Equal(t, "commit", request.Type)
		writeJSONFixture(t, w, "contracts/github/create_release/tag_object.json")
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
		var request struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, "refs/tags/"+providerContractTag, request.Ref)
		writeJSONFixture(t, w, "contracts/github/create_release/tag_ref.json")
	case isGitHubCreateReleaseRequest(r):
		var request struct {
			TagName         string `json:"tag_name"`
			TargetCommitish string `json:"target_commitish"`
			Name            string `json:"name"`
			Body            string `json:"body"`
			Prerelease      bool   `json:"prerelease"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, providerContractTag, request.TagName)
		testastic.Equal(t, providerContractBaseBranch, request.TargetCommitish)
		testastic.Equal(t, providerContractTag, request.Name)
		testastic.Equal(t, "release notes", request.Body)
		testastic.True(t, request.Prerelease)
		writeJSONFixture(t, w, "contracts/github/create_release/release.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubGetFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/contents/CHANGELOG.md" {
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("ref"))
		writeJSONFixture(t, w, "contracts/github/get_file/file.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubUpdateFilesContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
		writeJSONFixture(t, w, "contracts/github/update_files/base_ref.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/commits/6261736572656673686100000000000000000000":
		writeJSONFixture(t, w, "contracts/github/update_files/base_commit.json")
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/trees":
		writeJSONFixture(t, w, "contracts/github/update_files/tree.json")
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/commits":
		writeJSONFixture(t, w, "contracts/github/update_files/commit.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/release-main":
		http.NotFound(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
		writeJSONFixture(t, w, "contracts/github/update_files/create_ref.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubMissingFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/contents/MISSING.md" {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubMissingReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/"+providerContractTag {
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubMissingPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/search/issues" {
		writeJSONFixture(t, w, "contracts/github/missing_pr/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubBlockedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42" {
		writeJSONFixture(t, w, "contracts/github/blocked_merge/pr.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubUnsupportedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSONFixture(t, w, "contracts/github/unsupported_merge/pr.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
		writeJSONFixture(t, w, "contracts/github/unsupported_merge/repo.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubTagPaginationLimitContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	pages *atomic.Int32,
) {
	t.Helper()

	if r.Method != http.MethodGet || r.URL.Path != "/repos/o/r/tags" {
		fatalUnexpectedProviderRequest(t, "GitHub", r)

		return
	}

	page := pages.Add(1)
	w.Header().Set(
		"Link",
		fmt.Sprintf(`<http://%s/repos/o/r/tags?per_page=100&page=%d>; rel="next"`, r.Host, page+1),
	)
	writeJSON(t, w, []map[string]any{{
		"name":   fmt.Sprintf("v0.0.%d", page),
		"commit": map[string]any{"sha": fmt.Sprintf("sha-%d", page)},
	}})
}

func handleGitHubForcedMergeConflictedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42" {
		writeJSONFixture(t, w, "contracts/github/forced_merge_conflicted/pr.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubForcedMergeUntrustedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42" {
		writeJSONFixture(t, w, "contracts/github/forced_merge_untrusted/pr.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}
