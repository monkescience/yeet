// Package cli defines the command-line interface for yeet.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

func Execute() {
	err := rootCmd().Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "yeet",
		Short: "Automate releases based on conventional commits",
		Long: `yeet analyzes conventional commits to automatically determine the next
version, generate changelogs, and create release PRs/MRs on GitHub or GitLab.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is .yeet.toml)")

	cmd.AddCommand(
		releaseCmd(),
		tagCmd(),
		initCmd(),
	)

	return cmd
}
