package provider //nolint:testpackage // validates unexported HTTP transport policy directly

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

// fastRetryClient keeps the production attempt bound while shortening the waits
// so a real retry test finishes in milliseconds.
func fastRetryClient(t *testing.T) *http.Client {
	t.Helper()

	client := newTracedRetryableClient(providerNameGitHub)
	client.RetryWaitMin = time.Millisecond
	client.RetryWaitMax = 5 * time.Millisecond

	return client.StandardClient()
}

func countingServer(t *testing.T, status func(attempt int32) int) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status(attempts.Add(1)))
	}))
	t.Cleanup(server.Close)

	return server, &attempts
}

func TestCreateProviderBuildsSharedClientSettingsForEveryForge(t *testing.T) {
	// given: credentials for all three forges and no endpoint overrides
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "test-token")
	t.Setenv(githubURLEnv, "")
	t.Setenv(gitlabURLEnv, "")
	t.Setenv(azureURLEnv, "")

	descriptors := map[string]*repositoryDescriptor{
		providerNameGitHub: {
			Provider: providerNameGitHub,
			Owner:    "platform",
			Repo:     "yeet",
		},
		providerNameGitLab: {
			Provider: providerNameGitLab,
			Project:  "group/service",
		},
		providerNameAzureDevOps: {
			Provider:     providerNameAzureDevOps,
			Organization: "platform",
			Project:      "release-tools",
			Repo:         "yeet",
		},
	}

	constructedWith := map[string]*retryablehttp.Client{}

	// when: creating every provider through the factory
	for forge, descriptor := range descriptors {
		created, err := createProvider(descriptor, func(name string) *retryablehttp.Client {
			client := newTracedRetryableClient(name)
			constructedWith[name] = client

			return client
		})

		testastic.NoError(t, err)
		testastic.NotNil(t, created)
		testastic.MapHasKey(t, constructedWith, forge)
	}

	// then: every adapter receives the bounded shared client settings. Azure's
	// SDK creates its own transport and consumes only the timeout.
	testastic.Len(t, constructedWith, len(descriptors))
	defaults := config.Default().Network

	for forge, client := range constructedWith {
		testastic.Equal(t, defaults.Retry.MaxAttempts-1, client.RetryMax)
		testastic.Equal(t, defaults.Retry.MinBackoff, client.RetryWaitMin)
		testastic.Equal(t, defaults.Retry.MaxBackoff, client.RetryWaitMax)
		testastic.Equal(t, defaults.RequestTimeout, client.HTTPClient.Timeout)
		testastic.True(t, client.Logger == nil)
		testastic.True(t, client.RequestLogHook != nil)
		testastic.NotNil(t, client.HTTPClient.Transport)
		testastic.MapHasKey(t, forgeSpecs, forge)
	}
}

func TestTracedRetryableClientUsesConfiguredNetworkSettings(t *testing.T) {
	t.Parallel()

	network := config.NetworkConfig{
		RequestTimeout: 45 * time.Second,
		Retry: config.NetworkRetryConfig{
			MaxAttempts: 7,
			MinBackoff:  2 * time.Second,
			MaxBackoff:  20 * time.Second,
		},
	}

	client := newTracedRetryableClientWithConfig(providerNameGitHub, network)

	testastic.Equal(t, 6, client.RetryMax)
	testastic.Equal(t, 2*time.Second, client.RetryWaitMin)
	testastic.Equal(t, 20*time.Second, client.RetryWaitMax)
	testastic.Equal(t, 45*time.Second, client.HTTPClient.Timeout)
}

func TestTracedRetryableClientRetriesUntilSuccess(t *testing.T) {
	t.Parallel()

	// given: a server failing twice before succeeding
	server, attempts := countingServer(t, func(attempt int32) int {
		if attempt <= 2 {
			return http.StatusInternalServerError
		}

		return http.StatusOK
	})

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	testastic.NoError(t, err)

	// when: issuing one request through the shared client
	response, err := fastRetryClient(t).Do(request)
	testastic.NoError(t, err)

	defer func() {
		testastic.NoError(t, response.Body.Close())
	}()

	// then: the request survives both failures and the server saw three attempts
	testastic.Equal(t, http.StatusOK, response.StatusCode)
	testastic.Equal(t, int32(3), attempts.Load())
}

