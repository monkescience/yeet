package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
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
		return fmt.Errorf("%w: %s", ErrExists, resolvedPath)
	}

	content := renderInitial(deriveTargetName(resolvedPath))

	if err := os.WriteFile(resolvedPath, []byte(content), 0o600); err != nil { //nolint:mnd // secure file permissions
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
`, SchemaDirective(), targetName)
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
