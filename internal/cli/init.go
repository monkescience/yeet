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

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a .yeet.toml configuration file",
		Long:  `Creates a .yeet.toml configuration file with sensible defaults in the current directory.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInit()
		},
	}
}

func runInit() error {
	path := config.DefaultFile

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
