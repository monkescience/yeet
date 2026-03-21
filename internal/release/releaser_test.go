//nolint:testpackage // This test validates unexported release behavior.
package release

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

func TestReleasePreviewBuildMetadata(t *testing.T) {
	t.Parallel()

	t.Run("semver appends short hash as build metadata", func(t *testing.T) {
		t.Parallel()

		// given: semver release with one patch commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: calculating a preview release
		result, err := r.Release(context.Background(), true, true, DefaultPreviewHashLength)

		// then: metadata suffix is appended and base version stays stable
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.4", result.BaseVersion)
		testastic.Equal(t, "1.2.4+abcdef1", result.NextVersion)
		testastic.Equal(t, "v1.2.4", result.BaseTag)
		testastic.Equal(t, "v1.2.4+abcdef1", result.NextTag)
	})

	t.Run("calver appends short hash as build metadata", func(t *testing.T) {
		t.Parallel()

		// given: calver release with one patch commit
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer

		stub := newProviderStub()
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: calculating a preview release
		result, err := r.Release(context.Background(), true, true, DefaultPreviewHashLength)

		// then: calver version also gets build metadata suffix
		testastic.NoError(t, err)
		testastic.HasPrefix(t, result.NextVersion, result.BaseVersion+"+")
		testastic.HasSuffix(t, result.NextVersion, "+abcdef1")
		testastic.Equal(t, "v"+result.BaseVersion, result.BaseTag)
		testastic.Equal(t, "v"+result.NextVersion, result.NextTag)
	})

	t.Run("preview hash length must be positive", func(t *testing.T) {
		t.Parallel()

		// given: semver release with one patch commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: preview hash length is invalid
		_, err := r.Release(context.Background(), true, true, 0)

		// then: validation error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidPreviewHashLength)
	})
}

func TestReleaseSemVerPreMajorBumps(t *testing.T) {
	t.Parallel()

	t.Run("breaking changes do not jump to 1.0.0", func(t *testing.T) {
		t.Parallel()

		// given: a pre-1.0.0 semver release with one breaking commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat(api)!: remove deprecated endpoint",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: version bumps to next minor instead of 1.0.0
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.CurrentVersion)
		testastic.Equal(t, "0.5.0", result.NextVersion)
		testastic.Equal(t, "v0.5.0", result.NextTag)
	})

	t.Run("feature commits bump patch before 1.0.0", func(t *testing.T) {
		t.Parallel()

		// given: a pre-1.0.0 semver release with one feature commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add export command",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: version bumps patch instead of minor
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.CurrentVersion)
		testastic.Equal(t, "0.4.3", result.NextVersion)
		testastic.Equal(t, "v0.4.3", result.NextTag)
	})
}

func TestReleaseUsesLatestVersionRef(t *testing.T) {
	t.Parallel()

	// given: a repository with tags but no provider release objects
	cfg := config.Default()

	stub := newProviderStub()
	stub.latestVersionRef = "v1.2.3"
	stub.commitsByRef = map[string][]provider.CommitEntry{
		"v1.2.3": {{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}},
	}

	r := New(cfg, stub)

	// when: calculating a release
	result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

	// then: the latest version tag is used as the baseline and commit boundary
	testastic.NoError(t, err)
	testastic.Equal(t, "1.2.3", result.CurrentVersion)
	testastic.Equal(t, "1.2.4", result.NextVersion)
	testastic.Equal(t, 2, len(stub.getCommitsSinceOf))
	testastic.Equal(t, "v1.2.3", stub.getCommitsSinceOf[0])
	testastic.Equal(t, "v1.2.3", stub.getCommitsSinceOf[1])
}

func TestReleaseFallsBackToReachableTagWhenPreferredRefIsOffBranch(t *testing.T) {
	t.Parallel()

	// given: a preferred release ref that is not reachable from the configured branch
	cfg := config.Default()

	stub := newProviderStub()
	stub.latestVersionRef = "v2.0.0"
	stub.tagList = []string{"v1.2.3", "v2.0.0"}
	stub.commitsErrByRef["v2.0.0"] = &provider.CommitBoundaryNotFoundError{Ref: "v2.0.0", Branch: cfg.Branch}
	stub.commitsByRef = map[string][]provider.CommitEntry{
		"v1.2.3": {{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}},
	}

	r := New(cfg, stub)

	// when: calculating a release
	result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

	// then: the latest reachable stable tag on the branch is used instead
	testastic.NoError(t, err)
	testastic.Equal(t, "1.2.3", result.CurrentVersion)
	testastic.Equal(t, "1.2.4", result.NextVersion)
	testastic.Equal(t, 3, len(stub.getCommitsSinceOf))
	testastic.Equal(t, "v2.0.0", stub.getCommitsSinceOf[0])
	testastic.Equal(t, "v1.2.3", stub.getCommitsSinceOf[1])
	testastic.Equal(t, "v1.2.3", stub.getCommitsSinceOf[2])
}

