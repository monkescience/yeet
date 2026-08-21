package commands

import (
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/telemetry"
	"github.com/spf13/cobra"
)

func initCmd(manager *telemetry.Manager) *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a .yeet.yaml configuration file",
		Args:  cobra.NoArgs,
		Long: `Creates a yeet configuration file with sensible defaults.

By default this writes .yeet.yaml at the repository root when inside a git
repository, or in the current directory otherwise. Use --config to write a
different path.`,
		Example: `  yeet init
  yeet init --config .yeet.release.yaml`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			started := time.Now()
			resolvedConfigPath := strings.TrimSpace(configFile)

			err := config.Initialize(cmd.Context(), resolvedConfigPath)
			manager.RecordInit(cmd.Context(), started, resolvedConfigPath, err)

			return err //nolint:wrapcheck // preserve existing user-facing init diagnostics
		},
	}

	cmd.Flags().StringVar(
		&configFile,
		"config",
		"",
		"path to config file (default: nearest ancestor .yeet.yaml)",
	)

	return cmd
}
