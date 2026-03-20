package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/monkescience/yeet/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

var ErrConfigExists = errors.New("config file already exists")

func initCmd(options *bootstrapOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a .yeet.toml configuration file",
		Long: `Creates a yeet configuration file with sensible defaults.

By default this writes .yeet.toml in the current directory. Use --config to write a different path.`,
		Example: `  yeet init
  yeet init --config .yeet.release.toml`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInit(options.configPath())
		},
	}
}

func runInit(path string) error {
	slog.Debug("initializing config file", "path", path)

	_, statErr := os.Stat(path)
	if statErr == nil {
		return fmt.Errorf("%w: %s", ErrConfigExists, path)
	}

	cfg := config.Default()

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	content := append([]byte(config.SchemaDirective+"\n\n"), data...)

	err = os.WriteFile(path, content, 0o600) //nolint:mnd // secure file permissions
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	slog.Info("created config file", "path", path)

	return nil
}
