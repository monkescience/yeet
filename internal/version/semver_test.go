package version_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/version"
)

func TestSemVerCurrent(t *testing.T) {
	t.Parallel()

	sv := &version.SemVer{Prefix: "v"}

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
		noPrefixSV := &version.SemVer{Prefix: ""}

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

	sv := &version.SemVer{Prefix: "v"}

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

func TestSemVerTag(t *testing.T) {
	t.Parallel()

	// given: a semver strategy with prefix
	sv := &version.SemVer{Prefix: "v"}

	// when: generating a tag
	tag := sv.Tag("1.2.3")

	// then: tag has the prefix
	testastic.Equal(t, "v1.2.3", tag)
}

func TestSemVerInitialVersion(t *testing.T) {
	t.Parallel()

	// given: a semver strategy
	sv := &version.SemVer{Prefix: "v"}

	// when: getting initial version
	v := sv.InitialVersion()

	// then: returns 0.0.0
	testastic.Equal(t, "0.0.0", v)
}
