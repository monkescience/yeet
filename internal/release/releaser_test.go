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
	"github.com/monkescience/yeet/internal/versionfile"
)

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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: version bumps to next minor instead of 1.0.0
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "0.5.0", result.Plans[0].NextVersion)
		testastic.Equal(t, "v0.5.0", result.Plans[0].NextTag)
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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: version bumps patch instead of minor
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "0.4.3", result.Plans[0].NextVersion)
		testastic.Equal(t, "v0.4.3", result.Plans[0].NextTag)
	})
}

func TestReleaseSemVerPreMajorOptionsDisabled(t *testing.T) {
	t.Parallel()

	t.Run("breaking changes jump to 1.0.0 when both options disabled", func(t *testing.T) {
		t.Parallel()

		// given: a pre-1.0.0 release with both pre-major options disabled
		cfg := config.Default()
		cfg.PreMajorBreakingBumpsMinor = false
		cfg.PreMajorFeaturesBumpPatch = false

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat(api)!: remove deprecated endpoint",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: breaking change bumps major normally
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "1.0.0", result.Plans[0].NextVersion)
		testastic.Equal(t, "v1.0.0", result.Plans[0].NextTag)
	})

	t.Run("features bump minor when both options disabled", func(t *testing.T) {
		t.Parallel()

		// given: a pre-1.0.0 release with both pre-major options disabled
		cfg := config.Default()
		cfg.PreMajorBreakingBumpsMinor = false
		cfg.PreMajorFeaturesBumpPatch = false

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add export command",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: feature bumps minor normally
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "0.5.0", result.Plans[0].NextVersion)
		testastic.Equal(t, "v0.5.0", result.Plans[0].NextTag)
	})

	t.Run("breaking bumps major but features still bump patch when only breaking disabled", func(t *testing.T) {
		t.Parallel()

		// given: only pre_major_breaking_bumps_minor disabled
		cfg := config.Default()
		cfg.PreMajorBreakingBumpsMinor = false

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add export command",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: features still bump patch
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.3", result.Plans[0].NextVersion)
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

	r := newTestReleaser(t, cfg, stub)

	// when: calculating a release
	result, err := r.Release(context.Background(), true)

	// then: the latest version tag is used as the baseline and commit boundary
	testastic.NoError(t, err)
	testastic.Equal(t, "1.2.3", result.Plans[0].CurrentVersion)
	testastic.Equal(t, "1.2.4", result.Plans[0].NextVersion)
	testastic.Equal(t, 2, len(stub.getCommitsSinceOf))
	testastic.Equal(t, "v1.2.3", stub.getCommitsSinceOf[0])
	testastic.Equal(t, "v1.2.3", stub.getCommitsSinceOf[1])
}

func TestPrereleaseChannels(t *testing.T) {
	t.Parallel()

	t.Run("stable release ignores prerelease refs", func(t *testing.T) {
		t.Parallel()

		// given: a stable release with a newer prerelease tag present
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestVersionRef = "v1.3.0-beta.1"
		stub.tagList = []string{"v1.2.3", "v1.3.0-beta.1"}
		stub.commitsByRef = map[string][]provider.CommitEntry{
			"v1.2.3": {{
				Hash:    "abcdef1234567890",
				Message: "fix: patch bug",
			}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating the stable release
		result, err := r.Release(context.Background(), true)

		// then: the prerelease tag is ignored as a stable baseline
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.3", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "1.2.4", result.Plans[0].NextVersion)
	})

	t.Run("first channel release appends prerelease identifier", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel on the beta branch
		cfg := config.Default()
		cfg.Branch = "beta"
		cfg.ActiveChannel = "beta"
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add export command",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a prerelease
		result, err := r.Release(context.Background(), true)

		// then: the next version is a beta prerelease
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.3", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "1.3.0-beta.1", result.Plans[0].NextVersion)
		testastic.Equal(t, "v1.3.0-beta.1", result.Plans[0].NextTag)
	})

	t.Run("channel release increments existing prerelease", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel with an existing beta tag
		cfg := config.Default()
		cfg.Branch = "beta"
		cfg.ActiveChannel = "beta"
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		stub := newProviderStub()
		stub.latestVersionRef = "v1.2.3"
		stub.tagList = []string{"v1.2.3", "v1.3.0-beta.1"}
		stub.commitsByRef = map[string][]provider.CommitEntry{
			"v1.3.0-beta.1": {{
				Hash:    "abcdef1234567890",
				Message: "fix: patch bug",
			}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating the next beta release
		result, err := r.Release(context.Background(), true)

		// then: the prerelease counter increments for the same base version
		testastic.NoError(t, err)
		testastic.Equal(t, "1.3.0-beta.1", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "1.3.0-beta.2", result.Plans[0].NextVersion)
		testastic.Equal(t, "v1.3.0-beta.2", result.Plans[0].NextTag)
	})

	t.Run("channel release writes channel changelog", func(t *testing.T) {
		// given: a beta channel release with a version file
		cfg := config.Default()
		cfg.Branch = "beta"
		cfg.ActiveChannel = "beta"
		cfg.VersionFiles = []string{"VERSION"}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.files[providerFileKey("beta", "VERSION")] = "version = \"1.2.3\" # x-yeet-version\n"
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add export command",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: creating the prerelease PR
		_, err := r.Release(context.Background(), false)

		// then: only the beta changelog is updated
		testastic.NoError(t, err)

		updatedFiles := make(map[string]string, len(stub.updates))
		for _, update := range stub.updates {
			updatedFiles[update.path] = update.content
		}

		testastic.Contains(t, updatedFiles["CHANGELOG.beta.md"], "v1.3.0-beta.1")
		testastic.Contains(t, updatedFiles["VERSION"], "1.3.0-beta.1")
	})

	t.Run("auto-merged channel release creates provider prerelease", func(t *testing.T) {
		t.Parallel()

		// given: auto-merge enabled for a beta channel
		cfg := config.Default()
		cfg.Branch = "beta"
		cfg.ActiveChannel = "beta"
		cfg.Release.AutoMerge = true
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add export command",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: running the prerelease flow end-to-end
		result, err := r.Release(context.Background(), false)

		// then: provider release creation is marked as a prerelease
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.3.0-beta.1", result.Releases[0].TagName)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.True(t, stub.createReleaseOpts[0].Prerelease)
	})
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

	r := newTestReleaser(t, cfg, stub)

	// when: calculating a release
	result, err := r.Release(context.Background(), true)

	// then: the latest reachable stable tag on the branch is used instead
	testastic.NoError(t, err)
	testastic.Equal(t, "1.2.3", result.Plans[0].CurrentVersion)
	testastic.Equal(t, "1.2.4", result.Plans[0].NextVersion)
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

	r := newTestReleaser(t, cfg, stub)

	// when: calculating a release
	result, err := r.Release(context.Background(), true)

	// then: the newer reachable tag becomes the baseline even without a matching release object
	testastic.NoError(t, err)
	testastic.Equal(t, "1.2.4", result.Plans[0].CurrentVersion)
	testastic.Equal(t, "1.2.5", result.Plans[0].NextVersion)
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

	r := newTestReleaser(t, cfg, stub)

	// when: calculating a release
	result, err := r.Release(context.Background(), true)

	// then: the highest stable semver tag is used instead of trusting provider order
	testastic.NoError(t, err)
	testastic.Equal(t, "1.10.0", result.Plans[0].CurrentVersion)
	testastic.Equal(t, "1.10.1", result.Plans[0].NextVersion)
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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: explicit version override is used
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "1.0.0", result.Plans[0].NextVersion)
		testastic.Equal(t, "v1.0.0", result.Plans[0].NextTag)
		testastic.Equal(t, commit.BumpMajor, result.Plans[0].BumpType)
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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: exact semver override is used
		testastic.NoError(t, err)
		testastic.Equal(t, "1.4.0", result.Plans[0].NextVersion)
		testastic.Equal(t, "v1.4.0", result.Plans[0].NextTag)
		testastic.Equal(t, commit.BumpMinor, result.Plans[0].BumpType)
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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: footer key is recognized regardless of casing
		testastic.NoError(t, err)
		testastic.Equal(t, "1.3.0", result.Plans[0].NextVersion)
		testastic.Equal(t, "v1.3.0", result.Plans[0].NextTag)
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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true)

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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true)

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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true)

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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true)

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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true)

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

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: release-as footer is ignored for calver
		testastic.NoError(t, err)
		testastic.Equal(t, 0, len(result.Plans))
	})
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
			Body:   testManifestBody("v0.1.0", cfg.Changelog.File),
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]provider.CommitEntry{
			"v0.1.0": {},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: merged release is finalized and no new release PR is created
		testastic.NoError(t, err)
		testastic.True(t, len(result.Releases) > 0)
		testastic.Equal(t, "v0.1.0", result.Releases[0].TagName)
		testastic.Equal(t, 0, len(result.Plans))
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
			Body:   testManifestBody("v0.1.0", cfg.Changelog.File),
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]provider.CommitEntry{
			"v0.1.0": {{Hash: "abcdef1234567890", Message: "fix: patch after release"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: merged release is finalized and a new release PR is created for fresh commits
		testastic.NoError(t, err)
		testastic.True(t, len(result.Releases) > 0)
		testastic.Equal(t, "v0.1.0", result.Releases[0].TagName)
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

	r := newTestReleaser(t, cfg, stub)

	// when: running release end-to-end
	result, err := r.Release(context.Background(), false)

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

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: release PR is merged, tagged, and release is created immediately
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)
		testastic.True(t, len(result.Releases) > 0)
		testastic.Equal(t, result.Plans[0].NextTag, result.Releases[0].TagName)
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

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: merge is attempted in force mode and release is finalized
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)
		testastic.True(t, len(result.Releases) > 0)
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

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		_, err := r.Release(context.Background(), false)

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

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

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

	r := newTestReleaser(t, cfg, stub)

	// when: computing a new version while a pending PR already exists
	result, err := r.Release(context.Background(), false)

	// then: pending PR is updated instead of creating a second release PR
	testastic.NoError(t, err)
	testastic.Equal(t, "0.1.0", result.Plans[0].NextVersion)
	testastic.Equal(t, 0, stub.createPRCalls)
	testastic.Equal(t, 1, stub.updatePRCalls)
	testastic.Equal(t, 1, len(stub.markPendingCalls))
	testastic.Equal(t, "yeet/release-v0.0.1", result.PullRequest.Branch)
	testastic.Contains(t, result.PullRequest.Body, "<!-- yeet-release-manifest")
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

	r := newTestReleaser(t, cfg, stub)

	// when: attempting to create or update release PRs
	_, err := r.Release(context.Background(), false)

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

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: PR title and commit subject use unscoped release subject
		testastic.NoError(t, err)
		testastic.Equal(t, "chore: release "+result.Plans[0].NextVersion, result.PullRequest.Title)
		testastic.Equal(t, 1, stub.updateFilesCalls)
		testastic.Equal(t, "chore: release "+result.Plans[0].NextVersion, stub.updateFilesMessages[0])
		testastic.Equal(t, 1, len(stub.markPendingCalls))
		testastic.AssertFile(t, "testdata/release_subject_default_pr_body.expected.md", result.PullRequest.Body)
		testastic.NotContains(t, result.Plans[0].Changelog,
			"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._")

		releaseBranch := "yeet/release-main"
		updatedChangelog := stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)]
		testastic.Equal(t, prependChangelogEntry("", result.Plans[0].Changelog), updatedChangelog)
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

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: PR body includes custom wrapper text while changelog content stays clean
		testastic.NoError(t, err)
		testastic.HasPrefix(t, result.PullRequest.Body, cfg.Release.PRBodyHeader+"\n\n")
		testastic.HasSuffix(t, strings.TrimSpace(result.PullRequest.Body), cfg.Release.PRBodyFooter)
		testastic.NotContains(
			t,
			result.PullRequest.Body,
			"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
		)
		testastic.NotContains(t, result.Plans[0].Changelog, cfg.Release.PRBodyHeader)
		testastic.NotContains(t, result.Plans[0].Changelog, cfg.Release.PRBodyFooter)
	})
}

