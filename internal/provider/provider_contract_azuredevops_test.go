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
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/provider"
)

const (
	azureDevOpsContractOrg     = "contoso-org"
	azureDevOpsContractProject = "contoso-project"
	azureDevOpsContractRepo    = "contoso-repo"
)

func azureDevOpsContractRepoAPI(suffix string) string {
	return fmt.Sprintf(
		"/%s/%s/_apis/git/repositories/%s/%s",
		azureDevOpsContractOrg,
		azureDevOpsContractProject,
		azureDevOpsContractRepo,
		strings.TrimLeft(suffix, "/"),
	)
}

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
) forge.Provider {
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
		map[string]forge.FileUpdate{"VERSION.txt": {Content: "version=1.2.3\n"}},
		"chore: release v1.2.3",
	)

	testastic.NoError(t, err)
	testastic.Equal(t, int32(1), baseLookups.Load())
	testastic.Equal(t, int32(1), branchLookups.Load())
}

func TestAzureDevOpsGetBranchHeadFindsABranchOnALaterRefPage(t *testing.T) {
	t.Parallel()

	// given: the release branch sorts behind a full page of prefix-matching siblings
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		if isAzureDevOpsRefsRequest(r, "heads/"+providerContractReleaseBranch) {
			writeAzureDevOpsTruncatedRefs(
				t, w, r,
				"refs/heads/"+providerContractReleaseBranch,
				"72656c6561736573686100000000000000000000",
			)

			return
		}

		fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: the head of that branch is resolved
	head, err := p.GetBranchHead(context.Background(), providerContractReleaseBranch)

	// then: the continuation page answers with the branch tip
	testastic.NoError(t, err)
	testastic.Equal(t, "72656c6561736573686100000000000000000000", head)
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
						"commitId": "66696e616c736861000000000000000000000000",
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
					"commitId": "736f757263657368610000000000000000000000",
				},
			})
		case r.Method == http.MethodPatch && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42"):
			completed.Store(true)

			writeJSON(t, w, map[string]any{
				"pullRequestId": 42,
				"status":        "completed",
				"mergeStatus":   "queued",
				"lastMergeCommit": map[string]any{
					"commitId": "7072657669657773686100000000000000000000",
				},
				"lastMergeSourceCommit": map[string]any{
					"commitId": "736f757263657368610000000000000000000000",
				},
			})
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(
		t,
		server,
		provider.WithMergePolling(time.Millisecond, time.Millisecond, 5*time.Second),
	)

	// when: the release pull request is submitted for completion
	mergeSHA, err := p.MergeReleasePR(context.Background(), 42, forge.MergeReleasePROptions{
		Method: forge.MergeMethodSquash,
	})

	// then: the provisional commit is skipped and the applied merge commit is returned
	testastic.NoError(t, err)
	testastic.Equal(t, "66696e616c736861000000000000000000000000", mergeSHA)
}

