package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/monkescience/yeet/internal/config"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

var ErrConfigExists = errors.New("config file already exists")

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

	cfg := config.Default()
	cfg.Targets = map[string]config.Target{
		"default": {
			Type:      config.TargetTypePath,
			Path:      ".",
			TagPrefix: "v",
		},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	content := append([]byte(config.SchemaDirective+"\n\n"), data...)

	err = os.WriteFile(resolvedPath, content, 0o600) //nolint:mnd // secure file permissions
	if err != nil {
		return fmt.Errorf("write %s: %w", resolvedPath, err)
	}

	slog.Info("created config file", "path", resolvedPath)

	return nil
}
