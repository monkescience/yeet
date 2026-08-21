package version

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/commit"
)

var (
	ErrInvalidVersion = errors.New("invalid version")
	// ErrInvalidReleaseAs and ErrConflictingReleaseAs report a Release-As footer
	// a scheme cannot honour. Their wording is user facing.
	ErrInvalidReleaseAs     = errors.New("invalid release-as footer")
	ErrConflictingReleaseAs = errors.New("conflicting release-as footers")
)

var _ Strategy = (*SemVer)(nil)

// Strategy is one versioning scheme. Beyond reading and advancing a version it
// answers which release controls it supports, so release version decisions do
// not branch on the scheme.
type Strategy interface {
	Current(tag string) (string, error)
	Less(leftVersion, rightVersion, leftRef, rightRef string) bool
	InitialVersion() string
	// SupportsReleaseAs reports whether a Release-As footer can override the
	// next version. A scheme that says no has its footers ignored.
	SupportsReleaseAs() bool
	// SupportsPrerelease reports whether the scheme can run a prerelease channel.
	SupportsPrerelease() bool
	// NormalizeReleaseAs canonicalizes a Release-As footer value, wrapping
	// ErrInvalidReleaseAs when the value is not one this scheme accepts.
	NormalizeReleaseAs(value string) (string, error)
	// NextRelease resolves the version a release takes, honouring a Release-As
	// override and a prerelease channel where the scheme supports them. It
	// reports false when nothing is due.
	NextRelease(
		current string,
		bump commit.BumpType,
		releaseAs, prereleaseIdentifier string,
	) (string, commit.BumpType, bool, error)
	// PrereleaseAllowed reports whether a version belongs to the release channel
	// named by identifier. An empty identifier admits stable versions only.
	PrereleaseAllowed(version, identifier string) bool
}

// ValidatePrereleaseIdentifier reports whether identifier can name a prerelease
// channel.
func ValidatePrereleaseIdentifier(identifier string) error {
	_, err := semver.StrictNewVersion("1.0.0-" + identifier)
	if err != nil {
		return fmt.Errorf("invalid semver prerelease identifier %q: %w", identifier, err)
	}

	return nil
}

type SemVer struct {
	Prefix                     string
	PreMajorBreakingBumpsMinor bool
	PreMajorFeaturesBumpPatch  bool
}

func (s *SemVer) Current(tag string) (string, error) {
	cleaned := strings.TrimPrefix(tag, s.Prefix)

	v, err := semver.StrictNewVersion(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrInvalidVersion, tag, err)
	}

	return v.String(), nil
}

func (s *SemVer) Next(current string, bump commit.BumpType) (string, error) {
	v, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrInvalidVersion, current, err)
	}

	if v.Major() == 0 {
		switch bump {
		case commit.BumpMajor:
			if s.PreMajorBreakingBumpsMinor {
				bump = commit.BumpMinor
			}
		case commit.BumpMinor:
			if s.PreMajorFeaturesBumpPatch {
				bump = commit.BumpPatch
			}
		case commit.BumpPatch, commit.BumpNone:
		}
	}

	var next semver.Version

	switch bump {
	case commit.BumpMajor:
		next = v.IncMajor()
	case commit.BumpMinor:
		next = v.IncMinor()
	case commit.BumpPatch:
		next = v.IncPatch()
	case commit.BumpNone:
		return v.String(), nil
	default:
		return "", fmt.Errorf("%w: unknown bump type %q", ErrInvalidVersion, bump)
	}

	return next.String(), nil
}

func (s *SemVer) Less(leftVersion, rightVersion, leftRef, rightRef string) bool {
	leftSemver, err := semver.StrictNewVersion(leftVersion)
	if err != nil {
		return leftRef < rightRef
	}

	rightSemver, err := semver.StrictNewVersion(rightVersion)
	if err != nil {
		return leftRef < rightRef
	}

	if !leftSemver.Equal(rightSemver) {
		return leftSemver.LessThan(rightSemver)
	}

	return leftRef < rightRef
}

func (s *SemVer) InitialVersion() string {
	return "0.0.0"
}

func (s *SemVer) SupportsReleaseAs() bool {
	return true
}

func (s *SemVer) SupportsPrerelease() bool {
	return true
}

