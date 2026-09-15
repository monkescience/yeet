package commands

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"syscall"

	charmlog "charm.land/log/v2"
	"github.com/monkescience/yeet/internal/provider"
	"go.yaml.in/yaml/v4"
)

type diagnosticHandler struct {
	slog.Handler
	redact *strings.Replacer
}

var diagnosticURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)

func newDiagnosticLogger(output io.Writer, level slog.Level, noColor bool) *slog.Logger {
	logger := charmlog.NewWithOptions(output, charmlog.Options{
		Level:           charmlog.Level(level),
		ReportTimestamp: false,
	})
	logger.SetColorProfile(resolveColorProfile(output, noColor))

	var replacements []string

	for _, name := range provider.TokenEnvVars() {
		value := os.Getenv(name)
		if value == "" {
			continue
		}

		replacements = append(replacements, value, "[redacted]",
			base64.StdEncoding.EncodeToString([]byte(":"+value)), "[redacted]")
	}

	return slog.New(&diagnosticHandler{Handler: logger, redact: strings.NewReplacer(replacements...)})
}

func (h *diagnosticHandler) Handle(ctx context.Context, record slog.Record) error {
	safe := slog.NewRecord(record.Time, record.Level, h.text(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		safe.AddAttrs(h.attr(attr))

		return true
	})

	err := h.Handler.Handle(ctx, safe)
	if err != nil {
		return fmt.Errorf("write diagnostic: %w", err)
	}

	return nil
}

func (h *diagnosticHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	safe := make([]slog.Attr, len(attrs))
	for index, attr := range attrs {
		safe[index] = h.attr(attr)
	}

	return &diagnosticHandler{Handler: h.Handler.WithAttrs(safe), redact: h.redact}
}

func (h *diagnosticHandler) WithGroup(name string) slog.Handler {
	return &diagnosticHandler{Handler: h.Handler.WithGroup(h.text(name)), redact: h.redact}
}

func (h *diagnosticHandler) attr(attr slog.Attr) slog.Attr {
	attr.Key = h.text(attr.Key)
	attr.Value = attr.Value.Resolve()

	switch attr.Value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(h.text(attr.Value.String()))
	case slog.KindAny:
		switch value := attr.Value.Any().(type) {
		case error:
			attr.Value = slog.StringValue(h.text(safeCause(value)))
		case []string:
			if len(value) == 0 {
				attr.Value = slog.StringValue(h.text(fmt.Sprint(value)))

				break
			}

			attr.Value = slog.StringValue(h.text(strings.Join(value, ", ")))
		default:
			attr.Value = slog.StringValue(h.text(fmt.Sprint(value)))
		}
	case slog.KindGroup:
		group := attr.Value.Group()
		safe := make([]slog.Attr, len(group))

		for index, child := range group {
			safe[index] = h.attr(child)
		}

		attr.Value = slog.GroupValue(safe...)
	case slog.KindBool, slog.KindDuration, slog.KindFloat64, slog.KindInt64,
		slog.KindTime, slog.KindUint64, slog.KindLogValuer:
	}

	return attr
}

func (h *diagnosticHandler) text(value string) string {
	value = diagnosticURL.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[invalid URL]"
		}

		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""

		return parsed.String()
	})

	return h.redact.Replace(value)
}

func safeCause(err error) string {
	if status := provider.ErrorStatus(err); status != 0 {
		return fmt.Sprintf("http response status %d", status)
	}

	if cause := diagnosticCause(err); cause != "" {
		return cause
	}

	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			break
		}

		err = unwrapped
	}

	return err.Error()
}

func diagnosticCause(err error) string {
	if load, ok := errors.AsType[*yaml.LoadError](err); ok {
		return fmt.Sprintf("yaml %s at %d:%d", load.Stage, load.Mark.Line, load.Mark.Column)
	}

	if errors.Is(err, context.Canceled) {
		return "operation canceled"
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "operation timed out"
	}

	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "network operation timed out"
	}

	if errno, ok := errors.AsType[syscall.Errno](err); ok {
		return errno.Error()
	}

	return ""
}
