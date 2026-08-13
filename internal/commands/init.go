package commands

import (
	"strings"

	"github.com/monkescience/yeet/internal/config"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a .yeet.yaml configuration file",
		Long: `Creates a yeet configuration file with sensible defaults.

By default this writes .yeet.yaml at the repository root when inside a git
repository, or in the current directory otherwise. Use --config to write a
different path.`,
		Example: `  yeet init
  yeet init --config .yeet.release.yaml`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return config.Initialize(cmd.Context(), strings.TrimSpace(configFile))
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
