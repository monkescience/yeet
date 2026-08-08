package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// azureDevOpsContractPullRequestAPI returns the repo-scoped single pull request
// API path used by GetPullRequest.
func azureDevOpsContractPullRequestAPI() string {
	return azureDevOpsContractRepoAPI("pullRequests/42")
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

func newAzureDevOpsContractProvider(
	t *testing.T,
	server *httptest.Server,
	options ...provider.MergePollingOption,
) provider.Provider {
	t.Helper()

	return provider.NewAzureDevOps(
		server.Client(),
		server.URL,
		"contoso-pat",
		azureDevOpsContractOrg,
		azureDevOpsContractOrg,
		azureDevOpsContractProject,
		azureDevOpsContractRepo,
		options...,
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
	prs, err := p.FindOpenPendingReleasePRs(
		context.Background(),
		providerContractBaseBranch,
		testReleaseLabelPending,
	)

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

func TestAzureDevOpsCreateBranchFindsAnExistingBranchOnALaterRefPage(t *testing.T) {
	t.Parallel()

	var refUpdates atomic.Int32

	// given: the release branch sorts behind a full page of prefix-matching siblings
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case isAzureDevOpsRefsRequest(r, "heads/"+providerContractBaseBranch):
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_branch", "base_ref.json"))
		case isAzureDevOpsRefsRequest(r, "heads/"+providerContractReleaseBranch):
			writeAzureDevOpsTruncatedRefs(t, w, r, "refs/heads/"+providerContractReleaseBranch, "release-sha")
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("refs"):
			refUpdates.Add(1)
			writeJSONFixture(t, w, azureDevOpsContractFixture("create_branch", "ref_update.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: the branch that already exists is created
	err := p.CreateBranch(
		context.Background(),
		providerContractReleaseBranch,
		providerContractBaseBranch,
	)

	// then: the continuation page proves the branch exists, so nothing is created
	testastic.NoError(t, err)
	testastic.Equal(t, int32(0), refUpdates.Load())
}

func TestAzureDevOpsGetReleaseByTagFindsATagOnALaterRefPage(t *testing.T) {
	t.Parallel()

	// given: the tag sorts behind a full page of prerelease siblings
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case isAzureDevOpsRefsRequest(r, "tags/"+providerContractTag):
			writeAzureDevOpsTruncatedRefs(t, w, r, "refs/tags/"+providerContractTag, "tag-object-123")
		case r.Method == http.MethodGet &&
			r.URL.Path == azureDevOpsContractRepoAPI("annotatedTags/tag-object-123"):
			writeJSONFixture(t, w, azureDevOpsContractFixture("get_release_by_tag", "annotated_tag.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: the release is looked up by that tag
	release, err := p.GetReleaseByTag(context.Background(), providerContractTag)

	// then: the continuation page yields the released tag
	testastic.NoError(t, err)
	testastic.Equal(t, providerContractTag, release.TagName)
	testastic.Equal(t, "release notes", release.Body)
}

const azureDevOpsContractRefContinuationToken = "refs-page-2"

// writeAzureDevOpsTruncatedRefs answers a ref query the way a prefix filter
// does: a full first page of refs that merely start with the wanted name, and
// the wanted ref itself only on the continuation page.
func writeAzureDevOpsTruncatedRefs(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	name, objectID string,
) {
	t.Helper()

	if r.URL.Query().Get("continuationToken") == azureDevOpsContractRefContinuationToken {
		wanted := []map[string]any{{"name": name, "objectId": objectID}}
		writeJSON(t, w, map[string]any{"count": len(wanted), "value": wanted})

		return
	}

	const pageSize = 100

	siblings := make([]map[string]any, pageSize)
	for idx := range siblings {
		siblings[idx] = map[string]any{
			"name":     fmt.Sprintf("%s-%03d", name, idx),
			"objectId": fmt.Sprintf("sibling-sha-%03d", idx),
		}
	}

	w.Header().Set("x-ms-continuationtoken", azureDevOpsContractRefContinuationToken)
	writeJSON(t, w, map[string]any{"count": len(siblings), "value": siblings})
}

func TestAzureDevOpsMergeReleasePRWaitsForFinalMergeCommit(t *testing.T) {
	t.Parallel()

	// given: Azure queues completion while returning preview and source commits
	var completed atomic.Bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			if completed.Load() {
				writeJSON(t, w, map[string]any{
					"pullRequestId": 42,
					"status":        "completed",
					"mergeStatus":   "succeeded",
					"lastMergeCommit": map[string]any{
						"commitId": "final-sha",
					},
				})

				return
			}

			writeJSON(t, w, map[string]any{
				"pullRequestId": 42,
				"status":        "active",
				"mergeStatus":   "succeeded",
				"isDraft":       false,
				"sourceRefName": "refs/heads/yeet/release-main",
				"targetRefName": "refs/heads/main",
				"repository":    map[string]any{"name": azureDevOpsContractRepo},
				"lastMergeSourceCommit": map[string]any{
					"commitId": "source-sha",
				},
			})
		case r.Method == http.MethodPatch && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42"):
			completed.Store(true)

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

	p := newAzureDevOpsContractProvider(t, server, provider.WithMergePolling(time.Millisecond, 5*time.Second))

	// when: the release pull request is submitted for completion
	mergeSHA, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
		Method: provider.MergeMethodSquash,
	})

	// then: the provisional commit is skipped and the applied merge commit is returned
	testastic.NoError(t, err)
	testastic.Equal(t, "final-sha", mergeSHA)
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

	p := newAzureDevOpsContractProvider(t, server, provider.WithMergePolling(time.Millisecond, 50*time.Millisecond))

	// when: completion is retried for the already completed pull request
	_, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{})

	// then: the queued preview commit is never exposed as the final merge commit
	testastic.ErrorIs(t, err, provider.ErrMergeNotFinalized)
}

func TestAzureDevOpsMergeReleasePRRejectsPullRequestFromAnotherRepository(t *testing.T) {
	t.Parallel()

	pullRequest := map[string]any{
		"pullRequestId": 42,
		"status":        "active",
		"mergeStatus":   "succeeded",
		"isDraft":       false,
		"sourceRefName": "refs/heads/yeet/release-main",
		"targetRefName": "refs/heads/main",
		"repository": map[string]any{
			"id":   "00000000-0000-0000-0000-0000000000ff",
			"name": "attacker-repo",
		},
		"lastMergeSourceCommit": map[string]any{"commitId": "attacker-source-sha"},
	}

	var completionAttempts atomic.Int32

	// given: a pull request that belongs to a repository yeet is not configured for
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			writeJSON(t, w, pullRequest)
		case r.Method == http.MethodPatch && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42"):
			completionAttempts.Add(1)

			writeJSON(t, w, map[string]any{
				"pullRequestId":   42,
				"status":          "completed",
				"mergeStatus":     "succeeded",
				"lastMergeCommit": map[string]any{"commitId": "attacker-sha"},
			})
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server, provider.WithMergePolling(time.Millisecond, 50*time.Millisecond))

	// when: the foreign pull request is submitted for completion
	mergeSHA, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
		Method: provider.MergeMethodSquash,
	})

	// then: it is refused as untrusted before any completion is attempted
	testastic.ErrorIs(t, err, provider.ErrUntrustedReleasePR)
	testastic.Equal(t, "", mergeSHA)
	testastic.Equal(t, int32(0), completionAttempts.Load())
}

func TestAzureDevOpsFindMergedReleasePRRejectsQueuedCommit(t *testing.T) {
	t.Parallel()

	pullRequest := map[string]any{
		"pullRequestId": 42,
		"status":        "completed",
		"mergeStatus":   "queued",
		"sourceRefName": "refs/heads/yeet/release-main",
		"targetRefName": "refs/heads/main",
		"repository":    map[string]any{"name": azureDevOpsContractRepo},
		"lastMergeCommit": map[string]any{
			"commitId": "preview-sha",
		},
		"labels": []map[string]any{{
			"name":   testReleaseLabelPending,
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
	pr, err := p.FindMergedReleasePR(
		context.Background(),
		providerContractBaseBranch,
		testReleaseLabelPending,
	)

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
		"repository":    map[string]any{"name": azureDevOpsContractRepo},
		"lastMergeSourceCommit": map[string]any{
			"commitId": "source-sha",
		},
		"labels": []map[string]any{{
			"name":   testReleaseLabelPending,
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
	pr, err := p.FindMergedReleasePR(
		context.Background(),
		providerContractBaseBranch,
		testReleaseLabelPending,
	)

	// then: its source commit is not exposed as the final merge commit
	testastic.NoError(t, err)
	testastic.Equal(t, "", pr.MergeCommitSHA)
}

func TestAzureDevOpsSetReleasePRLabelsKeepsPartialFailureRetryable(t *testing.T) {
	t.Parallel()

	var pendingAttached atomic.Bool

	// given: Azure accepts the pending label but rejects a configured extra label
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels") {
			writeJSON(t, w, map[string]any{"value": []any{}})

			return
		}

		if r.Method != http.MethodPost || r.URL.Path != azureDevOpsContractRepoAPI("pullRequests/42/labels") {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		var request struct {
			Name string `json:"name"`
		}
		decodeJSONRequest(t, r, &request)

		if request.Name == providerContractPendingLabel {
			pendingAttached.Store(true)
			writeJSON(t, w, map[string]any{"name": request.Name})

			return
		}

		w.WriteHeader(http.StatusBadRequest)
		writeJSON(t, w, map[string]any{"message": "label rejected"})
	}))
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: marking a release pull request pending with the rejected extra label
	err := p.SetReleasePRLabels(context.Background(), 42, provider.ReleasePRLabels{
		Pending: providerContractPendingLabel,
		Tagged:  providerContractTaggedLabel,
		Extra:   []string{"rejected"},
	}, provider.ReleasePRPhasePending)

	// then: the failure is returned after attaching the pending marker for retry discovery
	testastic.Error(t, err)
	testastic.True(t, pendingAttached.Load())
}

func TestAzureDevOpsSetReleasePRLabelsAttachesLabelsAfterARejectedOne(t *testing.T) {
	t.Parallel()

	var attached sync.Map

	// given: Azure rejects one configured extra label but accepts every other
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels") {
			writeJSON(t, w, map[string]any{"value": []any{}})

			return
		}

		if r.Method != http.MethodPost || r.URL.Path != azureDevOpsContractRepoAPI("pullRequests/42/labels") {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		var request struct {
			Name string `json:"name"`
		}
		decodeJSONRequest(t, r, &request)

		if request.Name == "rejected" {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(t, w, map[string]any{"message": "label rejected"})

			return
		}

		attached.Store(request.Name, true)
		writeJSON(t, w, map[string]any{"name": request.Name})
	}))
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: marking pending with a rejected label positioned before other labels
	err := p.SetReleasePRLabels(context.Background(), 42, provider.ReleasePRLabels{
		Pending: providerContractPendingLabel,
		Tagged:  providerContractTaggedLabel,
		Extra:   []string{"rejected", "kept"},
		Yeet:    true,
	}, provider.ReleasePRPhasePending)

	// then: the rejection surfaces but the labels queued behind it are still attached
	testastic.Error(t, err)

	for _, label := range []string{providerContractPendingLabel, "kept", provider.ReleaseLabelYeet} {
		if _, ok := attached.Load(label); !ok {
			t.Errorf("label %q was not attached", label)
		}
	}
}

