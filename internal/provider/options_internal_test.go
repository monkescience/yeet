package provider

import (
	"testing"

	"github.com/google/go-github/v88/github"
	"github.com/monkescience/testastic"
)

func newGitHubForTest(t *testing.T, opts ...Option) *GitHub {
	t.Helper()

	client, err := github.NewClient()
	testastic.NoError(t, err)

	return NewGitHub(client, "o", "r", opts...)
}

func TestProviderMaxConcurrentRequests(t *testing.T) {
	t.Parallel()

	t.Run("defaults when no option is given", func(t *testing.T) {
		t.Parallel()

		// given: no concurrency option
		// when: a provider is constructed
		gh := newGitHubForTest(t)

		// then: the default limit applies
		testastic.Equal(t, DefaultMaxConcurrentRequests, gh.maxConcurrentRequests)
	})

	t.Run("uses a positive override", func(t *testing.T) {
		t.Parallel()

		// given: a positive concurrency override
		// when: a provider is constructed with the option
		gh := newGitHubForTest(t, WithMaxConcurrentRequests(20))

		// then: the override replaces the default
		testastic.Equal(t, 20, gh.maxConcurrentRequests)
	})

	t.Run("ignores a non-positive override", func(t *testing.T) {
		t.Parallel()

		// given: a non-positive concurrency override
		// when: a provider is constructed with the option
		gh := newGitHubForTest(t, WithMaxConcurrentRequests(0))

		// then: the default is kept
		testastic.Equal(t, DefaultMaxConcurrentRequests, gh.maxConcurrentRequests)
	})

	t.Run("threads through the azure devops constructor", func(t *testing.T) {
		t.Parallel()

		// given: a positive concurrency override
		// when: an azure devops provider is constructed with the option
		azureDevOps := NewAzureDevOps(nil, "https://dev.azure.com", "pat", "org", "", "proj", "repo",
			WithMaxConcurrentRequests(12))

		// then: the override reaches the shared concurrency config
		testastic.Equal(t, 12, azureDevOps.maxConcurrentRequests)
	})
}