func TestTracedRetryableClientStopsAtRetryMax(t *testing.T) {
	t.Parallel()

	// given: a server that never recovers
	server, attempts := countingServer(t, func(int32) int {
		return http.StatusInternalServerError
	})

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	testastic.NoError(t, err)

	// when: issuing one request through the shared client
	response, err := fastRetryClient(t).Do(request)
	if response != nil {
		testastic.NoError(t, response.Body.Close())
	}

	// then: the client gives up after the configured total attempt count
	testastic.Error(t, err)
	testastic.Equal(t, int32(config.Default().Network.Retry.MaxAttempts), attempts.Load())
}

func TestTracedRetryableClientDoesNotRetryMutationServerErrors(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			// given: a mutation endpoint that returns an ambiguous server failure
			server, attempts := countingServer(t, func(int32) int {
				return http.StatusInternalServerError
			})

			request, err := http.NewRequestWithContext(context.Background(), method, server.URL, nil)
			testastic.NoError(t, err)

			// when: issuing the mutation through the shared client
			response, err := fastRetryClient(t).Do(request)
			if response != nil {
				testastic.NoError(t, response.Body.Close())
			}

			// then: the mutation is attempted once
			testastic.NoError(t, err)
			testastic.Equal(t, int32(1), attempts.Load())
		})
	}
}

func TestTracedRetryableClientRetriesRateLimitsForMutations(t *testing.T) {
	t.Parallel()

	// given: a mutation endpoint that rate limits once before succeeding
	server, attempts := countingServer(t, func(attempt int32) int {
		if attempt == 1 {
			return http.StatusTooManyRequests
		}

		return http.StatusOK
	})

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	testastic.NoError(t, err)

	// when: issuing the mutation through the shared client
	response, err := fastRetryClient(t).Do(request)
	testastic.NoError(t, err)

	defer func() {
		testastic.NoError(t, response.Body.Close())
	}()

	// then: the rejected request is retried within the configured bound
	testastic.Equal(t, http.StatusOK, response.StatusCode)
	testastic.Equal(t, int32(2), attempts.Load())
}

func TestMethodAwareRetryPolicyRetriesIdempotentTransportFailures(t *testing.T) {
	t.Parallel()

	for _, method := range []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPut,
		http.MethodDelete,
	} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			// given: an idempotent request fails after the connection is established
			err := &requestMethodError{method: method, err: io.EOF}

			// when: the shared retry policy classifies the transport failure
			retry, policyErr := methodAwareRetryPolicy(context.Background(), nil, err)

			// then: the request is retried because repeating it is safe
			testastic.NoError(t, policyErr)
			testastic.True(t, retry)
		})
	}
}

func TestTracedRetryableClientStopsWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	// given: a canceled request context and a server that would otherwise retry
	server, attempts := countingServer(t, func(int32) int {
		return http.StatusInternalServerError
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	testastic.NoError(t, err)

	// when: issuing the canceled request through the shared client
	response, err := fastRetryClient(t).Do(request)
	if response != nil {
		testastic.NoError(t, response.Body.Close())
	}

	// then: cancellation is returned without any retry attempt
	testastic.ErrorIs(t, err, context.Canceled)
	testastic.Equal(t, int32(0), attempts.Load())
	testastic.True(t, errors.Is(err, context.Canceled))
}

func TestGitLabMutationRequestsAreNotRetried(t *testing.T) {
	// given: a GitLab release endpoint returning an ambiguous server failure
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			attempts.Add(1)
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv(gitlabURLEnv, server.URL+"/api/v4")

	created, err := createProvider(
		&repositoryDescriptor{Provider: providerNameGitLab, Project: "group/project"},
		func(forge string) *retryablehttp.Client {
			client := newTracedRetryableClient(forge)
			client.RetryWaitMin = time.Millisecond
			client.RetryWaitMax = 5 * time.Millisecond

			return client
		},
	)
	testastic.NoError(t, err)

	// when: creating a release through the GitLab SDK
	_, err = created.CreateRelease(context.Background(), forge.ReleaseOptions{
		TagName: "v1.2.3",
		Ref:     "0123456789abcdef0123456789abcdef01234567",
		Name:    "v1.2.3",
	})

	// then: the POST is attempted once
	testastic.Error(t, err)
	testastic.Equal(t, int32(1), attempts.Load())
}