func TestReleaseEditableReleaseNotes(t *testing.T) {
	t.Parallel()

	t.Run("new release PR includes editable notes block without committing empty markers", func(t *testing.T) {
		t.Parallel()

		// given: one releasable commit and no existing release PR
		cfg := config.Default()

		stub := newProviderStub()
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: the PR body has an editable notes block, but the committed changelog stays clean
		testastic.NoError(t, err)
		testastic.Contains(t, result.PullRequest.Body, "<!-- BEGIN_YEET_RELEASE_NOTES -->")
		testastic.Contains(t, result.PullRequest.Body, "<!-- END_YEET_RELEASE_NOTES -->")
		testastic.NotContains(t, result.Plans[0].Changelog, "BEGIN_YEET_RELEASE_NOTES")

		updatedChangelog := stub.files[providerFileKey("yeet/release-main", cfg.Changelog.File)]
		testastic.NotContains(t, updatedChangelog, "BEGIN_YEET_RELEASE_NOTES")
	})

	t.Run("existing release PR notes are preserved and written to changelog", func(t *testing.T) {
		t.Parallel()

		// given: an existing pending release PR with manually edited markdown notes
		cfg := config.Default()

		manualNotes := strings.TrimSpace(`### Action Required

Set the provider explicitly for custom hosts:

` + "```yaml" + `
provider: github
repository:
  host: github.company.com
  owner: platform
  repo: app
` + "```" + `
`)

		existingPR := &provider.PullRequest{
			Number: 42,
			Title:  "chore: release 1.2.4",
			Body: "## Release\n\n<!-- BEGIN_YEET_RELEASE_NOTES -->\n" +
				manualNotes +
				"\n<!-- END_YEET_RELEASE_NOTES -->\n",
			URL:    "https://example.com/pr/42",
			Branch: "yeet/release-main",
		}

		stub := newProviderStub()
		stub.openPending = []*provider.PullRequest{existingPR}
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: updating the existing release PR
		result, err := r.Release(context.Background(), false)

		// then: markdown notes remain editable in the PR body and are committed into the changelog entry
		testastic.NoError(t, err)
		testastic.Equal(t, 1, stub.updatePRCalls)
		testastic.Contains(t, result.PullRequest.Body, "<!-- BEGIN_YEET_RELEASE_NOTES -->")
		testastic.Contains(t, result.PullRequest.Body, manualNotes)
		testastic.Contains(t, result.Plans[0].Changelog, manualNotes)
		testastic.NotContains(t, result.Plans[0].Changelog, "BEGIN_YEET_RELEASE_NOTES")

		updatedChangelog := stub.files[providerFileKey("yeet/release-main", cfg.Changelog.File)]
		testastic.Contains(t, updatedChangelog, manualNotes)
		testastic.NotContains(t, updatedChangelog, "BEGIN_YEET_RELEASE_NOTES")
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

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: changelog keeps tag-to-tag compare while PR body links tag-to-head sha
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)

		canonicalCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v1.2.3", "v1.2.4")
		prCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v1.2.3", headSHA)

		testastic.Contains(t, result.Plans[0].Changelog, canonicalCompareURL)
		testastic.NotContains(t, result.Plans[0].Changelog, prCompareURL)
		testastic.Contains(t, result.PullRequest.Body, prCompareURL)
		testastic.NotContains(t, result.PullRequest.Body, canonicalCompareURL)

		releaseBranch := "yeet/release-main"
		updatedChangelog := stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)]
		testastic.Equal(t, prependChangelogEntry("", result.Plans[0].Changelog), updatedChangelog)
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

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: changelog keeps tag-to-tag compare while PR body links tag-to-head sha
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)

		canonicalCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v1.2.3", "v1.2.4")
		prCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v1.2.3", headSHA)

		testastic.Contains(t, result.Plans[0].Changelog, canonicalCompareURL)
		testastic.NotContains(t, result.Plans[0].Changelog, prCompareURL)
		testastic.Contains(t, result.PullRequest.Body, prCompareURL)
		testastic.NotContains(t, result.PullRequest.Body, canonicalCompareURL)

		releaseBranch := "yeet/release-main"
		updatedChangelog := stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)]
		testastic.Equal(t, prependChangelogEntry("", result.Plans[0].Changelog), updatedChangelog)
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
			Body:   testManifestBody("v1.2.3", cfg.Changelog.File),
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

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: release is created from matching changelog entry and PR is marked tagged
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.Equal(t, cfg.Branch, stub.createReleaseOpts[0].Ref)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 42, stub.markTaggedCalls[0])
		testastic.Contains(t, releases[0].Body, "## [v1.2.3]")
		testastic.NotContains(t, releases[0].Body, "## [v1.2.2]")
	})

	t.Run("includes merged PR release notes when changelog was not updated", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR with manual notes that were not committed to CHANGELOG.md
		cfg := config.Default()

		manualNotes := strings.TrimSpace(`### Action Required

Set the provider explicitly for custom hosts:

` + "```yaml" + `
provider: gitlab
repository:
  host: gitlab.company.com
  project: group/subgroup/app
` + "```" + `
`)

		manifest := testManifestBody("v1.2.3", cfg.Changelog.File)
		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 42,
			URL:    "https://example.com/pr/42",
			Body: "## Release\n\n<!-- BEGIN_YEET_RELEASE_NOTES -->\n" +
				manualNotes +
				"\n<!-- END_YEET_RELEASE_NOTES -->\n\n" + manifest,
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)
`)

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: provider release notes include the manual markdown without marker comments
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Contains(t, releases[0].Body, manualNotes)
		testastic.NotContains(t, releases[0].Body, "BEGIN_YEET_RELEASE_NOTES")
	})

	t.Run("creates release from manifest marker on versioned branch", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR with manifest marker on a versioned branch
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 33,
			URL:    "https://example.com/pr/33",
			Body:   testManifestBody("v1.2.3", cfg.Changelog.File),
			Branch: "yeet/release-v1.2.3",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)
`)

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: manifest marker tag is used
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].TagName)
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
			Body:   testManifestBody("v1.2.3", cfg.Changelog.File),
			Branch: "yeet/release-main",
		}

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: existing release is reused and PR is still marked tagged
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].TagName)
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
			Body:   testManifestBody("v1.2.3", cfg.Changelog.File),
			Branch: "yeet/release-main",
		}

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: the exact existing release is reused instead of checking only the latest release
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].TagName)
		testastic.Equal(t, "https://example.com/releases/v1.2.3", releases[0].URL)
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
			Body:   testManifestBody("v1.2.3", cfg.Changelog.File),
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)
`)

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: only the missing release object is created and no branch ref is forced
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].TagName)
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
			Body:           testManifestBody("v1.2.3", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)
`)

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: tag creation uses the merged commit ref instead of the current branch head
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.Equal(t, "merged-sha", stub.createReleaseOpts[0].Ref)
	})

	t.Run("returns no-pr error when no merged pending release PR exists", func(t *testing.T) {
		t.Parallel()

		// given: no merged pending release PR
		r := newTestReleaser(t, config.Default(), newProviderStub())

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: nothing is finalized
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrNoPR)
		testastic.Equal(t, 0, len(releases))
	})

	t.Run("fails when PR has no release manifest marker", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR without manifest marker
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 25,
			URL:    "https://example.com/pr/25",
			Branch: "yeet/release-main",
		}

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		_, err := r.finalizeMergedReleasePRs(context.Background())

		// then: manifest marker requirement is enforced
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseManifest)
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
			Body:   testManifestBody("v1.2.3", cfg.Changelog.File),
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = "# Changelog\n\n## v1.2.2 (2026-02-20)"

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		_, err := r.finalizeMergedReleasePRs(context.Background())

		// then: missing entry is reported
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrChangelogEntryNotFound)
	})
}

