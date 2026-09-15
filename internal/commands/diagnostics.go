package commands

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/release"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type diagnostic struct {
	message  string
	hint     string
	attrs    []slog.Attr
	rawCause bool
}

func (d *diagnostic) report(ctx context.Context, err error) {
	attrs := append([]slog.Attr(nil), d.attrs...)
	if d.hint != "" {
		attrs = append(attrs, slog.String("hint", d.hint))
	}

	if cause := d.cause(err); cause != "" {
		attrs = append(attrs, slog.String("cause", cause))
	}

	slog.LogAttrs(ctx, slog.LevelError, d.message, attrs...)
	slog.LogAttrs(ctx, slog.LevelDebug, "failure detail", slog.String("error", err.Error()))
}

func (d *diagnostic) cause(err error) string {
	if cause := diagnosticCause(err); cause != "" {
		return cause
	}

	if d.rawCause {
		return safeCause(err)
	}

	return ""
}

func (d *diagnostic) explain(problem string) {
	if problem == "" {
		return
	}

	d.message += ": " + problem
}

type argumentError struct {
	values         []string
	unknownCommand bool
	cause          error
}

func (e *argumentError) Error() string { return e.cause.Error() }

func (e *argumentError) Unwrap() error { return e.cause }

func annotateArgumentErrors(cmd *cobra.Command) {
	if validate := cmd.Args; validate != nil {
		cmd.Args = func(cmd *cobra.Command, args []string) error {
			err := validate(cmd, args)
			if err != nil {
				return &argumentError{values: args, cause: err}
			}

			return nil
		}
	}

	for _, child := range cmd.Commands() {
		annotateArgumentErrors(child)
	}
}

func reportCommandError(ctx context.Context, cmd *cobra.Command, err error) {
	if failure, ok := errors.AsType[*release.Failure](err); ok {
		reportReleaseError(ctx, failure)

		return
	}

	d := commandDiagnostic(cmd, err)
	d.report(ctx, err)
}

func commandDiagnostic(cmd *cobra.Command, err error) diagnostic {
	if errors.Is(err, errVerboseQuietConflict) {
		return diagnostic{message: "conflicting verbosity options", hint: "use either --verbose or --quiet"}
	}

	d := diagnostic{
		message:  "command failed",
		attrs:    []slog.Attr{slog.String("command", cmd.CommandPath())},
		rawCause: true,
	}
	if args, ok := errors.AsType[*argumentError](err); ok {
		applyArgumentDiagnostic(&d, cmd, args)

		return d
	}

	if flagDiagnostic(&d, cmd, err) {
		d.rawCause = false

		return d
	}

	if cmd.Name() == "init" {
		return initDiagnostic(err)
	}

	return d
}

func applyArgumentDiagnostic(d *diagnostic, cmd *cobra.Command, args *argumentError) {
	d.rawCause = false
	d.message = "missing argument"
	d.hint = "run " + cmd.CommandPath() + " --help"

	if len(args.values) == 0 {
		return
	}

	d.message = "unexpected argument"
	d.attrs = append(d.attrs, slog.String("argument", args.values[0]))

	if !args.unknownCommand {
		return
	}

	d.message = "unknown command"

	suggestions := cmd.SuggestionsFor(args.values[0])
	if len(suggestions) > 0 {
		d.hint = "did you mean " + strings.Join(suggestions, " or ") + "?"
	}
}

func flagDiagnostic(d *diagnostic, cmd *cobra.Command, err error) bool {
	d.hint = "run " + cmd.CommandPath() + " --help"

	if flag, ok := errors.AsType[*pflag.NotExistError](err); ok {
		d.message = "unknown flag"
		d.attrs = append(d.attrs, slog.String("flag", flag.GetSpecifiedName()))

		return true
	}

	if flag, ok := errors.AsType[*pflag.ValueRequiredError](err); ok {
		d.message = "flag requires a value"
		d.attrs = append(d.attrs, slog.String("flag", flag.GetFlag().Name))

		return true
	}

	if flag, ok := errors.AsType[*pflag.InvalidValueError](err); ok {
		d.message = "invalid flag value"
		d.attrs = append(d.attrs, slog.String("flag", flag.GetFlag().Name), slog.String("value", flag.GetValue()))

		if flag.GetFlag().Value.Type() == "bool" {
			d.hint = "use true or false"
		}

		return true
	}

	if flag, ok := errors.AsType[*pflag.InvalidSyntaxError](err); ok {
		d.message = "invalid flag syntax"
		d.attrs = append(d.attrs, slog.String("flag", flag.GetSpecifiedFlag()))

		return true
	}

	d.hint = ""

	return false
}

func initDiagnostic(err error) diagnostic {
	d := diagnostic{message: "could not create configuration file"}
	if file, ok := errors.AsType[*config.FileError](err); ok {
		d.attrs = append(d.attrs, slog.String("path", file.Path))
	} else if file, ok := errors.AsType[*os.PathError](err); ok {
		d.attrs = append(d.attrs, slog.String("path", file.Path))
	}

	switch {
	case errors.Is(err, config.ErrExists):
		d.message = "configuration file already exists"
		d.hint = "use the existing file or choose another --config path"
	case errors.Is(err, os.ErrNotExist):
		d.hint = "create the parent directory or choose another --config path"
	case errors.Is(err, os.ErrPermission):
		d.hint = "choose a writable --config path"
	}

	return d
}
