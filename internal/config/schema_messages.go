package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// schemaRule renders one shape violation in the wording validate.go used before
// the schema took the rule over. The table is ordered so that a config breaking
// several rules reports the same one it reported when Go owned them all.
type schemaRule struct {
	path    []string
	keyword string
	message func(found violation) string
}

const (
	keywordRequired      = "required"
	keywordEnum          = "enum"
	keywordConst         = "const"
	keywordType          = "type"
	keywordMinLength     = "minLength"
	keywordPattern       = "pattern"
	keywordMinItems      = "minItems"
	keywordMinProperties = "minProperties"
	keywordMinimum       = "minimum"
	keywordPropertyNames = "propertyNames"
	keywordUniqueItems   = "uniqueItems"
	keywordNot           = "not"

	anySegment  = "*"
	releaseNode = "release"

	// versionFileEntryDepth is the distance from a version file property back to
	// the version_files collection that validate.go named in its messages.
	versionFileEntryDepth = 2
)

var schemaRules = slices.Concat(
	[]schemaRule{
		enumRule([]string{"versioning"}),
		emptyRule([]string{"branch"}, keywordMinLength),
		enumRule([]string{"provider"}),
		containerRule([]string{"bump_types", "minor", anySegment}, keywordMinLength, "must not contain empty strings"),
		containerRule([]string{"bump_types", "patch", anySegment}, keywordMinLength, "must not contain empty strings"),
		emptyRule([]string{"repository", "remote"}, keywordMinLength),
		blankRule([]string{"repository", anySegment, anySegment}, keywordMinLength),
	},
	changelogRules(nil),
	calVerRules(nil),
	versionFileRules(nil),
	releaseRules(),
	targetRules(),
)

func changelogRules(prefix []string) []schemaRule {
	changelog := append(slices.Clone(prefix), "changelog")
	references := append(slices.Clone(changelog), "references")
	pattern := append(slices.Clone(references), "patterns", anySegment)
	include := append(slices.Clone(changelog), "include")
	section := append(slices.Clone(changelog), "sections", anySegment)

	return []schemaRule{
		emptyRule(append(slices.Clone(changelog), "file"), keywordMinLength),
		emptyRule(include, keywordMinItems),
		containerRule(append(slices.Clone(include), anySegment), keywordMinLength, "must not contain empty strings"),
		{
			path:    slices.Clone(include),
			keyword: keywordUniqueItems,
			message: func(found violation) string {
				return fmt.Sprintf("%s contains duplicate %q", dotted(found.location), duplicateValue(found))
			},
		},
		keysRule(append(slices.Clone(changelog), "sections")),
		blankRule(section, keywordMinLength),
		blankRule(section, keywordPattern),
		{
			path:    slices.Clone(section),
			keyword: keywordType,
			message: func(found violation) string {
				return dotted(found.location) + " must be a string"
			},
		},
		indexedRule(append(slices.Clone(pattern), "pattern"), keywordMinLength, "must not be empty"),
		indexedRule(append(slices.Clone(pattern), "pattern"), keywordRequired, "must not be empty"),
		indexedRule(append(slices.Clone(pattern), "url"), keywordType, "must be a string"),
		keysRule(append(slices.Clone(references), "footers")),
	}
}

func calVerRules(prefix []string) []schemaRule {
	return []schemaRule{
		emptyRule(append(slices.Clone(prefix), "calver", "format"), keywordMinLength),
	}
}

func versionFileRules(prefix []string) []schemaRule {
	entry := append(slices.Clone(prefix), "version_files", anySegment)
	format := append(slices.Clone(entry), "format")
	pointer := append(slices.Clone(entry), "json_pointer")

	return []schemaRule{
		containerRule(append(slices.Clone(entry), "path"), keywordMinLength, "must not contain empty paths"),
		containerRule(append(slices.Clone(entry), "path"), keywordRequired, "must not contain empty paths"),
		{
			path:    slices.Clone(format),
			keyword: keywordEnum,
			message: func(found violation) string {
				return fmt.Sprintf(
					"%s format must be %s, got %q",
					container(found.location, versionFileEntryDepth),
					orList(enumValues(found)),
					scalar(found.value),
				)
			},
		},
		containerRule(format, keywordRequired, "format is required"),
		containerRule(pointer, keywordRequired, "json_pointer is required for format "+strconv.Quote(
			string(VersionFileFormatJSON),
		)),
		{
			path:    slices.Clone(pointer),
			keyword: keywordPattern,
			message: func(found violation) string {
				err := validateJSONPointerSyntax(scalar(found.value))
				if err == nil {
					return container(found.location, versionFileEntryDepth) + " json_pointer is malformed"
				}

				return fmt.Sprintf("%s json_pointer: %v", container(found.location, versionFileEntryDepth), err)
			},
		},
		containerRule(entry, keywordNot, "json_pointer requires format "+strconv.Quote(
			string(VersionFileFormatJSON),
		)),
	}
}

