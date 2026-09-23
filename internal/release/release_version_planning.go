package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/logattr"
	"github.com/monkescience/yeet/internal/version"
)

func currentVersionOrInitial(target config.ResolvedTarget) string {
	return versionStrategyForResolvedTarget(target).strategy.InitialVersion()
}

func versionStrategyForResolvedTarget(target config.ResolvedTarget) versionStrategy {
	var strategy version.Strategy

	switch target.Versioning {
	case config.VersioningCalVer:
		strategy = calVerStrategy(target)
	case config.VersioningSemver:
		strategy = &version.SemVer{
			Prefix:                     target.TagPrefix,
			PreMajorBreakingBumpsMinor: target.PreMajorBreakingBumpsMinor,
			PreMajorFeaturesBumpPatch:  target.PreMajorFeaturesBumpPatch,
		}
	default:
		strategy = calVerStrategy(target)
	}

	return versionStrategy{strategy: strategy, prefix: target.TagPrefix}
}

func calVerStrategy(target config.ResolvedTarget) *version.CalVer {
	return &version.CalVer{
		Format: target.CalVer.Format,
		Prefix: target.TagPrefix,
	}
}

func (c *releaseCore) versionStrategyForPlanning(target config.ResolvedTarget) versionStrategy {
	strategy := versionStrategyForResolvedTarget(target)

	calver, isCalVer := strategy.strategy.(*version.CalVer)
	if isCalVer {
		calver.Now = c.timestamp
	}

	return strategy
}

func currentVersionWithInitial(target config.ResolvedTarget, currentVersion string) string {
	if currentVersion != "" {
		return currentVersion
	}

	return currentVersionOrInitial(target)
}

func (a *releaseAnalyzer) nextVersionPlan(
	ctx context.Context,
	target config.ResolvedTarget,
	commits []commit.Commit,
	currentVersion string,
	bumpType commit.BumpType,
) (string, commit.BumpType, bool, error) {
	strategy := a.core.versionStrategyForPlanning(target).strategy
	current := currentVersionWithInitial(target, currentVersion)

	releaseAsVersion, err := releaseAsOverride(ctx, strategy, target, commits, current)
	if err != nil {
		return "", commit.BumpNone, false, err
	}

	//nolint:wrapcheck // The scheme owns this wording and it reaches the user verbatim.
	return strategy.NextRelease(
		current,
		bumpType,
		releaseAsVersion,
		a.core.run.prerelease,
	)
}

func releaseAsOverride(
	ctx context.Context,
	strategy version.Strategy,
	target config.ResolvedTarget,
	commits []commit.Commit,
	currentVersion string,
) (string, error) {
	if !strategy.SupportsReleaseAs() {
		warnUnsupportedReleaseAs(ctx, target, commits)

		return "", nil
	}

	return detectReleaseAs(strategy, commits, currentVersion)
}

func warnUnsupportedReleaseAs(ctx context.Context, target config.ResolvedTarget, commits []commit.Commit) {
	for _, c := range commits {
		for _, footer := range c.Footers {
			if !isReleaseAsFooter(footer.Key) {
				continue
			}

			slog.WarnContext(ctx, "ignoring unsupported release override",
				slog.String("target", target.ID),
				slog.String("commit", c.Hash),
				slog.String("versioning", string(target.Versioning)),
				slog.String("field", "Release-As"),
				slog.String("value", strings.TrimSpace(footer.Value)),
				logattr.Hint("remove the Release-As footer for this target, only semver supports it"),
			)
		}
	}
}

func detectReleaseAs(strategy version.Strategy, commits []commit.Commit, currentVersion string) (string, error) {
	releaseAsVersion := ""

	for _, c := range commits {
		for _, footer := range c.Footers {
			if !isReleaseAsFooter(footer.Key) {
				continue
			}

			candidate := strings.TrimSpace(footer.Value)
			if candidate == "" {
				//nolint:wrapcheck // version supplies the typed release-as context.
				return "", version.NewReleaseAsError(
					"",
					currentVersion,
					"Release-As footer has an empty value",
					fmt.Errorf("%w: empty value", errInvalidReleaseAs),
				)
			}

			normalizedCandidate, err := strategy.NormalizeReleaseAs(candidate)
			if err != nil {
				releaseAs, ok := errors.AsType[*version.ReleaseAsError](err)
				if ok && releaseAs.Current == "" {
					releaseAs.Current = currentVersion
				}

				//nolint:wrapcheck // The scheme owns this wording and it reaches the user verbatim.
				return "", err
			}

			if releaseAsVersion == "" {
				releaseAsVersion = normalizedCandidate

				continue
			}

			if releaseAsVersion != normalizedCandidate {
				//nolint:wrapcheck // version supplies the typed release-as context.
				return "", version.NewConflictingReleaseAsError(
					normalizedCandidate,
					currentVersion,
					releaseAsVersion,
					"two commits request different Release-As versions",
					fmt.Errorf("%w: %q and %q", errConflictingReleaseAs, releaseAsVersion, normalizedCandidate),
				)
			}
		}
	}

	return releaseAsVersion, nil
}

func isReleaseAsFooter(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), "Release-As")
}
