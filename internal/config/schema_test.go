package config //nolint:testpackage // exercises the embedded schema through unexported parse

import (
	"testing"

	"github.com/monkescience/testastic"
)

const schemaTestTarget = "targets:\n  app:\n    type: path\n    path: .\n    tag_prefix: v\n"

func TestEmbeddedSchemaCompiles(t *testing.T) {
	t.Parallel()

	// given: the schema embedded from the repository root

	// when: compiling it
	schema, err := compiledSchema()

	// then: it compiles once and is ready to validate
	testastic.NoError(t, err)
	testastic.NotNil(t, schema)
}

func TestSchemaTightenings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "version file object form must name its format",
			input: "version_files:\n  - path: VERSION\n" + schemaTestTarget,
			want:  "invalid config: version_files format is required",
		},
		{
			name:  "target type is matched exactly rather than trimmed",
			input: "targets:\n  app:\n    type: \" path \"\n    path: .\n    tag_prefix: v\n",
			want:  "invalid config: targets.app.type must be \"path\" or \"derived\", got \" path \"",
		},
		{
			name:  "stable branch must not be whitespace only",
			input: "branch: \"   \"\n" + schemaTestTarget,
			want:  "invalid config: branch must not be blank",
		},
		{
			name: "repository string fields reject an empty value",
			input: "provider: github\nrepository:\n  github:\n    host: \"\"\n    owner: o\n    repo: r\n" +
				schemaTestTarget,
			want: "invalid config: repository.github.host must not be blank",
		},
		{
			name:  "changelog section keys must not be empty",
			input: "changelog:\n  sections:\n    \"\": Things\n" + schemaTestTarget,
			want:  "invalid config: changelog.sections keys must not be empty",
		},
		{
			name:  "changelog section headings must not be empty",
			input: "changelog:\n  sections:\n    fix: \"\"\n" + schemaTestTarget,
			want:  "invalid config: changelog.sections.fix must not be blank",
		},
		{
			name:  "changelog section headings must not be whitespace only",
			input: "changelog:\n  sections:\n    fix: \"   \"\n" + schemaTestTarget,
			want:  "invalid config: changelog.sections.fix must not be blank",
		},
		{
			name:  "changelog section headings must be strings",
			input: "changelog:\n  sections:\n    fix: null\n" + schemaTestTarget,
			want:  "invalid config: changelog.sections.fix must be a string",
		},
		{
			name: "target changelog section headings must not be blank",
			input: "targets:\n  app:\n    type: path\n    path: .\n    tag_prefix: v\n" +
				"    changelog:\n      sections:\n        fix: \"  \"\n",
			want: "invalid config: targets.app.changelog.sections.fix must not be blank",
		},
		{
			name:  "changelog section headings must not have surrounding whitespace",
			input: "changelog:\n  sections:\n    fix: \" Bug Fixes\"\n" + schemaTestTarget,
			want:  "invalid config: changelog.sections.fix must not have leading or trailing whitespace",
		},
		{
			name:  "breaking must not be in changelog include",
			input: "changelog:\n  include:\n    - feat\n    - breaking\n" + schemaTestTarget,
			want: "invalid config: changelog.include must not contain \"breaking\" because breaking changes are " +
				"included automatically",
		},
		{
			name:  "changelog section headings must be a single line",
			input: "changelog:\n  sections:\n    fix: \"Bug\\nFixes\"\n" + schemaTestTarget,
			want:  "invalid config: changelog.sections.fix must be a single line",
		},
		{
			name:  "changelog section headings reject Unicode line separators",
			input: "changelog:\n  sections:\n    fix: \"Bug\\u2028Fixes\"\n" + schemaTestTarget,
			want:  "invalid config: changelog.sections.fix must be a single line",
		},
		{
			name:  "changelog section headings reject leading Markdown markers",
			input: "changelog:\n  sections:\n    fix: \"### Bug Fixes\"\n" + schemaTestTarget,
			want: "invalid config: changelog.sections.fix must contain heading text without leading or closing " +
				"Markdown # markers",
		},
		{
			name: "target changelog section headings reject closing Markdown markers",
			input: "targets:\n  app:\n    type: path\n    path: .\n    tag_prefix: v\n" +
				"    changelog:\n      sections:\n        fix: \"Bug Fixes ###\"\n",
			want: "invalid config: targets.app.changelog.sections.fix must contain heading text without leading or " +
				"closing Markdown # markers",
		},
		{
			name:  "reference footer keys must not be empty",
			input: "changelog:\n  references:\n    footers:\n      \"\": https://tracker/{value}\n" + schemaTestTarget,
			want:  "invalid config: changelog.references.footers keys must not be empty",
		},
		{
			name: "reference pattern url must be a string",
			input: "changelog:\n  references:\n    patterns:\n      - pattern: 'X-\\d+'\n        url: 123\n" +
				schemaTestTarget,
			want: "invalid config: changelog.references.patterns[0].url must be a string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: a config that older versions accepted

			// when: parsing the config
			_, err := parse([]byte(tc.input))

			// then: the tightened rule rejects it
			testastic.Error(t, err)
			testastic.ErrorIs(t, err, ErrInvalidConfig)
			testastic.Equal(t, tc.want, err.Error())
		})
	}
}

