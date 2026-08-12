package provider_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/provider"
)

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
			writeJSON(t, w, map[string]any{
				"pullRequestId":       42,
				"status":              "active",
				"mergeStatus":         "rejectedByPolicy",
				"mergeFailureMessage": "the merge was rejected by a branch policy",
			})
		default:
			t.Errorf("unexpected Azure DevOps request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	p := newAzureDevOpsContractProvider(t, server, provider.WithMergePolling(time.Millisecond, 50*time.Millisecond))

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
		name        string
		mergeStatus string
		message     string
		reason      forge.MergeBlockedReason
	}{
		{
			name:        "policy rejection",
			mergeStatus: "rejectedByPolicy",
			message:     "the merge was rejected by a branch policy",
			reason:      forge.MergeBlockedReasonPolicy,
		},
		{
			name:        "conflicts",
			mergeStatus: "conflicts",
			message:     "the source branch conflicts with the target branch",
			reason:      forge.MergeBlockedReasonConflicts,
		},
		{
			name:        "provider failure",
			mergeStatus: "failure",
			message:     "the provider could not create the merge commit",
			reason:      forge.MergeBlockedReasonFailure,
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
						writeJSON(t, w, map[string]any{
							"pullRequestId":       42,
							"status":              "active",
							"mergeStatus":         testCase.mergeStatus,
							"mergeFailureMessage": testCase.message,
						})

						return
					}

					writeJSONFixture(t, w, azureDevOpsContractFixture("merge_release_pr", "pull_request.json"))
				case r.Method == http.MethodPatch && r.URL.Path == azureDevOpsContractRepoAPI("pullRequests/42"):
					completed.Store(true)
					writeJSON(t, w, map[string]any{
						"pullRequestId": 42,
						"status":        "active",
						"mergeStatus":   "queued",
					})
				default:
					t.Errorf("unexpected Azure DevOps request: %s %s", r.Method, r.URL.String())
				}
			}))
			defer server.Close()

			p := newAzureDevOpsContractProvider(
				t,
				server,
				provider.WithMergePolling(time.Millisecond, 50*time.Millisecond),
			)

			// when: MergeReleasePR polls a completion that Azure later refuses
			mergeSHA, err := p.MergeReleasePR(context.Background(), 42, forge.MergeReleasePROptions{
				Method: forge.MergeMethodSquash,
			})

			// then: the terminal refusal is preserved instead of becoming a polling timeout
			var blocked *forge.MergeBlockedError
			if !errors.As(err, &blocked) {
				t.Fatalf("expected forge.MergeBlockedError, got %v", err)
			}

			testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
			testastic.Equal(t, string(testCase.reason), string(blocked.Reason))
			testastic.Equal(t, "was refused: "+testCase.message, blocked.Detail)
			testastic.Equal(t, "", mergeSHA)
			testastic.Equal(t, int32(1), polls.Load())
		})
	}
}

func decodedPathTail(t *testing.T, request *http.Request) string {
	t.Helper()

	segments := strings.Split(request.URL.EscapedPath(), "/")
	if len(segments) == 0 {
		t.Fatalf("unexpected path: %s", request.URL.EscapedPath())
	}

	label, err := url.PathUnescape(segments[len(segments)-1])
	testastic.NoError(t, err)

	return label
}

func isGitLabRawFilePath(request *http.Request, path string) bool {
	if request.URL.Query().Get("ref") != "main" {
		return false
	}

	prefix := "/api/v4/projects/o%2Fr/repository/files/"
	suffix := "/raw"

	escapedPath := request.URL.EscapedPath()
	if !strings.HasPrefix(escapedPath, prefix) || !strings.HasSuffix(escapedPath, suffix) {
		return false
	}

	rawPath := strings.TrimSuffix(strings.TrimPrefix(escapedPath, prefix), suffix)

	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return false
	}

	return decodedPath == path
}