func TestAzureDevOpsMergeReleasePRFastRefusal(t *testing.T) {
	t.Parallel()

	// given: an Azure DevOps server whose completion response reports a refusal
	var completed atomic.Bool

	var polls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			if completed.Load() {
				polls.Add(1)
			}

			writeJSONFixture(t, w, azureDevOpsContractFixture("merge_release_pr", "pull_request.json"))
		case r.Method == http.MethodPatch && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42"):
			completed.Store(true)

			assertJSONRequest(t, r, azureDevOpsContractFixture("merge_release_pr_fast_refusal", "request.json"))
			writeJSONFixture(t, w, azureDevOpsContractFixture("merge_release_pr_fast_refusal", "refused.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}))
	defer server.Close()

	p := newAzureDevOpsContractProvider(
		t,
		server,
		provider.WithMergePolling(time.Millisecond, time.Millisecond, 50*time.Millisecond),
	)

	// when: MergeReleasePR is invoked on the refused pull request
	mergeSHA, err := p.MergeReleasePR(context.Background(), 42, forge.MergeReleasePROptions{
		Method: forge.MergeMethodSquash,
	})

	// then: the refusal is reported as a blocked merge without waiting for the forge
	testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
	testastic.Equal(t, "", mergeSHA)
	testastic.Equal(t, int32(0), polls.Load())
}

func TestAzureDevOpsMergeReleasePRPollingRefusal(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		refusal string
		message string
		reason  forge.MergeBlockedReason
	}{
		{
			name:    "policy rejection",
			refusal: "refused_policy.json",
			message: "the merge was rejected by a branch policy",
			reason:  forge.MergeBlockedReasonPolicy,
		},
		{
			name:    "conflicts",
			refusal: "refused_conflicts.json",
			message: "the source branch conflicts with the target branch",
			reason:  forge.MergeBlockedReasonConflicts,
		},
		{
			name:    "provider failure",
			refusal: "refused_failure.json",
			message: "the provider could not create the merge commit",
			reason:  forge.MergeBlockedReasonFailure,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: Azure queues completion and reports a terminal refusal on the next read
			var completed atomic.Bool

			var polls atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if handleAzureDevOpsBootstrap(t, w, r) {
					return
				}

				switch {
				case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
					if completed.Load() {
						polls.Add(1)
						writeJSONFixture(
							t, w,
							azureDevOpsContractFixture("merge_release_pr_polling_refusal", testCase.refusal),
						)

						return
					}

					writeJSONFixture(t, w, azureDevOpsContractFixture("merge_release_pr", "pull_request.json"))
				case r.Method == http.MethodPatch && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42"):
					completed.Store(true)

					assertJSONRequest(
						t, r,
						azureDevOpsContractFixture("merge_release_pr_polling_refusal", "request.json"),
					)
					writeJSONFixture(
						t, w,
						azureDevOpsContractFixture("merge_release_pr_polling_refusal", "queued.json"),
					)
				default:
					fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
				}
			}))
			defer server.Close()

			p := newAzureDevOpsContractProvider(
				t,
				server,
				provider.WithMergePolling(time.Millisecond, time.Millisecond, 50*time.Millisecond),
			)

			// when: MergeReleasePR polls a completion that Azure later refuses
			mergeSHA, err := p.MergeReleasePR(context.Background(), 42, forge.MergeReleasePROptions{
				Method: forge.MergeMethodSquash,
			})

			// then: the terminal refusal is preserved instead of becoming a polling timeout
			var blocked *forge.MergeBlockedError
			testastic.ErrorAs(t, err, &blocked)

			if blocked == nil {
				return
			}

			testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
			testastic.Equal(t, string(testCase.reason), string(blocked.Reason))
			testastic.Equal(t, "was refused: "+testCase.message, blocked.Detail)
			testastic.Equal(t, "", mergeSHA)
			testastic.Equal(t, int32(1), polls.Load())
		})
	}
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
				"commitId": "7072657669657773686100000000000000000000",
			},
		})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(
		t,
		server,
		provider.WithMergePolling(time.Millisecond, time.Millisecond, 50*time.Millisecond),
	)

	// when: completion is retried for the already completed pull request
	_, err := p.MergeReleasePR(context.Background(), 42, forge.MergeReleasePROptions{})

	// then: the queued preview commit is never exposed as the final merge commit
	testastic.ErrorIs(t, err, forge.ErrMergeNotFinalized)
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
		"lastMergeSourceCommit": map[string]any{"commitId": "61747461636b6572736f75726365736861000000"},
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
				"lastMergeCommit": map[string]any{"commitId": "61747461636b6572736861000000000000000000"},
			})
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(
		t,
		server,
		provider.WithMergePolling(time.Millisecond, time.Millisecond, 50*time.Millisecond),
	)

	// when: the foreign pull request is submitted for completion
	mergeSHA, err := p.MergeReleasePR(context.Background(), 42, forge.MergeReleasePROptions{
		Method: forge.MergeMethodSquash,
	})

	// then: it is refused as untrusted before any completion is attempted
	testastic.ErrorIs(t, err, forge.ErrUntrustedReleasePR)
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
			"commitId": "7072657669657773686100000000000000000000",
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
			"commitId": "736f757263657368610000000000000000000000",
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

func TestAzureDevOpsFindMergedReleasePRReportsWithheldCloseDate(t *testing.T) {
	t.Parallel()

	// given: two completed candidates listed without close dates, where re-reading
	// the first still withholds one and re-reading the second fails
	var rereads []string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case isAzureDevOpsPullRequestsListRequest(r):
			writeJSONFixture(t, w, azureDevOpsContractFixture("find_merged_pr_undated", "pull_requests.json"))
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/41"):
			rereads = append(rereads, "41")

			writeJSONFixture(t, w, azureDevOpsContractFixture("find_merged_pr_undated", "pull_request_41.json"))
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			rereads = append(rereads, "42")

			w.WriteHeader(http.StatusInternalServerError)
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: finding the merged release pull request
	_, err := p.FindMergedReleasePR(
		context.Background(),
		providerContractBaseBranch,
		providerContractPendingLabel,
	)

	// then: the ambiguity names the pull request that caused it, unmasked by the later failure
	testastic.Contains(t, err.Error(), "merged release PR completion time is unavailable: pull request !41")
	testastic.SliceEqual(t, []string{"41"}, rereads)
}

