package version_test

import (
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/version"
)

func fixedTime(year int, month time.Month) func() time.Time {
	return func() time.Time {
		return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	}
}

func TestCalVerCurrent(t *testing.T) {
	t.Parallel()

	cv := &version.CalVer{Prefix: "v"}

	t.Run("parses valid tag", func(t *testing.T) {
		t.Parallel()

		// given: a valid calver tag
		tag := "v2026.02.1"

		// when: parsing current version
		v, err := cv.Current(tag)

		// then: version is extracted
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.1", v)
	})

	t.Run("rejects invalid format", func(t *testing.T) {
		t.Parallel()

		// given: an invalid calver tag
		tag := "v1.2"

		// when: parsing current version
		_, err := cv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects non-numeric micro", func(t *testing.T) {
		t.Parallel()

		// given: a tag with non-numeric micro
		tag := "v2026.02.abc"

		// when: parsing current version
		_, err := cv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
	})
}

func TestCalVerNext(t *testing.T) {
	t.Parallel()

	t.Run("first release of the month", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy set to Feb 2026
		cv := &version.CalVer{
			Prefix: "v",
			Now:    fixedTime(2026, time.February),
		}

		// when: calculating next from an empty version
		next, err := cv.Next("", commit.BumpMinor)

		// then: first release of the month
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.1", next)
	})

	t.Run("increment within same month", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy with existing version in same month
		cv := &version.CalVer{
			Prefix: "v",
			Now:    fixedTime(2026, time.February),
		}

		// when: calculating next from existing version in same month
		next, err := cv.Next("2026.02.3", commit.BumpPatch)

		// then: micro increments
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.4", next)
	})

	t.Run("new month resets micro", func(t *testing.T) {
		t.Parallel()

		// given: current version from January, now it's February
		cv := &version.CalVer{
			Prefix: "v",
			Now:    fixedTime(2026, time.February),
		}

		// when: calculating next
		next, err := cv.Next("2026.01.5", commit.BumpPatch)

		// then: micro resets to 1
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.1", next)
	})

	t.Run("no bump returns same", func(t *testing.T) {
		t.Parallel()

		// given: current version
		cv := &version.CalVer{
			Prefix: "v",
			Now:    fixedTime(2026, time.February),
		}

		// when: applying no bump
		next, err := cv.Next("2026.02.1", commit.BumpNone)

		// then: version unchanged
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.1", next)
	})

	t.Run("new year resets", func(t *testing.T) {
		t.Parallel()

		// given: current version from last year
		cv := &version.CalVer{
			Prefix: "v",
			Now:    fixedTime(2027, time.January),
		}

		// when: calculating next
		next, err := cv.Next("2026.12.7", commit.BumpMinor)

		// then: new year, new month, micro resets
		testastic.NoError(t, err)
		testastic.Equal(t, "2027.01.1", next)
	})
}
