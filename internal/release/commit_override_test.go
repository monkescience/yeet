//nolint:testpackage // This test validates unexported release behavior.
package release

import (
	"context"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

func TestCommitOverrideMessages(t *testing.T) {
	t.Parallel()

	knownTypes := knownCommitTypes(config.Default())

	t.Run("returns no override when markers are absent", func(t *testing.T) {
		t.Parallel()

		// when: parsing a body without override markers
		messages, ok, err := commitOverrideMessages(t.Context(), "plain pull request body", knownTypes)

		// then: no override is reported
		testastic.NoError(t, err)
		testastic.False(t, ok)
		testastic.Equal(t, 0, len(messages))
	})

	t.Run("extracts multiple conventional messages", func(t *testing.T) {
		t.Parallel()

		// given: a body with two conventional messages between override markers
		body := `Release notes:

BEGIN_COMMIT_OVERRIDE
feat(auth): add OAuth token refresh

fix(api): return 401 for expired sessions
END_COMMIT_OVERRIDE
`

		// when: parsing the override body
		messages, ok, err := commitOverrideMessages(t.Context(), body, knownTypes)

		// then: both messages are returned in order
		testastic.NoError(t, err)
		testastic.True(t, ok)
		testastic.SliceEqual(t, []string{
			"feat(auth): add OAuth token refresh",
			"fix(api): return 401 for expired sessions",
		}, messages)
	})

	t.Run("keeps body and footer lines with an override message", func(t *testing.T) {
		t.Parallel()

		// given: an override block with subject, body, and footer
		body := `BEGIN_COMMIT_OVERRIDE
feat(auth)!: replace session cookie format

Session cookies now use a keyed format.

BREAKING CHANGE: existing session cookies are invalid after upgrade
END_COMMIT_OVERRIDE`

		// when: parsing the override body
		messages, ok, err := commitOverrideMessages(t.Context(), body, knownTypes)

		// then: the message keeps its subject prefix and the body and footer are preserved
		testastic.NoError(t, err)
		testastic.True(t, ok)
		testastic.Equal(t, 1, len(messages))

		expectedPrefix := "feat(auth)!: replace session cookie format"
		testastic.Equal(t, expectedPrefix, messages[0][:len(expectedPrefix)])
	})

	t.Run("keeps issue-reference footers with their commit message", func(t *testing.T) {
		t.Parallel()

		// given: a single override commit whose body ends with a git-trailer footer
		body := `BEGIN_COMMIT_OVERRIDE
feat(api): add pagination

Adds cursor-based pagination to the list endpoint.

Closes: #45
END_COMMIT_OVERRIDE`

		// when: parsing the override body
		messages, ok, err := commitOverrideMessages(t.Context(), body, knownTypes)

		// then: the footer stays attached instead of becoming a second commit
		testastic.NoError(t, err)
		testastic.True(t, ok)
		testastic.Equal(t, 1, len(messages))
		testastic.Contains(t, messages[0], "Closes: #45")
	})

	t.Run("rejects missing end marker", func(t *testing.T) {
		t.Parallel()

		// when: parsing an override body without END_COMMIT_OVERRIDE
		_, _, err := commitOverrideMessages(t.Context(), "BEGIN_COMMIT_OVERRIDE\nfix: patch bug", knownTypes)

		// then: the missing end marker is reported as an invalid override
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidCommitOverride)
	})
}

func TestReleaseCommitOverrides(t *testing.T) {
	t.Parallel()

	t.Run("override changes bump and changelog", func(t *testing.T) {
		t.Parallel()

		// given: a vague patch commit associated with a PR body override containing a feature and fix
		cfg := config.Default()
		cfg.PreMajorFeaturesBumpPatch = false

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: auth stuff",
		}}
		stub.commitOverrideBodies = map[string]string{
			"abcdef1234567890": `BEGIN_COMMIT_OVERRIDE
feat(auth): add OAuth token refresh

fix(api): return 401 for expired sessions
END_COMMIT_OVERRIDE`,
		}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: the override controls both version bump and changelog entries
		testastic.NoError(t, err)
		testastic.Equal(t, "1.3.0", result.Plans[0].NextVersion)
		testastic.Contains(t, result.Plans[0].Changelog, "### Features")
		testastic.Contains(t, result.Plans[0].Changelog, "- **auth:** add OAuth token refresh")
		testastic.Contains(t, result.Plans[0].Changelog, "### Bug Fixes")
		testastic.Contains(t, result.Plans[0].Changelog, "- **api:** return 401 for expired sessions")
		testastic.NotContains(t, result.Plans[0].Changelog, "auth stuff")
	})

	t.Run("override can introduce breaking change", func(t *testing.T) {
		t.Parallel()

		// given: a non-breaking commit whose PR body declares a breaking override
		cfg := config.Default()
		cfg.PreMajorBreakingBumpsMinor = false

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: auth stuff",
		}}
		stub.commitOverrideBodies = map[string]string{
			"abcdef1234567890": `BEGIN_COMMIT_OVERRIDE
feat(auth)!: replace session cookie format

BREAKING CHANGE: existing session cookies are invalid after upgrade
END_COMMIT_OVERRIDE`,
		}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: the breaking override controls the bump and breaking section
		testastic.NoError(t, err)
		testastic.Equal(t, "2.0.0", result.Plans[0].NextVersion)
		testastic.Contains(t, result.Plans[0].Changelog, "### ⚠ BREAKING CHANGES")
		testastic.Contains(t, result.Plans[0].Changelog, "existing session cookies are invalid after upgrade")
	})
}
