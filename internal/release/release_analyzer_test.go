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
		result := calVerVersionRefLess("YYYY.0M.MICRO", "2025.01.1", "2026.01.1", "ref-a", "ref-b")

		// then: earlier year is less
		testastic.True(t, result)
	})

	t.Run("same year different month", func(t *testing.T) {
		t.Parallel()

		// given: two versions with same year but different months
		// when: comparing
		result := calVerVersionRefLess("YYYY.0M.MICRO", "2026.02.1", "2026.03.1", "ref-a", "ref-b")

		// then: earlier month is less
		testastic.True(t, result)
	})

	t.Run("same year and month different micro", func(t *testing.T) {
		t.Parallel()

		// given: two versions with same year/month but different micro
		// when: comparing
		result := calVerVersionRefLess("YYYY.0M.MICRO", "2026.03.1", "2026.03.2", "ref-a", "ref-b")

		// then: smaller micro is less
		testastic.True(t, result)
	})

	t.Run("equal versions falls back to ref compare", func(t *testing.T) {
		t.Parallel()

		// given: two identical versions but different refs
		// when: comparing
		result := calVerVersionRefLess("YYYY.0M.MICRO", "2026.03.1", "2026.03.1", "ref-a", "ref-b")

		// then: falls back to string comparison of refs
		testastic.True(t, result)
	})

	t.Run("invalid left version falls back to ref compare", func(t *testing.T) {
		t.Parallel()

		// given: an invalid left version
		// when: comparing
		result := calVerVersionRefLess("YYYY.0M.MICRO", "invalid", "2026.03.1", "ref-a", "ref-b")

		// then: falls back to string comparison of refs
		testastic.True(t, result)
	})

	t.Run("invalid right version falls back to ref compare", func(t *testing.T) {
		t.Parallel()

		// given: an invalid right version
		// when: comparing
		result := calVerVersionRefLess("YYYY.0M.MICRO", "2026.03.1", "invalid", "ref-a", "ref-b")

		// then: falls back to string comparison of refs
		testastic.True(t, result)
	})

	t.Run("right is less than left", func(t *testing.T) {
		t.Parallel()

		// given: right version is earlier than left
		// when: comparing
		result := calVerVersionRefLess("YYYY.0M.MICRO", "2026.03.2", "2026.03.1", "ref-a", "ref-b")

		// then: left is not less than right
		testastic.False(t, result)
	})

	t.Run("configured short year and unpadded month", func(t *testing.T) {
		t.Parallel()

		// given: two YY.MM.MICRO versions that do not sort correctly as strings
		// when: comparing with the configured format
		result := calVerVersionRefLess("YY.MM.MICRO", "26.2.9", "26.10.1", "ref-a", "ref-b")

		// then: the earlier month is less
		testastic.True(t, result)
	})
}

func TestOrderedPlans(t *testing.T) {
	t.Parallel()

	t.Run("empty map yields empty slice", func(t *testing.T) {
		t.Parallel()

		// given: no plans
		plans := map[string]TargetPlan{}

		// when: ordering
		ordered := orderedPlans(plans)

		// then: the slice is empty
		testastic.Equal(t, 0, len(ordered))
	})

	t.Run("single entry is returned as is", func(t *testing.T) {
		t.Parallel()

		// given: one plan
		plans := map[string]TargetPlan{
			"only": {ID: "only", Type: "service"},
		}

		// when: ordering
		ordered := orderedPlans(plans)

		// then: that plan is the sole element
		testastic.Equal(t, 1, len(ordered))
		testastic.Equal(t, "only", ordered[0].ID)
	})

	t.Run("sorts by type before id", func(t *testing.T) {
		t.Parallel()

		// given: plans with mixed types and ids
		plans := map[string]TargetPlan{
			"svc-b": {ID: "svc-b", Type: "service"},
			"lib-a": {ID: "lib-a", Type: "library"},
			"svc-a": {ID: "svc-a", Type: "service"},
			"lib-b": {ID: "lib-b", Type: "library"},
		}

		// when: ordering
		ordered := orderedPlans(plans)

		// then: type is the primary key, id breaks ties
		testastic.Equal(t, 4, len(ordered))
		testastic.Equal(t, "lib-a", ordered[0].ID)
		testastic.Equal(t, "lib-b", ordered[1].ID)
		testastic.Equal(t, "svc-a", ordered[2].ID)
		testastic.Equal(t, "svc-b", ordered[3].ID)
	})

	t.Run("same type sorts by id lexicographically", func(t *testing.T) {
		t.Parallel()

		// given: plans sharing a type
		plans := map[string]TargetPlan{
			"gamma": {ID: "gamma", Type: "service"},
			"alpha": {ID: "alpha", Type: "service"},
			"beta":  {ID: "beta", Type: "service"},
		}

		// when: ordering
		ordered := orderedPlans(plans)

		// then: ids appear in ascending order
		testastic.Equal(t, "alpha", ordered[0].ID)
		testastic.Equal(t, "beta", ordered[1].ID)
		testastic.Equal(t, "gamma", ordered[2].ID)
	})
}
