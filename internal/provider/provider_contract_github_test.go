package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
)

func newGitHubContractProvider(t *testing.T, server *httptest.Server) provider.Provider {
	t.Helper()

	client := newGitHubTestClient(t, server)

	return provider.NewGitHub(client, "o", "r")
}

func newGitHubContractHandler(t *testing.T, scenario providerContractScenario) http.Handler {
	t.Helper()

	var reviewersRequested atomic.Bool

	if scenario == providerContractCreateReleasePRReviewers {
		t.Cleanup(func() {
			if !reviewersRequested.Load() {
				t.Error("GitHub requested_reviewers endpoint was never called")
			}
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case providerContractLatestRelease:
			handleGitHubLatestReleaseContract(t, w, r)
		case providerContractLatestFallbackTags:
			handleGitHubLatestFallbackTagsContract(t, w, r)
		case providerContractListTags:
			handleGitHubListTagsContract(t, w, r)
		case providerContractGetCommitsSinceRefs:
			handleGitHubGetCommitsSinceContract(t, w, r)
		case providerContractGetCommitsSinceRefsMissing:
			handleGitHubGetCommitsSinceMissingContract(t, w, r)
		case providerContractGetCommitsSinceRefsUnresolved:
			handleGitHubGetCommitsSinceUnresolvedContract(t, w, r)
		case providerContractGetCommitsSinceRefsMultiBoundary:
			handleGitHubGetCommitsSinceMultiBoundaryContract(t, w, r)
		case providerContractGetReleaseByTag:
			handleGitHubGetReleaseByTagContract(t, w, r)
		case providerContractTagExists:
			handleGitHubTagExistsContract(t, w, r)
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
		case providerContractFindMergedPR:
			handleGitHubFindMergedPRContract(t, w, r)
		case providerContractMarkReleasePR:
			handleGitHubMarkReleasePRContract(t, w, r)
		case providerContractMergeReleasePR:
			handleGitHubMergeReleasePRContract(t, w, r)
		case providerContractCommitPRBody:
			handleGitHubCommitPRBodyContract(t, w, r)
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
		default:
			t.Fatalf("unhandled GitHub contract scenario: %s", scenario)
		}
	})
}

func handleGitHubLatestReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/latest" {
		writeJSONFixture(t, w, "contracts/github/latest_release/release.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubLatestFallbackTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/latest":
		http.NotFound(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/tags":
		writeJSONFixture(t, w, "contracts/github/latest_fallback_tags/tags.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubListTagsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/tags" {
		writeJSONFixture(t, w, "contracts/github/list_tags/tags.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

// gitHubComparePath is the URL the compare endpoint receives for
// base...baseBranch.
func gitHubComparePath(base string) string {
	return "/repos/o/r/compare/" + base + "..." + providerContractBaseBranch
}

func handleGitHubGetCommitsSinceContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == gitHubComparePath(providerContractTag):
		writeJSONFixture(t, w, "contracts/github/get_commits_since/compare.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+providerContractHeadSHA:
		writeJSONFixture(t, w, "contracts/github/get_commits_since/detail.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubGetCommitsSinceMissingContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == gitHubComparePath(providerContractTag):
		writeJSONFixture(t, w, "contracts/github/get_commits_since/compare.json")
	case r.Method == http.MethodGet &&
		r.URL.Path == gitHubComparePath(providerContractMissingTag):
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+providerContractHeadSHA:
		writeJSONFixture(t, w, "contracts/github/get_commits_since/detail.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubGetCommitsSinceUnresolvedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == gitHubComparePath(providerContractTag):
		writeJSONFixture(t, w, "contracts/github/get_commits_since/compare.json")
	case r.Method == http.MethodGet &&
		r.URL.Path == gitHubComparePath(providerContractMissingTag):
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+providerContractHeadSHA:
		writeJSONFixture(t, w, "contracts/github/get_commits_since/detail.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubGetCommitsSinceMultiBoundaryContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet &&
		r.URL.Path == gitHubComparePath(providerContractIntermediateTag):
		writeJSONFixture(t, w, "contracts/github/get_commits_since_multi_boundary/intermediate_compare.json")
	case r.Method == http.MethodGet && r.URL.Path == gitHubComparePath(providerContractTag):
		writeJSONFixture(t, w, "contracts/github/get_commits_since_multi_boundary/older_compare.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubGetReleaseByTagContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/"+providerContractTag {
		writeJSONFixture(t, w, "contracts/github/get_release_by_tag/release.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubTagExistsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/"+providerContractTag {
		writeJSONFixture(t, w, "contracts/github/tag_exists/ref.json")

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
	case r.Method == http.MethodGet && r.URL.Path == "/user":
		writeJSONFixture(t, w, "contracts/github/create_release_pr_reviewers/user.json")
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
	case r.Method == http.MethodGet && r.URL.Path == "/user":
		writeJSONFixture(t, w, "contracts/github/create_release_pr_reviewers/user.json")
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
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			writeJSONFixture(t, w, "contracts/github/create_release_pr_reviewers/user.json")
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
	testastic.ErrorContains(t, err, providerContractReviewerAlice)
}

func TestGitHubRejectsAuthenticatedUserAsReviewer(t *testing.T) {
	t.Parallel()

	// given: a GitHub server whose authenticated token identity is release-bot
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/user" {
			writeJSONFixture(t, w, "contracts/github/create_release_pr_reviewers/user.json")

			return
		}

		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}))
	defer server.Close()

	p := newGitHubContractProvider(t, server)

	// when: creating a release PR that lists the token identity as reviewer
	_, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
		Title:         providerContractReleaseTitle,
		Body:          providerContractReleaseBody,
		BaseBranch:    providerContractBaseBranch,
		ReleaseBranch: providerContractReleaseBranch,
		Reviewers:     []string{"release-bot"},
	})

	// then: the run fails before any PR is created, naming the reviewer
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, provider.ErrReviewerInvalid)
	testastic.ErrorContains(t, err, "release-bot")
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

func handleGitHubFindMergedPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls":
		testastic.Equal(t, "closed", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("base"))
		writeJSONFixture(t, w, "contracts/github/find_merged_pr/prs.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSONFixture(t, w, "contracts/github/find_merged_pr/pr.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubMarkReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
		writeGitHubLabelFixture(t, w, pathLabel(t, r))
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
		var labels []string
		decodeJSONRequest(t, r, &labels)
		writeGitHubLabelsFixture(t, w, strings.Join(labels, ","))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
		w.WriteHeader(http.StatusNoContent)
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
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

func handleGitHubCommitPRBodyContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+providerContractMergeSHA+"/pulls" {
		writeJSONFixture(t, w, "contracts/github/commit_pr_body/prs.json")

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
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
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/commits/base-ref-sha":
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

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls" {
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

func writeGitHubLabelFixture(t *testing.T, w http.ResponseWriter, label string) {
	t.Helper()

	switch label {
	case provider.ReleaseLabelPending:
		writeJSONFixture(t, w, "contracts/github/mark_release_pr/label_pending.json")
	case provider.ReleaseLabelTagged:
		writeJSONFixture(t, w, "contracts/github/mark_release_pr/label_tagged.json")
	default:
		t.Fatalf("unexpected GitHub label: %s", label)
	}
}

func writeGitHubLabelsFixture(t *testing.T, w http.ResponseWriter, labels string) {
	t.Helper()

	switch labels {
	case provider.ReleaseLabelPending:
		writeJSONFixture(t, w, "contracts/github/mark_release_pr/add_pending.json")
	case provider.ReleaseLabelTagged:
		writeJSONFixture(t, w, "contracts/github/mark_release_pr/add_tagged.json")
	default:
		t.Fatalf("unexpected GitHub labels: %s", labels)
	}
}
