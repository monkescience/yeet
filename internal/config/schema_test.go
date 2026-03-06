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
	Release releaseDefinition `json:"release"`
}

type releaseDefinition struct {
	Properties releaseProperties `json:"properties"`
}

type releaseProperties struct {
	AutoMergeMethod enumDefinition `json:"auto_merge_method"`
}

type enumDefinition struct {
	Enum []string `json:"enum"`
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
}

func schemaFilePath(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	testastic.True(t, ok)

	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "schema", "yeet.schema.json"))
}