func TestAzureDevOpsSetReleasePRLabelsKeepsManagedFailureRetryable(t *testing.T) {
	t.Parallel()

	var pendingAttached atomic.Bool

	// given: Azure rejects the managed label
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels") {
			writeJSON(t, w, map[string]any{"value": []any{}})

			return
		}

		if r.Method != http.MethodPost || r.URL.Path != azureDevOpsContractRepoAPI("pullRequests/42/labels") {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		var request struct {
			Name string `json:"name"`
		}
		decodeJSONRequest(t, r, &request)

		if request.Name == providerContractPendingLabel {
			pendingAttached.Store(true)
			writeJSON(t, w, map[string]any{"name": request.Name})

			return
		}

		w.WriteHeader(http.StatusBadRequest)
		writeJSON(t, w, map[string]any{"message": "label rejected"})
	}))
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: marking a release pull request pending with the managed label enabled
	err := p.SetReleasePRLabels(context.Background(), 42, provider.ReleasePRLabels{
		Pending: providerContractPendingLabel,
		Tagged:  providerContractTaggedLabel,
		Yeet:    true,
	}, provider.ReleasePRPhasePending)

	// then: the failure is returned after attaching the pending marker for retry discovery
	testastic.Error(t, err)
	testastic.True(t, pendingAttached.Load())

	// when: marking the release pull request pending with the managed label disabled
	err = p.SetReleasePRLabels(context.Background(), 42, provider.ReleasePRLabels{
		Pending: providerContractPendingLabel,
		Tagged:  providerContractTaggedLabel,
	}, provider.ReleasePRPhasePending)

	// then: Azure attaches the pending marker without requesting the managed label
	testastic.NoError(t, err)
	testastic.True(t, pendingAttached.Load())
}