func TestReleasePrefersNewerReachableTagOverOlderPublishedRelease(t *testing.T) {
	t.Parallel()

	// given: the latest published release is older than a newer stable tag on the release branch
	cfg := config.Default()

	stub := newProviderStub()
	stub.latestVersionRef = "v1.2.3"
	stub.tagList = []string{"v1.2.4", "v1.2.3"}
	stub.commitsByRef = map[string][]provider.CommitEntry{
		"v1.2.4": {{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}},
	}

	r := New(cfg, stub)

	// when: calculating a release
	result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

	// then: the newer reachable tag becomes the baseline even without a matching release object
	testastic.NoError(t, err)
	testastic.Equal(t, "1.2.4", result.CurrentVersion)
	testastic.Equal(t, "1.2.5", result.NextVersion)
	testastic.Equal(t, 2, len(stub.getCommitsSinceOf))
	testastic.Equal(t, "v1.2.4", stub.getCommitsSinceOf[0])
	testastic.Equal(t, "v1.2.4", stub.getCommitsSinceOf[1])
}

func TestReleaseChoosesHighestStableTagFromFallbackList(t *testing.T) {
	t.Parallel()

	// given: no published release and an unsorted provider tag list
	cfg := config.Default()

	stub := newProviderStub()
	stub.tagList = []string{"v1.2.3", "v1.10.0", "preview-build", "v1.9.9"}
	stub.commitsByRef = map[string][]provider.CommitEntry{
		"v1.10.0": {{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}},
	}

	r := New(cfg, stub)

	// when: calculating a release
	result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

	// then: the highest stable semver tag is used instead of trusting provider order
	testastic.NoError(t, err)
	testastic.Equal(t, "1.10.0", result.CurrentVersion)
	testastic.Equal(t, "1.10.1", result.NextVersion)
	testastic.Equal(t, 2, len(stub.getCommitsSinceOf))
	testastic.Equal(t, "v1.10.0", stub.getCommitsSinceOf[0])
	testastic.Equal(t, "v1.10.0", stub.getCommitsSinceOf[1])
}

func TestReleaseAsFooter(t *testing.T) {
	t.Parallel()

	t.Run("forces explicit version without releasable commit", func(t *testing.T) {
		t.Parallel()

		// given: a semver release with only a chore commit and Release-As footer
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: trigger stable release\n\nRelease-As: 1.0.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: explicit version override is used
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.CurrentVersion)
		testastic.Equal(t, "1.0.0", result.NextVersion)
		testastic.Equal(t, "v1.0.0", result.NextTag)
		testastic.Equal(t, commit.BumpMajor, result.BumpType)
	})

	t.Run("supports arbitrary semver override", func(t *testing.T) {
		t.Parallel()

		// given: a semver release with Release-As footer for minor update
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch issue\n\nRelease-As: 1.4.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: exact semver override is used
		testastic.NoError(t, err)
		testastic.Equal(t, "1.4.0", result.NextVersion)
		testastic.Equal(t, "v1.4.0", result.NextTag)
		testastic.Equal(t, commit.BumpMinor, result.BumpType)
	})

	t.Run("footer key matching is case-insensitive", func(t *testing.T) {
		t.Parallel()

		// given: a semver release with lowercase release-as footer key
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nrelease-as: 1.3.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: footer key is recognized regardless of casing
		testastic.NoError(t, err)
		testastic.Equal(t, "1.3.0", result.NextVersion)
		testastic.Equal(t, "v1.3.0", result.NextTag)
	})

	t.Run("rejects non-strict override value", func(t *testing.T) {
		t.Parallel()

		// given: a commit with semver missing patch segment
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: 1.3",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: non-strict semver values are rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("rejects v-prefixed override value", func(t *testing.T) {
		t.Parallel()

		// given: a commit with v-prefixed release-as value
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: v1.3.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: values must be strict semver without v-prefix
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("fails on conflicting override values", func(t *testing.T) {
		t.Parallel()

		// given: two commits with different Release-As values
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{
			{
				Hash:    "abcdef1234567890",
				Message: "chore: request release\n\nRelease-As: 1.0.0",
			},
			{
				Hash:    "1234567890abcdef",
				Message: "chore: request different release\n\nRelease-As: 1.1.0",
			},
		}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: conflict is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrConflictingReleaseAs)
	})

	t.Run("fails on invalid override value", func(t *testing.T) {
		t.Parallel()

		// given: a commit with malformed Release-As value
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: not-a-version",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: invalid value is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("fails when override is not greater than current version", func(t *testing.T) {
		t.Parallel()

		// given: a commit requesting the same version as current release
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: 1.2.3",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: non-incrementing override is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("ignores override for calver", func(t *testing.T) {
		t.Parallel()

		// given: a calver repo with only a Release-As chore commit
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer

		stub := newProviderStub()
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: 1.0.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: release-as footer is ignored for calver
		testastic.NoError(t, err)
		testastic.Equal(t, commit.BumpNone, result.BumpType)
		testastic.Equal(t, "", result.NextVersion)
		testastic.Equal(t, "", result.NextTag)
	})
}

