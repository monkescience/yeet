//nolint:testpackage // This test validates unexported release behavior.
package release

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/history"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/versionfile"
)

func (r *releaser) Release(ctx context.Context, dryRun bool) (*Result, error) {
	return r.releaseTargets(ctx, dryRun, nil)
}

func TestReleaseSemVerPreMajorBumps(t *testing.T) {
	t.Parallel()

	t.Run("breaking changes do not jump to 1.0.0", func(t *testing.T) {
		t.Parallel()

		// given: a pre-1.0.0 semver release with one breaking commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.tagList = []string{"v0.4.2"}
		stub.commits = []history.CommitEntry{{
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
		stub.tagList = []string{"v0.4.2"}
		stub.commits = []history.CommitEntry{{
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
		stub.tagList = []string{"v0.4.2"}
		stub.commits = []history.CommitEntry{{
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
		stub.tagList = []string{"v0.4.2"}
		stub.commits = []history.CommitEntry{{
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
		stub.tagList = []string{"v0.4.2"}
		stub.commits = []history.CommitEntry{{
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

func TestReleaseUsesLatestTag(t *testing.T) {
	t.Parallel()

	// given: a repository with tags but no provider release objects
	cfg := config.Default()

	stub := newProviderStub()
	stub.tagList = []string{"v1.2.3"}
	stub.commitsByRef = map[string][]history.CommitEntry{
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
	testastic.Equal(t, 1, len(stub.singleRefProbes()))
	testastic.Equal(t, "v1.2.3", stub.singleRefProbes()[0])
}

func TestNewHistorySource(t *testing.T) {
	t.Parallel()

	t.Run("the injected history source serves every history call", func(t *testing.T) {
		t.Parallel()

		// given: provider deps and a separate history source with its own data
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"default": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		deps := newProviderStub()
		deps.tagList = []string{"v9.9.9"}

		historySource := newProviderStub()
		historySource.tagList = []string{"v2.0.0"}
		historySource.commitsByRef = map[string][]history.CommitEntry{
			"v2.0.0": {{Hash: "abcdef1234567890", Message: "feat: new feature"}},
		}

		r, err := newStubReleaserWithSource(context.Background(), cfg, deps, historySource)
		testastic.NoError(t, err)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: the history source answered every history call and deps none
		testastic.NoError(t, err)
		testastic.Equal(t, "2.0.0", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "2.1.0", result.Plans[0].NextVersion)
		testastic.Equal(t, 0, deps.getCommitsSinceRefsCalls)
		testastic.True(t, historySource.getCommitsSinceRefsCalls > 0)
	})

	t.Run("nil history source is rejected", func(t *testing.T) {
		t.Parallel()

		// given: provider deps without a history source
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"default": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: constructing the releaser with a nil history source
		_, err := newStubReleaserWithSource(context.Background(), cfg, newProviderStub(), nil)

		// then: construction fails
		testastic.ErrorIs(t, err, errNilHistorySource)
	})
}

func TestPrereleaseChannels(t *testing.T) {
	t.Parallel()

	t.Run("calver target is rejected with an actionable error", func(t *testing.T) {
		t.Parallel()

		// given: a prerelease channel with a calver target
		cfg := config.Default()
		cfg.ActiveChannel = "beta"
		cfg.Versioning = config.VersioningCalVer
		cfg.Targets = map[string]config.Target{
			"default": {
				Type:       config.TargetTypePath,
				Path:       ".",
				TagPrefix:  "v",
				Versioning: config.VersioningCalVer,
			},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		stub := newProviderStub()

		// when: constructing the releaser
		_, err := newStubReleaser(context.Background(), cfg, stub)

		// then: the error identifies the incompatible target
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: prerelease channel \"beta\" supports semver targets only. "+
				"Target \"default\" uses \"calver\"",
			err.Error(),
		)
	})

	t.Run("stable release ignores prerelease refs", func(t *testing.T) {
		t.Parallel()

		// given: a stable release with a newer prerelease tag present
		cfg := config.Default()

		stub := newProviderStub()
		stub.tagList = []string{"v1.2.3", "v1.3.0-beta.1"}
		stub.commitsByRef = map[string][]history.CommitEntry{
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
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
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
		stub.tagList = []string{"v1.2.3", "v1.3.0-beta.1"}
		stub.commitsByRef = map[string][]history.CommitEntry{
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

	t.Run("derived target keeps prerelease identifier from child bump", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel with a derived target whose release comes from a child feature
		cfg := config.Default()
		cfg.Branch = "beta"
		cfg.ActiveChannel = "beta"
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}
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
		stub.tagList = []string{"v3.0.0", "api-v1.2.0"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add token rotation",
			Paths:   []string{"services/api/main.go"},
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating the beta release wave
		result, err := r.Release(context.Background(), true)

		// then: both child and derived versions stay in the beta channel
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(result.Plans))
		testastic.Equal(t, "api-v1.3.0-beta.1", result.Plans[0].NextTag)
		testastic.Equal(t, "v3.1.0-beta.1", result.Plans[1].NextTag)
	})

	t.Run("channel release writes channel changelog", func(t *testing.T) {
		// given: a beta channel release with a version file
		cfg := config.Default()
		cfg.Branch = "beta"
		cfg.ActiveChannel = "beta"
		cfg.VersionFiles = []config.VersionFile{{Path: "VERSION"}}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.tagList = []string{"v1.2.3"}
		stub.files[providerFileKey("beta", "VERSION")] = "version = \"1.2.3\" # x-yeet-version\n"
		stub.commits = []history.CommitEntry{{
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

		testastic.AssertFile(
			t,
			"testdata/prerelease_channels/channel_release_writes_channel_changelog/changelog_beta.expected.md",
			updatedFiles["CHANGELOG.beta.md"],
		)
		testastic.AssertFile(
			t,
			"testdata/prerelease_channels/channel_release_writes_channel_changelog/version.expected.txt",
			updatedFiles["VERSION"],
		)
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
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add export command",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: running the prerelease flow end-to-end
		result, err := r.Release(context.Background(), false)

		// then: provider release creation is marked as a prerelease
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.3.0-beta.1", result.Releases[0].Release.TagName)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.True(t, stub.createReleaseOpts[0].Prerelease)
	})
}

func TestReleaseFallsBackToReachableTagWhenPreferredRefIsOffBranch(t *testing.T) {
	t.Parallel()

	// given: a preferred release ref that is not reachable from the configured branch
	cfg := config.Default()

	stub := newProviderStub()
	stub.tagList = []string{"v1.2.3", "v2.0.0"}
	stub.commitsErrByRef["v2.0.0"] = &provider.CommitBoundaryNotFoundError{Ref: "v2.0.0", Branch: cfg.Branch}
	stub.commitsByRef = map[string][]history.CommitEntry{
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
	testastic.Equal(t, 2, len(stub.singleRefProbes()))
	testastic.Equal(t, "v2.0.0", stub.singleRefProbes()[0])
	testastic.Equal(t, "v1.2.3", stub.singleRefProbes()[1])
}

func TestReleasePrefersNewerReachableTagOverOlderPublishedRelease(t *testing.T) {
	t.Parallel()

	// given: the latest published release is older than a newer stable tag on the release branch
	cfg := config.Default()

	stub := newProviderStub()
	stub.tagList = []string{"v1.2.4", "v1.2.3"}
	stub.commitsByRef = map[string][]history.CommitEntry{
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
	testastic.Equal(t, 1, len(stub.singleRefProbes()))
	testastic.Equal(t, "v1.2.4", stub.singleRefProbes()[0])
}

func TestReleaseChoosesHighestStableTagFromFallbackList(t *testing.T) {
	t.Parallel()

	// given: no published release and an unsorted provider tag list
	cfg := config.Default()

	stub := newProviderStub()
	stub.tagList = []string{"v1.2.3", "v1.10.0", "preview-build", "v1.9.9"}
	stub.commitsByRef = map[string][]history.CommitEntry{
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
	testastic.Equal(t, 1, len(stub.singleRefProbes()))
	testastic.Equal(t, "v1.10.0", stub.singleRefProbes()[0])
}

func TestReleaseAsFooter(t *testing.T) {
	t.Parallel()

	t.Run("forces explicit version without releasable commit", func(t *testing.T) {
		t.Parallel()

		// given: a semver release with only a chore commit and Release-As footer
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.tagList = []string{"v0.4.2"}
		stub.commits = []history.CommitEntry{{
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
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
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
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
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
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: 1.3",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true)

		// then: non-strict semver values are rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, errInvalidReleaseAs)
	})

	t.Run("rejects v-prefixed override value", func(t *testing.T) {
		t.Parallel()

		// given: a commit with v-prefixed release-as value
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: v1.3.0",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true)

		// then: values must be strict semver without v-prefix
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, errInvalidReleaseAs)
	})

	t.Run("fails on conflicting override values", func(t *testing.T) {
		t.Parallel()

		// given: two commits with different Release-As values
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.tagList = []string{"v0.4.2"}
		stub.commits = []history.CommitEntry{
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
		testastic.ErrorIs(t, err, errConflictingReleaseAs)
	})

	t.Run("fails on invalid override value", func(t *testing.T) {
		t.Parallel()

		// given: a commit with malformed Release-As value
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.tagList = []string{"v0.4.2"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: not-a-version",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true)

		// then: invalid value is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, errInvalidReleaseAs)
	})

	t.Run("fails when override is not greater than current version", func(t *testing.T) {
		t.Parallel()

		// given: a commit requesting the same version as current release
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: 1.2.3",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true)

		// then: non-incrementing override is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, errInvalidReleaseAs)
	})

	t.Run("ignores override for calver", func(t *testing.T) {
		t.Parallel()

		// given: a calver repo with only a Release-As chore commit
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer

		stub := newProviderStub()
		stub.commits = []history.CommitEntry{{
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

	changelogBody := readTestFile(t, "testdata/release_after_finalize_merged_release/changelog.input.md")

	t.Run("invalid target does not finalize a merged release", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR and an unknown selected target
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.tagList = []string{"v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number:         2,
			URL:            "https://example.com/pr/2",
			Body:           testManifestBody(t, "v0.1.0", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)

		r := newTestReleaser(t, cfg, stub)

		// when: running a non-dry release for the unknown target
		result, err := r.releaseTargets(context.Background(), false, []string{"missing"})

		// then: target validation fails before provider reads or mutations
		testastic.ErrorIs(t, err, errUnknownTarget)
		testastic.Equal(t, (*Result)(nil), result)
		testastic.Equal(t, 0, stub.findMergedPRCalls)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 0, len(stub.markTaggedCalls))
	})

	t.Run("does not create PR when no commits exist after finalized tag", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR with no commits after its tag
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.tagList = []string{"v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number:         3,
			URL:            "https://example.com/pr/3",
			Body:           testManifestBody(t, "v0.1.0", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]history.CommitEntry{
			"v0.1.0": {},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: merged release is finalized and no new release PR is created
		testastic.NoError(t, err)
		testastic.True(t, len(result.Releases) > 0)
		testastic.Equal(t, "v0.1.0", result.Releases[0].Release.TagName)
		testastic.Equal(t, 0, len(result.Plans))
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.SliceEqual(t, []string{"v0.0.9", "v0.1.0"}, stub.singleRefProbes())
	})

	t.Run("creates PR when commits exist after finalized tag", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR and new commits after its tag
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.tagList = []string{"v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number:         4,
			URL:            "https://example.com/pr/4",
			Body:           testManifestBody(t, "v0.1.0", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]history.CommitEntry{
			"v0.1.0": {{Hash: "abcdef1234567890", Message: "fix: patch after release"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: merged release is finalized and a new release PR is created for fresh commits
		testastic.NoError(t, err)
		testastic.True(t, len(result.Releases) > 0)
		testastic.Equal(t, "v0.1.0", result.Releases[0].Release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, stub.createPRCalls)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)
		testastic.SliceEqual(t, []string{"v0.0.9", "v0.1.0"}, stub.singleRefProbes())
	})

	t.Run("plans past a published tag the forge tag list has not caught up to", func(t *testing.T) {
		t.Parallel()

		// given: a merged release PR for v0.1.0 and a forge whose tag list still
		//        predates that publish when the run reads it again
		cfg := config.Default()

		stub := newProviderStub()
		stub.tagList = []string{"v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number:         7,
			URL:            "https://example.com/pr/7",
			Body:           testManifestBody(t, "v0.1.0", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]history.CommitEntry{
			"v0.0.9": {
				{Hash: "abcdef1234567890", Message: "feat: released change\n\nRelease-As: 0.1.0"},
				{Hash: "fedcba0987654321", Message: "fix: patch after release"},
			},
			"v0.1.0": {{Hash: "fedcba0987654321", Message: "fix: patch after release"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: the next wave starts from the tag this run published, not the one before it
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Releases))
		testastic.Equal(t, "v0.1.0", result.Releases[0].Release.TagName)
		testastic.Equal(t, 1, len(result.Plans))
		testastic.Equal(t, "0.1.0", result.Plans[0].CurrentVersion)
		testastic.Equal(t, "v0.1.1", result.Plans[0].NextTag)
	})

	t.Run("reports the finalized release and the auto-merged wave from one run", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR, fresh commits after its tag, and auto-merge enabled
		cfg := config.Default()
		cfg.Release.AutoMerge = true

		stub := newProviderStub()
		stub.mergePRSHA = "second-merged-sha"
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.tagList = []string{"v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number:         4,
			URL:            "https://example.com/pr/4",
			Body:           testManifestBody(t, "v0.1.0", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]history.CommitEntry{
			"v0.1.0": {{Hash: "abcdef1234567890", Message: "fix: patch after release"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: neither release is dropped from the result
		testastic.NoError(t, err)

		tags := make([]string, 0, len(result.Releases))
		for _, release := range result.Releases {
			tags = append(tags, release.Release.TagName)
		}

		testastic.SliceEqual(t, []string{"v0.1.0", "v0.1.1"}, tags)
	})

	t.Run("finalizes a merged release the unfinalized window cannot analyze", func(t *testing.T) {
		t.Parallel()

		// given: a merged release PR whose own override conflicts with a newer one after its tag
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.tagList = []string{"v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number:         6,
			URL:            "https://example.com/pr/6",
			Body:           testManifestBody(t, "v0.1.0", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]history.CommitEntry{
			"v0.0.9": {
				{Hash: "abcdef1234567890", Message: "feat: released change\n\nRelease-As: 0.1.0"},
				{Hash: "fedcba0987654321", Message: "fix: later patch\n\nRelease-As: 0.2.0"},
			},
			"v0.1.0": {
				{Hash: "fedcba0987654321", Message: "fix: later patch\n\nRelease-As: 0.2.0"},
			},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: finalization clears the conflict instead of wedging every rerun
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Releases))
		testastic.Equal(t, "v0.1.0", result.Releases[0].Release.TagName)
		testastic.Equal(t, "0.2.0", result.Plans[0].NextVersion)
		testastic.Equal(t, 1, stub.createPRCalls)
	})

	t.Run("reports the analysis failure when no merged release can clear it", func(t *testing.T) {
		t.Parallel()

		// given: conflicting overrides after the latest tag and no merged release PR
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.tagList = []string{"v0.0.9"}
		stub.commits = []history.CommitEntry{
			{Hash: "abcdef1234567890", Message: "feat: change\n\nRelease-As: 0.1.0"},
			{Hash: "fedcba0987654321", Message: "fix: patch\n\nRelease-As: 0.2.0"},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false)

		// then: the conflict surfaces instead of being swallowed by the finalization probe
		testastic.ErrorIs(t, err, errConflictingReleaseAs)
		testastic.Equal(t, (*Result)(nil), result)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
	})
}

func TestReleaseValidatesRenderedTitlesBeforeMutation(t *testing.T) {
	t.Parallel()

	t.Run("dry run rejects an empty title for actual release data", func(t *testing.T) {
		t.Parallel()

		// given: a stable release whose title template only renders for a channel
		cfg := config.Default()
		cfg.Release.PRTitle = "{{ if .Channel }}release {{ .Channel }}{{ end }}"

		stub := newProviderStub()
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: analyzing the release without mutations
		result, err := r.Release(context.Background(), true)

		// then: actual template output is validated during the dry run
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, (*Result)(nil), result)
		testastic.Equal(t, 0, stub.findMergedPRCalls)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
	})

	t.Run("merged release is not finalized before an invalid title fails", func(t *testing.T) {
		t.Parallel()

		// given: an unfinalized merged release and a template that is empty for the stable channel
		cfg := config.Default()
		cfg.Release.PRTitle = "{{ if .Channel }}release {{ .Channel }}{{ end }}"
		changelogBody := readTestFile(t, "testdata/release_after_finalize_merged_release/changelog.input.md")

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.tagList = []string{"v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number:         5,
			URL:            "https://example.com/pr/5",
			Body:           testManifestBody(t, "v0.1.0", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]history.CommitEntry{
			"v0.0.9": {{Hash: "abcdef1234567890", Message: "fix: release patch"}},
			"v0.1.0": {{Hash: "fedcba0987654321", Message: "fix: later patch"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: running the release with invalid actual template output
		result, err := r.Release(context.Background(), false)

		// then: validation fails before the merged release is published or relabeled
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, (*Result)(nil), result)
		testastic.Equal(t, 0, stub.findMergedPRCalls)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 0, len(stub.markTaggedCalls))
		testastic.Equal(t, 0, stub.createPRCalls)
	})
}

func TestReleaseFailsWhenPreviousReleaseIsNotReachableFromBranch(t *testing.T) {
	t.Parallel()

	// given: the latest release ref exists but is not on the configured release branch
	cfg := config.Default()

	stub := newProviderStub()
	stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
	stub.tagList = []string{"v1.2.3"}
	stub.commitsErr = &provider.CommitBoundaryNotFoundError{Ref: "v1.2.3", Branch: cfg.Branch}

	r := newTestReleaser(t, cfg, stub)

	// when: running release end-to-end
	result, err := r.Release(context.Background(), false)

	// then: release stops before creating a PR, tag, or release
	testastic.Error(t, err)
	testastic.Equal(t, (*Result)(nil), result)
	testastic.ErrorIs(t, err, provider.ErrCommitBoundaryNotFound)
	testastic.Equal(
		t,
		"previous release ref \"v1.2.3\" is not reachable from release branch \"main\" for target "+
			"\"default\". Verify the latest tag or release and branch ancestry: commit boundary not found: "+
			"ref \"v1.2.3\" is not reachable from branch \"main\"",
		err.Error(),
	)
	testastic.Equal(t, 0, stub.createPRCalls)
	testastic.Equal(t, 0, stub.createReleaseCalls)
	testastic.Equal(t, 0, len(stub.markPendingCalls))
	testastic.Equal(t, 1, len(stub.singleRefProbes()))
	testastic.Equal(t, "v1.2.3", stub.singleRefProbes()[0])
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
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
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
		testastic.Equal(t, result.Plans[0].NextTag, result.Releases[0].Release.TagName)
		testastic.Equal(t, 1, stub.createPRCalls)
		testastic.Equal(t, 1, stub.mergePRCalls)
		testastic.Equal(t, 1, len(stub.mergePRNumbers))
		testastic.Equal(t, result.PullRequest.Number, stub.mergePRNumbers[0])
		testastic.Equal(t, 1, len(stub.mergePROptions))
		testastic.False(t, stub.mergePROptions[0].BypassMergeChecks)
		testastic.Equal(t, provider.MergeMethodAuto, stub.mergePROptions[0].Method)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markPendingCalls))
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, result.PullRequest.Number, stub.markTaggedCalls[0])
		testastic.Equal(t, 1, stub.findMergedPRCalls)
	})

	t.Run("creates release at merged commit", func(t *testing.T) {
		t.Parallel()

		// given: auto-merge enabled and the base branch may advance after the merge
		cfg := config.Default()
		cfg.Release.AutoMerge = true

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}
		stub.mergePRSHA = "merged-sha"

		r := newTestReleaser(t, cfg, stub)

		// when: running release end-to-end
		_, err := r.Release(context.Background(), false)

		// then: tag creation uses the merged commit instead of the moving base branch
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.Equal(t, "merged-sha", stub.createReleaseOpts[0].Ref)
		testastic.Equal(t, 1, stub.findMergedPRCalls)
	})

	t.Run("force mode forwards force option to provider merge", func(t *testing.T) {
		t.Parallel()

		// given: force auto-merge enabled
		cfg := config.Default()
		cfg.Release.AutoMergeForce = true

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
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
		testastic.True(t, stub.mergePROptions[0].BypassMergeChecks)
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
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
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
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
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
	cfg.Release.PRTitle = "release {{ .Tag }}"

	stub := newProviderStub()
	stub.openPending = []*provider.PullRequest{{
		Number: 7,
		URL:    "https://example.com/pr/7",
		Branch: "yeet/release-v0.0.1",
	}}
	stub.commits = []history.CommitEntry{{
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
	testastic.Equal(t, 0, len(stub.markPendingCalls))
	testastic.Equal(t, "release v0.1.0", stub.updatePROptions[0].Title)
	testastic.Equal(t, "yeet/release-v0.0.1", result.PullRequest.Branch)
	testastic.AssertFile(
		t,
		"testdata/release_reuses_single_pending_p_r/pull_request_body.expected.md",
		result.PullRequest.Body,
	)
}

func TestReleaseAdoptsUnlabelledPendingPR(t *testing.T) {
	t.Parallel()

	// given: an open release PR that a previous run created but never labelled
	cfg := config.Default()

	stub := newProviderStub()
	stub.openPending = []*provider.PullRequest{{
		Number:            7,
		URL:               "https://example.com/pr/7",
		Branch:            "yeet/release-main",
		NeedsPendingLabel: true,
	}}
	stub.commits = []history.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "feat: add feature",
	}}

	r := newTestReleaser(t, cfg, stub)

	// when: releasing while that unlabelled PR is open
	result, err := r.Release(context.Background(), false)

	// then: the PR is relabelled and reused instead of wedging or creating a second one
	testastic.NoError(t, err)
	testastic.Equal(t, 0, stub.createPRCalls)
	testastic.Equal(t, 1, stub.updatePRCalls)
	testastic.SliceEqual(t, []int{7}, stub.markPendingCalls)
	testastic.SliceEqual(t, pendingPhaseOnly(), stub.releasePRWorkflowStub.setLabelPhases)
	testastic.Equal(t, "yeet/release-main", result.PullRequest.Branch)
	testastic.False(t, result.PullRequest.NeedsPendingLabel)
}

func TestReleasePRCarriesReviewers(t *testing.T) {
	t.Parallel()

	// given: a config with release reviewers and a releasable commit
	cfg := config.Default()
	cfg.Release.Reviewers = []string{"alice", "bob"}

	stub := newProviderStub()
	stub.commits = []history.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "feat: add feature",
	}}

	r := newTestReleaser(t, cfg, stub)

	// when: creating a release PR
	_, err := r.Release(context.Background(), false)

	// then: the create options carry the configured reviewers
	testastic.NoError(t, err)
	testastic.Equal(t, 1, stub.createPRCalls)
	testastic.SliceEqual(t, []string{"alice", "bob"}, stub.createPROptions[0].Reviewers)
}

func TestReleasePRCarriesConfiguredLabels(t *testing.T) {
	t.Parallel()

	// given: custom lifecycle and create-only labels
	cfg := config.Default()
	cfg.Release.Labels = config.ReleaseLabelsConfig{
		Pending: "release: waiting",
		Tagged:  "release: complete",
		Yeet:    true,
		Extra:   []string{"release", "automated"},
	}

	stub := newProviderStub()
	stub.commits = []history.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "fix: add labels",
	}}

	r := newTestReleaser(t, cfg, stub)

	// when: creating a release PR
	_, err := r.Release(context.Background(), false)

	// then: the configured labels reach creation and the pending phase
	testastic.NoError(t, err)
	testastic.SliceEqual(t, pendingPhaseOnly(), stub.releasePRWorkflowStub.setLabelPhases)
	testastic.Equal(t, cfg.Release.Labels.Pending, stub.createPROptions[0].Labels.Pending)
	testastic.Equal(t, cfg.Release.Labels.Tagged, stub.createPROptions[0].Labels.Tagged)
	testastic.True(t, stub.createPROptions[0].Labels.Yeet)
	testastic.SliceEqual(t, cfg.Release.Labels.Extra, stub.createPROptions[0].Labels.Extra)
	testastic.Equal(t, 1, len(stub.markPendingLabels))
	testastic.Equal(t, cfg.Release.Labels.Pending, stub.markPendingLabels[0].Pending)
	testastic.Equal(t, cfg.Release.Labels.Tagged, stub.markPendingLabels[0].Tagged)
	testastic.True(t, stub.markPendingLabels[0].Yeet)
	testastic.SliceEqual(t, cfg.Release.Labels.Extra, stub.markPendingLabels[0].Extra)
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
	stub.commits = []history.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "fix: patch bug",
	}}

	r := newTestReleaser(t, cfg, stub)

	// when: attempting to create or update release PRs
	_, err := r.Release(context.Background(), false)

	// then: release fails fast with actionable pending PR details
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, ErrMultiplePendingReleasePRs)
	testastic.Equal(
		t,
		"multiple pending release PRs found: #1 https://example.com/pr/1, #2 "+
			"https://example.com/pr/2",
		err.Error(),
	)
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
		stub.commits = []history.CommitEntry{{
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
		testastic.AssertFile(
			t,
			"testdata/release_subject_formatting/default_subject_omits_branch_and_tag_prefix/pull_request_body.expected.md",
			result.PullRequest.Body,
		)
		testastic.AssertFile(
			t,
			"testdata/release_subject_formatting/default_subject_omits_branch_and_tag_prefix/plan_0_changelog.expected.md",
			changelog.Render(result.Plans[0].Entry),
		)

		releaseBranch := "yeet/release-main"
		updatedChangelog := stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)]
		testastic.Equal(t, prependChangelogEntry("", changelog.Render(result.Plans[0].Entry)), updatedChangelog)
	})

	t.Run("custom PR and commit subjects are independent", func(t *testing.T) {
		t.Parallel()

		// given: distinct templates and one releasable commit
		cfg := config.Default()
		cfg.Release.PRTitle = "PR {{ .Tag }}"
		cfg.Release.CommitSubject = "commit {{ .Branch }} {{ .Version }}"

		stub := newProviderStub()
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: provider title and branch commit use their respective templates
		testastic.NoError(t, err)
		testastic.Equal(t, "PR "+result.Plans[0].NextTag, result.PullRequest.Title)
		testastic.Equal(t, "commit main "+result.Plans[0].NextVersion, stub.updateFilesMessages[0])
	})

	t.Run("rejects an empty commit subject before creating a pull request", func(t *testing.T) {
		t.Parallel()

		// given: a commit subject template that is empty for the stable channel
		cfg := config.Default()
		cfg.Release.CommitSubject = "{{ if .Channel }}release {{ .Channel }}{{ end }}"

		stub := newProviderStub()
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release pull request
		_, err := r.Release(context.Background(), false)

		// then: rendered subject validation fails before any provider mutation
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, 0, stub.updateFilesCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
		testastic.Equal(t, 0, len(stub.markPendingCalls))
	})

	t.Run("rejects an empty commit subject before updating a pull request", func(t *testing.T) {
		t.Parallel()

		// given: an existing release pull request and a template that renders empty
		cfg := config.Default()
		cfg.Release.CommitSubject = "{{ if .Channel }}release {{ .Channel }}{{ end }}"

		stub := newProviderStub()
		stub.openPending = []*provider.PullRequest{{
			Number: 7,
			URL:    "https://example.com/pr/7",
			Branch: "yeet/release-main",
		}}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: refreshing the existing release pull request
		_, err := r.Release(context.Background(), false)

		// then: rendered subject validation fails before remote content changes
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, 0, stub.updatePRCalls)
		testastic.Equal(t, 0, stub.updateFilesCalls)
	})

	t.Run("custom header and footer wrap PR body only", func(t *testing.T) {
		t.Parallel()

		// given: custom PR body header/footer and one releasable commit
		cfg := config.Default()
		cfg.Release.PRBodyHeader = "## Release checklist\n- [ ] smoke test"
		cfg.Release.PRBodyFooter = "Please review"

		stub := newProviderStub()
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: PR body includes custom wrapper text while changelog content stays clean
		testastic.NoError(t, err)
		testastic.AssertFile(
			t,
			"testdata/release_subject_formatting/custom_header_and_footer_wrap_p_r_body_only/pull_request_body.expected.md",
			result.PullRequest.Body,
		)
		testastic.AssertFile(
			t,
			"testdata/release_subject_formatting/custom_header_and_footer_wrap_p_r_body_only/plan_0_changelog.expected.md",
			changelog.Render(result.Plans[0].Entry),
		)
	})
}

func TestReleaseChangelogSourceOfTruth(t *testing.T) {
	t.Parallel()

	t.Run("reads a shared release branch changelog once", func(t *testing.T) {
		t.Parallel()

		// given: two release plans sharing one changelog on an existing release branch
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
				Changelog: config.ChangelogConfig{File: "CHANGELOG.md"},
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
				Changelog: config.ChangelogConfig{File: "CHANGELOG.md"},
			},
		}

		stub := newProviderStub()
		existing := &provider.PullRequest{Branch: "yeet/release-main"}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"reads_a_shared_release_branch_changelog_once/existing_changelog.input.md",
		))
		stub.files[providerFileKey(existing.Branch, "CHANGELOG.md")] = existingChangelog

		r := newTestReleaser(t, cfg, stub)
		workflow := newReleasePRWorkflow(r.core, r.source, r.prs, r.files, r.publisher)
		result := &Result{Plans: []TargetPlan{
			{ID: "api", NextTag: "api-v1.2.3", Entry: changelog.ParseEntry("## api-v1.2.3 (2026-03-01)")},
			{ID: "web", NextTag: "web-v2.3.4", Entry: changelog.ParseEntry("## web-v2.3.4 (2026-03-01)")},
		}}

		// when: preserving edits for both release plans
		err := workflow.preserveExistingChangelogEdits(t.Context(), existing, result.Plans)

		// then: the shared branch and path are fetched once
		testastic.NoError(t, err)
		testastic.Equal(t, 1, stub.getFileCallsByKey[providerFileKey(existing.Branch, "CHANGELOG.md")])
	})

	t.Run("caches a missing shared release branch changelog", func(t *testing.T) {
		t.Parallel()

		// given: two release plans sharing a missing changelog on an existing release branch
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
				Changelog: config.ChangelogConfig{File: "CHANGELOG.md"},
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
				Changelog: config.ChangelogConfig{File: "CHANGELOG.md"},
			},
		}

		stub := newProviderStub()
		existing := &provider.PullRequest{Branch: "yeet/release-main"}
		r := newTestReleaser(t, cfg, stub)
		workflow := newReleasePRWorkflow(r.core, r.source, r.prs, r.files, r.publisher)
		result := &Result{Plans: []TargetPlan{
			{ID: "api", NextTag: "api-v1.2.3"},
			{ID: "web", NextTag: "web-v2.3.4"},
		}}

		// when: preserving edits for both release plans
		err := workflow.preserveExistingChangelogEdits(t.Context(), existing, result.Plans)

		// then: the shared missing branch and path are fetched once
		testastic.NoError(t, err)
		testastic.Equal(t, 1, stub.getFileCallsByKey[providerFileKey(existing.Branch, "CHANGELOG.md")])
	})

	t.Run("preserves manual edits written before the changelog path changed", func(t *testing.T) {
		t.Parallel()

		// given: a release branch written when the changelog lived at a different path
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"default": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				Changelog: config.ChangelogConfig{File: "docs/CHANGELOG.md"},
			},
		}

		stub := newProviderStub()
		existing := &provider.PullRequest{
			Branch: "yeet/release-main",
			Body:   testManifestBody(t, "v1.2.3", "CHANGELOG.md"),
		}
		stub.files[providerFileKey(existing.Branch, "CHANGELOG.md")] = strings.TrimSpace(`
# Changelog

## v1.2.3 (2026-03-01)

### Features

- add a feature (abc1234)

### Upgrade notes

- rotate the signing key by hand
`)

		r := newTestReleaser(t, cfg, stub)
		workflow := newReleasePRWorkflow(r.core, r.source, r.prs, r.files, r.publisher)
		result := &Result{Plans: []TargetPlan{{
			ID:      "default",
			NextTag: "v1.2.3",
			Entry:   changelog.ParseEntry("## v1.2.3 (2026-03-01)\n\n### Features\n\n- add a feature (abc1234)"),
		}}}

		// when: preserving edits after the configured changelog path moved
		err := workflow.preserveExistingChangelogEdits(t.Context(), existing, result.Plans)

		// then: the manual section recorded at the manifest path survives
		testastic.NoError(t, err)
		testastic.True(t, strings.Contains(changelog.Render(result.Plans[0].Entry), "rotate the signing key by hand"))
	})

	t.Run("new release PR includes changelog guidance without editable notes markers", func(t *testing.T) {
		t.Parallel()

		// given: one releasable commit and no existing release PR
		cfg := config.Default()

		stub := newProviderStub()
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: the PR body tells users to edit the changelog and has no editable notes block
		testastic.NoError(t, err)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"new_release_p_r_includes_changelog_guidance_without_editable_notes_markers/"+
				"pull_request_body.expected.md",
			result.PullRequest.Body,
		)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"new_release_p_r_includes_changelog_guidance_without_editable_notes_markers/"+
				"plan_0_changelog.expected.md",
			changelog.Render(result.Plans[0].Entry),
		)

		updatedChangelog := stub.files[providerFileKey("yeet/release-main", cfg.Changelog.File)]
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"new_release_p_r_includes_changelog_guidance_without_editable_notes_markers/"+
				"updated_changelog.expected.md",
			updatedChangelog,
		)
	})

	t.Run("existing release PR body notes are ignored when updating changelog", func(t *testing.T) {
		t.Parallel()

		// given: an existing pending release PR with old manually edited markdown notes
		cfg := config.Default()

		manualNotes := strings.TrimSpace(readTestFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_p_r_body_notes_are_ignored_when_updating_changelog/"+
				"manual_notes.input.md",
		))

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
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: updating the existing release PR
		result, err := r.Release(context.Background(), false)

		// then: old PR body notes do not affect the generated changelog entry
		testastic.NoError(t, err)
		testastic.Equal(t, 1, stub.updatePRCalls)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_p_r_body_notes_are_ignored_when_updating_changelog/"+
				"pull_request_body.expected.md",
			result.PullRequest.Body,
		)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_p_r_body_notes_are_ignored_when_updating_changelog/"+
				"plan_0_changelog.expected.md",
			changelog.Render(result.Plans[0].Entry),
		)

		updatedChangelog := stub.files[providerFileKey("yeet/release-main", cfg.Changelog.File)]
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_p_r_body_notes_are_ignored_when_updating_changelog/"+
				"updated_changelog.expected.md",
			updatedChangelog,
		)
	})

	t.Run("existing release branch changelog manual section is preserved on rerun", func(t *testing.T) {
		t.Parallel()

		// given: an existing pending release PR whose release branch changelog was manually edited
		cfg := config.Default()
		existingPR := &provider.PullRequest{
			Number: 42,
			Title:  "chore: release 1.2.4",
			Body:   "## Release\n\nPreview only.",
			URL:    "https://example.com/pr/42",
			Branch: "yeet/release-main",
		}

		stub := newProviderStub()
		stub.openPending = []*provider.PullRequest{existingPR}
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_branch_changelog_manual_section_is_preserved_on_rerun/"+
				"existing_changelog.input.md",
		))
		stub.files[providerFileKey(existingPR.Branch, cfg.Changelog.File)] = existingChangelog

		r := newTestReleaser(t, cfg, stub)

		// when: updating the existing release PR
		result, err := r.Release(context.Background(), false)

		// then: manual changelog-only content remains in the updated changelog entry
		testastic.NoError(t, err)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_branch_changelog_manual_section_is_preserved_on_rerun/"+
				"plan_0_changelog.expected.md",
			changelog.Render(result.Plans[0].Entry),
		)

		updatedChangelog := stub.files[providerFileKey("yeet/release-main", cfg.Changelog.File)]
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_branch_changelog_manual_section_is_preserved_on_rerun/"+
				"updated_changelog.expected.md",
			updatedChangelog,
		)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_branch_changelog_manual_section_is_preserved_on_rerun/"+
				"pull_request_body.expected.md",
			result.PullRequest.Body,
		)
	})

	t.Run("existing release branch manual section is preserved when planned tag changes", func(t *testing.T) {
		t.Parallel()

		// given: a patch release PR with manual notes and a new feature commit that raises the planned version
		cfg := config.Default()
		existingPR := &provider.PullRequest{
			Number: 42,
			Title:  "chore: release 1.2.4",
			Body:   testManifestBody(t, "v1.2.4", cfg.Changelog.File),
			URL:    "https://example.com/pr/42",
			Branch: "yeet/release-main",
		}

		stub := newProviderStub()
		stub.openPending = []*provider.PullRequest{existingPR}
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.tagList = []string{"v1.2.3"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add release automation",
		}}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_branch_manual_section_is_preserved_when_planned_tag_changes/"+
				"existing_changelog.input.md",
		))
		stub.files[providerFileKey(existingPR.Branch, cfg.Changelog.File)] = existingChangelog

		r := newTestReleaser(t, cfg, stub)

		// when: refreshing the existing release PR
		result, err := r.Release(context.Background(), false)

		// then: the old manifest tag locates and preserves manual notes in the new minor release entry
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.3.0", result.Plans[0].NextTag)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_branch_manual_section_is_preserved_when_planned_tag_changes/"+
				"plan_0_changelog.expected.md",
			changelog.Render(result.Plans[0].Entry),
		)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"existing_release_branch_manual_section_is_preserved_when_planned_tag_changes/"+
				"updated_changelog.expected.md",
			stub.files[providerFileKey(existingPR.Branch, cfg.Changelog.File)],
		)
	})

	t.Run("released manifest tag does not seed the next entry with published sections", func(t *testing.T) {
		t.Parallel()

		// given: a pending release PR whose manifest tag was already published
		cfg := config.Default()
		existingPR := &provider.PullRequest{
			Number: 42,
			Title:  "chore: release 1.2.4",
			Body:   testManifestBody(t, "v1.2.4", cfg.Changelog.File),
			URL:    "https://example.com/pr/42",
			Branch: "yeet/release-main",
		}

		stub := newProviderStub()
		stub.openPending = []*provider.PullRequest{existingPR}
		stub.latestRelease = &provider.Release{TagName: "v1.2.4"}
		stub.tagList = []string{"v1.2.4"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}
		stub.files[providerFileKey(existingPR.Branch, cfg.Changelog.File)] = strings.TrimSpace(readTestFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"released_manifest_tag_does_not_seed_the_next_entry_with_published_sections/"+
				"existing_changelog.input.md",
		))

		r := newTestReleaser(t, cfg, stub)

		// when: refreshing the existing release PR
		result, err := r.Release(context.Background(), false)

		// then: sections belonging to the published entry stay out of the new one
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.5", result.Plans[0].NextTag)
		testastic.False(t, strings.Contains(changelog.Render(result.Plans[0].Entry), "BREAKING CHANGES"))
		testastic.False(t, strings.Contains(changelog.Render(result.Plans[0].Entry), "Features"))
	})

	t.Run("generated section headings are not preserved as manual edits", func(t *testing.T) {
		t.Parallel()

		// given: a release branch entry for the planned tag carrying sections this release does not produce
		cfg := config.Default()
		existingPR := &provider.PullRequest{
			Number: 42,
			Title:  "chore: release 1.2.5",
			Body:   testManifestBody(t, "v1.2.5", cfg.Changelog.File),
			URL:    "https://example.com/pr/42",
			Branch: "yeet/release-main",
		}

		stub := newProviderStub()
		stub.openPending = []*provider.PullRequest{existingPR}
		stub.latestRelease = &provider.Release{TagName: "v1.2.4"}
		stub.tagList = []string{"v1.2.4"}
		stub.commits = []history.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}
		stub.files[providerFileKey(existingPR.Branch, cfg.Changelog.File)] = strings.TrimSpace(readTestFile(
			t,
			"testdata/release_changelog_source_of_truth/"+
				"generated_section_headings_are_not_preserved_as_manual_edits/"+
				"existing_changelog.input.md",
		))

		r := newTestReleaser(t, cfg, stub)

		// when: refreshing the existing release PR
		result, err := r.Release(context.Background(), false)

		// then: only headings the generator cannot produce survive
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.5", result.Plans[0].NextTag)
		testastic.False(t, strings.Contains(changelog.Render(result.Plans[0].Entry), "BREAKING CHANGES"))
		testastic.False(t, strings.Contains(changelog.Render(result.Plans[0].Entry), "### Features"))
		testastic.True(t, strings.Contains(changelog.Render(result.Plans[0].Entry), "### Upgrade Notes"))
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
		stub.tagList = []string{"v1.2.3"}

		const headSHA = "abcdef1234567890abcdef1234567890abcdef12"

		stub.commits = []history.CommitEntry{{
			Hash:    headSHA,
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: changelog keeps tag-to-tag compare while PR body links tag-to-head sha
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)

		testastic.AssertFile(
			t,
			"testdata/release_p_r_body_compare_u_r_l_uses_head_commit/"+
				"github_compare_link_uses_latest_commit_sha_in_p_r_body/plan_0_changelog.expected.md",
			changelog.Render(result.Plans[0].Entry),
		)
		testastic.AssertFile(
			t,
			"testdata/release_p_r_body_compare_u_r_l_uses_head_commit/"+
				"github_compare_link_uses_latest_commit_sha_in_p_r_body/pull_request_body.expected.md",
			result.PullRequest.Body,
		)

		releaseBranch := "yeet/release-main"
		updatedChangelog := stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)]
		testastic.Equal(t, prependChangelogEntry("", changelog.Render(result.Plans[0].Entry)), updatedChangelog)
	})

	t.Run("gitlab compare link uses latest commit sha in PR body", func(t *testing.T) {
		t.Parallel()

		// given: GitLab repo with existing release and one new commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.repoURL = "https://gitlab.example.com/group/repo"
		stub.pathPrefix = "/-"
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.tagList = []string{"v1.2.3"}

		const headSHA = "1234567890abcdef1234567890abcdef12345678"

		stub.commits = []history.CommitEntry{{
			Hash:    headSHA,
			Message: "fix: patch bug",
		}}

		r := newTestReleaser(t, cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false)

		// then: changelog keeps tag-to-tag compare while PR body links tag-to-head sha
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)

		testastic.AssertFile(
			t,
			"testdata/release_p_r_body_compare_u_r_l_uses_head_commit/"+
				"gitlab_compare_link_uses_latest_commit_sha_in_p_r_body/plan_0_changelog.expected.md",
			changelog.Render(result.Plans[0].Entry),
		)
		testastic.AssertFile(
			t,
			"testdata/release_p_r_body_compare_u_r_l_uses_head_commit/"+
				"gitlab_compare_link_uses_latest_commit_sha_in_p_r_body/pull_request_body.expected.md",
			result.PullRequest.Body,
		)

		releaseBranch := "yeet/release-main"
		updatedChangelog := stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)]
		testastic.Equal(t, prependChangelogEntry("", changelog.Render(result.Plans[0].Entry)), updatedChangelog)
	})
}

func TestFinalizeMergedReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("rejects merged pull request without merge commit", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR whose provider has not reported the final merge commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 42,
			URL:    "https://example.com/pr/42",
			Body:   testManifestBody(t, "v1.2.3", cfg.Changelog.File),
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"creates_release_from_latest_changelog_entry_and_marks_p_r_tagged/"+
				"existing_changelog.input.md",
		))

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing the merged release PR
		_, err := r.finalizeMergedReleasePRs(context.Background())

		// then: finalization stops before resolving a mutable branch or publishing a release
		testastic.ErrorIs(t, err, provider.ErrEmptyCommitSHA)
		testastic.Equal(t, 0, stub.getReleaseByTagCalls)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 0, len(stub.markTaggedCalls))
	})

	t.Run("reads a shared changelog once for multiple releases", func(t *testing.T) {
		t.Parallel()

		// given: a merged release PR with two tags sharing one base-branch changelog
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
				Changelog: config.ChangelogConfig{File: "CHANGELOG.md"},
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
				Changelog: config.ChangelogConfig{File: "CHANGELOG.md"},
			},
		}

		manifest, err := releaseManifestMarker(releaseManifest{
			BaseBranch: cfg.Branch,
			Targets: []releaseManifestEntry{
				{ID: "api", Type: string(config.TargetTypePath), Tag: "api-v1.2.3", ChangelogFile: "CHANGELOG.md"},
				{ID: "web", Type: string(config.TargetTypePath), Tag: "web-v2.3.4", ChangelogFile: "CHANGELOG.md"},
			},
		})
		testastic.NoError(t, err)

		deps := newProviderStub()
		deps.mergedPR = &provider.PullRequest{
			Number:         42,
			URL:            "https://example.com/pr/42",
			Body:           manifest,
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		source := newProviderStub()
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"reads_a_shared_changelog_once_for_multiple_releases/"+
				"existing_changelog.input.md",
		))
		source.files[providerFileKey(cfg.Branch, "CHANGELOG.md")] = existingChangelog

		r, err := newStubReleaserWithSource(t.Context(), cfg, deps, source)
		testastic.NoError(t, err)

		// when: finalizing both releases
		releases, err := r.finalizeMergedReleasePRs(t.Context())

		// then: each release gets its entry from one shared changelog read
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(releases))
		testastic.AssertFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"reads_a_shared_changelog_once_for_multiple_releases/release_0_body.expected.md",
			releases[0].Release.Body,
		)
		testastic.AssertFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"reads_a_shared_changelog_once_for_multiple_releases/release_1_body.expected.md",
			releases[1].Release.Body,
		)
		testastic.Equal(t, 0, deps.getFileCallsByKey[providerFileKey(cfg.Branch, "CHANGELOG.md")])
		testastic.Equal(t, 1, source.getFileCallsByKey[providerFileKey(cfg.Branch, "CHANGELOG.md")])
	})

	t.Run("creates release from latest changelog entry and marks PR tagged", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR and changelog entry on main
		cfg := config.Default()
		cfg.Release.Labels = config.ReleaseLabelsConfig{
			Pending: "release: waiting",
			Tagged:  "release: complete",
			Extra:   []string{"automated"},
		}

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number:         42,
			URL:            "https://example.com/pr/42",
			Body:           testManifestBody(t, "v1.2.3", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"creates_release_from_latest_changelog_entry_and_marks_p_r_tagged/"+
				"existing_changelog.input.md",
		))
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = existingChangelog

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: release is created from matching changelog entry and PR is marked tagged
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].Release.TagName)
		testastic.Equal(t, 1, stub.getReleaseByTagCalls)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.Equal(t, "merged-sha", stub.createReleaseOpts[0].Ref)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 42, stub.markTaggedCalls[0])
		testastic.SliceEqual(t, taggedPhaseOnly(), stub.releasePublishingStub.setLabelPhases)
		testastic.Equal(t, 1, len(stub.markTaggedLabels))
		testastic.Equal(t, cfg.Release.Labels.Pending, stub.markTaggedLabels[0].Pending)
		testastic.Equal(t, cfg.Release.Labels.Tagged, stub.markTaggedLabels[0].Tagged)
		testastic.SliceEqual(t, cfg.Release.Labels.Extra, stub.markTaggedLabels[0].Extra)
		testastic.AssertFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"creates_release_from_latest_changelog_entry_and_marks_p_r_tagged/"+
				"release_0_body.expected.md",
			releases[0].Release.Body,
		)
	})

	t.Run("ignores merged PR body release notes when changelog was not updated", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR with old manual notes that were not committed to CHANGELOG.md
		cfg := config.Default()

		manualNotes := strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"ignores_merged_p_r_body_release_notes_when_changelog_was_not_updated/"+
				"manual_notes.input.md",
		))

		manifest := testManifestBody(t, "v1.2.3", cfg.Changelog.File)
		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 42,
			URL:    "https://example.com/pr/42",
			Body: "## Release\n\n<!-- BEGIN_YEET_RELEASE_NOTES -->\n" +
				manualNotes +
				"\n<!-- END_YEET_RELEASE_NOTES -->\n\n" + manifest,
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"ignores_merged_p_r_body_release_notes_when_changelog_was_not_updated/"+
				"existing_changelog.input.md",
		))
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = existingChangelog

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: provider release notes come only from the matching changelog entry
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.AssertFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"ignores_merged_p_r_body_release_notes_when_changelog_was_not_updated/"+
				"release_0_body.expected.md",
			releases[0].Release.Body,
		)
	})

	t.Run("includes manual changelog notes in provider release", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR with manual notes committed to CHANGELOG.md
		cfg := config.Default()
		manifest := gitLabNormalizeYeetMarkers(testManifestBody(t, "v1.2.3", cfg.Changelog.File))

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number:         42,
			URL:            "https://example.com/pr/42",
			Body:           "## Release\n\n" + manifest,
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"includes_manual_changelog_notes_in_provider_release/"+
				"existing_changelog.input.md",
		))
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = existingChangelog

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: the release is created with the manual changelog notes
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.AssertFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"includes_manual_changelog_notes_in_provider_release/release_0_body.expected.md",
			releases[0].Release.Body,
		)
		testastic.Equal(t, 1, stub.createReleaseCalls)
	})

	t.Run("ignores malformed old PR body notes block", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR with a start notes marker but no end marker
		cfg := config.Default()
		manifest := testManifestBody(t, "v1.2.3", cfg.Changelog.File)

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 42,
			URL:    "https://example.com/pr/42",
			Body: "## Release\n\n<!-- BEGIN_YEET_RELEASE_NOTES -->\n" +
				"### Upgrade notes\n\nRestart workers after deploying.\n\n" + manifest,
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"ignores_malformed_old_p_r_body_notes_block/existing_changelog.input.md",
		))
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = existingChangelog

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: old malformed release notes markers do not block changelog-based finalization
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.AssertFile(
			t,
			"testdata/finalize_merged_release_p_r/ignores_malformed_old_p_r_body_notes_block/release_0_body.expected.md",
			releases[0].Release.Body,
		)
	})

	t.Run("rejects manifest from unexpected release branch", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR from a branch other than the configured release branch
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number:         33,
			URL:            "https://example.com/pr/33",
			Body:           testManifestBody(t, "v1.2.3", cfg.Changelog.File),
			Branch:         "yeet/release-v1.2.3",
			MergeCommitSHA: "merged-sha",
		}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"rejects_manifest_from_unexpected_release_branch/"+
				"existing_changelog.input.md",
		))
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = existingChangelog

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing the untrusted merged release PR
		_, err := r.finalizeMergedReleasePRs(context.Background())

		// then: finalization fails before creating or marking a release
		testastic.ErrorIs(t, err, errInvalidReleaseManifest)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 0, len(stub.markTaggedCalls))
	})

	t.Run("skips creation when latest release already exists", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR already tagged in provider releases
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3", URL: "https://example.com/releases/v1.2.3"}
		stub.tagList = []string{"v1.2.3"}
		stub.mergedPR = &provider.PullRequest{
			Number:         9,
			URL:            "https://example.com/pr/9",
			Body:           testManifestBody(t, "v1.2.3", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: existing release is reused and PR is still marked tagged
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].Release.TagName)
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
		stub.tagList = []string{"v1.2.4"}
		stub.releasesByTag["v1.2.3"] = &provider.Release{
			TagName: "v1.2.3",
			URL:     "https://example.com/releases/v1.2.3",
		}
		stub.mergedPR = &provider.PullRequest{
			Number:         10,
			URL:            "https://example.com/pr/10",
			Body:           testManifestBody(t, "v1.2.3", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: the exact existing release is reused instead of checking only the latest release
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].Release.TagName)
		testastic.Equal(t, "https://example.com/releases/v1.2.3", releases[0].Release.URL)
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
			Number:         11,
			URL:            "https://example.com/pr/11",
			Body:           testManifestBody(t, "v1.2.3", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"creates_missing_release_when_tag_already_exists/"+
				"existing_changelog.input.md",
		))
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = existingChangelog

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: the provider owns the single tag lookup and reuses the existing tag
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].Release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.createReleaseOpts))
		testastic.Equal(t, "merged-sha", stub.createReleaseOpts[0].Ref)
	})

	t.Run("creates missing tag from merged commit ref", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR with a known merged commit SHA and no existing tag
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number:         13,
			URL:            "https://example.com/pr/13",
			Body:           testManifestBody(t, "v1.2.3", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		existingChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/finalize_merged_release_p_r/"+
				"creates_missing_tag_from_merged_commit_ref/existing_changelog.input.md",
		))
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = existingChangelog

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		releases, err := r.finalizeMergedReleasePRs(context.Background())

		// then: tag creation uses the merged commit ref instead of the current branch head
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(releases))
		testastic.Equal(t, "v1.2.3", releases[0].Release.TagName)
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
		testastic.ErrorIs(t, err, errInvalidReleaseManifest)
		testastic.Equal(t, 0, stub.createReleaseCalls)
	})

	t.Run("fails when matching changelog entry is missing", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR but changelog lacks target tag entry
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number:         12,
			URL:            "https://example.com/pr/12",
			Body:           testManifestBody(t, "v1.2.3", cfg.Changelog.File),
			Branch:         "yeet/release-main",
			MergeCommitSHA: "merged-sha",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = "# Changelog\n\n## v1.2.2 (2026-02-20)"

		r := newTestReleaser(t, cfg, stub)

		// when: finalizing merged release PR
		_, err := r.finalizeMergedReleasePRs(context.Background())

		// then: missing entry is reported
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, changelog.ErrEntryNotFound)
	})
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
				Entry: changelog.ParseEntry(readTestFile(
					t,
					"testdata/update_release_branch_files/"+
						"creates_missing_changelog_with_top_level_header/changelog.input.md",
				)),
			}},
		}

		// when: updating release branch files
		err := newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			context.Background(), branch, result.Plans, "commit subject",
		)

		// then: changelog is created with the release-please style header
		testastic.NoError(t, err)

		updated := stub.files[providerFileKey(branch, cfg.Changelog.File)]
		testastic.AssertFile(
			t,
			"testdata/update_release_branch_files/creates_missing_changelog_with_top_level_header/"+
				"changelog.expected.md",
			strings.TrimSpace(updated),
		)
		testastic.False(t, stub.updates[0].exists)
	})

	t.Run("updates configured version files", func(t *testing.T) {
		t.Parallel()

		// given: releaser with one configured version file containing yeet markers
		cfg := config.Default()
		cfg.VersionFiles = []config.VersionFile{{Path: "VERSION.txt"}}

		stub := newProviderStub()
		branch := "yeet/release-v1.2.4"
		stub.files[providerFileKey(cfg.Branch, "VERSION.txt")] = "version=1.2.3 # x-yeet-version"

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "1.2.4",
				NextTag:     "v1.2.4",
				Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
			}},
		}

		// when: updating release branch files
		err := newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			context.Background(), branch, result.Plans, "commit subject",
		)

		// then: changelog and version file are updated
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(stub.updates))
		testastic.Equal(t, "version=1.2.4 # x-yeet-version", stub.files[providerFileKey(branch, "VERSION.txt")])
	})

	t.Run("rejects a changelog that collides with another target's version file", func(t *testing.T) {
		t.Parallel()

		// given: one target's version file sharing a path with another target's changelog
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:         config.TargetTypePath,
				Path:         "services/api",
				TagPrefix:    "api-v",
				Changelog:    config.ChangelogConfig{File: "api/CHANGELOG.md"},
				VersionFiles: []config.VersionFile{{Path: "shared.md"}},
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
				Changelog: config.ChangelogConfig{File: "shared.md"},
			},
		}

		stub := newProviderStub()
		branch := "yeet/release-main"
		stub.files[providerFileKey(cfg.Branch, "shared.md")] = "version=1.2.3 # x-yeet-version"
		stub.files[providerFileKey(cfg.Branch, "api/CHANGELOG.md")] = "# Changelog\n"

		r := newTestReleaser(t, cfg, stub)
		result := &Result{Plans: []TargetPlan{
			{
				ID:          "api",
				NextVersion: "1.2.4",
				NextTag:     "api-v1.2.4",
				Entry:       changelog.ParseEntry("## api-v1.2.4 (2026-03-01)\n"),
			},
			{
				ID:          "web",
				NextVersion: "2.3.4",
				NextTag:     "web-v2.3.4",
				Entry:       changelog.ParseEntry("## web-v2.3.4 (2026-03-01)\n"),
			},
		}}

		// when: updating release branch files
		err := newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			context.Background(), branch, result.Plans, "commit subject",
		)

		// then: the collision is reported instead of prepending markdown into the version file
		testastic.ErrorIs(t, err, errConflictingFileUpdate)
	})

	t.Run("reads base files only from the local release source", func(t *testing.T) {
		t.Parallel()

		// given: separate local and provider sources with two base branch files
		cfg := config.Default()
		cfg.VersionFiles = []config.VersionFile{{Path: "VERSION.txt"}}
		cfg.Targets = map[string]config.Target{
			"default": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		localSource := newProviderStub()
		localSource.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = "# Changelog\n"
		localSource.files[providerFileKey(cfg.Branch, "VERSION.txt")] = "version=1.2.3 # x-yeet-version"

		remote := newProviderStub()
		r, err := newStubReleaserWithSource(t.Context(), cfg, remote, localSource)
		testastic.NoError(t, err)

		result := &Result{Plans: []TargetPlan{{
			ID:          "default",
			NextVersion: "1.2.4",
			NextTag:     "v1.2.4",
			Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
		}}}

		// when: release branch files are prepared and written
		err = newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			t.Context(),
			"yeet/release-v1.2.4",
			result.Plans,
			"commit subject",
		)

		// then: local blobs are read and the provider only receives one batched write
		testastic.NoError(t, err)
		testastic.Equal(t, 2, localSource.getFileCalls)
		testastic.Equal(t, 0, remote.getFileCalls)
		testastic.Equal(t, 1, remote.updateFilesCalls)
		testastic.Equal(t, 2, len(remote.updates))

		for _, update := range remote.updates {
			testastic.True(t, update.exists)
		}
	})

	t.Run("updates configured json version file", func(t *testing.T) {
		t.Parallel()

		// given: releaser with one configured JSON version file using an explicit pointer
		cfg := config.Default()
		cfg.VersionFiles = []config.VersionFile{{
			Path:        "package.json",
			Format:      config.VersionFileFormatJSON,
			JSONPointer: "/version",
		}}

		stub := newProviderStub()
		branch := "yeet/release-v1.2.4"
		stub.files[providerFileKey(cfg.Branch, "package.json")] = strings.Join([]string{
			`{`,
			`  "name": "app",`,
			`  "version": "1.2.3"`,
			`}`,
		}, "\n")

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "1.2.4",
				NextTag:     "v1.2.4",
				Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
			}},
		}

		// when: updating release branch files
		err := newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			context.Background(), branch, result.Plans, "commit subject",
		)

		// then: changelog and JSON version file are updated
		expected := strings.Join([]string{
			`{`,
			`  "name": "app",`,
			`  "version": "1.2.4"`,
			`}`,
		}, "\n")

		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(stub.updates))
		testastic.Equal(t, expected, stub.files[providerFileKey(branch, "package.json")])
	})

	t.Run("updates configured calver json version file", func(t *testing.T) {
		t.Parallel()

		// given: calver releaser with one configured JSON version file using an explicit pointer
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer
		cfg.VersionFiles = []config.VersionFile{{
			Path:        "package.json",
			Format:      config.VersionFileFormatJSON,
			JSONPointer: "/version",
		}}

		stub := newProviderStub()
		branch := "yeet/release-v2026.03.1"
		stub.files[providerFileKey(cfg.Branch, "package.json")] = `{"name":"app","version":"2026.02.7"}`

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "2026.03.1",
				NextTag:     "v2026.03.1",
				Entry:       changelog.ParseEntry("## v2026.03.1 (2026-03-01)\n"),
			}},
		}

		// when: updating release branch files
		err := newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			context.Background(), branch, result.Plans, "commit subject",
		)

		// then: changelog and JSON version file are updated with the calver string
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(stub.updates))
		testastic.Equal(t, `{"name":"app","version":"2026.03.1"}`, stub.files[providerFileKey(branch, "package.json")])
	})

	t.Run("fails when configured version file has no yeet markers", func(t *testing.T) {
		t.Parallel()

		// given: releaser with one configured version file without markers
		cfg := config.Default()
		cfg.VersionFiles = []config.VersionFile{{Path: "VERSION.txt"}}

		stub := newProviderStub()
		branch := "yeet/release-v1.2.4"
		stub.files[providerFileKey(cfg.Branch, "VERSION.txt")] = "version=1.2.3"

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "1.2.4",
				NextTag:     "v1.2.4",
				Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
			}},
		}

		// when: updating release branch files
		err := newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			context.Background(), branch, result.Plans, "commit subject",
		)

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
		stub.files[changelogPath] = strings.TrimSpace(readTestFile(
			t,
			"testdata/update_release_branch_files/"+
				"prepends_changelog_entry_and_normalizes_headerless_history/"+
				"existing_changelog.input.md",
		))

		r := newTestReleaser(t, cfg, stub)

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "0.1.1",
				NextTag:     "v0.1.1",
				Entry: changelog.ParseEntry(readTestFile(
					t,
					"testdata/update_release_branch_files/"+
						"prepends_changelog_entry_and_normalizes_headerless_history/"+
						"changelog.input.md",
				)),
			}},
		}

		// when: updating release branch files
		err := newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			context.Background(), branch, result.Plans, "commit subject",
		)

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
					Entry: changelog.ParseEntry(readTestFile(
						t,
						"testdata/update_release_branch_files/"+
							"merges_multiple_target_entries_into_a_shared_changelog_file/"+
							"api_changelog.input.md",
					)),
				},
				{
					ID: "web",
					Entry: changelog.ParseEntry(readTestFile(
						t,
						"testdata/update_release_branch_files/"+
							"merges_multiple_target_entries_into_a_shared_changelog_file/"+
							"web_changelog.input.md",
					)),
				},
			},
		}

		// when: updating release branch files
		err := newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			context.Background(), branch, result.Plans, "commit subject",
		)

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
		cfg.VersionFiles = []config.VersionFile{{Path: "VERSION.txt"}}

		r := newTestReleaser(t, cfg, newProviderStub())

		result := &Result{
			Plans: []TargetPlan{{
				ID:          "default",
				NextVersion: "1.2.4",
				NextTag:     "v1.2.4",
				Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
			}},
		}

		// when: updating release branch files
		err := newReleaseBranchUpdater(r.core, r.source, r.files).updateFiles(
			context.Background(),
			"yeet/release-v1.2.4",
			result.Plans,
			"commit subject",
		)

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
		stub.commits = []history.CommitEntry{
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
		stub.commits = []history.CommitEntry{
			{Hash: "abcdef1234567890", Message: "feat: add token rotation", Paths: []string{"services/api/main.go"}},
			{Hash: "1234567890abcdef", Message: "feat: refresh landing page", Paths: []string{"README.md"}},
			{Hash: "fedcba0987654321", Message: "fix: patch dashboard", Paths: []string{"apps/web/src/app.tsx"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: planning only the api target
		result, err := r.releaseTargets(context.Background(), true, []string{"api"})

		// then: root still derives from the selected child target but ignores unselected direct commits and web changes
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(result.Plans))
		testastic.Equal(t, "api", result.Plans[0].ID)
		testastic.Equal(t, "root", result.Plans[1].ID)
		testastic.Equal(t, "v3.1.0", result.Plans[1].NextTag)
		testastic.AssertFile(
			t,
			"testdata/release_targets_monorepo/"+
				"selected_child_targets_still_compute_derived_targets_without_unselected_direct_commits/"+
				"plan_1_changelog.expected.md",
			changelog.Render(result.Plans[1].Entry),
		)
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
		stub.commits = []history.CommitEntry{
			{Hash: "abcdef1234567890", Message: "feat: add token rotation", Paths: []string{"services/api/main.go"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: planning only the derived root target
		result, err := r.releaseTargets(context.Background(), true, []string{"root"})

		// then: root still releases based on child changes, but child targets are not emitted as top-level plans
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Plans))
		testastic.Equal(t, "root", result.Plans[0].ID)
		testastic.Equal(t, "v3.1.0", result.Plans[0].NextTag)
		testastic.Equal(t, 1, result.Plans[0].CommitCount)
		testastic.SliceEqual(t, []string{"api"}, result.Plans[0].IncludedTargets)
		testastic.AssertFile(
			t,
			"testdata/release_targets_monorepo/"+
				"selected_derived_target_analyzes_included_child_targets_without_emitting_them/"+
				"plan_0_changelog.expected.md",
			changelog.Render(result.Plans[0].Entry),
		)
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

		stub.commits = []history.CommitEntry{
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
		result, err := r.releaseTargets(context.Background(), false, []string{"root"})

		// then: the derived target compare link points at the newest included child commit
		// instead of include order or the unreleased tag
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Plans))
		testastic.Equal(t, "root", result.Plans[0].ID)
		testastic.Equal(t, 2, result.Plans[0].CommitCount)

		testastic.AssertFile(
			t,
			"testdata/release_targets_monorepo/"+
				"selected_derived_target_p_r_compare_link_uses_newest_child_sha/"+
				"plan_0_pr_changelog.expected.md",
			changelog.Render(result.Plans[0].PREntry),
		)
		testastic.AssertFile(
			t,
			"testdata/release_targets_monorepo/"+
				"selected_derived_target_p_r_compare_link_uses_newest_child_sha/"+
				"pull_request_body.expected.md",
			result.PullRequest.Body,
		)
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

		stub.commits = []history.CommitEntry{
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
		result, err := r.releaseTargets(context.Background(), false, []string{"root"})

		// then: the derived target compare link points at the newest included commit overall
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Plans))
		testastic.Equal(t, "root", result.Plans[0].ID)
		testastic.Equal(t, apiSHA, result.Plans[0].PRCompareRef)

		testastic.AssertFile(
			t,
			"testdata/release_targets_monorepo/"+
				"selected_derived_target_p_r_compare_link_prefers_newer_child_sha_over_older_direct_sha/"+
				"plan_0_pr_changelog.expected.md",
			changelog.Render(result.Plans[0].PREntry),
		)
		testastic.AssertFile(
			t,
			"testdata/release_targets_monorepo/"+
				"selected_derived_target_p_r_compare_link_prefers_newer_child_sha_over_older_direct_sha/"+
				"pull_request_body.expected.md",
			result.PullRequest.Body,
		)
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
		stub.commits = []history.CommitEntry{
			{Hash: "abcdef1234567890", Message: "feat: add token rotation", Paths: []string{"services/api/main.go"}},
			{Hash: "1234567890abcdef", Message: "fix: patch dashboard", Paths: []string{"apps/web/src/app.tsx"}},
		}

		r := newTestReleaser(t, cfg, stub)

		// when: planning the selected child target and its derived parent together
		result, err := r.releaseTargets(context.Background(), true, []string{"root", "api"})

		// then: api is emitted explicitly, root is emitted as selected, and unselected web is only used for analysis
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(result.Plans))
		testastic.Equal(t, "api", result.Plans[0].ID)
		testastic.Equal(t, "root", result.Plans[1].ID)
		testastic.SliceEqual(t, []string{"api", "web"}, result.Plans[1].IncludedTargets)
		testastic.AssertFile(
			t,
			"testdata/release_targets_monorepo/"+
				"selected_derived_and_child_targets_emit_only_explicitly_selected_path_targets/"+
				"plan_1_changelog.expected.md",
			changelog.Render(result.Plans[1].Entry),
		)
	})
}
