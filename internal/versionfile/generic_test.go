package versionfile_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/versionfile"
)

func TestApplyGenericMarkers(t *testing.T) {
	t.Parallel()

	t.Run("replaces inline version marker", func(t *testing.T) {
		t.Parallel()

		// given: a line with an inline yeet version marker
		content := "image: ghcr.io/acme/app:1.2.3 # x-yeet-version"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.3.0")

		// then: the version is updated
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, "image: ghcr.io/acme/app:1.3.0 # x-yeet-version", updated)
	})

	t.Run("replaces major minor and patch markers", func(t *testing.T) {
		t.Parallel()

		// given: inline markers for each numeric scope
		content := "MAJOR=1 # x-yeet-major\nMINOR=2 # x-yeet-minor\nPATCH=3 # x-yeet-patch"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "4.5.6")

		// then: all numeric scopes are updated
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, "MAJOR=4 # x-yeet-major\nMINOR=5 # x-yeet-minor\nPATCH=6 # x-yeet-patch", updated)
	})

	t.Run("replaces version markers in block", func(t *testing.T) {
		t.Parallel()

		// given: a yeet version block with multiple version strings
		content := "# x-yeet-start-version\nversion = \"1.2.3\"\napp = \"0.0.1\"\n# x-yeet-end\noutside = \"1.2.3\""

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2.0.0")

		// then: versions inside the block are updated and outside is unchanged
		testastic.NoError(t, err)
		testastic.True(t, changed)

		expected := "# x-yeet-start-version\nversion = \"2.0.0\"\napp = \"2.0.0\"\n# x-yeet-end\noutside = \"1.2.3\""
		testastic.Equal(t, expected, updated)
	})

	t.Run("returns no markers error for release please markers", func(t *testing.T) {
		t.Parallel()

		// given: release-please markers instead of yeet markers
		content := "version = \"1.2.3\" # x-release-please-version"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.2.4")

		// then: absence of yeet markers surfaces as a config error
		testastic.ErrorIs(t, err, versionfile.ErrNoMarkersFound)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("replaces calver values with yeet version marker", func(t *testing.T) {
		t.Parallel()

		// given: a calver value with a yeet version marker
		content := "version = \"2026.02.7\" # x-yeet-version"

		// when: applying marker replacements with next calver version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2026.03.1")

		// then: calver value is updated
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.Equal(t, "version = \"2026.03.1\" # x-yeet-version", updated)
	})

	t.Run("replaces calver alias markers", func(t *testing.T) {
		t.Parallel()

		// given: calver alias markers for year month and micro
		content := "YEAR=2025 # x-yeet-year\nMONTH=11 # x-yeet-month\nMICRO=9 # x-yeet-micro"

		// when: applying marker replacements with next calver version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2026.03.1")

		// then: aliases update to year month and micro parts
		testastic.NoError(t, err)
		testastic.True(t, changed)

		expected := "YEAR=2026 # x-yeet-year\nMONTH=03 # x-yeet-month\nMICRO=1 # x-yeet-micro"
		testastic.Equal(t, expected, updated)
	})

	t.Run("replaces calver aliases in block markers", func(t *testing.T) {
		t.Parallel()

		// given: a month block using calver alias marker
		content := "# x-yeet-start-month\nmonth = 02\nwindow = 12\n# x-yeet-end\noutside = 99"

		// when: applying marker replacements with next calver version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2026.03.1")

		// then: values inside block are updated and outside is unchanged
		testastic.NoError(t, err)
		testastic.True(t, changed)

		expected := "# x-yeet-start-month\nmonth = 03\nwindow = 03\n# x-yeet-end\noutside = 99"
		testastic.Equal(t, expected, updated)
	})

	t.Run("version with too few parts leaves scoped markers unchanged", func(t *testing.T) {
		t.Parallel()

		// given: scoped markers but a version with only two parts
		content := "MAJOR=1 # x-yeet-major\nMINOR=2 # x-yeet-minor\nPATCH=3 # x-yeet-patch"

		// when: applying marker replacements with a two-part version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.2")

		// then: content is unchanged since splitVersion returns empty parts
		testastic.NoError(t, err)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("version with prerelease suffix strips suffix for patch", func(t *testing.T) {
		t.Parallel()

		// given: an inline patch marker with a different patch value
		content := "PATCH=1 # x-yeet-patch"

		// when: applying marker replacements with a prerelease version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.2.3-rc.1")

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
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.0.0")

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
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2.0.0")

		// then: the mismatch surfaces as a config error
		testastic.ErrorIs(t, err, versionfile.ErrMarkerNoMatch)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("unclosed block marker returns error", func(t *testing.T) {
		t.Parallel()

		// given: a block start with no matching end marker
		content := "# x-yeet-start-version\nversion = \"1.2.3\"\nmore content"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2.0.0")

		// then: the unclosed block surfaces as a structural error
		testastic.ErrorIs(t, err, versionfile.ErrUnclosedBlockMarker)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("nested block start returns error", func(t *testing.T) {
		t.Parallel()

		// given: a second block start inside an already-open block
		content := "# x-yeet-start-version\n# x-yeet-start-major\nversion = \"1.2.3\"\n# x-yeet-end"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "2.0.0")

		// then: the nested start surfaces as a structural error
		testastic.ErrorIs(t, err, versionfile.ErrNestedBlockMarker)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("file without yeet markers returns error", func(t *testing.T) {
		t.Parallel()

		// given: a non-empty file with no yeet markers at all
		content := "version=1.2.3\nname=app\n"

		// when: applying marker replacements
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.2.4")

		// then: missing markers surface as a config error
		testastic.ErrorIs(t, err, versionfile.ErrNoMarkersFound)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("marker already at target version succeeds without changes", func(t *testing.T) {
		t.Parallel()

		// given: an inline marker whose line already shows the target version
		content := "version = \"1.2.3\" # x-yeet-version"

		// when: applying marker replacements with the same version
		updated, changed, err := versionfile.ApplyGenericMarkers(content, "1.2.3")

		// then: no error, no change
		testastic.NoError(t, err)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})
}