func releaseRules() []schemaRule {
	labels := []string{releaseNode, "labels"}
	extra := append(slices.Clone(labels), "extra")
	reviewers := []string{releaseNode, "reviewers"}

	return slices.Concat(
		labelRules(append(slices.Clone(labels), "pending")),
		labelRules(append(slices.Clone(labels), "tagged")),
		entryRules(extra, "must not contain blank labels", "release.labels.extra entry"),
		[]schemaRule{
			duplicateRule(extra, "release.labels.extra entry %q duplicates release.labels.extra"),
			enumRule([]string{releaseNode, "auto_merge_method"}),
			{
				path:    []string{releaseNode, "pr_body_max_length"},
				keyword: keywordMinimum,
				message: func(found violation) string {
					return "release.pr_body_max_length must not be negative, got " + scalar(found.value)
				},
			},
		},
		entryRules(reviewers, "must not contain empty strings", "release.reviewers entry"),
		[]schemaRule{duplicateRule(reviewers, "release.reviewers contains duplicate %q")},
		releaseChannelRules(),
	)
}

func releaseChannelRules() []schemaRule {
	channels := []string{releaseNode, "channels"}
	channel := append(slices.Clone(channels), anySegment)

	return []schemaRule{
		keysRule(channels),
		emptyRule(append(slices.Clone(channel), "branch"), keywordMinLength),
		emptyRule(append(slices.Clone(channel), "branch"), keywordRequired),
		emptyRule(append(slices.Clone(channel), "prerelease"), keywordMinLength),
		emptyRule(append(slices.Clone(channel), "prerelease"), keywordRequired),
		blankRule(append(slices.Clone(channel), "changelog_file"), keywordMinLength),
	}
}

func targetRules() []schemaRule {
	targets := []string{"targets"}
	target := append(slices.Clone(targets), anySegment)
	includes := append(slices.Clone(target), "includes")

	return slices.Concat(
		[]schemaRule{
			emptyRule(targets, keywordMinProperties),
			emptyRule(targets, keywordRequired),
			{
				path:    slices.Clone(targets),
				keyword: keywordPropertyNames,
				message: func(violation) string { return "target IDs must be unique and non-empty" },
			},
			targetTypeRule(keywordConst),
			targetTypeRule(keywordRequired),
			emptyRule(append(slices.Clone(target), "tag_prefix"), keywordMinLength),
			emptyRule(append(slices.Clone(target), "tag_prefix"), keywordRequired),
			enumRule(append(slices.Clone(target), "versioning")),
		},
		calVerRules(target),
		changelogRules(target),
		versionFileRules(target),
		[]schemaRule{
			emptyRule(append(slices.Clone(target), "path"), keywordMinLength),
			emptyRule(append(slices.Clone(target), "path"), keywordRequired),
			containerRule(
				append(slices.Clone(target), "exclude_paths", anySegment),
				keywordMinLength,
				"contains must not be empty",
			),
			emptyRule(includes, keywordMinItems),
			emptyRule(includes, keywordRequired),
			{
				path:    append(slices.Clone(includes), anySegment),
				keyword: keywordMinLength,
				message: func(found violation) string {
					return fmt.Sprintf(
						"%s entry %q does not refer to a defined target",
						container(found.location, 1),
						scalar(found.value),
					)
				},
			},
		},
	)
}

func labelRules(path []string) []schemaRule {
	blank := func(found violation) string { return dotted(found.location) + " must not be blank" }

	return []schemaRule{
		{path: slices.Clone(path), keyword: keywordMinLength, message: blank},
		{path: slices.Clone(path), keyword: keywordRequired, message: blank},
		{
			path:    slices.Clone(path),
			keyword: keywordPattern,
			message: func(found violation) string {
				value := scalar(found.value)
				if strings.TrimSpace(value) == "" {
					return blank(found)
				}

				return dotted(found.location) + " " + strconv.Quote(value) +
					" must not have leading or trailing whitespace"
			},
		},
	}
}

// entryRules covers a string array whose blank entries and padded entries carry
// different wording, the way validate.go phrased extra labels and reviewers.
func entryRules(path []string, blankSuffix, entryPrefix string) []schemaRule {
	item := append(slices.Clone(path), anySegment)
	blank := containerRule(item, keywordMinLength, blankSuffix)

	return []schemaRule{
		blank,
		{
			path:    slices.Clone(item),
			keyword: keywordPattern,
			message: func(found violation) string {
				value := scalar(found.value)
				if strings.TrimSpace(value) == "" {
					return blank.message(found)
				}

				return entryPrefix + " " + strconv.Quote(value) +
					" must not have leading or trailing whitespace"
			},
		},
	}
}

func duplicateRule(path []string, format string) schemaRule {
	return schemaRule{
		path:    slices.Clone(path),
		keyword: keywordUniqueItems,
		message: func(found violation) string {
			return fmt.Sprintf(format, duplicateValue(found))
		},
	}
}

