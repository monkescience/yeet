package provider_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
)

func TestReleaseBranchName(t *testing.T) {
	t.Parallel()

	t.Run("uses the compatibility convention without provider settings", func(t *testing.T) {
		t.Parallel()

		// when: resolving a release branch for a directly constructed provider
		branch := provider.ReleaseBranchNameForTest("", "main")

		// then: the historical convention remains available to adapters and tests
		testastic.Equal(t, "yeet/release-main", branch)
	})

	t.Run("uses the configured branch exactly", func(t *testing.T) {
		t.Parallel()

		// when: resolving a configured release branch
		branch := provider.ReleaseBranchNameForTest("automation/main/release", "main")

		// then: no provider-specific naming is added
		testastic.Equal(t, "automation/main/release", branch)
		testastic.True(t, provider.IsExpectedReleaseBranchForTest(branch, "main", branch))
		testastic.False(t, provider.IsExpectedReleaseBranchForTest("yeet/release-main", "main", branch))
	})
}