func TestTagAcceptsStableTagsWithHyphenatedPrefix(t *testing.T) {
	t.Parallel()

	// given: a releaser with a hyphenated tag prefix
	cfg := config.Default()
	cfg.Targets = map[string]config.Target{
		"default": {
			Type:      config.TargetTypePath,
			Path:      ".",
			TagPrefix: "release-",
		},
	}

	stub := newProviderStub()
	r := newTestReleaser(t, cfg, stub)

	// when: creating a stable tag
	_, err := r.Tag(context.Background(), "release-1.2.3", "")

	// then: tag is accepted
	testastic.NoError(t, err)
	testastic.Equal(t, 1, stub.createReleaseCalls)
	testastic.Equal(t, 1, len(stub.createReleaseOpts))
	testastic.Equal(t, cfg.Branch, stub.createReleaseOpts[0].Ref)
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

	r := newTestReleaser(t, cfg, stub)

	// when: creating the same stable tag again
	result, err := r.Tag(context.Background(), "v1.2.3", "release notes")

	// then: the existing release is reused without another create call
	testastic.NoError(t, err)
	testastic.NotEqual(t, (*Result)(nil), result)
	testastic.True(t, len(result.Releases) > 0)
	testastic.Equal(t, "v1.2.3", result.Releases[0].TagName)
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

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "0.1.0",
				NextTag:     "v0.1.0",
				Changelog: strings.TrimSpace(`## v0.1.0 (2026-03-01)

### Features

- initial release (abc1234)
`),
			}},
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

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "1.2.4",
				NextTag:     "v1.2.4",
				Changelog:   "## v1.2.4 (2026-03-01)\n",
			}},
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: changelog and version file are updated
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(stub.updates))
		testastic.Equal(t, "version=1.2.4 # x-yeet-version", stub.files[providerFileKey(branch, "VERSION.txt")])
	})

	t.Run("fails when configured version file has no yeet markers", func(t *testing.T) {
		t.Parallel()

		// given: releaser with one configured version file without markers
		cfg := config.Default()
		cfg.VersionFiles = []string{"VERSION.txt"}

		stub := newProviderStub()
		branch := "yeet/release-v1.2.4"
		stub.files[providerFileKey(cfg.Branch, "VERSION.txt")] = "version=1.2.3"

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "1.2.4",
				NextTag:     "v1.2.4",
				Changelog:   "## v1.2.4 (2026-03-01)\n",
			}},
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: missing markers abort the release and no provider updates are dispatched
		testastic.ErrorIs(t, err, versionfile.ErrNoMarkersFound)
		testastic.Equal(t, 0, len(stub.updates))
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

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "0.1.1",
				NextTag:     "v0.1.1",
				Changelog: strings.TrimSpace(`## [v0.1.1](https://example.com/compare/v0.1.0...v0.1.1) (2026-03-02)

### Bug Fixes

- follow-up fix (def5678)
`),
			}},
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: new entry is prepended and the changelog gains a top-level header
		testastic.NoError(t, err)

		updated := stub.files[providerFileKey(branch, cfg.Changelog.File)]
		testastic.AssertFile(t, "testdata/update_release_branch_files_prepends_header.expected.md", updated)
	})

	t.Run("merges multiple target entries into a shared changelog file", func(t *testing.T) {
		t.Parallel()

		// given: two path targets that both write to the default shared changelog file
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
			},
		}

		stub := newProviderStub()
		branch := "yeet/release-wave"

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{
				{
					ID: "api",
					Changelog: strings.TrimSpace(`## [api-v1.3.0](https://example.com/compare/api-v1.2.0...api1234) (2026-03-21)

### Features

- add token rotation (api1234)
`),
				},
				{
					ID: "web",
					Changelog: strings.TrimSpace(`## [web-v2.1.4](https://example.com/compare/web-v2.1.3...web5678) (2026-03-21)

### Bug Fixes

- fix dashboard filters (web5678)
`),
				},
			},
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: the shared changelog contains both new entries instead of conflicting
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(stub.updates))

		updated := stub.files[providerFileKey(branch, cfg.Changelog.File)]
		testastic.AssertFile(t, "testdata/update_release_branch_files_shared_changelog.expected.md", updated)
	})

	t.Run("fails when configured version file is missing", func(t *testing.T) {
		t.Parallel()

		// given: releaser with a missing configured version file
		cfg := config.Default()
		cfg.VersionFiles = []string{"VERSION.txt"}

		r := newTestReleaser(t, cfg, newProviderStub())

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "1.2.4",
				NextTag:     "v1.2.4",
				Changelog:   "## v1.2.4 (2026-03-01)\n",
			}},
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), "yeet/release-v1.2.4", result)

		// then: missing file error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrFileNotFound)
	})
}

