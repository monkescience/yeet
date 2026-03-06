package cli //nolint:testpackage // validates unexported runInit behavior directly

import (
	"os"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestRunInit(t *testing.T) {
	t.Run("writes config with schema directive", func(t *testing.T) {
		// given: an empty temporary workspace
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		// when: initializing config
		err := runInit()

		// then: config is created with schema directive and can be parsed
		testastic.NoError(t, err)

		content, readErr := os.ReadFile(config.DefaultFile)
		testastic.NoError(t, readErr)
		testastic.HasPrefix(t, string(content), config.SchemaDirective+"\n")

		_, parseErr := config.Parse(content)
		testastic.NoError(t, parseErr)
	})

	t.Run("fails when config already exists", func(t *testing.T) {
		// given: a workspace where config was already created
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		firstErr := runInit()
		testastic.NoError(t, firstErr)

		// when: initializing config again
		err := runInit()

		// then: command fails with existing-config error
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrConfigExists)
	})
}