func TestSchemaAcceptsLiteralChangelogHeadingText(t *testing.T) {
	t.Parallel()

	// given: headings with emoji, literal hashes, and internal whitespace
	input := "changelog:\n  sections:\n    feat: 🚀 Features\n    fix: C# Integration\n" +
		"    perf: Release###\n    revert: \"#123\"\n    docs: \"API\\tDocumentation\"\n" + schemaTestTarget

	// when: parsing the config
	_, err := parse([]byte(input))

	// then: the heading text is accepted unchanged
	testastic.NoError(t, err)
}

func TestSchemaRejectsDuplicateChangelogIncludes(t *testing.T) {
	t.Parallel()

	// given: a YAML config with the same changelog commit type twice
	input := "changelog:\n  include:\n    - fix\n    - fix\n" + schemaTestTarget

	// when: parsing the config
	_, err := parse([]byte(input))

	// then: schema validation rejects the duplicate by name
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, ErrInvalidConfig)
	testastic.Equal(t, "invalid config: changelog.include contains duplicate \"fix\"", err.Error())
}

func TestSchemaKeepsGoOwnedRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "stable is reserved regardless of case",
			input: "release:\n  channels:\n    Stable:\n      branch: next\n      prerelease: rc\n" +
				schemaTestTarget,
			want: "invalid config: release.channels.Stable must not use reserved name stable",
		},
		{
			name:  "a whitespace-only target key is rejected",
			input: "targets:\n  \"   \":\n    type: path\n    path: .\n    tag_prefix: v\n",
			want:  "invalid config: target IDs must be unique and non-empty",
		},
		{
			name:  "a whitespace-only tag prefix is rejected",
			input: "targets:\n  app:\n    type: path\n    path: .\n    tag_prefix: \"   \"\n",
			want:  "invalid config: targets.app.tag_prefix must not be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: a config the schema alone would accept

			// when: parsing the config
			_, err := parse([]byte(tc.input))

			// then: the Go-owned rule still rejects it
			testastic.Error(t, err)
			testastic.ErrorIs(t, err, ErrInvalidConfig)
			testastic.Equal(t, tc.want, err.Error())
		})
	}
}

func TestSchemaErrorTranslation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "a nested target version file names its target",
			input: "targets:\n  app:\n    type: path\n    path: .\n    tag_prefix: v\n" +
				"    version_files:\n      - path: package.json\n        format: json\n",
			want: "invalid config: targets.app.version_files json_pointer is required for format \"json\"",
		},
		{
			name: "a marker version file may not carry a pointer",
			input: "targets:\n  app:\n    type: path\n    path: .\n    tag_prefix: v\n" +
				"    version_files:\n      - path: package.json\n        format: markers\n" +
				"        json_pointer: /version\n",
			want: "invalid config: targets.app.version_files json_pointer requires format \"json\"",
		},
		{
			name: "a malformed pointer keeps the escape wording",
			input: "version_files:\n  - path: package.json\n    format: json\n    json_pointer: /a~x\n" +
				schemaTestTarget,
			want: "invalid config: version_files json_pointer: contains invalid escape",
		},
		{
			name:  "an enum lists four alternatives with an oxford comma",
			input: "release:\n  auto_merge_method: wrongo\n" + schemaTestTarget,
			want: "invalid config: release.auto_merge_method must be \"auto\", \"squash\", \"rebase\", " +
				"or \"merge\", got \"wrongo\"",
		},
		{
			name:  "a nested changelog reference is reported by index",
			input: "changelog:\n  references:\n    patterns:\n      - pattern: \"\"\n" + schemaTestTarget,
			want:  "invalid config: changelog.references.patterns[0].pattern must not be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: a config breaking a rule the schema owns

			// when: parsing the config
			_, err := parse([]byte(tc.input))

			// then: the schema failure reads like the rule validate.go used to carry
			testastic.Error(t, err)
			testastic.ErrorIs(t, err, ErrInvalidConfig)
			testastic.Equal(t, tc.want, err.Error())
		})
	}
}

func TestSchemaRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "release tag pins the ref", version: "v1.2.3", want: "v1.2.3"},
		{name: "goreleaser strips the v prefix", version: "1.2.3", want: "v1.2.3"},
		{name: "prerelease tag pins the ref", version: "v1.2.3-rc.1", want: "v1.2.3-rc.1"},
		{name: "go build reports a development module", version: "(devel)", want: schemaDevelopmentRef},
		{name: "missing build info falls back", version: "unknown", want: schemaDevelopmentRef},
		{name: "git describe below a tag is not a ref", version: "v1.2.3-4-gabc1234", want: schemaDevelopmentRef},
		{name: "a dirty worktree is not a ref", version: "v1.2.3-dirty", want: schemaDevelopmentRef},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: a version string the build package could report

			// when: deriving the schema ref
			got := schemaRef(tc.version)

			// then: only an exact release tag is pinned
			testastic.Equal(t, tc.want, got)
		})
	}
}

func TestSchemaDirective(t *testing.T) {
	t.Parallel()

	// given: a development build, which is what the test binary reports

	// when: rendering the directive yeet init writes
	got := SchemaDirective()

	// then: it points at the unreleased schema
	testastic.Equal(
		t,
		"# yaml-language-server: $schema=https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json",
		got,
	)
}
