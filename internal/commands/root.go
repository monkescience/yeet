// Package commands defines the command-line interface for yeet.
package commands

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	charmlog "charm.land/log/v2"
	"github.com/charmbracelet/colorprofile"
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

func NewRoot() *cobra.Command {
	options := &bootstrapOptions{}

	cmd := &cobra.Command{
		Use:   build.ServiceName,
		Short: "Automate releases based on conventional commits",
		Long: `yeet analyzes conventional commits to automatically determine the next
version, generate changelogs, and create release PRs/MRs on GitHub or GitLab.

On the default branch it also finalizes merged release PRs/MRs labeled
autorelease: pending by creating the provider release and relabeling them as
autorelease: tagged.`,
		Example: `  yeet init
  yeet release --dry-run
  yeet release --auto-merge`,
		Version:       build.Version(),
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

	logger.SetColorProfile(resolveColorProfile(cmd.ErrOrStderr(), options.noColor))

	slog.SetDefault(slog.New(logger))

	return nil
}

// resolveColorProfile picks the color profile for an output stream based on
// the explicit --no-color flag, the destination writer's TTY-ness, and
// standard env vars (NO_COLOR, CLICOLOR, CLICOLOR_FORCE, TERM, COLORTERM).
// When noColor is set, all color sequences are stripped; text decoration
// like bold/faint is preserved (per the NO_COLOR spec). Otherwise the
// profile follows colorprofile.Detect, which strips everything for
// non-TTY destinations such as pipes, files, and CI.
func resolveColorProfile(out io.Writer, noColor bool) colorprofile.Profile {
	if noColor {
		return colorprofile.Ascii
	}

	return colorprofile.Detect(out, os.Environ())
}

// newColorWriter wraps w in a color-aware writer that downsamples ANSI
// sequences according to the resolved color profile.
func newColorWriter(w io.Writer, noColor bool) *colorprofile.Writer {
	return &colorprofile.Writer{
		Forward: w,
		Profile: resolveColorProfile(w, noColor),
	}
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
