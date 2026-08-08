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
			handleGitHubFindOpenPRsListContract(t, w, r, []map[string]any{gitHubOpenPRResponse(
				providerContractPRNumber,
				providerContractPendingBranch,
				[]string{testReleaseLabelPending},
			)})
		case providerContractFindOpenPRsAdoptable:
			handleGitHubFindOpenPRsListContract(t, w, r, []map[string]any{gitHubOpenPRResponse(
				providerContractPRNumber,
				providerContractPendingBranch,
				nil,
			)})
		case providerContractFindMergedPR:
			handleGitHubFindMergedPRContract(t, w, r)
		case providerContractMarkReleasePR:
			handleGitHubMarkReleasePRContract(t, w, r)
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
		case providerContractMissingExtraLabel:
			handleGitHubExtraLabelLookupContract(t, w, r, providerContractMissingExtraLabelName, http.StatusNotFound)
		case providerContractUnreachableExtraLabel:
			handleGitHubExtraLabelLookupContract(
				t, w, r,
				providerContractUnreachableLabelName,
				http.StatusInternalServerError,
			)
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
		writeJSON(t, w, gitHubTagsResponse())

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
		writeJSON(t, w, gitHubTagsPageTwoResponse())

		return
	}

	w.Header().Set("Link", fmt.Sprintf(`<http://%s/repos/o/r/tags?per_page=100&page=2>; rel="next"`, r.Host))
	writeJSON(t, w, gitHubTagsResponse())
}

func handleGitHubBranchHeadContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/heads/"+providerContractBaseBranch {
		writeJSON(t, w, gitHubSHAResponse(providerContractHeadSHA))

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubBranchHeadMissingContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/heads/missing-branch" {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(t, w, gitHubNotFoundResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubGetReleaseByTagContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/"+providerContractTag {
		writeJSON(t, w, gitHubReleaseResponse())

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

	writeJSON(t, w, gitHubReleasePRResponse(providerContractReleaseBody))
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
		writeJSON(t, w, gitHubReleasePRResponse(providerContractReleaseBody))
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
		writeJSON(t, w, gitHubReleasePRResponse(providerContractReleaseBody))
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
			writeJSON(t, w, gitHubReleasePRResponse(providerContractReleaseBody))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/pulls/42/requested_reviewers":
			w.WriteHeader(http.StatusUnprocessableEntity)
			writeJSON(t, w, gitHubReviewerRefusalResponse())
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

	writeJSON(t, w, gitHubReleasePRResponse(providerContractUpdatedReleaseBody))
}

func handleGitHubFindOpenPRsContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls" {
		testastic.Equal(t, "open", r.URL.Query().Get("state"))
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("base"))
		writeJSON(t, w, gitHubOpenPRsResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubFindOpenPRsListContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	prs []map[string]any,
) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls" {
		writeJSON(t, w, prs)

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
		writeJSON(t, w, gitHubSearchIssuesResponse(map[string]any{"number": providerContractPRNumber}))
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSON(t, w, gitHubMergedPRResponse())
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubMarkReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
		writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
		var labels []string
		decodeJSONRequest(t, r, &labels)

		if len(labels) == 1 {
			testastic.SliceEqual(t, []string{providerContractTaggedLabel}, labels)
		} else {
			testastic.SliceEqual(
				t,
				[]string{providerContractPendingLabel, "release", "automated", "yeet"},
				labels,
			)
		}

		writeJSON(t, w, []map[string]any{{"name": labels[0]}})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
		removed := decodedPathTail(t, r)
		testastic.True(t, removed == providerContractPendingLabel || removed == providerContractTaggedLabel)
		w.WriteHeader(http.StatusNoContent)
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubMergeReleasePRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSON(t, w, gitHubMergeStatePRResponse("clean"))
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
		writeJSON(t, w, gitHubSquashOnlyRepoResponse())
	case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/42/merge":
		var request struct {
			MergeMethod string `json:"merge_method"`
			SHA         string `json:"sha"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, string(provider.MergeMethodSquash), request.MergeMethod)
		testastic.Equal(t, providerContractHeadSHA, request.SHA)
		writeJSON(t, w, gitHubMergeResultResponse())
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

		writeJSON(t, w, gitHubMergeStatePRResponse("clean"))
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
		writeJSON(t, w, gitHubSquashOnlyRepoResponse())
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
		writeJSON(t, w, gitHubRefResponse("refs/heads/"+providerContractBaseBranch, gitHubContractBaseRefSHA, "commit"))
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
		writeJSON(t, w, gitHubRefResponse("refs/heads/"+providerContractReleaseBranch, gitHubContractBaseRefSHA, "commit"))
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubCreateReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/"+providerContractTag:
		w.WriteHeader(http.StatusNotFound)
		writeJSON(t, w, gitHubNotFoundResponse())
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+providerContractBaseBranch:
		writeJSON(t, w, gitHubSHAResponse(providerContractHeadSHA))
	case r.Method == http.MethodGet && r.URL.Path == "/user":
		writeJSON(t, w, gitHubUserResponse())
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
		writeJSON(t, w, gitHubTagObjectResponse())
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
		var request struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}
		decodeJSONRequest(t, r, &request)
		testastic.Equal(t, "refs/tags/"+providerContractTag, request.Ref)
		writeJSON(t, w, gitHubRefResponse("refs/tags/"+providerContractTag, gitHubContractTagObjectSHA, "tag"))
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
		writeJSON(t, w, gitHubCreatedReleaseResponse())
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubGetFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/contents/CHANGELOG.md" {
		testastic.Equal(t, providerContractBaseBranch, r.URL.Query().Get("ref"))
		writeJSON(t, w, gitHubFileResponse("CHANGELOG.md", providerContractChangelogContent))

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubUpdateFilesContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
		writeJSON(t, w, gitHubRefResponse("refs/heads/"+providerContractBaseBranch, gitHubContractBaseRefSHA, "commit"))
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/commits/base-ref-sha":
		writeJSON(t, w, gitHubBaseCommitResponse())
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/trees":
		writeJSON(t, w, gitHubSHAResponse(gitHubContractNewTreeSHA))
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/commits":
		writeJSON(t, w, gitHubSHAResponse(gitHubContractNewCommitSHA))
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/release-main":
		http.NotFound(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
		writeJSON(t, w, gitHubRefResponse("refs/heads/"+providerContractReleaseBranch, gitHubContractNewCommitSHA, "commit"))
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubMissingFileContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/contents/MISSING.md" {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(t, w, gitHubNotFoundResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubMissingReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/"+providerContractTag {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(t, w, gitHubNotFoundResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubMissingPRContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/search/issues" {
		writeJSON(t, w, gitHubSearchIssuesResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubBlockedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42" {
		writeJSON(t, w, gitHubMergeStatePRResponse("blocked"))

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubUnsupportedMergeContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSON(t, w, gitHubMergeStatePRResponse("clean"))
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
		writeJSON(t, w, gitHubSquashOnlyRepoResponse())
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func handleGitHubExtraLabelLookupContract(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	label string,
	status int,
) {
	t.Helper()

	if r.Method == http.MethodGet && decodedPathTail(t, r) == label {
		w.WriteHeader(status)
		writeJSON(t, w, gitHubNotFoundResponse())

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
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
		writeJSON(t, w, gitHubMergeStatePRResponse("dirty"))

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}

func handleGitHubForcedMergeUntrustedContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42" {
		pr := gitHubMergeStatePRResponse("clean")
		pr["head"] = map[string]any{
			"sha":  providerContractHeadSHA,
			"ref":  providerContractPendingBranch,
			"repo": map[string]any{"full_name": gitHubContractForkFullName},
		}
		writeJSON(t, w, pr)

		return
	}

	fatalUnexpectedProviderRequest(t, "GitHub", r)
}
