package config //nolint:testpackage // exercises the embedded schema alongside Default

import (
	"encoding/json/v2"
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

	// when: comparing every default reachable through root properties and local references
	err = validateSchemaDefaultParity(schemaDocument, defaultDocument)

	// then: schema and Go defaults agree
	testastic.NoError(t, err)
}

func TestSchemaDefaultsRejectGoOnlyDefault(t *testing.T) {
	t.Parallel()

	// given: the published schema without the default for a nonempty Go default
	var schemaDocument map[string]any

	err := json.Unmarshal(yeet.ConfigSchema, &schemaDocument)
	testastic.NoError(t, err)

	propertiesValue, found := schemaDocument["properties"]
	testastic.True(t, found)

	if !found {
		return
	}

	properties, ok := propertiesValue.(map[string]any)
	testastic.True(t, ok)

	if !ok {
		return
	}

	branchValue, found := properties["branch"]
	testastic.True(t, found)

	if !found {
		return
	}

	branch, ok := branchValue.(map[string]any)
	testastic.True(t, ok)

	if !ok {
		return
	}

	delete(branch, "default")

	defaultYAML, err := yaml.Marshal(Default())
	testastic.NoError(t, err)

	var defaultDocument map[string]any

	err = yaml.Unmarshal(defaultYAML, &defaultDocument)
	testastic.NoError(t, err)

	// when: comparing the incomplete schema with the Go defaults
	err = validateSchemaDefaultParity(schemaDocument, defaultDocument)

	// then: the Go-only default is rejected
	testastic.ErrorContains(t, err, "branch has Go default but no schema default")
}

func validateSchemaDefaultParity(schemaDocument, defaultDocument map[string]any) error {
	schemaDefaults, err := collectSchemaDefaults(schemaDocument)
	if err != nil {
		return err
	}

	slices.SortFunc(schemaDefaults, func(left, right schemaDefault) int {
		return strings.Compare(strings.Join(left.path, "."), strings.Join(right.path, "."))
	})

	schemaDefaultsByPath := make(map[string]any, len(schemaDefaults))

	for _, schemaValue := range schemaDefaults {
		expected := schemaValue.value

		actual, found := valueAtConfigPath(defaultDocument, schemaValue.path)
		if !found && isEmptyJSONCollection(expected) {
			actual = expected
			found = true
		}

		path := strings.Join(schemaValue.path, ".")
		schemaDefaultsByPath[path] = expected

		if !found {
			return fmt.Errorf("%s has schema default but no Go default", path)
		}

		expectedJSON, err := json.Marshal(expected, json.Deterministic(true))
		if err != nil {
			return fmt.Errorf("encode schema default at %s: %w", path, err)
		}

		actualJSON, err := json.Marshal(actual, json.Deterministic(true))
		if err != nil {
			return fmt.Errorf("encode Go default at %s: %w", path, err)
		}

		if string(expectedJSON) != string(actualJSON) {
			return fmt.Errorf("default mismatch at %s: schema %s, Go %s", path, expectedJSON, actualJSON)
		}
	}

	goDefaults, err := collectComparableGoDefaults(schemaDocument, schemaDocument, defaultDocument, nil)
	if err != nil {
		return err
	}

	for _, goValue := range goDefaults {
		path := strings.Join(goValue.path, ".")
		if _, found := schemaDefaultsByPath[path]; !found {
			return fmt.Errorf("%s has Go default but no schema default", path)
		}
	}

	return nil
}

func collectComparableGoDefaults(
	document, schemaNode map[string]any,
	goValue any,
	configPath []string,
) ([]schemaDefault, error) {
	if _, found := schemaNode["default"]; found {
		return []schemaDefault{{path: slices.Clone(configPath), value: goValue}}, nil
	}

	if refValue, found := schemaNode["$ref"]; found {
		ref, ok := refValue.(string)
		if !ok {
			return nil, fmt.Errorf("schema reference at %s is not a string", strings.Join(configPath, "."))
		}

		referenced, err := resolveLocalSchemaRef(document, ref)
		if err != nil {
			return nil, err
		}

		return collectComparableGoDefaults(document, referenced, goValue, configPath)
	}

	propertiesValue, hasProperties := schemaNode["properties"]
	if !hasProperties {
		if isEmptyJSONCollection(goValue) || goValue == nil {
			return nil, nil
		}

		return []schemaDefault{{path: slices.Clone(configPath), value: goValue}}, nil
	}

	properties, ok := propertiesValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema properties at %s are not an object", strings.Join(configPath, "."))
	}

	return collectComparableGoObjectDefaults(document, properties, goValue, configPath)
}

func collectComparableGoObjectDefaults(
	document, properties map[string]any,
	goValue any,
	configPath []string,
) ([]schemaDefault, error) {
	goObject, ok := goValue.(map[string]any)
	if !ok && (isEmptyJSONCollection(goValue) || goValue == nil) {
		return nil, nil
	}

	if !ok {
		return nil, fmt.Errorf("Go default at %s is not an object", strings.Join(configPath, "."))
	}

	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}

	slices.Sort(names)

	var defaults []schemaDefault

	for _, name := range names {
		value, found := goObject[name]
		if !found {
			continue
		}

		property, ok := properties[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("schema property %s is not an object", strings.Join(append(configPath, name), "."))
		}

		propertyDefaults, err := collectComparableGoDefaults(document, property, value, append(configPath, name))
		if err != nil {
			return nil, err
		}

		defaults = append(defaults, propertyDefaults...)
	}

	return defaults, nil
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

			err := walk(property, append(path, name))
			if err != nil {
				return err
			}
		}

		return nil
	}

	err := walk(document, nil)
	if err != nil {
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
