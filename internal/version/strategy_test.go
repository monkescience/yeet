package version_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/version"
)

func TestSemVerNextRelease(t *testing.T) {
	t.Parallel()

	sv := &version.SemVer{Prefix: "v"}

	for _, testCase := range []struct {
		name          string
		current       string
		bump          commit.BumpType
		releaseAs     string
		identifier    string
		expected      string
		expectedBump  commit.BumpType
		shouldRelease bool
	}{
		{
			name:          "release-as wins over the commit bump",
			current:       "1.2.3",
			bump:          commit.BumpPatch,
			releaseAs:     "2.0.0",
			expected:      "2.0.0",
			expectedBump:  commit.BumpMajor,
			shouldRelease: true,
		},
		{
			name:          "release-as opens the channel at its first prerelease",
			current:       "1.2.3",
			bump:          commit.BumpPatch,
			releaseAs:     "2.0.0",
			identifier:    "beta",
			expected:      "2.0.0-beta.1",
			expectedBump:  commit.BumpMajor,
			shouldRelease: true,
		},
		{
			name:          "release-as advances past a prerelease current",
			current:       "1.3.0-beta.1",
			bump:          commit.BumpPatch,
			releaseAs:     "1.4.0",
			expected:      "1.4.0",
			expectedBump:  commit.BumpMinor,
			shouldRelease: true,
		},
		{
			name:          "release-as in a channel compares against the stable base",
			current:       "1.3.0-beta.1",
			bump:          commit.BumpPatch,
			releaseAs:     "1.4.0",
			identifier:    "beta",
			expected:      "1.4.0-beta.1",
			expectedBump:  commit.BumpMinor,
			shouldRelease: true,
		},
		{
			name:         "nothing is due without a bump",
			current:      "1.2.3",
			bump:         commit.BumpNone,
			expectedBump: commit.BumpNone,
		},
		{
			name:          "a stable version advances",
			current:       "1.2.3",
			bump:          commit.BumpPatch,
			expected:      "1.2.4",
			expectedBump:  commit.BumpPatch,
			shouldRelease: true,
		},
		{
			name:          "a stable version enters the channel at its first prerelease",
			current:       "1.2.3",
			bump:          commit.BumpMinor,
			identifier:    "beta",
			expected:      "1.3.0-beta.1",
			expectedBump:  commit.BumpMinor,
			shouldRelease: true,
		},
		{
			name:          "a prerelease in the channel advances its counter",
			current:       "1.3.0-beta.1",
			bump:          commit.BumpMinor,
			identifier:    "beta",
			expected:      "1.3.0-beta.2",
			expectedBump:  commit.BumpMinor,
			shouldRelease: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a semver target at testCase.current
			// when: the next release is resolved
			next, bump, shouldRelease, err := sv.NextRelease(
				testCase.current,
				testCase.bump,
				testCase.releaseAs,
				testCase.identifier,
			)

			// then: the scheme reports the version, the bump it implies and whether it is due
			testastic.NoError(t, err)
			testastic.Equal(t, testCase.expected, next)
			testastic.Equal(t, testCase.expectedBump, bump)
			testastic.Equal(t, testCase.shouldRelease, shouldRelease)
		})
	}
}

func TestSemVerNextReleaseErrors(t *testing.T) {
	t.Parallel()

	sv := &version.SemVer{Prefix: "v"}

	t.Run("rejects a release-as that does not advance", func(t *testing.T) {
		t.Parallel()

		// given: a footer asking for a version already released
		// when: the next release is resolved
		_, _, _, err := sv.NextRelease("1.2.3", commit.BumpPatch, "1.0.0", "")

		// then: the footer is refused with the scheme's own wording
		testastic.ErrorIs(t, err, version.ErrInvalidReleaseAs)
		testastic.Contains(t, err.Error(), "must be greater than current version 1.2.3")
	})

	t.Run("rejects a release-as that is not stable", func(t *testing.T) {
		t.Parallel()

		// given: a footer asking for a prerelease version
		// when: the next release is resolved
		_, _, _, err := sv.NextRelease("1.2.3", commit.BumpPatch, "2.0.0-rc.1", "")

		// then: the footer is refused
		testastic.ErrorIs(t, err, version.ErrInvalidReleaseAs)
		testastic.Contains(t, err.Error(), "must be a stable version")
	})

	t.Run("reports a current version it cannot parse", func(t *testing.T) {
		t.Parallel()

		// given: a boundary version that is not semver
		// when: the next release is resolved
		_, _, _, err := sv.NextRelease("garbage", commit.BumpPatch, "", "")

		// then: the failure names the step that could not read it
		testastic.Contains(t, err.Error(), "calculate next version: invalid version: garbage")
	})

	t.Run("reports a current version it cannot parse inside a channel", func(t *testing.T) {
		t.Parallel()

		// given: a boundary version that is not semver and an active channel
		// when: the next release is resolved
		_, _, _, err := sv.NextRelease("garbage", commit.BumpPatch, "", "beta")

		// then: the prerelease path names the value it could not read
		testastic.Contains(t, err.Error(), `parse current prerelease version "garbage"`)
	})
}

