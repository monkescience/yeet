package version_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/version"
)

func TestSemVerCurrent(t *testing.T) {
	t.Parallel()

	sv := version.NewSemVer(version.SemVerOptions{Prefix: "v"})

	t.Run("parses valid tag", func(t *testing.T) {
		t.Parallel()

		// given: a valid semver tag
		tag := "v1.2.3"

		// when: parsing current version
		v, err := sv.Current(tag)

		// then: version is extracted
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.3", v)
	})

	t.Run("parses tag without prefix", func(t *testing.T) {
		t.Parallel()

		// given: a semver with no matching prefix
		noPrefixSV := version.NewSemVer(version.SemVerOptions{Prefix: ""})

		// when: parsing current version
		v, err := noPrefixSV.Current("1.0.0")

		// then: version is extracted
		testastic.NoError(t, err)
		testastic.Equal(t, "1.0.0", v)
	})

	t.Run("rejects invalid tag", func(t *testing.T) {
		t.Parallel()

		// given: an invalid tag
		tag := "vnot-a-version"

		// when: parsing current version
		_, err := sv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects non-strict tag", func(t *testing.T) {
		t.Parallel()

		// given: a semver tag missing patch segment
		tag := "v1.2"

		// when: parsing current version
		_, err := sv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})
}

func TestSemVerNext(t *testing.T) {
	t.Parallel()

	sv := version.NewSemVer(version.SemVerOptions{
		Prefix:                     "v",
		PreMajorBreakingBumpsMinor: true,
		PreMajorFeaturesBumpPatch:  true,
	})

	t.Run("major bump", func(t *testing.T) {
		t.Parallel()

		// given: current version 1.2.3
		// when: applying a major bump
		next, err := sv.Next("1.2.3", commit.BumpMajor)

		// then: major version increments, minor and patch reset
		testastic.NoError(t, err)
		testastic.Equal(t, "2.0.0", next)
	})

	t.Run("major bump before 1.0.0 becomes minor", func(t *testing.T) {
		t.Parallel()

		// given: current version 0.4.2
		// when: applying a major bump
		next, err := sv.Next("0.4.2", commit.BumpMajor)

		// then: minor version increments, patch resets
		testastic.NoError(t, err)
		testastic.Equal(t, "0.5.0", next)
	})

	t.Run("minor bump", func(t *testing.T) {
		t.Parallel()

		// given: current version 1.2.3
		// when: applying a minor bump
		next, err := sv.Next("1.2.3", commit.BumpMinor)

		// then: minor version increments, patch resets
		testastic.NoError(t, err)
		testastic.Equal(t, "1.3.0", next)
	})

	t.Run("minor bump before 1.0.0 becomes patch", func(t *testing.T) {
		t.Parallel()

		// given: current version 0.4.2
		// when: applying a minor bump
		next, err := sv.Next("0.4.2", commit.BumpMinor)

		// then: patch version increments
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.3", next)
	})

	t.Run("patch bump", func(t *testing.T) {
		t.Parallel()

		// given: current version 1.2.3
		// when: applying a patch bump
		next, err := sv.Next("1.2.3", commit.BumpPatch)

		// then: patch version increments
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.4", next)
	})

	t.Run("patch bump before 1.0.0 stays patch", func(t *testing.T) {
		t.Parallel()

		// given: current version 0.4.2
		// when: applying a patch bump
		next, err := sv.Next("0.4.2", commit.BumpPatch)

		// then: patch version increments
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.3", next)
	})

	t.Run("no bump returns same", func(t *testing.T) {
		t.Parallel()

		// given: current version 1.2.3
		// when: applying no bump
		next, err := sv.Next("1.2.3", commit.BumpNone)

		// then: version unchanged
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.3", next)
	})

	t.Run("initial version bump", func(t *testing.T) {
		t.Parallel()

		// given: initial version 0.0.0
		// when: applying a minor bump
		next, err := sv.Next("0.0.0", commit.BumpMinor)

		// then: version becomes 0.0.1
		testastic.NoError(t, err)
		testastic.Equal(t, "0.0.1", next)
	})

	t.Run("invalid version", func(t *testing.T) {
		t.Parallel()

		// given: an invalid version string
		// when: applying a bump
		_, err := sv.Next("invalid", commit.BumpPatch)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects non-strict current version", func(t *testing.T) {
		t.Parallel()

		// given: a version missing patch segment
		// when: applying a bump
		_, err := sv.Next("1.2", commit.BumpPatch)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})
}