func (s *SemVer) NormalizeReleaseAs(value string) (string, error) {
	parsed, err := semver.StrictNewVersion(value)
	if err != nil {
		return "", fmt.Errorf("%w: invalid version %q: %v", ErrInvalidReleaseAs, value, err)
	}

	return parsed.String(), nil
}

func (s *SemVer) NextRelease(
	current string,
	bump commit.BumpType,
	releaseAs, prereleaseIdentifier string,
) (string, commit.BumpType, bool, error) {
	if prereleaseIdentifier != "" {
		return s.nextPrerelease(current, bump, releaseAs, prereleaseIdentifier)
	}

	if releaseAs != "" {
		next, overrideBump, err := s.applyReleaseAs(current, releaseAs)
		if err != nil {
			return "", commit.BumpNone, false, err
		}

		return next, overrideBump, true, nil
	}

	if bump == commit.BumpNone {
		return "", bump, false, nil
	}

	next, err := s.Next(current, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next version: %w", err)
	}

	return next, bump, true, nil
}

func (s *SemVer) PrereleaseAllowed(version, identifier string) bool {
	parsed, err := semver.StrictNewVersion(version)
	if err != nil {
		return false
	}

	prerelease := strings.TrimSpace(parsed.Prerelease())
	if identifier == "" {
		return prerelease == ""
	}

	if prerelease == "" {
		return true
	}

	return prerelease == identifier || strings.HasPrefix(prerelease, identifier+".")
}

func (s *SemVer) nextPrerelease(
	current string,
	bump commit.BumpType,
	releaseAs, prereleaseIdentifier string,
) (string, commit.BumpType, bool, error) {
	if releaseAs != "" {
		next, overrideBump, err := s.applyReleaseAs(stableBase(current), releaseAs)
		if err != nil {
			return "", commit.BumpNone, false, err
		}

		return firstPrerelease(next, prereleaseIdentifier), overrideBump, true, nil
	}

	if bump == commit.BumpNone {
		return "", bump, false, nil
	}

	nextPrerelease, ok, err := incrementPrerelease(current, prereleaseIdentifier)
	if ok || err != nil {
		return nextPrerelease, bump, ok, err
	}

	nextBase, err := s.Next(current, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next prerelease base version: %w", err)
	}

	return firstPrerelease(nextBase, prereleaseIdentifier), bump, true, nil
}

func (s *SemVer) applyReleaseAs(current, releaseAs string) (string, commit.BumpType, error) {
	target, err := semver.StrictNewVersion(releaseAs)
	if err != nil {
		return "", commit.BumpNone, fmt.Errorf("%w: invalid version %q: %v", ErrInvalidReleaseAs, releaseAs, err)
	}

	if target.Prerelease() != "" || target.Metadata() != "" {
		return "", commit.BumpNone, fmt.Errorf("%w: %q must be a stable version", ErrInvalidReleaseAs, releaseAs)
	}

	currentVersion, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", commit.BumpNone, fmt.Errorf("%w: parse current version %q: %v", ErrInvalidReleaseAs, current, err)
	}

	if !target.GreaterThan(currentVersion) {
		return "", commit.BumpNone, fmt.Errorf(
			"%w: %s must be greater than current version %s",
			ErrInvalidReleaseAs,
			target.String(),
			currentVersion.String(),
		)
	}

	return target.String(), inferBump(currentVersion, target), nil
}

func stableBase(version string) string {
	parsed, err := semver.StrictNewVersion(version)
	if err != nil {
		return version
	}

	return fmt.Sprintf("%d.%d.%d", parsed.Major(), parsed.Minor(), parsed.Patch())
}

func firstPrerelease(baseVersion, prereleaseIdentifier string) string {
	return baseVersion + "-" + prereleaseIdentifier + ".1"
}

func incrementPrerelease(current, prereleaseIdentifier string) (string, bool, error) {
	parsed, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", false, fmt.Errorf("parse current prerelease version %q: %w", current, err)
	}

	prerelease := parsed.Prerelease()
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

	return fmt.Sprintf("%s-%s.%d", stableBase(current), prereleaseIdentifier, counter+1), true, nil
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

func inferBump(currentVersion, targetVersion *semver.Version) commit.BumpType {
	if targetVersion.Major() > currentVersion.Major() {
		return commit.BumpMajor
	}

	if targetVersion.Minor() > currentVersion.Minor() {
		return commit.BumpMinor
	}

	return commit.BumpPatch
}
