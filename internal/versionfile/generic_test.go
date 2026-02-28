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
		updated, changed := versionfile.ApplyGenericMarkers(content, "1.3.0")

		// then: the version is updated
		testastic.True(t, changed)
		testastic.Equal(t, "image: ghcr.io/acme/app:1.3.0 # x-yeet-version", updated)
	})

	t.Run("replaces major minor and patch markers", func(t *testing.T) {
		t.Parallel()

		// given: inline markers for each numeric scope
		content := "MAJOR=1 # x-yeet-major\nMINOR=2 # x-yeet-minor\nPATCH=3 # x-yeet-patch"

		// when: applying marker replacements
		updated, changed := versionfile.ApplyGenericMarkers(content, "4.5.6")

		// then: all numeric scopes are updated
		testastic.True(t, changed)
		testastic.Equal(t, "MAJOR=4 # x-yeet-major\nMINOR=5 # x-yeet-minor\nPATCH=6 # x-yeet-patch", updated)
	})

	t.Run("replaces version markers in block", func(t *testing.T) {
		t.Parallel()

		// given: a yeet version block with multiple version strings
		content := "# x-yeet-start-version\nversion = \"1.2.3\"\napp = \"0.0.1\"\n# x-yeet-end\noutside = \"1.2.3\""

		// when: applying marker replacements
		updated, changed := versionfile.ApplyGenericMarkers(content, "2.0.0")

		// then: versions inside the block are updated and outside is unchanged
		testastic.True(t, changed)

		expected := "# x-yeet-start-version\nversion = \"2.0.0\"\napp = \"2.0.0\"\n# x-yeet-end\noutside = \"1.2.3\""
		testastic.Equal(t, expected, updated)
	})

	t.Run("ignores release please markers", func(t *testing.T) {
		t.Parallel()

		// given: release-please markers instead of yeet markers
		content := "version = \"1.2.3\" # x-release-please-version"

		// when: applying marker replacements
		updated, changed := versionfile.ApplyGenericMarkers(content, "1.2.4")

		// then: content is unchanged
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("replaces calver values with yeet version marker", func(t *testing.T) {
		t.Parallel()

		// given: a calver value with a yeet version marker
		content := "version = \"2026.02.7\" # x-yeet-version"

		// when: applying marker replacements with next calver version
		updated, changed := versionfile.ApplyGenericMarkers(content, "2026.03.1")

		// then: calver value is updated
		testastic.True(t, changed)
		testastic.Equal(t, "version = \"2026.03.1\" # x-yeet-version", updated)
	})

	t.Run("replaces calver alias markers", func(t *testing.T) {
		t.Parallel()

		// given: calver alias markers for year month and micro
		content := "YEAR=2025 # x-yeet-year\nMONTH=11 # x-yeet-month\nMICRO=9 # x-yeet-micro"

		// when: applying marker replacements with next calver version
		updated, changed := versionfile.ApplyGenericMarkers(content, "2026.03.1")

		// then: aliases update to year month and micro parts
		testastic.True(t, changed)

		expected := "YEAR=2026 # x-yeet-year\nMONTH=03 # x-yeet-month\nMICRO=1 # x-yeet-micro"
		testastic.Equal(t, expected, updated)
	})

	t.Run("replaces calver aliases in block markers", func(t *testing.T) {
		t.Parallel()

		// given: a month block using calver alias marker
		content := "# x-yeet-start-month\nmonth = 02\nwindow = 12\n# x-yeet-end\noutside = 99"

		// when: applying marker replacements with next calver version
		updated, changed := versionfile.ApplyGenericMarkers(content, "2026.03.1")

		// then: values inside block are updated and outside is unchanged
		testastic.True(t, changed)

		expected := "# x-yeet-start-month\nmonth = 03\nwindow = 03\n# x-yeet-end\noutside = 99"
		testastic.Equal(t, expected, updated)
	})
}