func TestSemVerPreMajorOptions(t *testing.T) {
	t.Parallel()

	t.Run("breaking bumps major when pre_major_breaking_bumps_minor disabled", func(t *testing.T) {
		t.Parallel()

		// given: pre_major_breaking_bumps_minor is false
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                     "v",
			PreMajorBreakingBumpsMinor: false,
			PreMajorFeaturesBumpPatch:  true,
		})

		// when: applying a major bump before 1.0.0
		next, err := sv.Next("0.4.2", commit.BumpMajor)

		// then: major version increments normally
		testastic.NoError(t, err)
		testastic.Equal(t, "1.0.0", next)
	})

	t.Run("features still bump patch when only breaking scaling disabled", func(t *testing.T) {
		t.Parallel()

		// given: pre_major_breaking_bumps_minor is false, pre_major_features_bump_patch is true
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                     "v",
			PreMajorBreakingBumpsMinor: false,
			PreMajorFeaturesBumpPatch:  true,
		})

		// when: applying a minor bump before 1.0.0
		next, err := sv.Next("0.4.2", commit.BumpMinor)

		// then: features still bump patch
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.3", next)
	})

	t.Run("features bump minor when pre_major_features_bump_patch disabled", func(t *testing.T) {
		t.Parallel()

		// given: pre_major_features_bump_patch is false
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                     "v",
			PreMajorBreakingBumpsMinor: true,
			PreMajorFeaturesBumpPatch:  false,
		})

		// when: applying a minor bump before 1.0.0
		next, err := sv.Next("0.4.2", commit.BumpMinor)

		// then: minor version increments normally
		testastic.NoError(t, err)
		testastic.Equal(t, "0.5.0", next)
	})

	t.Run("breaking still bumps minor when only features scaling disabled", func(t *testing.T) {
		t.Parallel()

		// given: pre_major_breaking_bumps_minor is true, pre_major_features_bump_patch is false
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                     "v",
			PreMajorBreakingBumpsMinor: true,
			PreMajorFeaturesBumpPatch:  false,
		})

		// when: applying a major bump before 1.0.0
		next, err := sv.Next("0.4.2", commit.BumpMajor)

		// then: breaking changes bump minor
		testastic.NoError(t, err)
		testastic.Equal(t, "0.5.0", next)
	})

	t.Run("both disabled allows standard semver before 1.0.0", func(t *testing.T) {
		t.Parallel()

		// given: both pre-major options disabled
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                     "v",
			PreMajorBreakingBumpsMinor: false,
			PreMajorFeaturesBumpPatch:  false,
		})

		// when: applying a major bump before 1.0.0
		next, err := sv.Next("0.4.2", commit.BumpMajor)

		// then: major version increments normally
		testastic.NoError(t, err)
		testastic.Equal(t, "1.0.0", next)
	})

	t.Run("both disabled features bump minor before 1.0.0", func(t *testing.T) {
		t.Parallel()

		// given: both pre-major options disabled
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                     "v",
			PreMajorBreakingBumpsMinor: false,
			PreMajorFeaturesBumpPatch:  false,
		})

		// when: applying a minor bump before 1.0.0
		next, err := sv.Next("0.4.2", commit.BumpMinor)

		// then: minor version increments normally
		testastic.NoError(t, err)
		testastic.Equal(t, "0.5.0", next)
	})

	t.Run("options have no effect after 1.0.0", func(t *testing.T) {
		t.Parallel()

		// given: both pre-major options enabled, version >= 1.0.0
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                     "v",
			PreMajorBreakingBumpsMinor: true,
			PreMajorFeaturesBumpPatch:  true,
		})

		// when: applying a major bump after 1.0.0
		next, err := sv.Next("1.2.3", commit.BumpMajor)

		// then: standard semver applies
		testastic.NoError(t, err)
		testastic.Equal(t, "2.0.0", next)
	})

	t.Run("scaled breaking change stays inside a minor prerelease line", func(t *testing.T) {
		t.Parallel()

		// given: breaking changes bump minor before 1.0.0
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                     "v",
			PreMajorBreakingBumpsMinor: true,
		})

		// when: a breaking change lands on a minor prerelease line and on a patch prerelease line
		minorLine, _, _, minorErr := sv.NextRelease("0.5.0-beta.1", commit.BumpMajor, "", "beta")
		patchLine, _, _, patchErr := sv.NextRelease("0.4.3-beta.1", commit.BumpMajor, "", "beta")

		// then: the minor line advances its counter and the patch line opens the next minor line
		testastic.NoError(t, minorErr)
		testastic.Equal(t, "0.5.0-beta.2", minorLine)
		testastic.NoError(t, patchErr)
		testastic.Equal(t, "0.5.0-beta.1", patchLine)
	})

	t.Run("scaled feature stays inside a patch prerelease line", func(t *testing.T) {
		t.Parallel()

		// given: features bump patch before 1.0.0
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                    "v",
			PreMajorFeaturesBumpPatch: true,
		})

		// when: a feature lands on a patch prerelease line
		next, _, _, err := sv.NextRelease("0.5.1-beta.1", commit.BumpMinor, "", "beta")

		// then: the patch line advances its counter
		testastic.NoError(t, err)
		testastic.Equal(t, "0.5.1-beta.2", next)
	})

	t.Run("a 0.0.0 prerelease line opens the line the bump requires", func(t *testing.T) {
		t.Parallel()

		// given: breaking changes bump minor before 1.0.0
		sv := version.NewSemVer(version.SemVerOptions{
			Prefix:                     "v",
			PreMajorBreakingBumpsMinor: true,
		})

		// when: a fix and a breaking change land on a 0.0.0 prerelease line
		fixLine, _, _, fixErr := sv.NextRelease("0.0.0-beta.1", commit.BumpPatch, "", "beta")
		breakingLine, _, _, breakingErr := sv.NextRelease("0.0.0-beta.1", commit.BumpMajor, "", "beta")

		// then: each opens the line its bump requires from 0.0.0
		testastic.NoError(t, fixErr)
		testastic.Equal(t, "0.0.1-beta.1", fixLine)
		testastic.NoError(t, breakingErr)
		testastic.Equal(t, "0.1.0-beta.1", breakingLine)
	})
}

func TestSemVerInitialVersion(t *testing.T) {
	t.Parallel()

	// given: a semver strategy
	sv := version.NewSemVer(version.SemVerOptions{Prefix: "v"})

	// when: getting initial version
	v := sv.InitialVersion()

	// then: returns 0.0.0
	testastic.Equal(t, "0.0.0", v)
}
