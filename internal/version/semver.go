package version

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/commit"
)

var ErrInvalidVersion = errors.New("invalid version")

var _ Strategy = (*SemVer)(nil)

type Strategy interface {
	Current(tag string) (string, error)
	Next(current string, bump commit.BumpType) (string, error)
	Less(leftVersion, rightVersion, leftRef, rightRef string) bool
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

func (s *SemVer) Tag(version string) string {
	return s.Prefix + version
}

func (s *SemVer) InitialVersion() string {
	return "0.0.0"
}