func TestAzureDevOpsFindMergedReleasePRSelectsGreatestClosedDate(t *testing.T) {
	t.Parallel()

	// given: two completed candidates whose newer close time belongs to PR 42
	var detailRequests atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAzureDevOpsBootstrap(t, w, r) {
			return
		}

		switch {
		case isAzureDevOpsPullRequestsListRequest(r):
			testastic.Equal(
				t,
				"refs/heads/"+providerContractPendingBranch,
				r.URL.Query().Get("searchCriteria.sourceRefName"),
			)
			writeJSONFixture(t, w, azureDevOpsContractFixture("find_merged_pr", "pull_requests.json"))
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			detailRequests.Add(1)
			writeJSONFixture(t, w, azureDevOpsContractFixture("find_merged_pr", "pull_request.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server)

	// when: finding the merged release pull request
	pr, err := p.FindMergedReleasePR(
		context.Background(),
		providerContractBaseBranch,
		providerContractPendingLabel,
	)

	// then: only the candidate with the greatest ClosedDate is fetched
	testastic.NoError(t, err)
	testastic.Equal(t, 42, pr.Number)
	testastic.Equal(t, int32(1), detailRequests.Load())
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
	err := p.SetReleasePRLabels(context.Background(), 42, forge.ReleasePRLabels{
		Pending: providerContractPendingLabel,
		Tagged:  providerContractTaggedLabel,
		Extra:   []string{"rejected"},
	}, forge.ReleasePRPhasePending)

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
	err := p.SetReleasePRLabels(context.Background(), 42, forge.ReleasePRLabels{
		Pending: providerContractPendingLabel,
		Tagged:  providerContractTaggedLabel,
		Extra:   []string{"rejected", "kept"},
		Yeet:    true,
	}, forge.ReleasePRPhasePending)

	// then: the rejection surfaces but the labels queued behind it are still attached
	testastic.Error(t, err)

	for _, label := range []string{providerContractPendingLabel, "kept", provider.ReleaseLabelYeet} {
		_, ok := attached.Load(label)

		testastic.True(t, ok)
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
	err := p.SetReleasePRLabels(context.Background(), 42, forge.ReleasePRLabels{
		Pending: providerContractPendingLabel,
		Tagged:  providerContractTaggedLabel,
		Yeet:    true,
	}, forge.ReleasePRPhasePending)

	// then: the failure is returned after attaching the pending marker for retry discovery
	testastic.Error(t, err)
	testastic.True(t, pendingAttached.Load())

	// when: marking the release pull request pending with the managed label disabled
	err = p.SetReleasePRLabels(context.Background(), 42, forge.ReleasePRLabels{
		Pending: providerContractPendingLabel,
		Tagged:  providerContractTaggedLabel,
	}, forge.ReleasePRPhasePending)

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
	err := p.SetReleasePRLabels(context.Background(), 42, forge.ReleasePRLabels{
		Pending: "release pending",
		Tagged:  providerContractTaggedLabel,
	}, forge.ReleasePRPhaseTagged)

	// then: the case-variant tag is recognised as the configured label and removed
	testastic.NoError(t, err)
	testastic.True(t, deleted.Load())
}

// Azure's SDK performs lazy bootstrap requests before scenario-specific calls.
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
			failProviderContractHandler(
				t,
				fmt.Sprintf(
					"unhandled Azure DevOps contract scenario: %s (request %s %s)",
					scenario,
					r.Method,
					r.URL.String(),
				),
			)
		}
	}
}

func isAzureDevOpsRefsRequest(r *http.Request, filter string) bool {
	return r.Method == http.MethodGet &&
		r.URL.Path == azureDevOpsContractRepoAPI("refs") &&
		r.URL.Query().Get("filter") == filter
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
		failProviderContractHandler(t, "unexpected Azure DevOps identity lookup: "+filterValue)
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
	_, err := p.CreateReleasePR(context.Background(), forge.ReleasePROptions{
		Title:         providerContractReleaseTitle,
		Body:          providerContractReleaseBody,
		BaseBranch:    providerContractBaseBranch,
		ReleaseBranch: providerContractReleaseBranch,
		Reviewers:     []string{"alex"},
	})

	// then: the run fails before any PR is created, flagging the ambiguity
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, forge.ErrReviewerAmbiguous)
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
		testastic.Equal(
			t,
			"refs/heads/"+providerContractPendingBranch,
			r.URL.Query().Get("searchCriteria.sourceRefName"),
		)
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
			testastic.Equal(
				t,
				"refs/heads/"+providerContractPendingBranch,
				r.URL.Query().Get("searchCriteria.sourceRefName"),
			)
			writeJSONFixture(t, w, azureDevOpsContractFixture("find_merged_pr", "pull_requests.json"))
		case r.Method == http.MethodGet && r.URL.Path == azureDevOpsContractPullRequestAPI():
			writeJSONFixture(t, w, azureDevOpsContractFixture("find_merged_pr", "pull_request.json"))
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

// Azure DevOps creates label definitions on attachment, so registry state does not apply.
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
				"lastMergeCommit": map[string]any{"commitId": "7072657669657773686100000000000000000000"},
			})
		default:
			fatalUnexpectedProviderRequest(t, "Azure DevOps", r)
		}
	}
}

func azureDevOpsCreateReleaseHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
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
			testastic.Equal(t, "6261736573686100000000000000000000000000", request[0].NewObjectID)

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
			testastic.Equal(t, "6261736573686100000000000000000000000000", push.RefUpdates[0].OldObjectID)
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
