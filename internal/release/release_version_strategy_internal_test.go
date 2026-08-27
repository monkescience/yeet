package release

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/version"
)

type prereleaseCountingStrategy struct {
	version.Strategy
	calls int
}

func (s *prereleaseCountingStrategy) PrereleaseAllowed(currentVersion, identifier string) bool {
	s.calls++

	return s.Strategy.PrereleaseAllowed(currentVersion, identifier)
}

func TestVersionStrategyForResolvedTarget(t *testing.T) {
	t.Parallel()

	t.Run("resolves a strategy for a scheme config validation would reject", func(t *testing.T) {
		t.Parallel()

		// given: a target carrying a versioning scheme validation never admits
		target := config.ResolvedTarget{ID: "app", Versioning: "unknown", TagPrefix: "v"}

		// when: the strategy is resolved and asked what it supports
		strategy := versionStrategyForResolvedTarget(target).strategy

		// then: callers get an answer instead of a nil interface
		testastic.Equal(t, false, strategy.SupportsPrerelease())
		testastic.Equal(t, false, strategy.SupportsReleaseAs())
	})
}

func TestChannelRefAllowed(t *testing.T) {
	t.Parallel()

	analyzerWithPrerelease := func(prerelease string) *releaseAnalyzer {
		cfg := config.Default()

		return &releaseAnalyzer{core: &releaseCore{
			cfg: cfg, run: releaseRun{baseBranch: cfg.Branch, prerelease: prerelease},
		}}
	}

	t.Run("keeps a stable ref for a stable run", func(t *testing.T) {
		t.Parallel()

		// given: a stable run
		analyzer := analyzerWithPrerelease("")

		// when: a stable version is offered as a version boundary
		allowed := analyzer.channelRefAllowed(&version.SemVer{Prefix: "v"}, "1.2.3")

		// then: it still counts, so the target does not re-plan from no version
		testastic.Equal(t, true, allowed)
	})

	t.Run("rejects a prerelease ref for a stable run", func(t *testing.T) {
		t.Parallel()

		// given: a stable run
		analyzer := analyzerWithPrerelease("")

		// when: a prerelease version is offered as a version boundary
		allowed := analyzer.channelRefAllowed(&version.SemVer{Prefix: "v"}, "1.2.3-beta.1")

		// then: no channel claims it
		testastic.Equal(t, false, allowed)
	})

	t.Run("checks prerelease membership once", func(t *testing.T) {
		t.Parallel()

		// given: an active beta channel and a strategy that counts membership checks
		analyzer := analyzerWithPrerelease("beta")
		strategy := &prereleaseCountingStrategy{Strategy: &version.SemVer{Prefix: "v"}}

		// when: a beta version is offered as a version boundary
		allowed := analyzer.channelRefAllowed(strategy, "1.2.3-beta.1")

		// then: one parse establishes its channel membership
		testastic.Equal(t, true, allowed)
		testastic.Equal(t, 1, strategy.calls)
	})
}

func TestReleaseRunWithChannelChangelogs(t *testing.T) {
	t.Parallel()

	t.Run("normalizes an explicit channel changelog file", func(t *testing.T) {
		t.Parallel()

		// given: an active channel with a changelog path requiring normalization
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {
				Branch: "beta", Prerelease: "beta", ChangelogFile: " ./docs/../CHANGELOG.beta.md ",
			},
		}
		targets := map[string]config.ResolvedTarget{
			"app": {
				ID: "app", Versioning: config.VersioningSemver, TagPrefix: "v",
				Changelog: config.ChangelogConfig{File: "CHANGELOG.md"},
			},
		}

		// when: resolving targets for the active channel
		run, err := resolveRun(cfg, "beta", Options{})
		testastic.NoError(t, err)
		resolved, err := run.withChannelChangelogs(targets)

		// then: the channel changelog path is cleaned before use
		testastic.NoError(t, err)
		testastic.Equal(t, "CHANGELOG.beta.md", resolved["app"].Changelog.File)
	})

	t.Run("rejects a scheme config validation would reject", func(t *testing.T) {
		t.Parallel()

		// given: an active prerelease channel and a target with an unknown scheme
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}
		targets := map[string]config.ResolvedTarget{
			"app": {ID: "app", Versioning: "unknown", TagPrefix: "v"},
		}

		// when: the channel narrows the target set
		run, err := resolveRun(cfg, "beta", Options{})
		testastic.NoError(t, err)
		_, err = run.withChannelChangelogs(targets)

		// then: the target is reported as unsupported rather than panicking
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Contains(t, err.Error(), `prerelease channel "beta" supports semver targets only`)
	})
}