func TestReleaseTargetsMonorepo(t *testing.T) {
	t.Parallel()

	t.Run("plans path and derived targets from changed paths", func(t *testing.T) {
		t.Parallel()

		// given: a monorepo config with one path target and one derived root target
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
				Changelog: config.ChangelogConfig{File: "services/api/CHANGELOG.md"},
			},
			"root": {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api"},
				Includes:     []string{"api"},
			},
		}

		stub := newProviderStub()
		stub.tagList = []string{"v3.0.0", "api-v1.2.0"}
		stub.commits = []provider.CommitEntry{
			{Hash: "abcdef1234567890", Message: "feat: add token rotation", Paths: []string{"services/api/main.go"}},
			{Hash: "1234567890abcdef", Message: "fix: tidy repo metadata", Paths: []string{"README.md"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: planning a release wave
		result, err := r.Release(context.Background(), true)

		// then: the path target and derived target are both planned independently
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(result.Plans))
		testastic.Equal(t, "api", result.Plans[0].ID)
		testastic.Equal(t, 1, result.Plans[0].CommitCount)
		testastic.Equal(t, "api-v1.3.0", result.Plans[0].NextTag)
		testastic.Equal(t, "root", result.Plans[1].ID)
		testastic.Equal(t, "v3.1.0", result.Plans[1].NextTag)
	})

	t.Run("selected child targets still compute derived targets without unselected direct commits", func(t *testing.T) {
		t.Parallel()

		// given: a monorepo config with selected api target and an unselected root direct commit
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
			},
			"root": {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api", "apps/web"},
				Includes:     []string{"api", "web"},
			},
		}

		stub := newProviderStub()
		stub.tagList = []string{"v3.0.0", "web-v2.0.0", "api-v1.2.0"}
		stub.commits = []provider.CommitEntry{
			{Hash: "abcdef1234567890", Message: "feat: add token rotation", Paths: []string{"services/api/main.go"}},
			{Hash: "1234567890abcdef", Message: "feat: refresh landing page", Paths: []string{"README.md"}},
			{Hash: "fedcba0987654321", Message: "fix: patch dashboard", Paths: []string{"apps/web/src/app.tsx"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: planning only the api target
		result, err := r.ReleaseTargets(context.Background(), true, []string{"api"})

		// then: root still derives from the selected child target but ignores unselected direct commits and web changes
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(result.Plans))
		testastic.Equal(t, "api", result.Plans[0].ID)
		testastic.Equal(t, "root", result.Plans[1].ID)
		testastic.Equal(t, "v3.1.0", result.Plans[1].NextTag)
		testastic.NotContains(t, result.Plans[1].Changelog, "landing page")
		testastic.NotContains(t, result.Plans[1].Changelog, "dashboard")
	})

	t.Run("selected derived target analyzes included child targets without emitting them", func(t *testing.T) {
		t.Parallel()

		// given: a derived target selected on its own with changes only in an included child target
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
			},
			"root": {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api", "apps/web"},
				Includes:     []string{"api", "web"},
			},
		}

		stub := newProviderStub()
		stub.tagList = []string{"v3.0.0", "web-v2.0.0", "api-v1.2.0"}
		stub.commits = []provider.CommitEntry{
			{Hash: "abcdef1234567890", Message: "feat: add token rotation", Paths: []string{"services/api/main.go"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: planning only the derived root target
		result, err := r.ReleaseTargets(context.Background(), true, []string{"root"})

		// then: root still releases based on child changes, but child targets are not emitted as top-level plans
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Plans))
		testastic.Equal(t, "root", result.Plans[0].ID)
		testastic.Equal(t, "v3.1.0", result.Plans[0].NextTag)
		testastic.Equal(t, 1, result.Plans[0].CommitCount)
		testastic.SliceEqual(t, []string{"api"}, result.Plans[0].IncludedTargets)
		testastic.Contains(t, result.Plans[0].Changelog, "token rotation")
	})

	t.Run("selected derived target PR compare link uses newest child sha", func(t *testing.T) {
		t.Parallel()

		// given: a derived root target selected on its own with child commits ordered newest-first on a later include
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
			},
			"root": {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api", "apps/web"},
				Includes:     []string{"api", "web"},
			},
		}

		stub := newProviderStub()
		stub.repoURL = "https://github.example.com/owner/repo"
		stub.tagList = []string{"v3.0.0", "web-v2.0.0", "api-v1.2.0"}

		const (
			webSHA = "fedcba0987654321fedcba0987654321fedcba09"
			apiSHA = "abcdef1234567890abcdef1234567890abcdef12"
		)

		stub.commits = []provider.CommitEntry{
			{
				Hash:    webSHA,
				Message: "fix: patch dashboard",
				Paths:   []string{"apps/web/src/app.tsx"},
			},
			{
				Hash:    apiSHA,
				Message: "feat: add token rotation",
				Paths:   []string{"services/api/main.go"},
			},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR for only the derived root target
		result, err := r.ReleaseTargets(context.Background(), false, []string{"root"})

		// then: the derived target compare link points at the newest included child commit
		// instead of include order or the unreleased tag
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Plans))
		testastic.Equal(t, "root", result.Plans[0].ID)
		testastic.Equal(t, 2, result.Plans[0].CommitCount)

		prCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v3.0.0", webSHA)
		staleChildCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v3.0.0", apiSHA)
		canonicalCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v3.0.0", "v3.1.0")

		testastic.Contains(t, result.Plans[0].PRChangelog, prCompareURL)
		testastic.NotContains(t, result.Plans[0].PRChangelog, staleChildCompareURL)
		testastic.NotContains(t, result.Plans[0].PRChangelog, canonicalCompareURL)
		testastic.Contains(t, result.PullRequest.Body, prCompareURL)
		testastic.NotContains(t, result.PullRequest.Body, staleChildCompareURL)
		testastic.NotContains(t, result.PullRequest.Body, canonicalCompareURL)
	})

	t.Run("selected derived target PR compare link prefers newer child sha over older direct sha", func(t *testing.T) {
		t.Parallel()

		// given: a derived root target with both direct commits and newer child commits
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"root": {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api"},
				Includes:     []string{"api"},
			},
		}

		stub := newProviderStub()
		stub.repoURL = "https://github.example.com/owner/repo"
		stub.tagList = []string{"v3.0.0", "api-v1.2.0"}

		const (
			apiSHA  = "abcdef1234567890abcdef1234567890abcdef12"
			rootSHA = "fedcba0987654321fedcba0987654321fedcba09"
		)

		stub.commits = []provider.CommitEntry{
			{
				Hash:    apiSHA,
				Message: "feat: add token rotation",
				Paths:   []string{"services/api/main.go"},
			},
			{
				Hash:    rootSHA,
				Message: "fix: tidy repo metadata",
				Paths:   []string{"README.md"},
			},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR for only the derived root target
		result, err := r.ReleaseTargets(context.Background(), false, []string{"root"})

		// then: the derived target compare link points at the newest included commit overall
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Plans))
		testastic.Equal(t, "root", result.Plans[0].ID)
		testastic.Equal(t, apiSHA, result.Plans[0].PRCompareRef)

		prCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v3.0.0", apiSHA)
		staleDirectCompareURL := compareURL(stub.repoURL, stub.pathPrefix, "v3.0.0", rootSHA)

		testastic.Contains(t, result.Plans[0].PRChangelog, prCompareURL)
		testastic.NotContains(t, result.Plans[0].PRChangelog, staleDirectCompareURL)
		testastic.Contains(t, result.PullRequest.Body, prCompareURL)
		testastic.NotContains(t, result.PullRequest.Body, staleDirectCompareURL)
	})

	t.Run("selected derived and child targets emit only explicitly selected path targets", func(t *testing.T) {
		t.Parallel()

		// given: an explicitly selected child target plus its derived root target
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
			},
			"root": {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api", "apps/web"},
				Includes:     []string{"api", "web"},
			},
		}

		stub := newProviderStub()
		stub.tagList = []string{"v3.0.0", "web-v2.0.0", "api-v1.2.0"}
		stub.commits = []provider.CommitEntry{
			{Hash: "abcdef1234567890", Message: "feat: add token rotation", Paths: []string{"services/api/main.go"}},
			{Hash: "1234567890abcdef", Message: "fix: patch dashboard", Paths: []string{"apps/web/src/app.tsx"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: planning the selected child target and its derived parent together
		result, err := r.ReleaseTargets(context.Background(), true, []string{"root", "api"})

		// then: api is emitted explicitly, root is emitted as selected, and unselected web is only used for analysis
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(result.Plans))
		testastic.Equal(t, "api", result.Plans[0].ID)
		testastic.Equal(t, "root", result.Plans[1].ID)
		testastic.SliceEqual(t, []string{"api", "web"}, result.Plans[1].IncludedTargets)
		testastic.Contains(t, result.Plans[1].Changelog, "token rotation")
		testastic.Contains(t, result.Plans[1].Changelog, "patch dashboard")
	})
}