func TestReleasePreviewUsesStableBranch(t *testing.T) {
	t.Parallel()

	// given: semver release flow with preview enabled
	cfg := config.Default()

	stub := newProviderStub()
	stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
	stub.commits = []provider.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "fix: patch bug",
	}}

	r := New(cfg, stub)

	// when: creating the first release PR
	first, err := r.Release(context.Background(), false, true, DefaultPreviewHashLength)

	// then: release branch is stable and based on the target branch
	testastic.NoError(t, err)
	testastic.Equal(t, 1, stub.createPRCalls)
	testastic.Equal(t, 0, stub.updatePRCalls)
	testastic.Equal(t, 1, len(stub.createdBranches))
	testastic.Equal(t, "yeet/release-main", stub.createdBranches[0])

	// given: a new head commit changes preview hash
	stub.commits = []provider.CommitEntry{{
		Hash:    "1234567890abcdef",
		Message: "fix: patch bug",
	}}

	// when: running release again
	second, err := r.Release(context.Background(), false, true, DefaultPreviewHashLength)

	// then: same release branch/PR is reused
	testastic.NoError(t, err)
	testastic.Equal(t, 1, stub.createPRCalls)
	testastic.Equal(t, 1, stub.updatePRCalls)
	testastic.Equal(t, 2, len(stub.markPendingCalls))
	testastic.Equal(t, 1, len(stub.createdBranches))
	testastic.Equal(t, first.BaseTag, second.BaseTag)
	testastic.NotEqual(t, first.NextTag, second.NextTag)
}

func TestReleaseAfterFinalizeMergedRelease(t *testing.T) {
	t.Parallel()

	const changelogBody = `# Changelog

## [v0.1.0](https://example.com/compare/v0.0.9...v0.1.0) (2026-03-01)

### Features

- add release flow (abc1234)
`

	t.Run("does not create PR when no commits exist after finalized tag", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR with no commits after its tag
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number: 3,
			URL:    "https://example.com/pr/3",
			Body:   "<!-- yeet-release-tag: v0.1.0 -->",
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]provider.CommitEntry{
			"v0.1.0": {},
		}

		r := New(cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: merged release is finalized and no new release PR is created
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.Release)(nil), result.Release)
		testastic.Equal(t, "v0.1.0", result.Release.TagName)
		testastic.Equal(t, commit.BumpNone, result.BumpType)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 2, len(stub.getCommitsSinceOf))
		testastic.Equal(t, "v0.1.0", stub.getCommitsSinceOf[0])
		testastic.Equal(t, "v0.1.0", stub.getCommitsSinceOf[1])
	})

	t.Run("creates PR when commits exist after finalized tag", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR and new commits after its tag
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number: 4,
			URL:    "https://example.com/pr/4",
			Body:   "<!-- yeet-release-tag: v0.1.0 -->",
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]provider.CommitEntry{
			"v0.1.0": {{Hash: "abcdef1234567890", Message: "fix: patch after release"}},
		}

		r := New(cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: merged release is finalized and a new release PR is created for fresh commits
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.Release)(nil), result.Release)
		testastic.Equal(t, "v0.1.0", result.Release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, stub.createPRCalls)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)
		testastic.Equal(t, 2, len(stub.getCommitsSinceOf))
		testastic.Equal(t, "v0.1.0", stub.getCommitsSinceOf[0])
		testastic.Equal(t, "v0.1.0", stub.getCommitsSinceOf[1])
	})
}

func TestReleaseFailsWhenPreviousReleaseIsNotReachableFromBranch(t *testing.T) {
	t.Parallel()

	// given: the latest release ref exists but is not on the configured release branch
	cfg := config.Default()

	stub := newProviderStub()
	stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
	stub.commitsErr = &provider.CommitBoundaryNotFoundError{Ref: "v1.2.3", Branch: cfg.Branch}

	r := New(cfg, stub)

	// when: running release end-to-end
	result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

	// then: release stops before creating a PR, tag, or release
	testastic.Error(t, err)
	testastic.Equal(t, (*Result)(nil), result)
	testastic.ErrorIs(t, err, provider.ErrCommitBoundaryNotFound)
	testastic.ErrorContains(t, err, "v1.2.3")
	testastic.ErrorContains(t, err, cfg.Branch)
	testastic.ErrorContains(t, err, "verify the latest tag/release and branch ancestry")
	testastic.Equal(t, 0, stub.createPRCalls)
	testastic.Equal(t, 0, stub.createReleaseCalls)
	testastic.Equal(t, 0, len(stub.markPendingCalls))
	testastic.Equal(t, 1, len(stub.getCommitsSinceOf))
	testastic.Equal(t, "v1.2.3", stub.getCommitsSinceOf[0])
	testastic.Equal(t, 1, len(stub.getCommitsSinceBranches))
	testastic.Equal(t, cfg.Branch, stub.getCommitsSinceBranches[0])
}

