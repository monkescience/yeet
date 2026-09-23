package commands

import (
	"cmp"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"regexp"
	"slices"
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

const minRedactedTokenBytes = 8

func newDiagnosticLogger(output io.Writer, level slog.Level, noColor bool) *slog.Logger {
	logger := charmlog.NewWithOptions(output, charmlog.Options{
		Level:           charmlog.Level(level),
		ReportTimestamp: false,
	})
	logger.SetColorProfile(resolveColorProfile(output, noColor))

	return slog.New(&diagnosticHandler{Handler: logger, redact: tokenRedactor()})
}

func tokenRedactor() *strings.Replacer {
	return newTokenRedactor(os.Getenv)
}

func newTokenRedactor(getenv func(string) string) *strings.Replacer {
	var secrets []string

	for _, name := range provider.TokenEnvVars() {
		value := getenv(name)
		if len(value) < minRedactedTokenBytes {
			continue
		}

		secrets = append(secrets, value, base64.StdEncoding.EncodeToString([]byte(":"+value)))
	}

	slices.SortStableFunc(secrets, func(a, b string) int { return cmp.Compare(len(b), len(a)) })

	replacements := make([]string, 0, len(secrets)+len(secrets))
	for _, secret := range secrets {
		replacements = append(replacements, secret, "[redacted]")
	}

	return strings.NewReplacer(replacements...)
}

func (h *diagnosticHandler) Handle(ctx context.Context, record slog.Record) error {
	safe := slog.NewRecord(record.Time, record.Level, h.redacted(record.Message), record.PC)
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
	return &diagnosticHandler{Handler: h.Handler.WithGroup(name), redact: h.redact}
}

func (h *diagnosticHandler) redacted(value string) string {
	if h.redact == nil {
		return value
	}

	return h.redact.Replace(value)
}

func (h *diagnosticHandler) attr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()

	switch attr.Value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(h.redacted(sanitizeDiagnosticURLs(attr.Value.String())))
	case slog.KindAny:
		switch value := attr.Value.Any().(type) {
		case error:
			attr.Value = slog.StringValue(h.redacted(safeCause(value)))
		case []string:
			if len(value) == 0 {
				attr.Value = slog.StringValue(h.redacted(sanitizeDiagnosticURLs(fmt.Sprint(value))))

				break
			}

			attr.Value = slog.StringValue(
				h.redacted(sanitizeDiagnosticURLs(strings.Join(value, ", "))))
		default:
			attr.Value = slog.StringValue("[unsupported diagnostic value]")
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

func sanitizeDiagnosticURLs(value string) string {
	return diagnosticURL.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[invalid URL]"
		}

		parsed.User = nil
		query := make(url.Values)

		for _, key := range []string{"version", "baseVersion", "targetVersion"} {
			if value := parsed.Query().Get(key); value != "" {
				query.Set(key, value)
			}
		}

		parsed.RawQuery = query.Encode()
		parsed.Fragment = ""

		return parsed.String()
	})
}

func safeCause(err error) string {
	if status := provider.ErrorStatus(err); status != 0 {
		return fmt.Sprintf("http response status %d", status)
	}

	if cause := diagnosticCause(err); cause != "" {
		return cause
	}

	return "unclassified error"
}

func yamlLoadCause(load *yaml.LoadError) string {
	return fmt.Sprintf("yaml %s at %d:%d", load.Stage, load.Mark.Line, load.Mark.Column)
}

func diagnosticCause(err error) string {
	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return "response is not valid JSON"
	}

	if loads, multiple := errors.AsType[*yaml.LoadErrors](err); multiple {
		if len(loads.Errors) != 1 {
			return ""
		}

		return yamlLoadCause(loads.Errors[0])
	}

	if load, single := errors.AsType[*yaml.LoadError](err); single {
		return yamlLoadCause(load)
	}

	if timeout, ok := errors.AsType[*provider.MergeNotFinalizedError](err); ok {
		switch timeout.TimeoutKind() {
		case provider.MergeTimeoutResponsive:
			return "merge remained pending"
		case provider.MergeTimeoutTransport:
			return "provider request timed out"
		}
	}

	if errors.Is(err, context.Canceled) {
		return "operation canceled"
	}

	if lookup, ok := errors.AsType[*net.DNSError](err); ok {
		return fmt.Sprintf("host %s could not be resolved: %s", lookup.Name, lookup.Err)
	}

	if verification, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return "tls certificate verification failed: " + verification.Err.Error()
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "operation timed out" + requestHostSuffix(err)
	}

	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "network operation timed out" + requestHostSuffix(err)
	}

	if errno, ok := errors.AsType[syscall.Errno](err); ok {
		return errno.Error()
	}

	return ""
}

func requestHostSuffix(err error) string {
	request, ok := errors.AsType[*url.Error](err)
	if !ok {
		return ""
	}

	parsed, parseErr := url.Parse(request.URL)
	if parseErr != nil || parsed.Host == "" {
		return ""
	}

	return " on " + parsed.Host
}
