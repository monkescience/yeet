package release

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/version"
)

func currentVersionOrInitial(target config.ResolvedTarget) string {
	strategy := versionStrategyForResolvedTarget(target)
	if semverStrategy, ok := strategy.strategy.(*version.SemVer); ok {
		return semverStrategy.InitialVersion()
	}

	return ""
}

func versionStrategyForResolvedTarget(target config.ResolvedTarget) versionStrategy {
	var strategy version.Strategy

	switch target.Versioning {
	case config.VersioningCalVer:
		strategy = &version.CalVer{
			Format: target.CalVer.Format,
			Prefix: target.TagPrefix,
		}
	case config.VersioningSemver:
		strategy = &version.SemVer{
			Prefix:                     target.TagPrefix,
			PreMajorBreakingBumpsMinor: target.PreMajorBreakingBumpsMinor,
			PreMajorFeaturesBumpPatch:  target.PreMajorFeaturesBumpPatch,
		}
	}

	return versionStrategy{strategy: strategy, prefix: target.TagPrefix}
}

func currentVersionWithInitial(target config.ResolvedTarget, currentVersion string) string {
	if currentVersion != "" {
		return currentVersion
	}

	return currentVersionOrInitial(target)
}

func (a *releaseAnalyzer) nextVersionPlan(
	target config.ResolvedTarget,
	commits []commit.Commit,
	currentVersion string,
	bumpType commit.BumpType,
) (string, commit.BumpType, bool, error) {
	strategy := versionStrategyForResolvedTarget(target)

	releaseAsVersion, err := a.releaseAsVersion(target, commits)
	if err != nil {
		return "", commit.BumpNone, false, err
	}

	current := currentVersionWithInitial(target, currentVersion)

	return resolveNextVersion(
		strategy,
		target.Versioning,
		current,
		bumpType,
		releaseAsVersion,
		a.core.activePrereleaseIdentifier(),
	)
}

func resolveNextVersion(
	strategy versionStrategy,
	versioning config.VersioningStrategy,
	current string,
	bump commit.BumpType,
	releaseAsVersion string,
	prereleaseIdentifier string,
) (string, commit.BumpType, bool, error) {
	if prereleaseIdentifier != "" && versioning == config.VersioningSemver {
		return resolveNextPrereleaseVersion(strategy, current, bump, releaseAsVersion, prereleaseIdentifier)
	}

	if releaseAsVersion != "" && versioning == config.VersioningSemver {
		nextVersion, overrideBump, err := applyReleaseAs(current, releaseAsVersion)
		if err != nil {
			return "", commit.BumpNone, false, err
		}

		return nextVersion, overrideBump, true, nil
	}

	if bump == commit.BumpNone {
		return "", bump, false, nil
	}

	nextVersion, err := strategy.strategy.Next(current, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next version: %w", err)
	}

	return nextVersion, bump, true, nil
}

func (a *releaseAnalyzer) releaseAsVersion(target config.ResolvedTarget, commits []commit.Commit) (string, error) {
	if target.Versioning != config.VersioningSemver {
		return "", nil
	}

	return detectReleaseAs(commits)
}

func detectReleaseAs(commits []commit.Commit) (string, error) {
	releaseAsVersion := ""

	for _, c := range commits {
		for _, footer := range c.Footers {
			if !strings.EqualFold(strings.TrimSpace(footer.Key), "Release-As") {
				continue
			}

			candidate := strings.TrimSpace(footer.Value)
			if candidate == "" {
				return "", fmt.Errorf("%w: empty value", ErrInvalidReleaseAs)
			}

			normalizedCandidate, err := normalizeReleaseAsValue(candidate)
			if err != nil {
				return "", err
			}

			if releaseAsVersion == "" {
				releaseAsVersion = normalizedCandidate

				continue
			}

			if releaseAsVersion != normalizedCandidate {
				return "", fmt.Errorf("%w: %q and %q", ErrConflictingReleaseAs, releaseAsVersion, normalizedCandidate)
			}
		}
	}

	return releaseAsVersion, nil
}

func normalizeReleaseAsValue(releaseAsVersion string) (string, error) {
	v, err := semver.StrictNewVersion(releaseAsVersion)
	if err != nil {
		return "", fmt.Errorf("%w: invalid version %q: %v", ErrInvalidReleaseAs, releaseAsVersion, err)
	}

	return v.String(), nil
}

