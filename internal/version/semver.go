package version

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/commit"
)

var (
	ErrInvalidVersion       = errors.New("invalid version")
	ErrInvalidReleaseAs     = errors.New("invalid release-as footer")
	ErrConflictingReleaseAs = errors.New("conflicting release-as footers")
)

var _ Strategy = (*SemVer)(nil)

type Strategy interface {
	Current(tag string) (string, error)
	OrderedRefs(refs []string, prereleaseIdentifier string) []string
	Tag(version string) string
	InitialVersion() string
	SupportsReleaseAs() bool
	SupportsPrerelease() bool
	NormalizeReleaseAs(value string) (string, error)
	NextRelease(
		current string,
		bump commit.BumpType,
		releaseAs, prereleaseIdentifier string,
	) (string, commit.BumpType, bool, error)
	PrereleaseAllowed(version, identifier string) bool
}

func ValidatePrereleaseIdentifier(identifier string) error {
	_, err := semver.StrictNewVersion("1.0.0-" + identifier)
	if err != nil {
		return fmt.Errorf("invalid semver prerelease identifier %q: %w", identifier, err)
	}

	return nil
}

type SemVerOptions struct {
	Prefix                     string
	PreMajorBreakingBumpsMinor bool
	PreMajorFeaturesBumpPatch  bool
}

type SemVer struct {
	prefix                     string
	preMajorBreakingBumpsMinor bool
	preMajorFeaturesBumpPatch  bool
}

func NewSemVer(options SemVerOptions) *SemVer {
	return &SemVer{
		prefix:                     options.Prefix,
		preMajorBreakingBumpsMinor: options.PreMajorBreakingBumpsMinor,
		preMajorFeaturesBumpPatch:  options.PreMajorFeaturesBumpPatch,
	}
}

func (s *SemVer) Current(tag string) (string, error) {
	v, err := s.parseRef(tag)
	if err != nil {
		return "", err
	}

	return v.String(), nil
}

func (s *SemVer) Tag(version string) string {
	return s.prefix + version
}

func (s *SemVer) OrderedRefs(refs []string, prereleaseIdentifier string) []string {
	return orderedRefs(refs, s.parseRef,
		func(parsed *semver.Version) bool { return semverPrereleaseAllowed(parsed, prereleaseIdentifier) },
		func(left, right *semver.Version) int { return left.Compare(right) })
}

func (s *SemVer) Next(current string, bump commit.BumpType) (string, error) {
	v, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrInvalidVersion, current, err)
	}

	var next semver.Version

	switch s.effectiveBump(v, bump) {
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
		return "", releaseAsError(value, "", "requested version is not valid semver",
			fmt.Errorf("%w: invalid version %q: %v", ErrInvalidReleaseAs, value, err))
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

	return semverPrereleaseAllowed(parsed, identifier)
}

func (s *SemVer) effectiveBump(v *semver.Version, bump commit.BumpType) commit.BumpType {
	if v.Major() != 0 {
		return bump
	}

	switch bump {
	case commit.BumpMajor:
		if s.preMajorBreakingBumpsMinor {
			return commit.BumpMinor
		}
	case commit.BumpMinor:
		if s.preMajorFeaturesBumpPatch {
			return commit.BumpPatch
		}
	case commit.BumpPatch, commit.BumpNone:
	}

	return bump
}

func (s *SemVer) parseRef(tag string) (*semver.Version, error) {
	v, err := semver.StrictNewVersion(strings.TrimPrefix(tag, s.prefix))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidVersion, tag, err)
	}

	return v, nil
}

func semverPrereleaseAllowed(parsed *semver.Version, identifier string) bool {
	return prereleaseAllowed(parsed.Prerelease(), identifier)
}

func prereleaseAllowed(prerelease, identifier string) bool {
	prerelease = strings.TrimSpace(prerelease)
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

	parsed, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("parse current prerelease version %q: %w", current, err)
	}

	nextPrerelease, ok, err := incrementPrerelease(parsed, prereleaseIdentifier)
	if err != nil {
		return "", commit.BumpNone, false, err
	}

	base := current
	if ok {
		base = versionCore(parsed)

		if s.prereleaseBaseCovers(parsed, bump) {
			return nextPrerelease, bump, true, nil
		}
	}

	nextBase, err := s.Next(base, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next prerelease base version: %w", err)
	}

	return firstPrerelease(nextBase, prereleaseIdentifier), bump, true, nil
}