func TestAzureDevOpsLifecycleLabelRemovalMatchesCaseInsensitively(t *testing.T) {
	t.Parallel()

	var deleted atomic.Bool

	// given: a pull request with a tag that differs only by case from the pending label
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels"):
			writeJSON(t, w, map[string]any{"name": providerContractTaggedLabel})
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels"):
			writeJSON(t, w, map[string]any{"value": []map[string]any{{
				"id":   "00000000-0000-0000-0000-000000000043",
				"name": "Release Pending",
			}}})
		case r.Method == http.MethodDelete:
			deleted.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}))
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: transitioning a differently cased configured label to tagged
	err := p.SetReleasePRLabels(context.Background(), 42, provider.ReleasePRLabels{
		Pending: "release pending",
		Tagged:  providerContractTaggedLabel,
	}, provider.ReleasePRPhaseTagged)

	// then: the case-variant tag is recognised as the configured label and removed
	testastic.NoError(t, err)
	testastic.True(t, deleted.Load())
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
	case providerContractListTagsPaged:
		return azureDevOpsListTagsPagedHandler(t)
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
	case providerContractFindOpenPRsUnlabeled:
		return azureDevOpsFindOpenPRsFixtureHandler(t, "find_open_prs_unlabeled")
	case providerContractFindOpenPRsAdoptable:
		return azureDevOpsFindOpenPRsFixtureHandler(t, "find_open_prs_adoptable")
	case providerContractFindMergedPR:
		return azureDevOpsFindMergedPRHandler(t)
	case providerContractMergeReleasePR:
		return azureDevOpsMergeReleasePRHandler(t)
	case providerContractAsyncMergeReleasePR:
		return azureDevOpsAsyncMergeReleasePRHandler(t)
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
	case providerContractTagPaginationLimit:
		return azureDevOpsTagPaginationLimitHandler(t)
	case providerContractForcedMergeUntrusted:
		return azureDevOpsForcedMergeUntrustedHandler(t)
	case providerContractForcedMergeConflicted:
		return azureDevOpsForcedMergeConflictedHandler(t)
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

func azureDevOpsListTagsPagedHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if !isAzureDevOpsRefsRequest(r, "tags/") {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		if r.URL.Query().Get("continuationToken") == azureDevOpsContractRefContinuationToken {
			writeJSONFixture(t, w, azureDevOpsContractFixture("list_tags", "page_two.json"))

			return
		}

		w.Header().Set("x-ms-continuationtoken", azureDevOpsContractRefContinuationToken)
		writeJSONFixture(t, w, azureDevOpsContractFixture("list_tags", "tags.json"))
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

func azureDevOpsFindOpenPRsFixtureHandler(t *testing.T, dir string) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if !isAzureDevOpsPullRequestsListRequest(r) {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		writeJSONFixture(t, w, azureDevOpsContractFixture(dir, "pull_requests.json"))
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

// newAzureDevOpsContractLabelHandler tracks the labels on PR 42 so a scenario
// can assert the set a phase leaves behind. The registry is ignored, because
// Azure DevOps creates a tag definition when a label is attached.
func newAzureDevOpsContractLabelHandler(
	t *testing.T,
	store *providerContractLabelStore,
	_ providerContractLabelRegistry,
) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels"):
			var request struct {
				Name string `json:"name"`
			}
			decodeJSONRequest(t, r, &request)
			store.attach(request.Name)
			writeJSON(t, w, map[string]any{
				"name":   request.Name,
				"id":     store.id(request.Name),
				"active": true,
			})
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42/labels"):
			writeJSON(t, w, map[string]any{"value": store.definitions()})
		case r.Method == http.MethodDelete &&
			strings.HasPrefix(r.URL.Path, azureDevOpsContractRepoAPI("pullRequests/42/labels/")):
			store.detachID(strings.TrimPrefix(r.URL.Path, azureDevOpsContractRepoAPI("pullRequests/42/labels/")))
			w.WriteHeader(http.StatusNoContent)
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})
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

