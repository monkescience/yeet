package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/logattr"
	"github.com/monkescience/yeet/internal/version"
)

func newVersionStrategies(
	targets map[string]config.ResolvedTarget,
	now func() time.Time,
) (map[string]version.Strategy, error) {
	strategies := make(map[string]version.Strategy, len(targets))
	for targetID, target := range targets {
		strategy, err := versionStrategyForResolvedTarget(target, now)
		if err != nil {
			return nil, fmt.Errorf("resolve version strategy for target %q: %w", targetID, err)
		}

		strategies[targetID] = strategy
	}

	return strategies, nil
}

func versionStrategyForResolvedTarget(
	target config.ResolvedTarget,
	now func() time.Time,
) (version.Strategy, error) {
	if target.Versioning == config.VersioningSemver {
		return version.NewSemVer(version.SemVerOptions{
			Prefix:                     target.TagPrefix,
			PreMajorBreakingBumpsMinor: target.PreMajorBreakingBumpsMinor,
			PreMajorFeaturesBumpPatch:  target.PreMajorFeaturesBumpPatch,
		}), nil
	}

	strategy, err := version.NewCalVer(target.CalVer.Format, target.TagPrefix, now)
	if err != nil {
		return nil, fmt.Errorf("compile calver format: %w", err)
	}

	return strategy, nil
}

func currentVersionWithInitial(strategy version.Strategy, currentVersion string) string {
	if currentVersion != "" {
		return currentVersion
	}

	return strategy.InitialVersion()
}

func (a *releaseAnalyzer) nextVersionPlan(
	ctx context.Context,
	target config.ResolvedTarget,
	commits []commit.Commit,
	currentVersion string,
	bumpType commit.BumpType,
) (string, commit.BumpType, bool, error) {
	strategy := a.core.strategies[target.ID]
	current := currentVersionWithInitial(strategy, currentVersion)

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
