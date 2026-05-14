package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
)

const schemaDirective = "# yaml-language-server: $schema=" +
	"https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json"

func TestInit(t *testing.T) {
	t.Parallel()

	t.Run("writes config at the explicit --config path", func(t *testing.T) {
		t.Parallel()

		// given: a fresh tempdir and an explicit --config target path
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".yeet.yaml")

		// when: running `yeet init --config <path>`
		result := binary.Run(t, "init", "--config", configPath)

		// then: the binary exits 0 and writes the schema-stamped config at that path
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)

		content, err := os.ReadFile(configPath)
		testastic.NoError(t, err)
		testastic.HasPrefix(t, string(content), schemaDirective+"\n")
	})

	t.Run("writes config in the working dir when --config is omitted", func(t *testing.T) {
		t.Parallel()

		// given: a fresh tempdir used as the working directory, no --config flag
		tempDir := t.TempDir()

		// when: running `yeet init` from that tempdir
		result := binary.RunWithOptions(t, []string{"init"}, testastic.WithRunWorkDir(tempDir))

		// then: the binary exits 0 and the config lands at ./.yeet.yaml
		testastic.Equal(t, 0, result.ExitCode)

		content, err := os.ReadFile(filepath.Join(tempDir, ".yeet.yaml"))
		testastic.NoError(t, err)
		testastic.HasPrefix(t, string(content), schemaDirective+"\n")
	})

	t.Run("fails when the config already exists", func(t *testing.T) {
		t.Parallel()

		// given: a config that was already written by a previous `yeet init`
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".yeet.yaml")

		first := binary.Run(t, "init", "--config", configPath)
		testastic.Equal(t, 0, first.ExitCode)

		// when: invoking `yeet init` against the same path a second time
		second := binary.Run(t, "init", "--config", configPath)

		// then: the binary exits 1 and stderr explains why
		testastic.Equal(t, 1, second.ExitCode)
		testastic.AssertFile(t, "testdata/init/config_already_exists/stderr.expected.txt", second.Stderr)
	})
}
