package versionfile_test

import (
	"os"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/version"
	"github.com/monkescience/yeet/internal/versionfile"
)

func TestApplyGenericMarkers_SemVer(t *testing.T) {
	t.Parallel()

	t.Run("replaces inline version marker", func(t *testing.T) {
		t.Parallel()

		// given: a line with an inline yeet version marker
		content := "image: ghcr.io/acme/app:1.2.3 # x-yeet-version"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.3.0", versionfile.SemVerScheme())

		// then: the version is updated
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, "image: ghcr.io/acme/app:1.3.0 # x-yeet-version", updated)
	})

	t.Run("replaces major minor and patch markers", func(t *testing.T) {
		t.Parallel()

		// given: inline markers for each numeric scope
		input, err := os.ReadFile("testdata/major_minor_patch/input.txt")
		testastic.NoError(t, err)

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "4.5.6", versionfile.SemVerScheme())

		// then: all numeric scopes are updated
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/major_minor_patch/expected.txt", updated)
	})

	t.Run("replaces version markers in block", func(t *testing.T) {
		t.Parallel()

		// given: a yeet version block with multiple version strings
		input, err := os.ReadFile("testdata/version_block/input.txt")
		testastic.NoError(t, err)

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "2.0.0", versionfile.SemVerScheme())

		// then: versions inside the block are updated and outside is unchanged
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/version_block/expected.txt", updated)
	})

	t.Run("returns no markers error for release please markers", func(t *testing.T) {
		t.Parallel()

		// given: release-please markers instead of yeet markers
		content := "version = \"1.2.3\" # x-release-please-version"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.2.4", versionfile.SemVerScheme())

		// then: absence of yeet markers surfaces as a config error
		testastic.ErrorIs(t, err, versionfile.ErrNoMarkersFound)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("two-part version returns invalid-next-version error", func(t *testing.T) {
		t.Parallel()

		// given: scoped markers but a semver next version with only two parts
		input, err := os.ReadFile("testdata/scoped_markers/input.txt")
		testastic.NoError(t, err)

		// when: applying marker replacements with a two-part version
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "1.2", versionfile.SemVerScheme())

		// then: the malformed next version surfaces as a loud error rather than silently no-op'ing
		testastic.ErrorIs(t, err, versionfile.ErrInvalidNextVersion)
		testastic.False(t, changed)
		testastic.Equal(t, string(input), updated)
	})

	t.Run("version with prerelease suffix strips suffix for patch", func(t *testing.T) {
		t.Parallel()

		// given: an inline patch marker with a different patch value
		content := "PATCH=1 # x-yeet-patch"

		// when: applying marker replacements with a prerelease version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.2.3-rc.1", versionfile.SemVerScheme())

		// then: patch value is the numeric part only, suffix stripped
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, "PATCH=3 # x-yeet-patch", updated)
	})

	t.Run("empty content returns unchanged", func(t *testing.T) {
		t.Parallel()

		// given: empty content
		content := ""

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.0.0", versionfile.SemVerScheme())

		// then: nothing changes and no marker-absence error is raised
		testastic.NoError(t, err)
		testastic.False(t, changed)
		testastic.Equal(t, "", updated)
	})

	t.Run("no numeric match on inline marker returns error", func(t *testing.T) {
		t.Parallel()

		// given: an inline major marker on a line with no numeric value
		content := "name = \"app\" # x-yeet-major"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2.0.0", versionfile.SemVerScheme())

		// then: the mismatch surfaces as a config error
		testastic.ErrorIs(t, err, versionfile.ErrMarkerNoMatch)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("unclosed block marker returns error", func(t *testing.T) {
		t.Parallel()

		// given: a block start with no matching end marker
		input, err := os.ReadFile("testdata/unclosed_block/input.txt")
		testastic.NoError(t, err)

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "2.0.0", versionfile.SemVerScheme())

		// then: the unclosed block surfaces as a structural error
		testastic.ErrorIs(t, err, versionfile.ErrUnclosedBlockMarker)
		testastic.False(t, changed)
		testastic.Equal(t, string(input), updated)
	})

	t.Run("nested block start returns error", func(t *testing.T) {
		t.Parallel()

		// given: a second block start inside an already-open block
		input, err := os.ReadFile("testdata/nested_block/input.txt")
		testastic.NoError(t, err)

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "2.0.0", versionfile.SemVerScheme())

		// then: the nested start surfaces as a structural error
		testastic.ErrorIs(t, err, versionfile.ErrNestedBlockMarker)
		testastic.False(t, changed)
		testastic.Equal(t, string(input), updated)
	})

	t.Run("file without yeet markers returns error", func(t *testing.T) {
		t.Parallel()

		// given: a non-empty file with no yeet markers at all
		input, err := os.ReadFile("testdata/no_markers/input.txt")
		testastic.NoError(t, err)

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "1.2.4", versionfile.SemVerScheme())

		// then: missing markers surface as a config error
		testastic.ErrorIs(t, err, versionfile.ErrNoMarkersFound)
		testastic.False(t, changed)
		testastic.Equal(t, string(input), updated)
	})

	t.Run("prose mentions of markers inside backticks are skipped", func(t *testing.T) {
		t.Parallel()

		// given: a README-like file mixing a real marker line with prose that references marker names in backticks
		input, err := os.ReadFile("testdata/prose_mentions/input.txt")
		testastic.NoError(t, err)

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "1.3.0", versionfile.SemVerScheme())

		// then: only the real marker line is rewritten; prose mentions are left alone
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/prose_mentions/expected.txt", updated)
	})

	t.Run("marker already at target version succeeds without changes", func(t *testing.T) {
		t.Parallel()

		// given: an inline marker whose line already shows the target version
		content := "version = \"1.2.3\" # x-yeet-version"

		// when: applying marker replacements with the same version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.2.3", versionfile.SemVerScheme())

		// then: no error, no change
		testastic.NoError(t, err)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("calver-only marker in semver scheme returns mismatch error", func(t *testing.T) {
		t.Parallel()

		// given: a calver-style day marker in a semver project
		content := "DAY=2 # x-yeet-day"

		// when: applying marker replacements with semver
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.2.3", versionfile.SemVerScheme())

		// then: the scope/scheme mismatch surfaces with a suggested replacement
		testastic.ErrorIs(t, err, versionfile.ErrMarkerSchemeMismatch)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
		testastic.True(t, strings.Contains(err.Error(), `x-yeet-patch`))
	})
}

func TestApplyGenericMarkers_CalVer(t *testing.T) {
	t.Parallel()

	defaultScheme := mustCalVerScheme(t, "")

	t.Run("replaces calver values with yeet version marker", func(t *testing.T) {
		t.Parallel()

		// given: a calver value with a yeet version marker
		content := "version = \"2026.02.7\" # x-yeet-version"

		// when: applying marker replacements with next calver version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2026.03.1", defaultScheme)

		// then: calver value is updated
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, "version = \"2026.03.1\" # x-yeet-version", updated)
	})

	t.Run("replaces year month and micro markers", func(t *testing.T) {
		t.Parallel()

		// given: calver markers for year month and micro on the default format
		input, err := os.ReadFile("testdata/calver_aliases/input.txt")
		testastic.NoError(t, err)

		// when: applying marker replacements with next calver version
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "2026.03.1", defaultScheme)

		// then: each marker is updated and 0M zero-padding is preserved
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/calver_aliases/expected.txt", updated)
	})

	t.Run("replaces day marker preserving zero-padding", func(t *testing.T) {
		t.Parallel()

		// given: a four-part calver format with a day marker
		input, err := os.ReadFile("testdata/calver_day/input.txt")
		testastic.NoError(t, err)
		scheme := mustCalVerScheme(t, "YYYY.0M.0D.MICRO")

		// when: applying marker replacements with next calver version on day 5
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "2026.03.05.1", scheme)

		// then: day segment renders as 05 not 5, preserving 0D width
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/calver_day/expected.txt", updated)
	})

	t.Run("replaces week marker", func(t *testing.T) {
		t.Parallel()

		// given: a YYYY.WW.MICRO format with a week marker
		input, err := os.ReadFile("testdata/calver_week/input.txt")
		testastic.NoError(t, err)
		scheme := mustCalVerScheme(t, "YYYY.WW.MICRO")

		// when: applying marker replacements with next calver version
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "2026.18.1", scheme)

		// then: week segment is updated and other segments follow
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/calver_week/expected.txt", updated)
	})

	t.Run("replaces version and micro markers in four-part format", func(t *testing.T) {
		t.Parallel()

		// given: a four-part calver format with full-version and micro markers
		content := strings.Join([]string{
			`version = "2026.04.25.3" # x-yeet-version`,
			`micro = 3 # x-yeet-micro`,
		}, "\n")
		scheme := mustCalVerScheme(t, "YYYY.0M.0D.MICRO")

		// when: applying marker replacements with a four-part calver version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2026.04.26.1", scheme)

		// then: the whole version and final micro segment are updated
		expected := strings.Join([]string{
			`version = "2026.04.26.1" # x-yeet-version`,
			`micro = 1 # x-yeet-micro`,
		}, "\n")

		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, expected, updated)
	})

	t.Run("replaces calver markers in block", func(t *testing.T) {
		t.Parallel()

		// given: a month block using calver marker
		input, err := os.ReadFile("testdata/calver_block/input.txt")
		testastic.NoError(t, err)

		// when: applying marker replacements with next calver version
		updated, changed, err := versionfile.ApplyGenericMarkers(string(input), "2026.03.1", defaultScheme)

		// then: values inside block are updated and outside is unchanged
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/calver_block/expected.txt", updated)
	})

	t.Run("replaces version marker for two-segment format", func(t *testing.T) {
		t.Parallel()

		// given: a YYYY.MICRO format and an x-yeet-version marker
		content := `version = "2026.7" # x-yeet-version`
		scheme := mustCalVerScheme(t, "YYYY.MICRO")

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2027.1", scheme)

		// then: the version pattern matches even with only two segments
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, `version = "2027.1" # x-yeet-version`, updated)
	})

	t.Run("YY token loses padding past year-100 boundary", func(t *testing.T) {
		t.Parallel()

		// given: a YY format and a year-100 boundary version
		content := "YEAR=99 # x-yeet-year"
		scheme := mustCalVerScheme(t, "YY.0M.MICRO")

		// when: applying marker replacements crossing the boundary
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "100.03.1", scheme)

		// then: the rendered width changes from 2 to 3 chars; YY is unpadded by spec
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, "YEAR=100 # x-yeet-year", updated)
	})

	t.Run("semver-only marker in calver scheme returns mismatch error", func(t *testing.T) {
		t.Parallel()

		// given: an x-yeet-major marker in a calver project
		content := "MAJOR=2025 # x-yeet-major"

		// when: applying marker replacements with default calver
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2026.03.1", defaultScheme)

		// then: the scope/scheme mismatch surfaces with the year suggestion
		testastic.ErrorIs(t, err, versionfile.ErrMarkerSchemeMismatch)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
		testastic.True(t, strings.Contains(err.Error(), `x-yeet-year`))
	})

	t.Run("day marker in format without day token returns mismatch error", func(t *testing.T) {
		t.Parallel()

		// given: an x-yeet-day marker in a calver format with no day token
		content := "DAY=2 # x-yeet-day"

		// when: applying marker replacements with default calver (YYYY.0M.MICRO)
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2026.03.1", defaultScheme)

		// then: the scope/format mismatch surfaces with a calver-format hint
		testastic.ErrorIs(t, err, versionfile.ErrMarkerSchemeMismatch)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
		testastic.True(t, strings.Contains(err.Error(), `calver format`))
	})

	t.Run("month marker substitutes first numeric on lines with multiple numbers", func(t *testing.T) {
		t.Parallel()

		// given: a month marker on a line containing multiple 1-2 digit numbers
		content := "config: month=11 retries=4 timeout=30 # x-yeet-month"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2026.03.1", defaultScheme)

		// then: only the first numeric (11→03) is replaced; later 4 and 30 are untouched
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, "config: month=03 retries=4 timeout=30 # x-yeet-month", updated)
	})
}

func mustCalVerScheme(t *testing.T, format string) versionfile.Scheme {
	t.Helper()

	calver, err := version.NewCalVerScheme(format)
	testastic.NoError(t, err)

	return versionfile.CalVerScheme(calver)
}