func TestReleaseAutoMerge(t *testing.T) {
	t.Parallel()

	t.Run("merges release PR and finalizes release in same run", func(t *testing.T) {
		t.Parallel()

		// given: auto-merge enabled with one releasable commit
		cfg := config.Default()
		cfg.Release.AutoMerge = true

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: release PR is merged, tagged, and release is created immediately
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)
		testastic.NotEqual(t, (*provider.Release)(nil), result.Release)
		testastic.Equal(t, result.NextTag, result.Release.TagName)
		testastic.Equal(t, 1, stub.createPRCalls)
		testastic.Equal(t, 1, stub.mergePRCalls)
		testastic.Equal(t, 1, len(stub.mergePRNumbers))
		testastic.Equal(t, result.PullRequest.Number, stub.mergePRNumbers[0])
		testastic.Equal(t, 1, len(stub.mergePROptions))
		testastic.False(t, stub.mergePROptions[0].Force)
		testastic.Equal(t, provider.MergeMethodAuto, stub.mergePROptions[0].Method)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markPendingCalls))
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, result.PullRequest.Number, stub.markTaggedCalls[0])
	})

	t.Run("force mode forwards force option to provider merge", func(t *testing.T) {
		t.Parallel()

		// given: force auto-merge enabled
		cfg := config.Default()
		cfg.Release.AutoMergeForce = true

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: merge is attempted in force mode and release is finalized
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)
		testastic.NotEqual(t, (*provider.Release)(nil), result.Release)
		testastic.Equal(t, 1, stub.mergePRCalls)
		testastic.Equal(t, 1, len(stub.mergePROptions))
		testastic.True(t, stub.mergePROptions[0].Force)
		testastic.Equal(t, provider.MergeMethodAuto, stub.mergePROptions[0].Method)
	})

	t.Run("passes configured merge method to provider", func(t *testing.T) {
		t.Parallel()

		// given: auto-merge enabled with explicit merge method
		cfg := config.Default()
		cfg.Release.AutoMerge = true
		cfg.Release.AutoMergeMethod = config.AutoMergeMethodSquash

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: running release end-to-end
		_, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: configured merge method is forwarded to provider
		testastic.NoError(t, err)
		testastic.Equal(t, 1, stub.mergePRCalls)
		testastic.Equal(t, 1, len(stub.mergePROptions))
		testastic.Equal(t, provider.MergeMethodSquash, stub.mergePROptions[0].Method)
	})

	t.Run("returns error when auto-merge is blocked", func(t *testing.T) {
		t.Parallel()

		// given: auto-merge enabled but provider refuses merge
		cfg := config.Default()
		cfg.Release.AutoMerge = true

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}
		stub.mergePRErr = fmt.Errorf("%w: required checks pending", provider.ErrMergeBlocked)

		r := New(cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: release fails after PR creation and no tag/release is created
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
		testastic.Equal(t, (*Result)(nil), result)
		testastic.Equal(t, 1, stub.createPRCalls)
		testastic.Equal(t, 1, stub.mergePRCalls)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markPendingCalls))
		testastic.Equal(t, 0, len(stub.markTaggedCalls))
	})

	t.Run("preview mode skips auto-merge", func(t *testing.T) {
		t.Parallel()

		// given: auto-merge enabled during preview release
		cfg := config.Default()
		cfg.Release.AutoMerge = true

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: running preview release
		result, err := r.Release(context.Background(), false, true, DefaultPreviewHashLength)

		// then: preview PR is created but no merge/tagging happens
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)
		testastic.Equal(t, (*provider.Release)(nil), result.Release)
		testastic.Equal(t, 1, stub.createPRCalls)
		testastic.Equal(t, 0, stub.mergePRCalls)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markPendingCalls))
		testastic.Equal(t, 0, len(stub.markTaggedCalls))
	})
}

