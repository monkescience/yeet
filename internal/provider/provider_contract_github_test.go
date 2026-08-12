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
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/provider"
)

const (
	gitHubReleaseCommitSHA = "6865616473686131323300000000000000000000"
	gitHubBaseCommitSHA    = "6261736572656673686100000000000000000000"
)

func newGitHubContractProvider(
	t *testing.T,
	server *httptest.Server,
	options ...provider.MergePollingOption,
) forge.Provider {
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

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/"+providerContractTag:
		writeJSONFixture(t, w, "contracts/github/get_release_by_tag/release.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/tags/"+providerContractTag:
		writeJSON(t, w, map[string]any{"sha": providerContractTagCommitSHA})
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
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
	_, err := p.CreateReleasePR(context.Background(), forge.ReleasePROptions{
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

// TestGitHubMatchesThePendingLabelCaseInsensitively pins a GitHub-only rule:
// GitHub label matching folds case, where GitLab requires an exact match.
func TestGitHubMatchesThePendingLabelCaseInsensitively(t *testing.T) {
	t.Parallel()

	// given: an open release PR labelled in a different case than the configured
	// pending label
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/repos/o/r/pulls" {
			fatalUnexpectedProviderRequest(t, "GitHub", r)

			return
		}

		assertJSONRequest(t, r, "contracts/github/find_open_prs_case_insensitive/request.json")
		writeJSONFixture(t, w, "contracts/github/find_open_prs_case_insensitive/prs.json")
	}))
	defer server.Close()

	p := newGitHubContractProvider(t, server)

	// when: finding open pending release PRs
	prs, err := p.FindOpenPendingReleasePRs(context.Background(), providerContractBaseBranch, providerContractPendingLabel)

	// then: the case variant is accepted as the configured pending label
	testastic.NoError(t, err)
	testastic.Equal(t, 1, len(prs))
	testastic.False(t, prs[0].NeedsPendingLabel)
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
		testastic.Equal(t, "o:"+providerContractPendingBranch, r.URL.Query().Get("head"))
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
		testastic.Equal(t, "o:"+providerContractPendingBranch, r.URL.Query().Get("head"))
		writeJSONFixture(t, w, "contracts/github/"+dir+"/prs.json")

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
		testastic.Equal(t, "o:"+providerContractPendingBranch, r.URL.Query().Get("head"))
		testastic.Equal(t, "100", r.URL.Query().Get("per_page"))
		writeJSONFixture(t, w, "contracts/github/find_merged_pr/prs.json")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
		writeJSONFixture(t, w, "contracts/github/find_merged_pr/pr.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func TestGitHubFindMergedReleasePRUsesOneListAndOneDetailRequest(t *testing.T) {
	t.Parallel()

	// given: two trusted merged candidates ordered differently by merge time
	var (
		listRequests   atomic.Int32
		detailRequests atomic.Int32
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls":
			listRequests.Add(1)
			writeJSONFixture(t, w, "contracts/github/find_merged_pr/prs.json")
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
			detailRequests.Add(1)
			writeJSONFixture(t, w, "contracts/github/find_merged_pr/pr.json")
		default:
			fatalUnexpectedProviderRequest(t, "GitHub", r)
		}
	}))
	defer server.Close()

	p := newGitHubContractProvider(t, server)

	// when: finding the merged release pull request
	pr, err := p.FindMergedReleasePR(
		context.Background(),
		providerContractBaseBranch,
		providerContractPendingLabel,
	)

	// then: the most recently merged candidate is fetched once after one list page
	testastic.NoError(t, err)
	testastic.Equal(t, 42, pr.Number)
	testastic.Equal(t, int32(1), listRequests.Load())
	testastic.Equal(t, int32(1), detailRequests.Load())
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

