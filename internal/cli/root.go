// Package cli defines the command-line interface for yeet.
package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	charmlog "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/monkescience/yeet/internal/build"
)

var errVerboseQuietConflict = errors.New("--verbose and --quiet cannot be used together")

type bootstrapOptions struct {
	configFile string
	verbose    bool
	quiet      bool
	noColor    bool
}

type buildInfo struct {
	version string
	commit  string
	built   string
	module  string
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
version, generate changelogs, and create release PRs/MRs on GitHub or GitLab.

On the default branch it also finalizes merged release PRs/MRs labeled
autorelease: pending by creating the provider release and relabeling them as
autorelease: tagged.`,
		Example: `  yeet init
  yeet release --dry-run
  yeet release --auto-merge`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return options.configureLogging(cmd)
		},
	}

	cmd.PersistentFlags().StringVar(
		&options.configFile,
		"config",
		"",
		"path to config file (default: nearest ancestor .yeet.yaml)",
	)
	cmd.PersistentFlags().BoolVarP(&options.verbose, "verbose", "v", false, "enable debug logging")
	cmd.PersistentFlags().BoolVar(&options.quiet, "quiet", false, "show warnings and errors only")
	cmd.PersistentFlags().BoolVar(&options.noColor, "no-color", false, "disable colored output")

	cmd.AddCommand(
		releaseCmd(options),
		initCmd(options),
		versionCmd(),
	)

	cmd.InitDefaultCompletionCmd()
	setExampleForSubcommand(cmd, "completion", `  yeet completion zsh
  yeet completion bash > /usr/local/etc/bash_completion.d/yeet`)

	return cmd
}

func setExampleForSubcommand(root *cobra.Command, name string, example string) {
	for _, command := range root.Commands() {
		if command.Name() == name {
			command.Example = example

			return
		}
	}
}

func (options *bootstrapOptions) configureLogging(cmd *cobra.Command) error {
	if options.verbose && options.quiet {
		return errVerboseQuietConflict
	}

	level := charmlog.InfoLevel
	if options.verbose {
		level = charmlog.DebugLevel
	}

	if options.quiet {
		level = charmlog.WarnLevel
	}

	logger := charmlog.NewWithOptions(cmd.ErrOrStderr(), charmlog.Options{
		Level:           level,
		ReportTimestamp: false,
	})

	if options.noColor {
		logger.SetColorProfile(0) // termenv.Ascii — disables all color
	}

	slog.SetDefault(slog.New(logger))

	return nil
}

func (options *bootstrapOptions) configPath() string {
	return strings.TrimSpace(options.configFile)
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print build information",
		Example: `  yeet version`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printVersion(cmd.OutOrStdout(), currentBuildInfo())
		},
	}
}

func currentBuildInfo() buildInfo {
	return buildInfo{
		version: build.Version(),
		commit:  build.Commit(),
		built:   build.Date(),
		module:  build.Module(),
	}
}

func printVersion(w io.Writer, info buildInfo) error {
	_, err := fmt.Fprintf(w, "version: %s\ncommit: %s\nbuilt: %s\n", info.version, info.commit, info.built)
	if err != nil {
		return fmt.Errorf("print version: %w", err)
	}

	if info.module != "" {
		_, err = fmt.Fprintf(w, "module: %s\n", info.module)
		if err != nil {
			return fmt.Errorf("print module: %w", err)
		}
	}

	return nil
}
