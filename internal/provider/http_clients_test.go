package provider //nolint:testpackage // validates unexported HTTP transport policy directly

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/monkescience/testastic"
)

// fastRetryClient keeps the production attempt bound while shortening the waits
// so a real retry test finishes in milliseconds.
func fastRetryClient(t *testing.T, forge string) *http.Client {
	t.Helper()

	client := newTracedRetryableClient(forge)
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

func TestCreateProviderBuildsEveryForgeOnOneHTTPPolicy(t *testing.T) {
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

	// then: each forge is constructed with a client carrying the same retry and trace policy
	testastic.Len(t, constructedWith, len(descriptors))

	for forge, client := range constructedWith {
		testastic.Equal(t, httpRetryMax, client.RetryMax)
		testastic.Equal(t, httpRetryWaitMin, client.RetryWaitMin)
		testastic.Equal(t, httpRetryWaitMax, client.RetryWaitMax)
		testastic.Equal(t, httpClientTimeout, client.HTTPClient.Timeout)
		testastic.True(t, client.Logger == nil)
		testastic.True(t, client.RequestLogHook != nil)
		testastic.NotNil(t, client.HTTPClient.Transport)
		testastic.MapHasKey(t, forgeSpecs, forge)
	}
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
	response, err := fastRetryClient(t, providerNameGitHub).Do(request)
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
	response, err := fastRetryClient(t, providerNameGitHub).Do(request)
	if response != nil {
		testastic.NoError(t, response.Body.Close())
	}

	// then: the client gives up after the initial attempt plus httpRetryMax retries
	testastic.Error(t, err)
	testastic.Equal(t, int32(httpRetryMax+1), attempts.Load())
}