func TestReleaseReusesSinglePendingPR(t *testing.T) {
	t.Parallel()

	// given: one open pending PR on a legacy release branch
	cfg := config.Default()

	stub := newProviderStub()
	stub.openPending = []*provider.PullRequest{{
		Number: 7,
		URL:    "https://example.com/pr/7",
		Branch: "yeet/release-v0.0.1",
	}}
	stub.commits = []provider.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "feat!: introduce breaking release flow",
	}}

	r := New(cfg, stub)

	// when: computing a new version while a pending PR already exists
	result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

	// then: pending PR is updated instead of creating a second release PR
	testastic.NoError(t, err)
	testastic.Equal(t, "0.1.0", result.BaseVersion)
	testastic.Equal(t, 0, stub.createPRCalls)
	testastic.Equal(t, 1, stub.updatePRCalls)
	testastic.Equal(t, 1, len(stub.markPendingCalls))
	testastic.Equal(t, "yeet/release-v0.0.1", result.PullRequest.Branch)
	testastic.Contains(t, result.PullRequest.Body, "<!-- yeet-release-tag: v0.1.0 -->")
}

func TestReleaseFailsOnMultiplePendingPRs(t *testing.T) {
	t.Parallel()

	// given: more than one open pending release PR
	cfg := config.Default()

	stub := newProviderStub()
	stub.openPending = []*provider.PullRequest{
		{Number: 1, URL: "https://example.com/pr/1", Branch: "yeet/release-v0.0.1"},
		{Number: 2, URL: "https://example.com/pr/2", Branch: "yeet/release-v0.1.0"},
	}
	stub.commits = []provider.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "fix: patch bug",
	}}

	r := New(cfg, stub)

	// when: attempting to create or update release PRs
	_, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

	// then: release fails fast with actionable pending PR details
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, ErrMultiplePendingReleasePRs)
	testastic.ErrorContains(t, err, "https://example.com/pr/1")
	testastic.ErrorContains(t, err, "https://example.com/pr/2")
	testastic.Equal(t, 0, stub.createPRCalls)
	testastic.Equal(t, 0, stub.updatePRCalls)
}

func TestReleaseSubjectFormatting(t *testing.T) {
	t.Parallel()

	t.Run("default subject omits branch and tag prefix", func(t *testing.T) {
		t.Parallel()

		// given: default config and one releasable commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: PR title and commit subject use unscoped release subject
		testastic.NoError(t, err)
		testastic.Equal(t, "chore: release "+result.BaseVersion, result.PullRequest.Title)
		testastic.Equal(t, 1, stub.updateFilesCalls)
		testastic.Equal(t, "chore: release "+result.BaseVersion, stub.updateFilesMessages[0])
		testastic.Equal(t, 1, len(stub.markPendingCalls))
		testastic.HasPrefix(t, result.PullRequest.Body, "## ٩(^ᴗ^)۶ release created\n\n")
		testastic.Contains(t, result.PullRequest.Body, "<!-- yeet-release-tag: "+result.BaseTag+" -->")
		testastic.HasSuffix(
			t,
			strings.TrimSpace(result.PullRequest.Body),
			"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
		)
		testastic.NotContains(t, result.Changelog, "_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._")

		releaseBranch := "yeet/release-main"
		updatedChangelog := stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)]
		testastic.Equal(t, prependChangelogEntry("", result.Changelog), updatedChangelog)
	})

	t.Run("optional branch scope uses stable base version in preview", func(t *testing.T) {
		t.Parallel()

		// given: branch scope enabled and preview release
		cfg := config.Default()
		cfg.Release.SubjectIncludeBranch = true

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: creating a preview release PR
		result, err := r.Release(context.Background(), false, true, DefaultPreviewHashLength)

		// then: PR title and commit subject include branch and stable base version
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.4", result.BaseVersion)
		testastic.Equal(t, "1.2.4+abcdef1", result.NextVersion)
		testastic.Equal(t, "chore(main): release 1.2.4", result.PullRequest.Title)
		testastic.Equal(t, 1, stub.updateFilesCalls)
		testastic.Equal(t, "chore(main): release 1.2.4", stub.updateFilesMessages[0])
		testastic.Contains(t, result.PullRequest.Body, "<!-- yeet-release-tag: v1.2.4 -->")
	})

	t.Run("custom header and footer wrap PR body only", func(t *testing.T) {
		t.Parallel()

		// given: custom PR body header/footer and one releasable commit
		cfg := config.Default()
		cfg.Release.PRBodyHeader = "## Release checklist\n- [ ] smoke test"
		cfg.Release.PRBodyFooter = "Please review"

		stub := newProviderStub()
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: PR body includes custom wrapper text while changelog content stays clean
		testastic.NoError(t, err)
		testastic.HasPrefix(t, result.PullRequest.Body, cfg.Release.PRBodyHeader+"\n\n")
		testastic.HasSuffix(t, strings.TrimSpace(result.PullRequest.Body), cfg.Release.PRBodyFooter)
		testastic.NotContains(
			t,
			result.PullRequest.Body,
			"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
		)
		testastic.NotContains(t, result.Changelog, cfg.Release.PRBodyHeader)
		testastic.NotContains(t, result.Changelog, cfg.Release.PRBodyFooter)
	})
}

