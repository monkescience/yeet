package config //nolint:testpackage // exercises unexported config path discovery directly

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v6"
	"github.com/monkescience/testastic"
)

func TestResolveConfigPath(t *testing.T) {
	t.Run("finds nearest ancestor config from nested directory", func(t *testing.T) {
		// given: nested directories with multiple ancestor config files
		repositoryPath := t.TempDir()
		rootConfigPath := filepath.Join(repositoryPath, DefaultFile)
		appsPath := filepath.Join(repositoryPath, "apps")
		servicePath := filepath.Join(appsPath, "service")

		err := os.WriteFile(rootConfigPath, []byte(SchemaDirective()+"\n"), 0o644)
		testastic.NoError(t, err)

		err = os.MkdirAll(servicePath, 0o755)
		testastic.NoError(t, err)

		appsConfigPath := filepath.Join(appsPath, DefaultFile)
		err = os.WriteFile(appsConfigPath, []byte(SchemaDirective()+"\n"), 0o644)
		testastic.NoError(t, err)
		t.Chdir(servicePath)

		// when: resolving the default config path
		resolvedPath, resolveErr := resolvePath(context.Background(), "")

		// then: the nearest ancestor config is selected
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, appsConfigPath, resolvedPath)
	})

	t.Run("explicit path bypasses ancestor discovery", func(t *testing.T) {
		// given: a nested directory with an ancestor config file
		repositoryPath := t.TempDir()
		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err := os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)

		err = os.WriteFile(filepath.Join(repositoryPath, DefaultFile), []byte(SchemaDirective()+"\n"), 0o644)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving an explicit config path
		resolvedPath, resolveErr := resolvePath(context.Background(), " custom.yaml ")

		// then: the explicit path is used as-is after trimming
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, "custom.yaml", resolvedPath)
	})

	t.Run("missing default config reports the default filename", func(t *testing.T) {
		// given: a directory tree without any yeet config file
		repositoryPath := t.TempDir()
		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err := os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default config path
		resolvedPath, resolveErr := resolvePath(context.Background(), "")

		// then: the missing path is reported against the default filename
		testastic.Equal(t, DefaultFile, resolvedPath)
		testastic.Error(t, resolveErr)
		testastic.ErrorIs(t, resolveErr, os.ErrNotExist)
	})

	t.Run("does not escape the repository root", func(t *testing.T) {
		// given: a git repository nested under a parent directory with an unrelated config file
		workspacePath := t.TempDir()
		parentConfigPath := filepath.Join(workspacePath, DefaultFile)
		err := os.WriteFile(parentConfigPath, []byte(SchemaDirective()+"\n"), 0o644)
		testastic.NoError(t, err)

		repositoryPath := filepath.Join(workspacePath, "service")
		_, err = git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default config path
		resolvedPath, resolveErr := resolvePath(context.Background(), "")

		// then: discovery stops at the repo root instead of using the parent config
		testastic.Equal(t, DefaultFile, resolvedPath)
		testastic.Error(t, resolveErr)
		testastic.ErrorIs(t, resolveErr, os.ErrNotExist)
	})
}

func TestResolveInitConfigPath(t *testing.T) {
	t.Run("targets repository root from nested directory", func(t *testing.T) {
		// given: a nested directory inside a git repository without an existing config file
		repositoryPath := t.TempDir()
		_, err := git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default init destination
		resolvedPath, resolveErr := resolveInitPath(context.Background(), "")

		// then: init targets the repository root config path
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, filepath.Join(repositoryPath, DefaultFile), resolvedPath)
	})

	t.Run("reuses existing ancestor config path", func(t *testing.T) {
		// given: a nested directory below an existing root config file
		repositoryPath := t.TempDir()
		_, err := git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		configPath := filepath.Join(repositoryPath, DefaultFile)
		err = os.WriteFile(configPath, []byte(SchemaDirective()+"\n"), 0o644)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "cmd", "yeet")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default init destination
		resolvedPath, resolveErr := resolveInitPath(context.Background(), "")

		// then: init points at the existing ancestor config file
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, configPath, resolvedPath)
	})

	t.Run("falls back to current directory outside git repositories", func(t *testing.T) {
		// given: a nested directory tree outside any git repository
		workspacePath := t.TempDir()
		nestedPath := filepath.Join(workspacePath, "internal", "cli")
		err := os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default init destination
		resolvedPath, resolveErr := resolveInitPath(context.Background(), "")

		// then: init falls back to the local default filename
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, DefaultFile, resolvedPath)
	})

	t.Run("does not reuse config outside the repository root", func(t *testing.T) {
		// given: a git repository nested under a parent directory with an unrelated config file
		workspacePath := t.TempDir()
		err := os.WriteFile(filepath.Join(workspacePath, DefaultFile), []byte(SchemaDirective()+"\n"), 0o644)
		testastic.NoError(t, err)

		repositoryPath := filepath.Join(workspacePath, "service")
		_, err = git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "cmd", "yeet")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default init destination
		resolvedPath, resolveErr := resolveInitPath(context.Background(), "")

		// then: init still targets the repo root config path
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, filepath.Join(repositoryPath, DefaultFile), resolvedPath)
	})
}