func TestGitHubCachesSuccessfulLabelDefinitionsAcrossReleasePhases(t *testing.T) {
	t.Parallel()

	// given: a GitHub label registry containing every managed label
	lookups := make(map[string]int)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
			name := decodedPathTail(t, r)
			lookups[strings.ToLower(name)]++
			writeJSON(t, w, map[string]any{"name": name})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
			writeJSON(t, w, []map[string]any{})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
			w.WriteHeader(http.StatusOK)
		default:
			fatalUnexpectedProviderRequest(t, "GitHub", r)
		}
	}))
	defer server.Close()

	p := newGitHubContractProvider(t, server)
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
		testastic.Equal(t, string(forge.MergeMethodSquash), request.MergeMethod)
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

func handleGitHubCreateReleaseContract(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/"+providerContractTag:
		w.WriteHeader(http.StatusNotFound)
		writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")
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
		testastic.Equal(t, providerContractHeadSHA, request.TargetCommitish)
		testastic.Equal(t, providerContractTag, request.Name)
		testastic.Equal(t, "release notes", request.Body)
		testastic.True(t, request.Prerelease)
		writeJSONFixture(t, w, "contracts/github/create_release/release.json")
	default:
		fatalUnexpectedProviderRequest(t, "GitHub", r)
	}
}

func TestGitHubConcurrentTagCreationValidatesCommit(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name       string
		commitSHA  string
		wantErr    bool
		wantCreate int32
	}{
		{name: "accepts same commit", commitSHA: providerContractHeadSHA, wantCreate: 1},
		{name: "rejects conflicting commit", commitSHA: providerContractTagCommitSHA, wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: another actor creates the tag ref after the initial lookup
			var (
				tagLookups     atomic.Int32
				releaseCreates atomic.Int32
			)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/"+providerContractTag:
					w.WriteHeader(http.StatusNotFound)
					writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")
				case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/tags/"+providerContractTag:
					tagLookups.Add(1)
					writeJSON(t, w, map[string]any{"sha": testCase.commitSHA})
				case r.Method == http.MethodGet && r.URL.Path == "/user":
					writeJSONFixture(t, w, "contracts/github/create_release/user.json")
				case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/tags":
					writeJSONFixture(t, w, "contracts/github/create_release/tag_object.json")
				case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
					w.WriteHeader(http.StatusUnprocessableEntity)
					writeJSON(t, w, map[string]any{"message": "Reference already exists"})
				case isGitHubCreateReleaseRequest(r):
					releaseCreates.Add(1)
					writeJSONFixture(t, w, "contracts/github/create_release/release.json")
				default:
					fatalUnexpectedProviderRequest(t, "GitHub", r)
				}
			}))
			defer server.Close()

			p := newGitHubContractProvider(t, server)

			// when: creating a release at the requested commit
			release, err := p.CreateRelease(context.Background(), forge.ReleaseOptions{
				TagName: providerContractTag,
				Ref:     providerContractHeadSHA,
				Name:    providerContractTag,
				Body:    "release notes",
			})

			// then: only a matching concurrent tag is accepted
			testastic.Equal(t, int32(1), tagLookups.Load())
			testastic.Equal(t, testCase.wantCreate, releaseCreates.Load())

			if testCase.wantErr {
				testastic.ErrorIs(t, err, forge.ErrReleaseTagMismatch)
				testastic.True(t, release == nil)

				return
			}

			testastic.NoError(t, err)
			testastic.Equal(t, providerContractHeadSHA, release.CommitSHA)
		})
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

