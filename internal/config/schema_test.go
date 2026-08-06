package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

type schemaDocument struct {
	Schema string     `json:"$schema"`
	ID     string     `json:"$id"`
	Defs   schemaDefs `json:"$defs"`
}

type schemaDefs struct {
	Release       releaseDefinition       `json:"release"`
	ReleaseLabels releaseLabelsDefinition `json:"release_labels"`
	VersionFile   versionFileDefinition   `json:"version_file"`
}

type releaseLabelsDefinition struct {
	Properties struct {
		Pending defaultDefinition `json:"pending"`
		Tagged  defaultDefinition `json:"tagged"`
		Yeet    boolDefinition    `json:"yeet"`
		Extra   arrayDefinition   `json:"extra"`
	} `json:"properties"`
}

type versionFileDefinition struct {
	OneOf []json.RawMessage `json:"oneOf"`
}

type releaseDefinition struct {
	Properties releaseProperties `json:"properties"`
}

type releaseProperties struct {
	AutoMergeMethod    enumDefinition    `json:"auto_merge_method"`
	Reviewers          arrayDefinition   `json:"reviewers"`
	Labels             refDefinition     `json:"labels"`
	PRTitle            defaultDefinition `json:"pr_title"`
	PRTitleGroup       defaultDefinition `json:"pr_title_group"`
	CommitSubject      defaultDefinition `json:"commit_subject"`
	CommitSubjectGroup defaultDefinition `json:"commit_subject_group"`
}

type arrayDefinition struct {
	Type  string          `json:"type"`
	Items json.RawMessage `json:"items"`
}

type enumDefinition struct {
	Enum []string `json:"enum"`
}

type refDefinition struct {
	Ref string `json:"$ref"`
}

type defaultDefinition struct {
	Default string `json:"default"`
}

type boolDefinition struct {
	Type    string `json:"type"`
	Default bool   `json:"default"`
}

func TestConfigSchema(t *testing.T) {
	t.Parallel()

	t.Run("is valid JSON schema", func(t *testing.T) {
		t.Parallel()

		// given: schema file path
		schemaPath := schemaFilePath(t)

		// when: reading and parsing schema json
		data, readErr := os.ReadFile(schemaPath)
		testastic.NoError(t, readErr)

		var doc schemaDocument

		unmarshalErr := json.Unmarshal(data, &doc)

		// then: schema is valid json and points to expected urls
		testastic.NoError(t, unmarshalErr)
		testastic.Equal(t, "https://json-schema.org/draft/2020-12/schema", doc.Schema)
		testastic.Equal(t, config.DefaultSchemaURL, doc.ID)
	})

	t.Run("contains auto merge method enum values", func(t *testing.T) {
		t.Parallel()

		// given: parsed schema document
		schemaPath := schemaFilePath(t)

		data, readErr := os.ReadFile(schemaPath)
		testastic.NoError(t, readErr)

		var doc schemaDocument

		unmarshalErr := json.Unmarshal(data, &doc)
		testastic.NoError(t, unmarshalErr)

		// then: all supported merge methods are present
		testastic.SliceEqual(
			t,
			[]string{"auto", "squash", "rebase", "merge"},
			doc.Defs.Release.Properties.AutoMergeMethod.Enum,
		)
	})

	t.Run("contains release reviewers array", func(t *testing.T) {
		t.Parallel()

		// given: parsed schema document
		schemaPath := schemaFilePath(t)

		data, readErr := os.ReadFile(schemaPath)
		testastic.NoError(t, readErr)

		var doc schemaDocument

		unmarshalErr := json.Unmarshal(data, &doc)
		testastic.NoError(t, unmarshalErr)

		// then: release.reviewers is an array of strings
		testastic.Equal(t, "array", doc.Defs.Release.Properties.Reviewers.Type)
		testastic.True(t, len(doc.Defs.Release.Properties.Reviewers.Items) > 0)
	})

	t.Run("contains release labels and title templates", func(t *testing.T) {
		t.Parallel()

		// given: parsed schema document
		data, readErr := os.ReadFile(schemaFilePath(t))
		testastic.NoError(t, readErr)

		var doc schemaDocument

		unmarshalErr := json.Unmarshal(data, &doc)
		testastic.NoError(t, unmarshalErr)

		// then: lifecycle defaults, extra labels, and both title fields are represented
		testastic.Equal(t, "#/$defs/release_labels", doc.Defs.Release.Properties.Labels.Ref)
		testastic.Equal(t, "autorelease: pending", doc.Defs.ReleaseLabels.Properties.Pending.Default)
		testastic.Equal(t, "autorelease: tagged", doc.Defs.ReleaseLabels.Properties.Tagged.Default)
		testastic.Equal(t, "boolean", doc.Defs.ReleaseLabels.Properties.Yeet.Type)
		testastic.True(t, doc.Defs.ReleaseLabels.Properties.Yeet.Default)
		testastic.Equal(t, "array", doc.Defs.ReleaseLabels.Properties.Extra.Type)
		testastic.Equal(t, "", doc.Defs.Release.Properties.PRTitle.Default)
		testastic.Equal(t, "", doc.Defs.Release.Properties.PRTitleGroup.Default)
		testastic.Equal(t, "", doc.Defs.Release.Properties.CommitSubject.Default)
		testastic.Equal(t, "", doc.Defs.Release.Properties.CommitSubjectGroup.Default)
	})

	t.Run("contains string and object version file forms", func(t *testing.T) {
		t.Parallel()

		// given: parsed schema document
		schemaPath := schemaFilePath(t)

		data, readErr := os.ReadFile(schemaPath)
		testastic.NoError(t, readErr)

		var doc schemaDocument

		unmarshalErr := json.Unmarshal(data, &doc)
		testastic.NoError(t, unmarshalErr)

		// then: version_files accepts legacy string paths and structured entries
		testastic.Equal(t, 2, len(doc.Defs.VersionFile.OneOf))
	})
}

func schemaFilePath(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	testastic.True(t, ok)

	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "yeet.schema.json"))
}
