// Package cli defines the command-line interface for yeet.
package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/monkescience/yeet/internal/config"
	"github.com/spf13/cobra"
)

var (
	buildVersion            = "dev"
	buildCommit             = "none"
	buildDate               = "unknown"
	errVerboseQuietConflict = errors.New("--verbose and --quiet cannot be used together")
)

type bootstrapOptions struct {
	configFile string
	verbose    bool
	quiet      bool
}

type buildInfo struct {
	version string
	commit  string
	built   string
}

func Execute() {
	err := rootCmd().Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	options := &bootstrapOptions{}

	cmd := &cobra.Command{
		Use:   "yeet",
		Short: "Automate releases based on conventional commits",
		Long: `yeet analyzes conventional commits to automatically determine the next
version, generate changelogs, and create release PRs/MRs on GitHub or GitLab.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return options.configureLogging(cmd)
		},
	}

	cmd.PersistentFlags().StringVar(&options.configFile, "config", "", "config file (default is .yeet.toml)")
	cmd.PersistentFlags().BoolVarP(&options.verbose, "verbose", "v", false, "enable debug logging")
	cmd.PersistentFlags().BoolVar(&options.quiet, "quiet", false, "show warnings and errors only")

	cmd.AddCommand(
		releaseCmd(options),
		initCmd(options),
		versionCmd(),
	)

	cmd.InitDefaultCompletionCmd()

	return cmd
}

func (options *bootstrapOptions) configureLogging(cmd *cobra.Command) error {
	if options.verbose && options.quiet {
		return errVerboseQuietConflict
	}

	level := slog.LevelInfo
	if options.verbose {
		level = slog.LevelDebug
	}

	if options.quiet {
		level = slog.LevelWarn
	}

	handler := slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.Attr{}
			}

			return attr
		},
	})

	slog.SetDefault(slog.New(handler))

	return nil
}

func (options *bootstrapOptions) configPath() string {
	path := strings.TrimSpace(options.configFile)
	if path == "" {
		return config.DefaultFile
	}

	return path
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printVersion(cmd.OutOrStdout(), currentBuildInfo())
		},
	}
}

func currentBuildInfo() buildInfo {
	return buildInfo{
		version: defaultBuildValue(buildVersion, "dev"),
		commit:  defaultBuildValue(buildCommit, "none"),
		built:   defaultBuildValue(buildDate, "unknown"),
	}
}

func defaultBuildValue(value, fallback string) string {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return fallback
	}

	return trimmedValue
}

func printVersion(w io.Writer, info buildInfo) error {
	_, err := fmt.Fprintf(w, "version: %s\ncommit: %s\nbuilt: %s\n", info.version, info.commit, info.built)
	if err != nil {
		return fmt.Errorf("print version: %w", err)
	}

	return nil
}