func applyReleaseAs(current, releaseAsVersion string) (string, commit.BumpType, error) {
	targetVersion, err := semver.StrictNewVersion(releaseAsVersion)
	if err != nil {
		return "", commit.BumpNone, fmt.Errorf("%w: invalid version %q: %v", ErrInvalidReleaseAs, releaseAsVersion, err)
	}

	if targetVersion.Prerelease() != "" || targetVersion.Metadata() != "" {
		return "", commit.BumpNone, fmt.Errorf("%w: %q must be a stable version", ErrInvalidReleaseAs, releaseAsVersion)
	}

	currentVersion, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", commit.BumpNone, fmt.Errorf("%w: parse current version %q: %v", ErrInvalidReleaseAs, current, err)
	}

	if !targetVersion.GreaterThan(currentVersion) {
		return "", commit.BumpNone, fmt.Errorf(
			"%w: %s must be greater than current version %s",
			ErrInvalidReleaseAs,
			targetVersion.String(),
			currentVersion.String(),
		)
	}

	bump := inferSemverBump(currentVersion, targetVersion)

	return targetVersion.String(), bump, nil
}

func resolveNextPrereleaseVersion(
	strategy versionStrategy,
	current string,
	bump commit.BumpType,
	releaseAsVersion string,
	prereleaseIdentifier string,
) (string, commit.BumpType, bool, error) {
	if releaseAsVersion != "" {
		nextVersion, overrideBump, err := applyReleaseAs(stableSemverBase(current), releaseAsVersion)
		if err != nil {
			return "", commit.BumpNone, false, err
		}

		return firstPrereleaseForBase(nextVersion, prereleaseIdentifier), overrideBump, true, nil
	}

	if bump == commit.BumpNone {
		return "", bump, false, nil
	}

	nextPrerelease, ok, err := incrementPrerelease(current, prereleaseIdentifier)
	if ok || err != nil {
		return nextPrerelease, bump, ok, err
	}

	nextBaseVersion, err := strategy.strategy.Next(current, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next prerelease base version: %w", err)
	}

	return firstPrereleaseForBase(nextBaseVersion, prereleaseIdentifier), bump, true, nil
}

func stableSemverBase(version string) string {
	parsedVersion, err := semver.StrictNewVersion(version)
	if err != nil {
		return version
	}

	return fmt.Sprintf("%d.%d.%d", parsedVersion.Major(), parsedVersion.Minor(), parsedVersion.Patch())
}

func firstPrereleaseForBase(baseVersion string, prereleaseIdentifier string) string {
	return baseVersion + "-" + prereleaseIdentifier + ".1"
}

func incrementPrerelease(current string, prereleaseIdentifier string) (string, bool, error) {
	parsedVersion, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", false, fmt.Errorf("parse current prerelease version %q: %w", current, err)
	}

	prerelease := parsedVersion.Prerelease()
	if prerelease == "" {
		return "", false, nil
	}

	counterText, found := strings.CutPrefix(prerelease, prereleaseIdentifier+".")
	if !found || counterText == "" {
		return "", false, nil
	}

	counter, err := parsePrereleaseCounter(counterText)
	if err != nil {
		return "", false, err
	}

	nextBase := stableSemverBase(current)

	return fmt.Sprintf("%s-%s.%d", nextBase, prereleaseIdentifier, counter+1), true, nil
}

func parsePrereleaseCounter(counterText string) (int64, error) {
	if counterText == "" {
		return 0, fmt.Errorf("%w: invalid prerelease counter %q", ErrInvalidReleaseAs, counterText)
	}

	for _, ch := range counterText {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("%w: invalid prerelease counter %q", ErrInvalidReleaseAs, counterText)
		}
	}

	var counter int64

	_, err := fmt.Sscanf(counterText, "%d", &counter)
	if err != nil || counter < 1 {
		return 0, fmt.Errorf("%w: invalid prerelease counter %q", ErrInvalidReleaseAs, counterText)
	}

	return counter, nil
}

func inferSemverBump(currentVersion, targetVersion *semver.Version) commit.BumpType {
	if targetVersion.Major() > currentVersion.Major() {
		return commit.BumpMajor
	}

	if targetVersion.Minor() > currentVersion.Minor() {
		return commit.BumpMinor
	}

	return commit.BumpPatch
}
