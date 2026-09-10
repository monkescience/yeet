package provider_test

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
)

type gitHubAutoMergeContract struct {
	name        string
	pullRequest string
	queue       string
	input       string
	response    string
	method      forge.MergeMethod
	mutation    string
	wantErr     error
	wantMessage string
}

func TestGitHubEnsureAutoMergeContract(t *testing.T) {
	t.Parallel()

	for _, scenario := range []gitHubAutoMergeContract{
		{
			name: "pending checks accepted", pullRequest: "pending", queue: "no_queue", input: "squash_input",
			response: "enabled_response", mutation: "enablePullRequestAutoMerge",
		},
		{
			name: "pending queue follows configured method", pullRequest: "pending", queue: "rebase_queue",
			input: "rebase_input", response: "enabled_response", mutation: "enablePullRequestAutoMerge",
		},
		{
			name: "ready request enters queue", pullRequest: "ready", queue: "squash_queue", input: "queue_input",
			response: "enqueued_response", mutation: "enqueuePullRequest",
		},
		{name: "already queued", pullRequest: "pending", queue: "queued"},
		{
			name: "explicit queue method conflicts", pullRequest: "pending", queue: "rebase_queue",
			method: forge.MergeMethodSquash, wantErr: forge.ErrMergeBlocked,
		},
		{
			name: "queue method cannot be guaranteed", pullRequest: "pending", queue: "unknown_queue",
			wantErr: forge.ErrMergeMethodUnsupported,
		},
		{name: "already enabled", pullRequest: "enabled", queue: "no_queue", method: forge.MergeMethodSquash},
		{
			name: "existing method conflicts", pullRequest: "enabled_merge", queue: "no_queue",
			method: forge.MergeMethodSquash, wantErr: forge.ErrMergeBlocked,
		},
		{
			name: "queue overrides stored auto merge method", pullRequest: "enabled_merge", queue: "squash_queue",
			method: forge.MergeMethodSquash,
		},
		{name: "already merged", pullRequest: "merged"},
		{name: "untrusted source", pullRequest: "untrusted", wantErr: forge.ErrUntrustedReleasePR},
		{
			name: "head protection unavailable", pullRequest: "no_head", queue: "no_queue",
			wantErr: forge.ErrAutoMergeUnsupported,
		},
		{
			name: "provider refuses scheduling", pullRequest: "pending", queue: "no_queue", input: "squash_input",
			response: "refused_response", mutation: "enablePullRequestAutoMerge",
			wantErr: forge.ErrMergeBlocked, wantMessage: "Auto-merge is not allowed for this repository",
		},
		{
			name: "null mutation is not accepted", pullRequest: "pending", queue: "no_queue", input: "squash_input",
			response: "null_response", mutation: "enablePullRequestAutoMerge",
			wantErr: forge.ErrMergeBlocked, wantMessage: "returned no accepted pull request",
		},
		{
			name: "missing GraphQL data is rejected", pullRequest: "pending", queue: "no_queue", input: "squash_input",
			response: "missing_data", mutation: "enablePullRequestAutoMerge",
			wantErr: forge.ErrMergeBlocked, wantMessage: "returned no data",
		},
		{
			name: "null GraphQL data is rejected", pullRequest: "pending", queue: "no_queue", input: "squash_input",
			response: "null_data", mutation: "enablePullRequestAutoMerge",
			wantErr: forge.ErrMergeBlocked, wantMessage: "returned no data",
		},
		{
			name: "changed head is refused", pullRequest: "pending", queue: "no_queue", input: "squash_input",
			response: "head_changed_response", mutation: "enablePullRequestAutoMerge",
			wantErr: forge.ErrMergeBlocked, wantMessage: "Head SHA does not match expectedHeadOid",
		},
		{
			name: "unsupported queue detection is refused", pullRequest: "pending", queue: "unsupported_response",
			wantErr: forge.ErrMergeBlocked, wantMessage: "Field 'isMergeQueueEnabled' does not exist",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a release request and the provider scheduling state for this scenario
			var mutations atomic.Int32

			server := httptest.NewTestServer(t, gitHubAutoMergeContractHandler(t, scenario, &mutations))
			p := newGitHubContractProvider(t, server)

			// when: ensuring provider-managed merging without waiting for completion
			err := p.EnsureAutoMerge(t.Context(), 42, forge.MergeReleasePROptions{
				BaseBranch: "main", ReleaseBranch: "yeet/release-main", Method: scenario.method,
			})

			// then: only the expected SHA-protected scheduling operation is allowed
			if scenario.wantErr != nil || scenario.wantMessage != "" {
				testastic.Error(t, err)
			} else {
				testastic.NoError(t, err)
			}

			if scenario.wantErr != nil {
				testastic.ErrorIs(t, err, scenario.wantErr)
			}

			if scenario.wantMessage != "" {
				testastic.ErrorContains(t, err, scenario.wantMessage)
			}

			if scenario.mutation == "" {
				testastic.Equal(t, int32(0), mutations.Load())
			} else {
				testastic.Equal(t, int32(1), mutations.Load())
			}
		})
	}
}

func gitHubAutoMergeContractHandler(
	t *testing.T,
	scenario gitHubAutoMergeContract,
	mutations *atomic.Int32,
) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
			writeJSONFixture(t, w, "contracts/github/auto_merge/"+scenario.pullRequest+".json")
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
			if scenario.pullRequest == "enabled" || scenario.pullRequest == "enabled_merge" {
				fatalUnexpectedProviderRequest(t, "GitHub native auto-merge", r)
			}

			writeJSONFixture(t, w, "contracts/github/merge_release_pr/repo.json")
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			handleGitHubAutoMergeGraphQL(t, w, r, scenario, mutations)
		default:
			fatalUnexpectedProviderRequest(t, "GitHub native auto-merge", r)
		}
	})
}

func handleGitHubAutoMergeGraphQL(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	scenario gitHubAutoMergeContract,
	mutations *atomic.Int32,
) {
	t.Helper()

	var request struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}

	err := json.UnmarshalRead(r.Body, &request)
	testastic.NoError(t, err)

	if strings.Contains(request.Query, "query YeetPullRequestMergeQueue") {
		testastic.Contains(t, request.Query, "isMergeQueueEnabled")
		testastic.Contains(t, request.Query, "mergeQueue(branch: $branch)")
		testastic.Equal(t, "main", request.Variables["branch"])
		writeJSONFixture(t, w, "contracts/github/auto_merge/"+scenario.queue+".json")

		return
	}

	testastic.NotEqual(t, "", scenario.mutation)
	testastic.Contains(t, request.Query, scenario.mutation+"(input: $input)")
	testastic.AssertJSON(t, "testdata/contracts/github/auto_merge/"+scenario.input+".json", request.Variables["input"])
	mutations.Add(1)
	writeJSONFixture(t, w, "contracts/github/auto_merge/"+scenario.response+".json")
}