func TestReleasePRBodyCompareURLUsesHeadCommit(t *testing.T) {
	t.Parallel()

	t.Run("github compare link uses latest commit sha in PR body", func(t *testing.T) {
		t.Parallel()

		// given: GitHub repo with existing release and one new commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.repoURL = "https://github.example.com/owner/repo"
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}

		const headSHA = "abcdef1234567890abcdef1234567890abcdef12"

		stub.commits = []provider.CommitEntry{{
			Hash:    headSHA,
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: changelog keeps tag-to-tag compare while PR body links tag-to-head sha
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)

		canonicalCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v1.2.3", "v1.2.4")
		prCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v1.2.3", headSHA)

		testastic.Contains(t, result.Changelog, canonicalCompareURL)
		testastic.NotContains(t, result.Changelog, prCompareURL)
		testastic.Contains(t, result.PullRequest.Body, prCompareURL)
		testastic.NotContains(t, result.PullRequest.Body, canonicalCompareURL)

		releaseBranch := "yeet/release-main"
		updatedChangelog := stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)]
		testastic.Equal(t, prependChangelogEntry("", result.Changelog), updatedChangelog)
	})

	t.Run("gitlab compare link uses latest commit sha in PR body", func(t *testing.T) {
		t.Parallel()

		// given: GitLab repo with existing release and one new commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.repoURL = "https://gitlab.example.com/group/repo"
		stub.pathPrefix = "/-"
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}

		const headSHA = "1234567890abcdef1234567890abcdef12345678"

		stub.commits = []provider.CommitEntry{{
			Hash:    headSHA,
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: changelog keeps tag-to-tag compare while PR body links tag-to-head sha
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)

		canonicalCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v1.2.3", "v1.2.4")
		prCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v1.2.3", headSHA)

		testastic.Contains(t, result.Changelog, canonicalCompareURL)
		testastic.NotContains(t, result.Changelog, prCompareURL)
		testastic.Contains(t, result.PullRequest.Body, prCompareURL)
		testastic.NotContains(t, result.PullRequest.Body, canonicalCompareURL)

		releaseBranch := "yeet/release-main"
		updatedChangelog := stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)]
		testastic.Equal(t, prependChangelogEntry("", result.Changelog), updatedChangelog)
	})
}

func TestFinalizeMergedReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("creates release from latest changelog entry and marks PR tagged", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR and changelog entry on main
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 42,
			URL:    "https://example.com/pr/42",
			Body:   "<!-- yeet-release-tag: v1.2.3 -->",
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)

## [v1.2.2](https://example.com/compare/v1.2.1...v1.2.2) (2026-02-20)

### Bug Fixes

