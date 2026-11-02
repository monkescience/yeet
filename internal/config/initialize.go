package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"go.yaml.in/yaml/v4"
)

var ErrExists = errors.New("config file already exists")

var targetNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

const fallbackTargetName = "root"

func Initialize(ctx context.Context, path string) error {
	resolvedPath, err := resolveInitPath(ctx, path)
	if err != nil {
		return fmt.Errorf("resolve init config path: %w", err)
	}

	slog.DebugContext(ctx, "initializing config file", slog.String("path", resolvedPath))

	_, statErr := os.Stat(resolvedPath)
	if statErr == nil {
		return newFileError(resolvedPath, ErrExists)
	}

	content := renderInitial(deriveTargetName(resolvedPath))

	err = os.WriteFile(resolvedPath, []byte(content), 0o600) //nolint:mnd // secure file permissions
	if err != nil {
		return fmt.Errorf("write %s: %w", resolvedPath, err)
	}

	slog.InfoContext(ctx, "created config file", slog.String("path", resolvedPath))

	return nil
}

func renderInitial(targetName string) string {
	return fmt.Sprintf(`%s

targets:
  %s:
    type: path
    path: .
    tag_prefix: v
`, SchemaDirective(), yamlStringKey(targetName))
}

func yamlStringKey(name string) string {
	var decoded any

	err := yaml.Unmarshal([]byte(name), &decoded)
	if value, ok := decoded.(string); err == nil && ok && value == name {
		return name
	}

	return strconv.Quote(name)
}

func deriveTargetName(configPath string) string {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fallbackTargetName
	}

	name := filepath.Base(filepath.Dir(absPath))
	if !targetNamePattern.MatchString(name) {
		return fallbackTargetName
	}

	return name
}
