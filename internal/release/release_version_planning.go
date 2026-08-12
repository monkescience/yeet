package release

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
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
		// Config validation admits semver and calver only. A scheme that reached
		// here without it keeps the pre-dispatch behaviour of anything that was
		// not semver, so callers report it instead of dereferencing nil.
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
	strategy := versionStrategyForResolvedTarget(target).strategy

	releaseAsVersion, err := releaseAsOverride(ctx, strategy, target, commits)
	if err != nil {
		return "", commit.BumpNone, false, err
	}

	//nolint:wrapcheck // The scheme owns this wording and it reaches the user verbatim.
	return strategy.NextRelease(
		currentVersionWithInitial(target, currentVersion),
		bumpType,
		releaseAsVersion,
		a.core.activePrereleaseIdentifier(),
	)
}

// releaseAsOverride reads the version a Release-As footer asks for. A scheme
// that cannot honour the footer has every commit carrying one reported, so the
// override is never dropped in silence.
func releaseAsOverride(
	ctx context.Context,
	strategy version.Strategy,
	target config.ResolvedTarget,
	commits []commit.Commit,
) (string, error) {
	if !strategy.SupportsReleaseAs() {
		warnUnsupportedReleaseAs(ctx, target, commits)

		return "", nil
	}

	return detectReleaseAs(strategy, commits)
}

func warnUnsupportedReleaseAs(ctx context.Context, target config.ResolvedTarget, commits []commit.Commit) {
	for _, c := range commits {
		for _, footer := range c.Footers {
			if !isReleaseAsFooter(footer.Key) {
				continue
			}

			slog.WarnContext(ctx, "ignoring Release-As footer unsupported by this versioning strategy",
				slog.String("commit", c.Hash),
				slog.String("versioning", string(target.Versioning)),
				slog.String("release_as", strings.TrimSpace(footer.Value)),
			)
		}
	}
}

func detectReleaseAs(strategy version.Strategy, commits []commit.Commit) (string, error) {
	releaseAsVersion := ""

	for _, c := range commits {
		for _, footer := range c.Footers {
			if !isReleaseAsFooter(footer.Key) {
				continue
			}

			candidate := strings.TrimSpace(footer.Value)
			if candidate == "" {
				return "", fmt.Errorf("%w: empty value", errInvalidReleaseAs)
			}

			normalizedCandidate, err := strategy.NormalizeReleaseAs(candidate)
			if err != nil {
				//nolint:wrapcheck // The scheme owns this wording and it reaches the user verbatim.
				return "", err
			}

			if releaseAsVersion == "" {
				releaseAsVersion = normalizedCandidate

				continue
			}

			if releaseAsVersion != normalizedCandidate {
				return "", fmt.Errorf("%w: %q and %q", errConflictingReleaseAs, releaseAsVersion, normalizedCandidate)
			}
		}
	}

	return releaseAsVersion, nil
}

func isReleaseAsFooter(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), "Release-As")
}
