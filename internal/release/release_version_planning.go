package release

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
)

func shortHash(hash string, length int) (string, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "", fmt.Errorf("%w: empty commit hash", ErrInvalidPreviewHashLength)
	}

	if length <= 0 {
		return "", fmt.Errorf("%w: got %d", ErrInvalidPreviewHashLength, length)
	}

	if len(hash) <= length {
		return hash, nil
	}

	return hash[:length], nil
}

func (r *Releaser) resolveNextVersion(
	current string,
	bump commit.BumpType,
	releaseAsVersion string,
) (string, commit.BumpType, bool, error) {
	if releaseAsVersion != "" && r.cfg.Versioning == config.VersioningSemver {
		nextVersion, overrideBump, err := applyReleaseAs(current, releaseAsVersion)
		if err != nil {
			return "", commit.BumpNone, false, err
		}

		return nextVersion, overrideBump, true, nil
	}

	if bump == commit.BumpNone {
		return "", bump, false, nil
	}

	nextVersion, err := r.strategy.strategy.Next(current, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next version: %w", err)
	}

	return nextVersion, bump, true, nil
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
		return "", fmt.Errorf("%w: invalid version %q: %w", ErrInvalidReleaseAs, releaseAsVersion, err)
	}

	return v.String(), nil
}

func applyReleaseAs(current, releaseAsVersion string) (string, commit.BumpType, error) {
	targetVersion, err := semver.StrictNewVersion(releaseAsVersion)
	if err != nil {
		return "", commit.BumpNone, fmt.Errorf("%w: invalid version %q: %w", ErrInvalidReleaseAs, releaseAsVersion, err)
	}

	if targetVersion.Prerelease() != "" || targetVersion.Metadata() != "" {
		return "", commit.BumpNone, fmt.Errorf("%w: %q must be a stable version", ErrInvalidReleaseAs, releaseAsVersion)
	}

	currentVersion, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", commit.BumpNone, fmt.Errorf("%w: parse current version %q: %w", ErrInvalidReleaseAs, current, err)
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

func inferSemverBump(currentVersion, targetVersion *semver.Version) commit.BumpType {
	if targetVersion.Major() > currentVersion.Major() {
		return commit.BumpMajor
	}

	if targetVersion.Minor() > currentVersion.Minor() {
		return commit.BumpMinor
	}

	return commit.BumpPatch
}