// azureDevOpsAsyncMergeReleasePRHandler models a completion Azure DevOps queues,
// returning a provisional commit before the merge is applied.
func azureDevOpsAsyncMergeReleasePRHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	var completed atomic.Bool

	return func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			if completed.Load() {
				writeJSON(t, w, map[string]any{
					"pullRequestId":   42,
					"status":          "completed",
					"mergeStatus":     "succeeded",
					"lastMergeCommit": map[string]any{"commitId": providerContractMergeSHA},
				})

				return
			}

			writeJSONFixture(t, w, azureDevOpsContractFixture("merge_release_pr", "pull_request.json"))
		case r.Method == http.MethodPatch && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42"):
			completed.Store(true)
			writeJSON(t, w, map[string]any{
				"pullRequestId":   42,
				"status":          "completed",
				"mergeStatus":     "queued",
				"lastMergeCommit": map[string]any{"commitId": "preview-sha"},
			})
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

func azureDevOpsTagPaginationLimitHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	var pages atomic.Int32

	return func(w http.ResponseWriter, r *http.Request) {
		if !isAzureDevOpsRefsRequest(r, "tags/") {
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)

			return
		}

		page := pages.Add(1)
		w.Header().Set("x-ms-continuationtoken", fmt.Sprintf("refs-page-%d", page+1))
		writeJSON(t, w, map[string]any{
			"count": 1,
			"value": []map[string]any{{
				"name":           fmt.Sprintf("refs/tags/v0.0.%d", page),
				"objectId":       fmt.Sprintf("tag-object-%d", page),
				"peeledObjectId": fmt.Sprintf("sha-%d", page),
			}},
		})
	}
}

func azureDevOpsForcedMergeUntrustedHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI() {
			writeJSONFixture(t, w, azureDevOpsContractFixture("forced_merge_untrusted", "pull_request.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}

func azureDevOpsForcedMergeConflictedHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI() {
			writeJSONFixture(t, w, azureDevOpsContractFixture("forced_merge_conflicted", "pull_request.json"))

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	}
}
