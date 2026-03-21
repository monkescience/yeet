//nolint:testpackage // This test validates unexported release identity behavior.
package release

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestReleaseTagFromBranch(t *testing.T) {
	t.Parallel()

	t.Run("parses tag from release branch", func(t *testing.T) {
		t.Parallel()

		// given: a valid release branch name
		branch := "yeet/release-v1.2.3"

		// when: parsing tag from branch
		tag, err := releaseTagFromBranch(branch)

		// then: release tag is returned
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", tag)
	})

	t.Run("rejects invalid release branch", func(t *testing.T) {
		t.Parallel()

		// given: a non-release branch name
		branch := "feature/something"

		// when: parsing tag from branch
		_, err := releaseTagFromBranch(branch)

		// then: branch format error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseBranch)
	})
}