func enumRule(path []string) schemaRule {
	return schemaRule{
		path:    slices.Clone(path),
		keyword: keywordEnum,
		message: func(found violation) string {
			return fmt.Sprintf(
				"%s must be %s, got %q",
				dotted(found.location),
				orList(enumValues(found)),
				scalar(found.value),
			)
		},
	}
}

func targetTypeRule(keyword string) schemaRule {
	return schemaRule{
		path:    []string{"targets", anySegment, "type"},
		keyword: keyword,
		message: func(found violation) string {
			return fmt.Sprintf(
				"%s must be %q or %q, got %q",
				dotted(found.location),
				TargetTypePath,
				TargetTypeDerived,
				scalar(found.value),
			)
		},
	}
}

func emptyRule(path []string, keyword string) schemaRule {
	return schemaRule{
		path:    slices.Clone(path),
		keyword: keyword,
		message: func(found violation) string { return dotted(found.location) + " must not be empty" },
	}
}

func blankRule(path []string, keyword string) schemaRule {
	return schemaRule{
		path:    slices.Clone(path),
		keyword: keyword,
		message: func(found violation) string { return dotted(found.location) + " must not be blank" },
	}
}

func keysRule(path []string) schemaRule {
	return schemaRule{
		path:    slices.Clone(path),
		keyword: keywordPropertyNames,
		message: func(found violation) string { return dotted(found.location) + " keys must not be empty" },
	}
}

func indexedRule(path []string, keyword, suffix string) schemaRule {
	return schemaRule{
		path:    slices.Clone(path),
		keyword: keyword,
		message: func(found violation) string { return indexed(found.location) + " " + suffix },
	}
}

// containerRule reports against the collection holding the offending entry,
// which is how validate.go worded rules on array items and nested objects.
func containerRule(path []string, keyword, suffix string) schemaRule {
	depth := 0

	for index := len(path) - 1; index >= 0 && path[index] != anySegment; index-- {
		depth++
	}

	return schemaRule{
		path:    slices.Clone(path),
		keyword: keyword,
		message: func(found violation) string {
			return container(found.location, depth+1) + " " + suffix
		},
	}
}

func (r schemaRule) matches(found violation) bool {
	if r.keyword != found.keyword || len(r.path) != len(found.location) {
		return false
	}

	for index, segment := range r.path {
		if segment != anySegment && segment != found.location[index] {
			return false
		}
	}

	return true
}

func keywordOf(errorKind jsonschema.ErrorKind) string {
	if _, isNot := errorKind.(*kind.Not); isNot {
		return keywordNot
	}

	path := errorKind.KeywordPath()
	if len(path) == 0 {
		return ""
	}

	return path[len(path)-1]
}

func enumValues(found violation) []string {
	enum, isEnum := found.detail.(*kind.Enum)
	if !isEnum {
		return nil
	}

	values := make([]string, 0, len(enum.Want))
	for _, want := range enum.Want {
		values = append(values, scalar(want))
	}

	return values
}

func duplicateValue(found violation) string {
	duplicates, isUnique := found.detail.(*kind.UniqueItems)
	if !isUnique {
		return ""
	}

	items, isArray := found.value.([]any)
	if !isArray || duplicates.Duplicates[1] >= len(items) {
		return ""
	}

	return scalar(items[duplicates.Duplicates[1]])
}

func fallbackMessage(found violation) string {
	if len(found.location) == 0 {
		return "config does not match the config schema"
	}

	return indexed(found.location) + " does not match the config schema"
}

func dotted(location []string) string {
	return strings.Join(location, ".")
}

func container(location []string, depth int) string {
	if depth >= len(location) {
		return dotted(location)
	}

	return dotted(location[:len(location)-depth])
}

// indexed renders array positions the way validate.go did, as patterns[0]
// rather than patterns.0.
func indexed(location []string) string {
	rendered := strings.Builder{}

	for _, segment := range location {
		_, err := strconv.Atoi(segment)
		if err == nil {
			rendered.WriteString("[" + segment + "]")

			continue
		}

		if rendered.Len() > 0 {
			rendered.WriteString(".")
		}

		rendered.WriteString(segment)
	}

	return rendered.String()
}

func orList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}

	const pair = 2

	switch len(quoted) {
	case 0:
		return ""
	case 1:
		return quoted[0]
	case pair:
		return quoted[0] + " or " + quoted[1]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + ", or " + quoted[len(quoted)-1]
	}
}

func scalar(value any) string {
	text, isString := value.(string)
	if isString {
		return text
	}

	if value == nil {
		return ""
	}

	return fmt.Sprint(value)
}

func validateJSONPointerSyntax(pointer string) error {
	if pointer == "" || pointer[0] != '/' {
		return errJSONPointerMustStartWithSlash
	}

	for i := 0; i < len(pointer); i++ {
		if pointer[i] != '~' {
			continue
		}

		if i+1 >= len(pointer) || (pointer[i+1] != '0' && pointer[i+1] != '1') {
			return errJSONPointerInvalidEscape
		}

		i++
	}

	return nil
}