func TestGitHubCreateRelease(t *testing.T) {
	t.Parallel()

	// given: a GitHub repository that does not carry the release tag yet
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/"+providerContractTag:
			w.WriteHeader(http.StatusNotFound)
			writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			writeJSONFixture(t, w, "contracts/github/create_release_prerelease/user.json")
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/tags":
			assertJSONRequest(t, r, "contracts/github/create_release_prerelease/tag_request.json")
			writeJSONFixture(t, w, "contracts/github/create_release_prerelease/tag_object.json")
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
			assertJSONRequest(t, r, "contracts/github/create_release_prerelease/ref_request.json")
			writeJSONFixture(t, w, "contracts/github/create_release_prerelease/tag_ref.json")
		case isGitHubCreateReleaseRequest(r):
			assertJSONRequest(t, r, "contracts/github/create_release_prerelease/release_request.json")
			writeJSONFixture(t, w, "contracts/github/create_release_prerelease/release.json")
		default:
			fatalUnexpectedProviderRequest(t, "GitHub", r)
		}
	}))
	defer server.Close()

	p := newGitHubContractProvider(t, server)

	// when: creating a prerelease with an explicit ref
	release, err := p.CreateRelease(context.Background(), forge.ReleaseOptions{
		TagName:    providerContractTag,
		Ref:        gitHubReleaseCommitSHA,
		Name:       providerContractTag,
		Body:       providerContractReleaseNotes,
		Prerelease: true,
	})

	// then: target_commitish and prerelease flag are forwarded to GitHub
	testastic.NoError(t, err)
	testastic.Equal(t, providerContractTag, release.TagName)
	testastic.Equal(t, gitHubReleaseCommitSHA, release.CommitSHA)
	testastic.Equal(t, providerContractReleaseNotes, release.Body)
	testastic.Equal(t, providerContractReleaseURL, release.URL)
}

func TestGitHubCreateReleaseReusesExistingTag(t *testing.T) {
	t.Parallel()

	// given: a GitHub repository where the target tag already exists
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/"+providerContractTag:
			writeJSONFixture(t, w, "contracts/github/create_release_existing_tag/tag_ref.json")
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/tags/"+providerContractTag:
			writeJSONFixture(t, w, "contracts/github/create_release_existing_tag/tag_commit.json")
		case isGitHubCreateReleaseRequest(r):
			writeJSONFixture(t, w, "contracts/github/create_release_existing_tag/release.json")
		default:
			fatalUnexpectedProviderRequest(t, "GitHub", r)
		}
	}))
	defer server.Close()

	p := newGitHubContractProvider(t, server)

	// when: creating a release for the existing tag
	release, err := p.CreateRelease(context.Background(), forge.ReleaseOptions{
		TagName: providerContractTag,
		Ref:     gitHubReleaseCommitSHA,
		Name:    providerContractTag,
		Body:    providerContractReleaseNotes,
	})

	// then: the existing tag is reused without another tag creation request
	testastic.NoError(t, err)
	testastic.Equal(t, providerContractTag, release.TagName)
	testastic.Equal(t, gitHubReleaseCommitSHA, release.CommitSHA)
	testastic.Equal(t, providerContractReleaseURL, release.URL)
}

func TestGitHubCreateReleaseRejectsRefsThatAreNotCommitSHAs(t *testing.T) {
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

			// given: a GitHub provider whose server fails any request it receives
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				fatalUnexpectedProviderRequest(t, "GitHub", r)
			}))
			defer server.Close()

			p := newGitHubContractProvider(t, server)

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

func TestGitHubEnsureLabel(t *testing.T) {
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

			// given: a GitHub API where the labels do not exist
			var created []string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/labels/"):
					w.WriteHeader(http.StatusNotFound)
					writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")
				case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/labels":
					var request struct {
						Name string `json:"name"`
					}
					decodeJSONRequest(t, r, &request)
					created = append(created, request.Name)

					w.WriteHeader(http.StatusCreated)
					writeJSON(t, w, map[string]any{"name": request.Name})
				case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
					writeJSONFixture(t, w, "contracts/github/ensure_label/attached.json")
				case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
					w.WriteHeader(http.StatusNoContent)
				default:
					fatalUnexpectedProviderRequest(t, "GitHub", r)
				}
			}))
			defer server.Close()

			p := newGitHubContractProvider(t, server)
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