- fix bug (def5678)
`)

		r := New(cfg, stub)

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: release is created from matching changelog entry and PR is marked tagged
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.Equal(t, cfg.Branch, stub.createReleaseOpts[0].Ref)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 42, stub.markTaggedCalls[0])
		testastic.Contains(t, release.Body, "## [v1.2.3]")
		testastic.NotContains(t, release.Body, "## [v1.2.2]")
	})

	t.Run("falls back to legacy release branch tag without marker", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR from legacy versioned branch naming
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 33,
			URL:    "https://example.com/pr/33",
			Branch: "yeet/release-v1.2.3",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)
`)

		r := New(cfg, stub)

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: fallback branch tag is used
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 33, stub.markTaggedCalls[0])
	})

	t.Run("skips creation when latest release already exists", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR already tagged in provider releases
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3", URL: "https://example.com/releases/v1.2.3"}
		stub.mergedPR = &provider.PullRequest{
			Number: 9,
			URL:    "https://example.com/pr/9",
			Body:   "<!-- yeet-release-tag: v1.2.3 -->",
			Branch: "yeet/release-main",
		}

		r := New(cfg, stub)

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: existing release is reused and PR is still marked tagged
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 9, stub.markTaggedCalls[0])
	})

	t.Run("reuses exact release for non-latest tag", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR for an older tag that already has a release
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.4", URL: "https://example.com/releases/v1.2.4"}
		stub.releasesByTag["v1.2.3"] = &provider.Release{
			TagName: "v1.2.3",
			URL:     "https://example.com/releases/v1.2.3",
		}
		stub.mergedPR = &provider.PullRequest{
			Number: 10,
			URL:    "https://example.com/pr/10",
			Body:   "<!-- yeet-release-tag: v1.2.3 -->",
			Branch: "yeet/release-main",
		}

		r := New(cfg, stub)

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: the exact existing release is reused instead of checking only the latest release
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, "https://example.com/releases/v1.2.3", release.URL)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 10, stub.markTaggedCalls[0])
	})

	t.Run("creates missing release when tag already exists", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR whose tag already exists without a release object
		cfg := config.Default()

		stub := newProviderStub()
		stub.tags["v1.2.3"] = true
		stub.mergedPR = &provider.PullRequest{
			Number: 11,
			URL:    "https://example.com/pr/11",
			Body:   "<!-- yeet-release-tag: v1.2.3 -->",
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)
`)

		r := New(cfg, stub)

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: only the missing release object is created and no branch ref is forced
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.Equal(t, "", stub.createReleaseOpts[0].Ref)
	})

	t.Run("creates missing tag from merged commit ref", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR with a known merged commit SHA and no existing tag
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number:         13,
			URL:            "https://example.com/pr/13",
			Body:           "<!-- yeet-release-tag: v1.2.3 -->",
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)
`)

		r := New(cfg, stub)

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: tag creation uses the merged commit ref instead of the current branch head
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.Equal(t, "merged-sha", stub.createReleaseOpts[0].Ref)
	})

	t.Run("returns no-pr error when no merged pending release PR exists", func(t *testing.T) {
		t.Parallel()

		// given: no merged pending release PR
		r := New(config.Default(), newProviderStub())

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: nothing is finalized
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrNoPR)
		testastic.Equal(t, (*provider.Release)(nil), release)
	})

	t.Run("fails when stable branch PR has no release marker", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR on stable branch without marker
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 25,
			URL:    "https://example.com/pr/25",
			Branch: "yeet/release-main",
		}

		r := New(cfg, stub)

		// when: finalizing merged release PR
		_, err := r.finalizeMergedReleasePR(context.Background())

		// then: marker requirement is enforced for stable branch naming
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseBranch)
		testastic.Equal(t, 0, stub.createReleaseCalls)
	})

	t.Run("fails when matching changelog entry is missing", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR but changelog lacks target tag entry
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 12,
			URL:    "https://example.com/pr/12",
			Branch: "yeet/release-v1.2.3",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = "# Changelog\n\n## v1.2.2 (2026-02-20)"

		r := New(cfg, stub)

		// when: finalizing merged release PR
		_, err := r.finalizeMergedReleasePR(context.Background())

		// then: missing entry is reported
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrChangelogEntryNotFound)
	})
}

func TestTagRejectsPreviewTags(t *testing.T) {
	t.Parallel()

	t.Run("rejects build metadata tags", func(t *testing.T) {
		t.Parallel()

		// given: a releaser
		cfg := config.Default()
		stub := newProviderStub()
		r := New(cfg, stub)

		// when: trying to create a preview tag
		_, err := r.Tag(context.Background(), "v1.2.3+abc1234", "")

		// then: tag creation is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrPreviewTagNotAllowed)
		testastic.Equal(t, 0, stub.createReleaseCalls)
	})

	t.Run("rejects prerelease tags", func(t *testing.T) {
		t.Parallel()

		// given: a releaser
		cfg := config.Default()
		stub := newProviderStub()
		r := New(cfg, stub)

		// when: trying to create a prerelease tag
		_, err := r.Tag(context.Background(), "v1.2.3-rc.1", "")

		// then: tag creation is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrPreviewTagNotAllowed)
		testastic.Equal(t, 0, stub.createReleaseCalls)
	})

	t.Run("allows stable tags with hyphenated prefix", func(t *testing.T) {
		t.Parallel()

		// given: a releaser with a hyphenated tag prefix
		cfg := config.Default()
		cfg.TagPrefix = "release-"

		stub := newProviderStub()
		r := New(cfg, stub)

		// when: creating a stable tag
		_, err := r.Tag(context.Background(), "release-1.2.3", "")

		// then: tag is accepted
		testastic.NoError(t, err)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.Equal(t, cfg.Branch, stub.createReleaseOpts[0].Ref)
	})
}

func TestTagReusesExistingRelease(t *testing.T) {
	t.Parallel()

	// given: a stable tag that already has a release object
	cfg := config.Default()

	stub := newProviderStub()
	stub.latestRelease = &provider.Release{TagName: "v1.2.4"}
	stub.releasesByTag["v1.2.3"] = &provider.Release{
		TagName: "v1.2.3",
		URL:     "https://example.com/releases/v1.2.3",
	}

	r := New(cfg, stub)

	// when: creating the same stable tag again
	result, err := r.Tag(context.Background(), "v1.2.3", "release notes")

	// then: the existing release is reused without another create call
	testastic.NoError(t, err)
	testastic.NotEqual(t, (*Result)(nil), result)
	testastic.Equal(t, "v1.2.3", result.Release.TagName)
	testastic.Equal(t, 0, stub.createReleaseCalls)
}

