package config

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"

	yeet "github.com/monkescience/yeet"
)

// schemaID matches the $id inside yeet.schema.json. It identifies the document
// rather than locating it, so it stays constant across releases while the URL
// yeet init writes moves with the version.
const schemaID = "https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json"

var compiledSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(yeet.ConfigSchema))
	if err != nil {
		return nil, fmt.Errorf("decode embedded config schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()

	err = compiler.AddResource(schemaID, document)
	if err != nil {
		return nil, fmt.Errorf("register embedded config schema: %w", err)
	}

	schema, err := compiler.Compile(schemaID)
	if err != nil {
		return nil, fmt.Errorf("compile embedded config schema: %w", err)
	}

	return schema, nil
})

// schemaNode carries the position of the ancestor a nested error is reported
// against.
type schemaNode struct {
	location  []string
	schemaURL string
}

// violation is one leaf failure of the embedded schema, paired with the value
// that produced it so a message can quote what the user wrote.
type violation struct {
	location []string
	keyword  string
	detail   jsonschema.ErrorKind
	value    any
}

func validateAgainstSchema(instance any) error {
	schema, err := compiledSchema()
	if err != nil {
		return err
	}

	err = schema.Validate(instance)
	if err == nil {
		return nil
	}

	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}

	violations := collectViolations(validationErr, instance, schemaNode{})
	if len(violations) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrInvalidConfig, describeViolations(violations))
}

func collectViolations(node *jsonschema.ValidationError, instance any, parent schemaNode) []violation {
	causes := node.Causes

	switch node.ErrorKind.(type) {
	case *kind.OneOf:
		causes = oneOfBranch(node, instance)
	case *kind.PropertyNames:
		// The nested cause validates the key on its own and carries no instance
		// location, so the offending object is only nameable from this node.
		causes = nil
	}

	if len(causes) > 0 {
		current := schemaNode{location: node.InstanceLocation, schemaURL: resolvedSchemaURL(node)}

		collected := make([]violation, 0, len(causes))
		for _, cause := range causes {
			collected = append(collected, collectViolations(cause, instance, current)...)
		}

		return collected
	}

	return leafViolations(node, instance, parent)
}

func leafViolations(node *jsonschema.ValidationError, instance any, parent schemaNode) []violation {
	switch detail := node.ErrorKind.(type) {
	case *kind.PropertyNames:
		location := propertyNamesLocation(parent, node.SchemaURL)

		return []violation{{
			location: location,
			keyword:  keywordPropertyNames,
			detail:   detail,
			value:    valueAt(instance, location),
		}}
	case *kind.AdditionalProperties:
		// The strict YAML decode names the offending key and its Go type, which
		// reads better than anything the schema output can give.
		return nil
	case *kind.Required:
		missing := make([]violation, 0, len(detail.Missing))
		for _, property := range detail.Missing {
			missing = append(missing, violation{
				location: append(slices.Clone(node.InstanceLocation), property),
				keyword:  keywordRequired,
				detail:   detail,
			})
		}

		return missing
	default:
		return []violation{{
			location: node.InstanceLocation,
			keyword:  keywordOf(node.ErrorKind),
			detail:   node.ErrorKind,
			value:    valueAt(instance, node.InstanceLocation),
		}}
	}
}

// propertyNamesLocation rebuilds the instance path of a propertyNames failure.
// The library assigns that error the validator's live location slice, which has
// moved on by the time the error is read, so the path is walked back from the
// nearest ancestor whose location was copied.
func propertyNamesLocation(parent schemaNode, schemaURL string) []string {
	suffix, found := strings.CutPrefix(schemaURL, parent.schemaURL)
	if !found {
		return parent.location
	}

	location := slices.Clone(parent.location)

	tokens := strings.Split(strings.TrimPrefix(suffix, "/"), "/")
	for index, token := range tokens {
		if token == "properties" && index+1 < len(tokens) {
			location = append(location, tokens[index+1])
		}
	}

	return location
}

func resolvedSchemaURL(node *jsonschema.ValidationError) string {
	reference, isReference := node.ErrorKind.(*kind.Reference)
	if isReference {
		return reference.URL
	}

	return node.SchemaURL
}

// oneOfBranch keeps only the branch the user wrote in, so a malformed path
// target does not also report every rule of a derived target.
func oneOfBranch(node *jsonschema.ValidationError, instance any) []*jsonschema.ValidationError {
	const firstBranch, secondBranch = 0, 1

	value := valueAt(instance, node.InstanceLocation)

	if isVersionFileEntry(node.InstanceLocation) {
		if _, isScalar := value.(string); isScalar {
			return branchAt(node, firstBranch)
		}

		return branchAt(node, secondBranch)
	}

	object, _ := value.(map[string]any)

	targetType, _ := object["type"].(string)
	if targetType == string(TargetTypeDerived) {
		return branchAt(node, secondBranch)
	}

	return branchAt(node, firstBranch)
}

func isVersionFileEntry(location []string) bool {
	const entryDepth = 2

	return len(location) >= entryDepth && location[len(location)-entryDepth] == "version_files"
}

func branchAt(node *jsonschema.ValidationError, index int) []*jsonschema.ValidationError {
	if index >= len(node.Causes) {
		return node.Causes
	}

	return []*jsonschema.ValidationError{node.Causes[index]}
}

func valueAt(instance any, location []string) any {
	current := instance

	for _, segment := range location {
		switch typed := current.(type) {
		case map[string]any:
			value, exists := typed[segment]
			if !exists {
				return nil
			}

			current = value
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(typed) {
				return nil
			}

			current = typed[index]
		default:
			return nil
		}
	}

	return current
}

func describeViolations(violations []violation) string {
	slices.SortStableFunc(violations, func(left, right violation) int {
		return slices.Compare(left.location, right.location)
	})

	for _, rule := range schemaRules {
		for _, found := range violations {
			if rule.matches(found) {
				return rule.message(found)
			}
		}
	}

	return fallbackMessage(violations[0])
}