func (s *SemVer) applyReleaseAs(current, releaseAs string) (string, commit.BumpType, error) {
	target, err := semver.StrictNewVersion(releaseAs)
	if err != nil {
		return "", commit.BumpNone, releaseAsError(releaseAs, current, "requested version is not valid semver",
			fmt.Errorf("%w: invalid version %q: %v", ErrInvalidReleaseAs, releaseAs, err))
	}

	if target.Prerelease() != "" || target.Metadata() != "" {
		return "", commit.BumpNone, releaseAsError(releaseAs, current, "requested version must be stable",
			fmt.Errorf("%w: %q must be a stable version", ErrInvalidReleaseAs, releaseAs))
	}

	currentVersion, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", commit.BumpNone, releaseAsError(releaseAs, current, "current version is not valid semver",
			fmt.Errorf("%w: parse current version %q: %v", ErrInvalidReleaseAs, current, err))
	}

	if !target.GreaterThan(currentVersion) {
		return "", commit.BumpNone, releaseAsError(target.String(), currentVersion.String(),
			"requested version is not newer than the current version",
			fmt.Errorf(
				"%w: %s must be greater than current version %s",
				ErrInvalidReleaseAs,
				target.String(),
				currentVersion.String(),
			))
	}

	return target.String(), inferBump(currentVersion, target), nil
}

func (s *SemVer) prereleaseBaseCovers(v *semver.Version, bump commit.BumpType) bool {
	return commit.CompareBump(s.effectiveBump(v, bump), baseBump(v)) <= 0
}

func baseBump(v *semver.Version) commit.BumpType {
	switch {
	case v.Patch() != 0:
		return commit.BumpPatch
	case v.Minor() != 0:
		return commit.BumpMinor
	case v.Major() != 0:
		return commit.BumpMajor
	default:
		return commit.BumpNone
	}
}

func stableBase(version string) string {
	parsed, err := semver.StrictNewVersion(version)
	if err != nil {
		return version
	}

	return versionCore(parsed)
}

func versionCore(v *semver.Version) string {
	return fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch())
}

func firstPrerelease(baseVersion, prereleaseIdentifier string) string {
	return baseVersion + "-" + prereleaseIdentifier + ".1"
}

func incrementPrerelease(current *semver.Version, prereleaseIdentifier string) (string, bool, error) {
	counter, ok, err := nextPrereleaseCounter(current.Prerelease(), prereleaseIdentifier)
	if err != nil || !ok {
		return "", false, err
	}

	return fmt.Sprintf("%s-%s.%d", versionCore(current), prereleaseIdentifier, counter), true, nil
}

// nextPrereleaseCounter returns the counter after the one in prerelease, such as 3 for beta.2.
// It reports false when prerelease has no counter for the identifier.
func nextPrereleaseCounter(prerelease, prereleaseIdentifier string) (int64, bool, error) {
	counterText, found := strings.CutPrefix(prerelease, prereleaseIdentifier+".")
	if !found || counterText == "" {
		return 0, false, nil
	}

	counter, err := parsePrereleaseCounter(counterText)
	if err != nil {
		return 0, false, err
	}

	return counter + 1, true, nil
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

type ReleaseAsError struct {
	Requested   string
	Current     string
	Conflicting string
	Problem     string
	cause       error
}

func (e *ReleaseAsError) Error() string { return e.cause.Error() }

func (e *ReleaseAsError) Unwrap() error { return e.cause }

func releaseAsError(requested, current, problem string, cause error) *ReleaseAsError {
	return &ReleaseAsError{Requested: requested, Current: current, Problem: problem, cause: cause}
}

func NewReleaseAsError(requested, current, problem string, cause error) error {
	return releaseAsError(requested, current, problem, cause)
}

func NewConflictingReleaseAsError(requested, current, conflicting, problem string, cause error) error {
	err := releaseAsError(requested, current, problem, cause)
	err.Conflicting = conflicting

	return err
}