// newGitHubMergeMethodHandler serves pull request 1 together with the repository
// merge settings a case pins. A case that must never reach the merge call leaves
// acceptsMerge false so an unexpected merge request fails the test.
func newGitHubMergeMethodHandler(t *testing.T, repoFixture string, acceptsMerge bool) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/1":
			writeJSONFixture(t, w, "contracts/github/resolve_merge_method/pr.json")
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
			writeJSONFixture(t, w, "contracts/github/resolve_merge_method/"+repoFixture)
		case acceptsMerge && r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/1/merge":
			writeJSONFixture(t, w, "contracts/github/resolve_merge_method/result.json")
		default:
			fatalUnexpectedProviderRequest(t, "GitHub", r)
		}
	})
}

func TestGitHubResolveGitHubMergeMethod(t *testing.T) {
	t.Parallel()

	t.Run("auto selects squash when enabled", func(t *testing.T) {
		t.Parallel()

		// given: a repository that allows squash merge
		server := httptest.NewServer(newGitHubMergeMethodHandler(t, "repo_squash.json", true))
		defer server.Close()

		p := newGitHubContractProvider(t, server)

		// when: merging with auto method
		_, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            forge.MergeMethodAuto,
		})

		// then: no error
		testastic.NoError(t, err)
	})

	t.Run("rejects disabled merge method", func(t *testing.T) {
		t.Parallel()

		// given: a repository that only allows merge commits
		server := httptest.NewServer(newGitHubMergeMethodHandler(t, "repo_merge_commit.json", false))
		defer server.Close()

		p := newGitHubContractProvider(t, server)

		// when: merging with squash method (which is disabled)
		_, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            forge.MergeMethodSquash,
		})

		// then: merge is blocked because squash is disabled
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
	})

	t.Run("auto falls back to rebase when squash disabled", func(t *testing.T) {
		t.Parallel()

		// given: a repository that allows only rebase
		server := httptest.NewServer(newGitHubMergeMethodHandler(t, "repo_rebase.json", true))
		defer server.Close()

		p := newGitHubContractProvider(t, server)

		// when: merging with auto method
		_, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            forge.MergeMethodAuto,
		})

		// then: no error - auto selects rebase
		testastic.NoError(t, err)
	})

	t.Run("auto fails when no merge methods enabled", func(t *testing.T) {
		t.Parallel()

		// given: a repository with all merge methods disabled
		server := httptest.NewServer(newGitHubMergeMethodHandler(t, "repo_none.json", false))
		defer server.Close()

		p := newGitHubContractProvider(t, server)

		// when: merging with auto method
		_, err := p.MergeReleasePR(context.Background(), 1, forge.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            forge.MergeMethodAuto,
		})

		// then: merge is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
	})
}

func TestGitHubReleasePRStateTransitions(t *testing.T) {
	t.Parallel()

	t.Run("marks pull request pending", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub server that records label additions and deletions for PR 42
		var addLabels []string

		removedLabel := ""

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
				writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
			case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
				decodeJSONRequest(t, r, &addLabels)
				writeJSONFixture(t, w, "contracts/github/mark_release_pr/attached.json")
			case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
				removedLabel = decodedPathTail(t, r)

				w.WriteHeader(http.StatusNotFound)
				writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitHub", r)
			}
		}))
		defer server.Close()

		p := newGitHubContractProvider(t, server)

		// when: PR 42 is put in the pending phase
		err := p.SetReleasePRLabels(context.Background(), 42, defaultReleasePRLabels(), forge.ReleasePRPhasePending)

		// then: the managed and pending labels are added and the tagged label is removed
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{testReleaseLabelPending, provider.ReleaseLabelYeet}, addLabels)
		testastic.Equal(t, testReleaseLabelTagged, removedLabel)

		// when: marking the same pull request pending with the managed label disabled
		labels := defaultReleasePRLabels()
		labels.Yeet = false
		err = p.SetReleasePRLabels(context.Background(), 42, labels, forge.ReleasePRPhasePending)

		// then: only the pending label is added
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{testReleaseLabelPending}, addLabels)
	})
}

