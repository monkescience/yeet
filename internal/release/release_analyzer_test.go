//nolint:testpackage // This test validates unexported release analyzer functions.
package release

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/version"
)

func TestParseCalVerVersion(t *testing.T) {
	t.Parallel()

	t.Run("valid version", func(t *testing.T) {
		t.Parallel()

		// given: a valid calver string
		raw := "2026.03.1"

		// when: parsing
		parts, err := parseCalVerVersion(raw)

		// then: all segments are parsed as integers
		testastic.NoError(t, err)
		testastic.Equal(t, [3]int{2026, 3, 1}, parts)
	})

	t.Run("too few segments", func(t *testing.T) {
		t.Parallel()

		// given: an invalid calver string with only two segments
		raw := "2026.03"

		// when: parsing
		_, err := parseCalVerVersion(raw)

		// then: error indicates invalid version
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("non-numeric segment", func(t *testing.T) {
		t.Parallel()

		// given: a calver string with a non-numeric micro
		raw := "2026.03.abc"

		// when: parsing
		_, err := parseCalVerVersion(raw)

		// then: error indicates invalid version
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("whitespace is trimmed", func(t *testing.T) {
		t.Parallel()

		// given: a version with surrounding whitespace
		raw := "  2026.03.5  "

		// when: parsing
		parts, err := parseCalVerVersion(raw)

		// then: whitespace is trimmed and version is parsed
		testastic.NoError(t, err)
		testastic.Equal(t, [3]int{2026, 3, 5}, parts)
	})
}

func TestCalVerVersionRefLess(t *testing.T) {
	t.Parallel()

	t.Run("different year", func(t *testing.T) {
		t.Parallel()

		// given: two versions with different years
		// when: comparing
		result := calVerVersionRefLess("2025.01.1", "2026.01.1", "ref-a", "ref-b")

		// then: earlier year is less
		testastic.True(t, result)
	})

	t.Run("same year different month", func(t *testing.T) {
		t.Parallel()

		// given: two versions with same year but different months
		// when: comparing
		result := calVerVersionRefLess("2026.02.1", "2026.03.1", "ref-a", "ref-b")

		// then: earlier month is less
		testastic.True(t, result)
	})

	t.Run("same year and month different micro", func(t *testing.T) {
		t.Parallel()

		// given: two versions with same year/month but different micro
		// when: comparing
		result := calVerVersionRefLess("2026.03.1", "2026.03.2", "ref-a", "ref-b")

		// then: smaller micro is less
		testastic.True(t, result)
	})

	t.Run("equal versions falls back to ref compare", func(t *testing.T) {
		t.Parallel()

		// given: two identical versions but different refs
		// when: comparing
		result := calVerVersionRefLess("2026.03.1", "2026.03.1", "ref-a", "ref-b")

		// then: falls back to string comparison of refs
		testastic.True(t, result)
	})

	t.Run("invalid left version falls back to ref compare", func(t *testing.T) {
		t.Parallel()

		// given: an invalid left version
		// when: comparing
		result := calVerVersionRefLess("invalid", "2026.03.1", "ref-a", "ref-b")

		// then: falls back to string comparison of refs
		testastic.True(t, result)
	})

	t.Run("invalid right version falls back to ref compare", func(t *testing.T) {
		t.Parallel()

		// given: an invalid right version
		// when: comparing
		result := calVerVersionRefLess("2026.03.1", "invalid", "ref-a", "ref-b")

		// then: falls back to string comparison of refs
		testastic.True(t, result)
	})

	t.Run("right is less than left", func(t *testing.T) {
		t.Parallel()

		// given: right version is earlier than left
		// when: comparing
		result := calVerVersionRefLess("2026.03.2", "2026.03.1", "ref-a", "ref-b")

		// then: left is not less than right
		testastic.False(t, result)
	})
}
