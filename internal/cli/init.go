package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"

	"github.com/monkescience/yeet/internal/config"
	"github.com/spf13/cobra"
)

var ErrConfigExists = errors.New("config file already exists")

var targetNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

const fallbackTargetName = "root"

func initCmd(options *bootstrapOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a .yeet.yaml configuration file",
		Long: `Creates a yeet configuration file with sensible defaults.

	By default this writes .yeet.yaml at the repository root when inside a git
	repository, or in the current directory otherwise. Use --config to write a
	different path.`,
		Example: `  yeet init
  yeet init --config .yeet.release.yaml`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInit(options.configPath())
		},
	}
}

func runInit(path string) error {
	resolvedPath, err := resolveInitConfigPath(path)
	if err != nil {
		return fmt.Errorf("resolve init config path: %w", err)
	}

	slog.Debug("initializing config file", "path", resolvedPath)

	_, statErr := os.Stat(resolvedPath)
	if statErr == nil {
		return fmt.Errorf("%w: %s", ErrConfigExists, resolvedPath)
	}

	content := renderInitConfig(deriveTargetName(resolvedPath))

	err = os.WriteFile(resolvedPath, []byte(content), 0o600) //nolint:mnd // secure file permissions
	if err != nil {
		return fmt.Errorf("write %s: %w", resolvedPath, err)
	}

	slog.Info("created config file", "path", resolvedPath)

	return nil
}

func renderInitConfig(targetName string) string {
	return fmt.Sprintf(`%s

targets:
  %s:
    type: path
    path: .
    tag_prefix: v
`, config.SchemaDirective, targetName)
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
