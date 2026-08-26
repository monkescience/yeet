package versionfile_test

import (
	"os"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/versionfile"
)

func TestApplyJSONPointer(t *testing.T) {
	t.Parallel()

	t.Run("replaces string at json pointer", func(t *testing.T) {
		t.Parallel()

		// given: a package.json-style document with a version string
		input, err := os.ReadFile("testdata/json_pointer_version/input.json")
		testastic.NoError(t, err)

		// when: applying a JSON pointer replacement
		updated, changed, err := versionfile.ApplyJSONPointer(string(input), "2.0.0", "/version")

		// then: only the pointed string value changes and formatting is preserved
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/json_pointer_version/expected.json", updated)
	})

	t.Run("replaces nested array string", func(t *testing.T) {
		t.Parallel()

		// given: a nested JSON document with array traversal
		content := readVersionFileTestdata(t, "testdata/json_pointer_nested_array/input.json")

		// when: applying a nested JSON pointer replacement
		updated, changed, err := versionfile.ApplyJSONPointer(content, "1.3.0", "/packages/0/release")

		// then: the nested string is updated
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/json_pointer_nested_array/expected.json", updated)
	})

	t.Run("replaces string in root array", func(t *testing.T) {
		t.Parallel()

		// given: a JSON document whose root is an array
		content := readVersionFileTestdata(t, "testdata/json_pointer_root_array/input.json")

		// when: applying a JSON pointer replacement through the root list
		updated, changed, err := versionfile.ApplyJSONPointer(content, "2.0.0", "/1/current")

		// then: only the addressed array element is updated
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/json_pointer_root_array/expected.json", updated)
	})

	t.Run("unescapes json pointer segments", func(t *testing.T) {
		t.Parallel()

		// given: JSON keys containing / and ~, which require RFC 6901 escaping
		content := readVersionFileTestdata(t, "testdata/json_pointer_escaped/input.json")

		// when: applying a JSON pointer replacement using ~1 and ~0 escapes
		updated, changed, err := versionfile.ApplyJSONPointer(content, "2.0.0", "/channels/app~1stable/next~0candidate")

		// then: escaped pointer segments resolve to the literal JSON keys
		testastic.NoError(t, err)
		testastic.True(t, changed)
		testastic.AssertFile(t, "testdata/json_pointer_escaped/expected.json", updated)
	})

	t.Run("missing pointer returns error", func(t *testing.T) {
		t.Parallel()

		// given: a JSON document without the requested path
		content := readVersionFileTestdata(t, "testdata/json_pointer_missing/input.json")

		// when: applying a missing JSON pointer replacement
		updated, changed, err := versionfile.ApplyJSONPointer(content, "1.0.0", "/release")

		// then: the missing pointer is reported without changing content
		testastic.ErrorIs(t, err, versionfile.ErrJSONPointerNotFound)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("non string value returns error", func(t *testing.T) {
		t.Parallel()

		// given: a JSON document whose pointer resolves to a number
		content := readVersionFileTestdata(t, "testdata/json_pointer_non_string/input.json")

		// when: applying a JSON pointer replacement
		updated, changed, err := versionfile.ApplyJSONPointer(content, "1.0.0", "/build")

		// then: only string values are accepted for version updates
		testastic.ErrorIs(t, err, versionfile.ErrJSONPointerNonString)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		t.Parallel()

		// given: malformed JSON content
		content := readVersionFileTestdata(t, "testdata/json_pointer_invalid_json/input.json")

		// when: applying a JSON pointer replacement
		updated, changed, err := versionfile.ApplyJSONPointer(content, "1.0.0", "/release")

		// then: the parse error is surfaced without changing content
		testastic.ErrorIs(t, err, versionfile.ErrInvalidJSON)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("rejects duplicate object names", func(t *testing.T) {
		t.Parallel()

		// given: a document with two members that have the same name
		content := `{"version":"1.0.0","version":"2.0.0"}`

		// when: applying a JSON pointer replacement
		updated, changed, err := versionfile.ApplyJSONPointer(content, "3.0.0", "/version")

		// then: strict JSON validation rejects the ambiguous document without changing it
		testastic.ErrorIs(t, err, versionfile.ErrInvalidJSON)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("rejects invalid utf8", func(t *testing.T) {
		t.Parallel()

		// given: a syntactically complete document containing invalid UTF-8 in a string
		content := "{\"version\":\"1.0.0" + string([]byte{0xff}) + "\"}"

		// when: applying a JSON pointer replacement
		updated, changed, err := versionfile.ApplyJSONPointer(content, "2.0.0", "/version")

		// then: strict JSON validation rejects the invalid encoding without changing it
		testastic.ErrorIs(t, err, versionfile.ErrInvalidJSON)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})

	t.Run("invalid pointer returns error", func(t *testing.T) {
		t.Parallel()

		// given: a pointer that does not use RFC 6901 syntax
		content := readVersionFileTestdata(t, "testdata/json_pointer_invalid_pointer/input.json")

		// when: applying the invalid JSON pointer
		updated, changed, err := versionfile.ApplyJSONPointer(content, "1.0.0", "version")

		// then: the pointer is rejected before changing content
		testastic.ErrorIs(t, err, versionfile.ErrInvalidJSONPointer)
		testastic.False(t, changed)
		testastic.Equal(t, content, updated)
	})
}

func readVersionFileTestdata(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	testastic.NoError(t, err)

	return string(content)
}