func TestGitHubKeepsTheOldLifecycleLabelWhenAttachingFails(t *testing.T) {
	t.Parallel()

	// given: a GitHub server that rejects the label addition on PR 42
	var removals atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
			writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
			w.WriteHeader(http.StatusInternalServerError)
			writeJSONFixture(t, w, "contracts/github/label_attach_failure/error.json")
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
			removals.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			fatalUnexpectedProviderRequest(t, "GitHub", r)
		}
	}))
	defer server.Close()

	p := newGitHubContractProvider(t, server)

	// when: the pull request is moved into the tagged phase
	err := p.SetReleasePRLabels(context.Background(), 42, defaultReleasePRLabels(), forge.ReleasePRPhaseTagged)

	// then: the failure surfaces and the pending label the next run finds it by
	// is never removed
	testastic.Error(t, err)
	testastic.Equal(t, int32(0), removals.Load())
}

func TestGitHubMergeReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("blocks readiness checks unless force is enabled", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub server reporting PR 42 as open with a blocked mergeable state
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/repos/o/r/pulls/42" {
				fatalUnexpectedProviderRequest(t, "GitHub", r)

				return
			}

			writeJSONFixture(t, w, "contracts/github/blocked_merge/pr.json")
		}))
		defer server.Close()

		p := newGitHubContractProvider(t, server)

		// when: MergeReleasePR is invoked without the force option
		_, err := p.MergeReleasePR(context.Background(), 42, forge.MergeReleasePROptions{})

		// then: forge.ErrMergeBlocked is returned with the blocked mergeable state in the message
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
		testastic.Equal(t, "release PR merge blocked: pull request #42 mergeable_state=blocked", err.Error())
	})

	t.Run("forces merge when readiness is otherwise blocked", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub server reporting PR 42 as blocked with squash merging allowed on the repo
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
				writeJSONFixture(t, w, "contracts/github/forced_merge/pr.json")
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
				writeJSONFixture(t, w, "contracts/github/forced_merge/repo.json")
			case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/42/merge":
				assertJSONRequest(t, r, "contracts/github/forced_merge/merge_request.json")
				writeJSONFixture(t, w, "contracts/github/forced_merge/result.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitHub", r)
			}
		}))
		defer server.Close()

		p := newGitHubContractProvider(t, server)

		// when: MergeReleasePR is invoked with merge checks bypassed and auto method selection
		_, err := p.MergeReleasePR(context.Background(), 42, forge.MergeReleasePROptions{
			BypassMergeChecks: true,
			Method:            forge.MergeMethodAuto,
		})

		// then: the squash merge method is chosen and the head SHA is sent in the merge request
		testastic.NoError(t, err)
	})
}

func TestGitHubUpdateFiles(t *testing.T) {
	t.Parallel()

	// given: a GitHub server exposing the base branch git data and accepting new objects
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/"+providerContractBaseBranch:
			writeJSONFixture(t, w, "contracts/github/update_files_git_data/base_ref.json")
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/commits/"+gitHubBaseCommitSHA:
			writeJSONFixture(t, w, "contracts/github/update_files_git_data/base_commit.json")
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/trees":
			assertJSONRequest(t, r, "contracts/github/update_files_git_data/tree_request.json")
			writeJSONFixture(t, w, "contracts/github/update_files_git_data/tree.json")
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/commits":
			assertJSONRequest(t, r, "contracts/github/update_files_git_data/commit_request.json")
			writeJSONFixture(t, w, "contracts/github/update_files_git_data/commit.json")
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/"+providerContractReleaseBranch:
			w.WriteHeader(http.StatusNotFound)
			writeJSONFixture(t, w, "contracts/github/_shared/not_found.json")
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
			assertJSONRequest(t, r, "contracts/github/update_files_git_data/ref_request.json")
			writeJSONFixture(t, w, "contracts/github/update_files_git_data/create_ref.json")
		default:
			fatalUnexpectedProviderRequest(t, "GitHub", r)
		}
	}))
	defer server.Close()

	p := newGitHubContractProvider(t, server)

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

	// then: the tree, commit, and branch ref requests match the recorded contract
	testastic.NoError(t, err)
}