func TestSemVerPrereleaseAllowed(t *testing.T) {
	t.Parallel()

	sv := &version.SemVer{Prefix: "v"}

	t.Run("admits stable versions when no channel is active", func(t *testing.T) {
		t.Parallel()

		// given: no prerelease identifier
		// when: versions are offered to the channel
		// then: only the stable one belongs to it
		testastic.Equal(t, true, sv.PrereleaseAllowed("1.2.3", ""))
		testastic.Equal(t, false, sv.PrereleaseAllowed("1.2.3-beta.1", ""))
	})

	t.Run("admits a prerelease of the active channel only", func(t *testing.T) {
		t.Parallel()

		// given: the beta channel
		// when: versions are offered to it
		// then: stable versions and its own prereleases belong to it
		testastic.Equal(t, true, sv.PrereleaseAllowed("1.2.3", "beta"))
		testastic.Equal(t, true, sv.PrereleaseAllowed("1.3.0-beta.2", "beta"))
		testastic.Equal(t, false, sv.PrereleaseAllowed("1.3.0-alpha.2", "beta"))
		testastic.Equal(t, false, sv.PrereleaseAllowed("garbage", "beta"))
	})
}

func TestSemVerSupportedReleaseControls(t *testing.T) {
	t.Parallel()

	sv := &version.SemVer{Prefix: "v"}

	// given: a semver target
	// when: its release controls are queried
	// then: it honours both a Release-As footer and a prerelease channel
	testastic.Equal(t, true, sv.SupportsReleaseAs())
	testastic.Equal(t, true, sv.SupportsPrerelease())
}

func TestCalVerReleaseControls(t *testing.T) {
	t.Parallel()

	cv := &version.CalVer{Format: "YY.MM.MICRO", Prefix: "v"}

	t.Run("supports neither release-as nor a prerelease channel", func(t *testing.T) {
		t.Parallel()

		// given: a calver target
		// when: its release controls are queried
		// then: both are refused, so callers never offer them
		testastic.Equal(t, false, cv.SupportsReleaseAs())
		testastic.Equal(t, false, cv.SupportsPrerelease())
	})

	t.Run("refuses to normalize a release-as value", func(t *testing.T) {
		t.Parallel()

		// given: a footer asking for an explicit version
		// when: the scheme is asked to canonicalize it
		_, err := cv.NormalizeReleaseAs("2.0.0")

		// then: the scheme reports that it has nothing to act on
		testastic.ErrorIs(t, err, version.ErrInvalidReleaseAs)
		testastic.Contains(t, err.Error(), `calver targets do not support "2.0.0"`)
	})

	t.Run("ignores release controls it does not support", func(t *testing.T) {
		t.Parallel()

		// given: a bump offered alongside a release-as value and a channel identifier
		// when: the next release is resolved both with and without them
		withControls, withBump, withShould, err := cv.NextRelease("24.01.0", commit.BumpPatch, "9.9.9", "beta")
		testastic.NoError(t, err)

		plain, plainBump, plainShould, err := cv.NextRelease("24.01.0", commit.BumpPatch, "", "")
		testastic.NoError(t, err)

		// then: the clock alone decides, so the extra arguments change nothing
		testastic.Equal(t, plain, withControls)
		testastic.Equal(t, plainBump, withBump)
		testastic.Equal(t, plainShould, withShould)
		testastic.Equal(t, true, withShould)
	})

	t.Run("reports nothing due without a bump", func(t *testing.T) {
		t.Parallel()

		// given: no bump
		// when: the next release is resolved
		next, bump, shouldRelease, err := cv.NextRelease("24.01.0", commit.BumpNone, "", "")

		// then: no release is due
		testastic.NoError(t, err)
		testastic.Equal(t, "", next)
		testastic.Equal(t, commit.BumpNone, bump)
		testastic.Equal(t, false, shouldRelease)
	})

	t.Run("claims a version only while no channel is active", func(t *testing.T) {
		t.Parallel()

		// given: a scheme that has no prerelease concept
		// when: versions are offered to a channel
		// then: it answers by identifier alone, which is why callers ask
		// SupportsPrerelease first
		testastic.Equal(t, true, cv.PrereleaseAllowed("24.01.0", ""))
		testastic.Equal(t, false, cv.PrereleaseAllowed("24.01.0", "beta"))
	})
}
