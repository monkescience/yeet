package commands

import (
	"errors"
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

func NewRoot() *cobra.Command {
	options := &bootstrapOptions{}

	cmd := &cobra.Command{
		Use:   build.ServiceName,
		Short: "Automate releases based on conventional commits",
		Long: `yeet analyzes conventional commits to automatically determine the next
version, generate changelogs, and create release PRs/MRs on GitHub, GitLab, or
Azure DevOps.

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

func (o *bootstrapOptions) configureLogging(cmd *cobra.Command) error {
	if o.verbose && o.quiet {
		return errVerboseQuietConflict
	}

	level := charmlog.InfoLevel
	if o.verbose {
		level = charmlog.DebugLevel
	}

	if o.quiet {
		level = charmlog.WarnLevel
	}

	logger := charmlog.NewWithOptions(cmd.ErrOrStderr(), charmlog.Options{
		Level:           level,
		ReportTimestamp: false,
	})

	logger.SetColorProfile(resolveColorProfile(cmd.ErrOrStderr(), o.noColor))

	slog.SetDefault(slog.New(logger))

	return nil
}

// resolveColorProfile picks the color profile for an output stream based on
// the explicit --no-color flag, the destination writer's TTY-ness, and
// standard env vars (NO_COLOR, CLICOLOR, CLICOLOR_FORCE, TERM, COLORTERM).
// When noColor is set, all color sequences are stripped. Text decoration
// like bold/faint is preserved (per the NO_COLOR spec). Otherwise the
// profile follows colorprofile.Detect, which strips everything for
// non-TTY destinations such as pipes, files, and CI.
func resolveColorProfile(out io.Writer, noColor bool) colorprofile.Profile {
	if noColor {
		return colorprofile.Ascii
	}

	return colorprofile.Detect(out, os.Environ())
}

func newColorWriter(w io.Writer, noColor bool) *colorprofile.Writer {
	return &colorprofile.Writer{
		Forward: w,
		Profile: resolveColorProfile(w, noColor),
	}
}

func (o *bootstrapOptions) configPath() string {
	return strings.TrimSpace(o.configFile)
}