func TestUpdateReleaseBranchFiles(t *testing.T) {
	t.Parallel()

	t.Run("creates missing changelog with top-level header", func(t *testing.T) {
		t.Parallel()

		// given: releaser without an existing changelog file
		cfg := config.Default()

		stub := newProviderStub()
		branch := "yeet/release-v0.1.0"

		r := New(cfg, stub)

		result := &Result{
			NextVersion: "0.1.0",
			NextTag:     "v0.1.0",
			Changelog: strings.TrimSpace(`## v0.1.0 (2026-03-01)

### Features

- initial release (abc1234)
`),
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: changelog is created with the release-please style header
		testastic.NoError(t, err)

		updated := stub.files[providerFileKey(branch, cfg.Changelog.File)]
		testastic.Equal(t, strings.TrimSpace(`# Changelog

## v0.1.0 (2026-03-01)

### Features

- initial release (abc1234)
`), strings.TrimSpace(updated))
	})

	t.Run("updates configured version files", func(t *testing.T) {
		t.Parallel()

		// given: releaser with one configured version file containing yeet markers
		cfg := config.Default()
		cfg.VersionFiles = []string{"VERSION.txt"}

		stub := newProviderStub()
		branch := "yeet/release-v1.2.4"
		stub.files[providerFileKey(cfg.Branch, "VERSION.txt")] = "version=1.2.3 # x-yeet-version"

		r := New(cfg, stub)

		result := &Result{
			NextVersion: "1.2.4",
			NextTag:     "v1.2.4",
			Changelog:   "## v1.2.4 (2026-03-01)\n",
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: changelog and version file are updated
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(stub.updates))
		testastic.Equal(t, "version=1.2.4 # x-yeet-version", stub.files[providerFileKey(branch, "VERSION.txt")])
	})

	t.Run("skips version files without yeet markers", func(t *testing.T) {
		t.Parallel()

		// given: releaser with one configured version file without markers
		cfg := config.Default()
		cfg.VersionFiles = []string{"VERSION.txt"}

		stub := newProviderStub()
		branch := "yeet/release-v1.2.4"
		stub.files[providerFileKey(cfg.Branch, "VERSION.txt")] = "version=1.2.3"

		r := New(cfg, stub)

		result := &Result{
			NextVersion: "1.2.4",
			NextTag:     "v1.2.4",
			Changelog:   "## v1.2.4 (2026-03-01)\n",
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: only changelog is updated
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(stub.updates))
		testastic.Equal(t, "version=1.2.3", stub.files[providerFileKey(branch, "VERSION.txt")])
	})

	t.Run("prepends changelog entry and normalizes headerless history", func(t *testing.T) {
		t.Parallel()

		// given: existing changelog without top header and a new release entry
		cfg := config.Default()

		stub := newProviderStub()
		branch := "yeet/release-v0.1.1"
		changelogPath := providerFileKey(cfg.Branch, cfg.Changelog.File)
		stub.files[changelogPath] = strings.TrimSpace(`## [v0.1.0](https://example.com/compare/v0.0.9...v0.1.0) (2026-03-01)

### Features

- first release entry (abc1234)
`)

		r := New(cfg, stub)

		result := &Result{
			NextVersion: "0.1.1",
			NextTag:     "v0.1.1",
			Changelog: strings.TrimSpace(`## [v0.1.1](https://example.com/compare/v0.1.0...v0.1.1) (2026-03-02)

### Bug Fixes

- follow-up fix (def5678)
`),
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: new entry is prepended and the changelog gains a top-level header
		testastic.NoError(t, err)

		updated := stub.files[providerFileKey(branch, cfg.Changelog.File)]
		testastic.HasPrefix(t, updated, "# Changelog")
		testastic.Contains(t, updated, "## [v0.1.1]")
		testastic.Contains(t, updated, "## [v0.1.0]")
		testastic.Contains(t, updated, "def5678)\n\n## [v0.1.0]")

		newEntryIndex := strings.Index(updated, "## [v0.1.1]")
		oldEntryIndex := strings.Index(updated, "## [v0.1.0]")

		testastic.GreaterOrEqual(t, newEntryIndex, 0)
		testastic.GreaterOrEqual(t, oldEntryIndex, 0)
		testastic.Less(t, newEntryIndex, oldEntryIndex)
	})

	t.Run("fails when configured version file is missing", func(t *testing.T) {
		t.Parallel()

		// given: releaser with a missing configured version file
		cfg := config.Default()
		cfg.VersionFiles = []string{"VERSION.txt"}

		r := New(cfg, newProviderStub())

		result := &Result{
			NextVersion: "1.2.4",
			NextTag:     "v1.2.4",
			Changelog:   "## v1.2.4 (2026-03-01)\n",
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), "yeet/release-v1.2.4", result)

		// then: missing file error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrFileNotFound)
	})
}
