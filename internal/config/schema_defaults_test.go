package config //nolint:testpackage // exercises the embedded schema alongside Default

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet"
	"go.yaml.in/yaml/v4"
)

type schemaDefault struct {
	path  []string
	value any
}

func TestSchemaDefaultsMatchGoDefaults(t *testing.T) {
	t.Parallel()

	// given: the published schema and the user-facing YAML form of the Go defaults
	var schemaDocument map[string]any

	err := json.Unmarshal(yeet.ConfigSchema, &schemaDocument)
	testastic.NoError(t, err)

	defaultYAML, err := yaml.Marshal(Default())
	testastic.NoError(t, err)

	var defaultDocument map[string]any

	err = yaml.Unmarshal(defaultYAML, &defaultDocument)
	testastic.NoError(t, err)

	// when: collecting every default reachable through root properties and local references
	schemaDefaults, err := collectSchemaDefaults(schemaDocument)
	testastic.NoError(t, err)
	testastic.True(t, len(schemaDefaults) > 0)

	slices.SortFunc(schemaDefaults, func(left, right schemaDefault) int {
		return strings.Compare(strings.Join(left.path, "."), strings.Join(right.path, "."))
	})

	// then: each schema default has the same canonical value as Default
	for _, schemaValue := range schemaDefaults {
		t.Run(strings.Join(schemaValue.path, "."), func(t *testing.T) {
			t.Parallel()

			// given: one root-reachable schema default
			expected := schemaValue.value

			// when: projecting the same path from the Go defaults
			actual, found := valueAtConfigPath(defaultDocument, schemaValue.path)
			if !found && isEmptyJSONCollection(expected) {
				actual = expected
				found = true
			}

			// then: only omitted empty collections are equivalent to a missing YAML value
			testastic.True(t, found)

			if !found {
				return
			}

			testastic.Equal(t, canonicalJSON(t, expected), canonicalJSON(t, actual))
		})
	}
}

func collectSchemaDefaults(document map[string]any) ([]schemaDefault, error) {
	defaults := make([]schemaDefault, 0)
	activeRefs := make(map[string]bool)

	var walk func(node map[string]any, path []string) error

	walk = func(node map[string]any, path []string) error {
		if value, found := node["default"]; found {
			defaults = append(defaults, schemaDefault{path: slices.Clone(path), value: value})
		}

		if refValue, found := node["$ref"]; found {
			ref, ok := refValue.(string)
			if !ok {
				return fmt.Errorf("schema reference at %s is not a string", strings.Join(path, "."))
			}

			if activeRefs[ref] {
				return fmt.Errorf("schema reference cycle at %s", ref)
			}

			referenced, err := resolveLocalSchemaRef(document, ref)
			if err != nil {
				return err
			}

			activeRefs[ref] = true
			err = walk(referenced, path)

			delete(activeRefs, ref)

			if err != nil {
				return err
			}
		}

		if node["type"] == "array" {
			return nil
		}

		propertiesValue, found := node["properties"]
		if !found {
			return nil
		}

		properties, ok := propertiesValue.(map[string]any)
		if !ok {
			return fmt.Errorf("schema properties at %s are not an object", strings.Join(path, "."))
		}

		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}

		slices.Sort(names)

		for _, name := range names {
			property, ok := properties[name].(map[string]any)
			if !ok {
				return fmt.Errorf("schema property %s is not an object", strings.Join(append(path, name), "."))
			}

			if err := walk(property, append(path, name)); err != nil {
				return err
			}
		}

		return nil
	}

	if err := walk(document, nil); err != nil {
		return nil, err
	}

	return defaults, nil
}

func resolveLocalSchemaRef(document map[string]any, ref string) (map[string]any, error) {
	pointer, found := strings.CutPrefix(ref, "#/")
	if !found {
		return nil, fmt.Errorf("schema reference %q is not local", ref)
	}

	var current any = document

	for token := range strings.SplitSeq(pointer, "/") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("schema reference %q traverses a non-object", ref)
		}

		name := strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")

		current, found = object[name]

		if !found {
			return nil, fmt.Errorf("schema reference %q does not exist", ref)
		}
	}

	resolved, ok := current.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema reference %q does not resolve to an object", ref)
	}

	return resolved, nil
}

func valueAtConfigPath(document map[string]any, path []string) (any, bool) {
	var current any = document

	for _, name := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}

		current, ok = object[name]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

func isEmptyJSONCollection(value any) bool {
	switch collection := value.(type) {
	case []any:
		return len(collection) == 0
	case map[string]any:
		return len(collection) == 0
	default:
		return false
	}
}

func canonicalJSON(t *testing.T, value any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	testastic.NoError(t, err)

	return string(encoded)
}
