package commands

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/monkescience/yeet/internal/build"
	"github.com/monkescience/yeet/internal/telemetry"
)

var errVerboseQuietConflict = errors.New("--verbose and --quiet cannot be used together")

type bootstrapOptions struct {
	verbose bool
	quiet   bool
	noColor bool
}

func NewRoot(manager *telemetry.Manager) *cobra.Command {
	options := &bootstrapOptions{}

	return newRoot(options, manager)
}

func newRoot(options *bootstrapOptions, manager *telemetry.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   build.ServiceName,
		Short: "Automate releases based on conventional commits",
		Long: `yeet analyzes conventional commits to automatically determine the next
version, generate changelogs, and create release PRs/MRs on GitHub, GitLab, or
Azure DevOps.

On the default branch it also finalizes merged release PRs/MRs carrying the
configured pending lifecycle label by creating the provider release and applying
the configured tagged lifecycle label.`,
		Example: `  yeet init
  yeet release --dry-run
  yeet release --auto-merge`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return options.configureLogging(cmd)
		},
	}

	cmd.PersistentFlags().BoolVarP(&options.verbose, "verbose", "v", false, "enable debug logging")
	cmd.PersistentFlags().BoolVar(&options.quiet, "quiet", false, "show warnings and errors only")
	cmd.PersistentFlags().BoolVar(&options.noColor, "no-color", false, "disable colored output")

	cmd.AddCommand(
		releaseCmd(options, manager),
		initCmd(manager),
		versionCmd(),
	)

	cmd.InitDefaultCompletionCmd()
	setExampleForSubcommand(cmd, "completion", `  yeet completion zsh
  yeet completion bash > /usr/local/etc/bash_completion.d/yeet`)
	annotateArgumentErrors(cmd)

	return cmd
}

func Execute(ctx context.Context, manager *telemetry.Manager) error {
	options := &bootstrapOptions{}
	root := newRoot(options, manager) //nolint:contextcheck // cobra passes ctx to callbacks through ExecuteContextC

	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		return nil
	}

	if cmd.CalledAs() == "" {
		_, remaining, _ := root.Find(os.Args[1:])
		_ = cmd.ParseFlags(remaining)
		err = &argumentError{values: cmd.Flags().Args(), unknownCommand: true, cause: err}
	}

	options.setLogger(cmd)
	reportCommandError(ctx, cmd, err)

	return err
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
	o.setLogger(cmd)

	if o.verbose && o.quiet {
		return errVerboseQuietConflict
	}

	return nil
}

func (o *bootstrapOptions) setLogger(cmd *cobra.Command) {
	level := slog.LevelInfo
	if o.verbose {
		level = slog.LevelDebug
	}

	if o.quiet {
		level = slog.LevelWarn
	}

	slog.SetDefault(newDiagnosticLogger(cmd.ErrOrStderr(), level, o.noColor))
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
