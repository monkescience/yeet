package versionfile_test

import (
	"errors"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/versionfile"
)

func TestNewLocation(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name    string
		format  versionfile.Format
		pointer string
		want    error
		reason  versionfile.JSONPointerReason
	}{
		{name: "defaults to markers"},
		{name: "accepts marker format", format: versionfile.FormatMarkers},
		{name: "accepts an empty object key", format: versionfile.FormatJSON, pointer: "/"},
		{name: "accepts escaped keys", format: versionfile.FormatJSON, pointer: "/a~1b/~0version"},
		{name: "rejects an unknown format", format: "yaml", want: versionfile.ErrInvalidLocation},
		{
			name: "rejects a pointer for markers", format: versionfile.FormatMarkers, pointer: "/version",
			want: versionfile.ErrInvalidLocation,
		},
		{
			name: "rejects an empty pointer", format: versionfile.FormatJSON,
			want: versionfile.ErrInvalidJSONPointer, reason: versionfile.JSONPointerMissingSlash,
		},
		{
			name: "rejects a missing slash", format: versionfile.FormatJSON, pointer: "version",
			want: versionfile.ErrInvalidJSONPointer, reason: versionfile.JSONPointerMissingSlash,
		},
		{
			name: "rejects an incomplete escape", format: versionfile.FormatJSON, pointer: "/version~",
			want: versionfile.ErrInvalidJSONPointer, reason: versionfile.JSONPointerInvalidEscape,
		},
		{
			name: "rejects an unknown escape", format: versionfile.FormatJSON, pointer: "/a~x",
			want: versionfile.ErrInvalidJSONPointer, reason: versionfile.JSONPointerInvalidEscape,
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a version file format and its optional JSON pointer
			// when: validating the location before any content is supplied
			_, err := versionfile.NewLocation(scenario.format, scenario.pointer)

			// then: invalid locations expose stable error identity and pointer reasons
			testastic.ErrorIs(t, err, scenario.want)

			if scenario.reason == 0 {
				return
			}

			pointerErr, ok := errors.AsType[*versionfile.JSONPointerError](err)
			testastic.True(t, ok)

			if ok {
				testastic.Equal(t, scenario.reason, pointerErr.Reason)
			}
		})
	}
}

func TestLocationOverlaps(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name                      string
		leftFormat, rightFormat   versionfile.Format
		leftPointer, rightPointer string
		want                      bool
	}{
		{
			name: "markers overlap", leftFormat: versionfile.FormatMarkers,
			rightFormat: versionfile.FormatMarkers, want: true,
		},
		{
			name: "mixed formats overlap", leftFormat: versionfile.FormatMarkers,
			rightFormat: versionfile.FormatJSON, rightPointer: "/version", want: true,
		},
		{
			name: "identical JSON pointers overlap", leftFormat: versionfile.FormatJSON,
			rightFormat: versionfile.FormatJSON, leftPointer: "/a~1b/~0version",
			rightPointer: "/a~1b/~0version", want: true,
		},
		{
			name: "distinct JSON pointers remain separate", leftFormat: versionfile.FormatJSON,
			rightFormat: versionfile.FormatJSON, leftPointer: "/api", rightPointer: "/web",
		},
		{
			name: "distinct escaped keys remain separate", leftFormat: versionfile.FormatJSON,
			rightFormat: versionfile.FormatJSON, leftPointer: "/a~1b", rightPointer: "/a/b",
		},
		{
			name: "parent and child pointers remain separate", leftFormat: versionfile.FormatJSON,
			rightFormat: versionfile.FormatJSON, leftPointer: "/api", rightPointer: "/api/version",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: two validated locations within the same version file
			left := newVersionFileLocation(t, scenario.leftFormat, scenario.leftPointer)
			right := newVersionFileLocation(t, scenario.rightFormat, scenario.rightPointer)

			// when: comparing the write locations in either order
			forward := left.Overlaps(right)
			backward := right.Overlaps(left)

			// then: overlap depends on addressing rather than the requested version
			testastic.Equal(t, scenario.want, forward)
			testastic.Equal(t, scenario.want, backward)
		})
	}
}

func TestLocationApply(t *testing.T) {
	t.Parallel()

	t.Run("leaves an unchanged JSON string intact", func(t *testing.T) {
		t.Parallel()

		// given: a JSON string already at the requested version and no marker scheme
		location := newVersionFileLocation(t, versionfile.FormatJSON, "/version")
		content := "{\n  \"version\": \"1.2.3\"\n}\n"

		// when: applying the same version
		updated, changed, err := location.Apply(content, "1.2.3", versionfile.Scheme{})

		// then: JSON updates do not require a marker scheme or change formatting
		testastic.NoError(t, err)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("rejects an uninitialized location", func(t *testing.T) {
		t.Parallel()

		// given: a location that has not been validated
		var location versionfile.Location

		content := "1.0.0 # x-yeet-version"

		// when: applying a version through the uninitialized location
		updated, changed, err := location.Apply(content, "2.0.0", versionfile.SemVerScheme())

		// then: the content is unchanged and the location error is returned
		testastic.ErrorIs(t, err, versionfile.ErrInvalidLocation)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})
}

func newVersionFileLocation(t *testing.T, format versionfile.Format, pointer string) versionfile.Location {
	t.Helper()

	location, err := versionfile.NewLocation(format, pointer)
	testastic.NoError(t, err)

	return location
}
