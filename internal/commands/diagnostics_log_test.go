package commands //nolint:testpackage // exercises handler transformations unavailable through CLI options

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/monkescience/testastic"
	"go.yaml.in/yaml/v4"
)

func TestDiagnosticHandlerStructuredValues(t *testing.T) {
	// given: a short synthetic token and nested diagnostic attributes
	t.Setenv("GITHUB_TOKEN", "e")

	var output bytes.Buffer

	logger := slog.New(&diagnosticHandler{
		Handler: slog.NewTextHandler(&output, nil),
	}).
		WithGroup("release").
		With(slog.String("remote", "origin"), slog.String("request_id", "e"))

	// when: logging a failure with an opaque error and external metadata
	logger.ErrorContext(t.Context(), "release could not be completed",
		slog.Group("details", slog.String("request_id", "OmU=")),
		slog.Any("error", errors.New("private-response-content")),
		slog.Any("object", struct{ Token string }{Token: "private-object-content"}))

	// then: messages and keys stay readable while raw errors and objects are omitted
	testastic.Contains(t, output.String(), "release could not be completed")
	testastic.Contains(t, output.String(), "release.remote=origin")
	testastic.Contains(t, output.String(), "release.request_id=e")
	testastic.Contains(t, output.String(), `release.details.request_id="OmU="`)
	testastic.NotContains(t, output.String(), "private-response-content")
	testastic.NotContains(t, output.String(), "private-object-content")
}

func TestDiagnosticHandlerRendersEverySupportedKind(t *testing.T) {
	// given: one attribute per slog kind the handler claims to support
	var output bytes.Buffer

	logger := slog.New(&diagnosticHandler{Handler: slog.NewTextHandler(&output, nil)})

	// when: logging all of them in one record
	logger.InfoContext(t.Context(), "kinds",
		slog.String("string_kind", "text"),
		slog.Bool("bool_kind", true),
		slog.Duration("duration_kind", time.Second),
		slog.Float64("float_kind", 1.5),
		slog.Int64("int_kind", 7),
		slog.Uint64("uint_kind", 8),
		slog.Time("time_kind", time.Unix(0, 0).UTC()),
		slog.Any("error_kind", errors.New("boom")),
		slog.Any("strings_kind", []string{"one", "two"}))

	// then: none of them degrade to the unsupported placeholder
	rendered := output.String()
	for _, key := range []string{
		"string_kind", "bool_kind", "duration_kind", "float_kind",
		"int_kind", "uint_kind", "time_kind", "error_kind", "strings_kind",
	} {
		testastic.Contains(t, rendered, key+"=")
	}

	testastic.NotContains(t, rendered, "[unsupported diagnostic value]")
}

func TestDiagnosticURLDisplay(t *testing.T) {
	// given: an external URL containing credentials, navigation parameters, and sensitive query data
	t.Setenv("GITHUB_TOKEN", "Tv1.2.3")

	var output bytes.Buffer

	logger := newDiagnosticLogger(&output, slog.LevelInfo, true)

	// when: logging the URL as a diagnostic field
	logger.InfoContext(t.Context(), "created release", slog.String("url",
		"https://user:synthetic-credential@example.test/repo?version=GTv1.2.3&baseVersion=GTv1.0.0"+
			"&targetVersion=GTv1.2.3&token=synthetic-credential#private-fragment"))

	// then: useful navigation survives without credentials or unapproved URL parts
	testastic.Contains(t, output.String(), "https://example.test/repo?")
	testastic.Contains(t, output.String(), "version=GTv1.2.3")
	testastic.Contains(t, output.String(), "baseVersion=GTv1.0.0")
	testastic.Contains(t, output.String(), "targetVersion=GTv1.2.3")
	testastic.NotContains(t, output.String(), "synthetic-credential")
	testastic.NotContains(t, output.String(), "user:")
	testastic.NotContains(t, output.String(), "private-fragment")
}

func TestDiagnosticCauseOmitsAmbiguousYAMLLocation(t *testing.T) {
	t.Parallel()

	// given: a decode failure reported at more than one location
	err := &yaml.LoadErrors{Errors: []*yaml.LoadError{
		{Stage: yaml.ConstructorStage, Mark: yaml.Mark{Line: 4, Column: 3}},
		{Stage: yaml.ConstructorStage, Mark: yaml.Mark{Line: 5, Column: 3}},
	}}

	// when: classifying the cause
	cause := diagnosticCause(err)

	// then: no single location is claimed
	testastic.Equal(t, "", cause)
}

func TestDiagnosticVerboseDetailIncludesFailureContext(t *testing.T) {
	// given: a diagnostic carrying both shared attributes and verbose detail
	var output bytes.Buffer

	previousLogger := slog.Default()

	slog.SetDefault(newDiagnosticLogger(&output, slog.LevelDebug, true))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	d := diagnostic{
		attrs:        []slog.Attr{slog.String("unit", "target:api"), slog.String("phase", "reconciliation")},
		verboseAttrs: []slog.Attr{slog.String("cause", "git reference was not found")},
	}

	// when: recording the detail
	d.logVerbose(t.Context())

	// then: the debug record keeps the failure context beside the cause
	testastic.Contains(
		t,
		ansi.Strip(output.String()),
		`DEBUG failure detail unit=target:api phase=reconciliation cause="git reference was not found"`,
	)
}

func TestTokenRedactorPrefersLongestToken(t *testing.T) {
	// given: one configured token that is a prefix of another
	tokens := map[string]string{"GH_TOKEN": "ghp_abcdefgh", "GITHUB_TOKEN": "ghp_abcdefghXYZ123456"}
	redactor := newTokenRedactor(func(name string) string { return tokens[name] })

	// when: redacting text containing the longer token
	redacted := redactor.Replace("token ghp_abcdefghXYZ123456 rejected")

	// then: no suffix of the longer token survives
	testastic.Equal(t, "token [redacted] rejected", redacted)
}

func TestDiagnosticCauseNamesNetworkFailures(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		err   error
		cause string
	}{
		{
			name: "unresolved host",
			err: &url.Error{Op: "Get", URL: "https://git.example.test/api", Err: &net.DNSError{
				Err: "no such host", Name: "git.example.test", IsNotFound: true,
			}},
			cause: "host git.example.test could not be resolved: no such host",
		},
		{
			name: "untrusted certificate",
			err: &url.Error{Op: "Get", URL: "https://git.example.test/api", Err: &tls.CertificateVerificationError{
				Err: x509.UnknownAuthorityError{},
			}},
			cause: "tls certificate verification failed: x509: certificate signed by unknown authority",
		},
		{
			name: "request deadline",
			err: fmt.Errorf("get branch head: %w", &url.Error{
				Op: "Get", URL: "https://user:secret@git.example.test/api?token=secret", Err: context.DeadlineExceeded,
			}),
			cause: "operation timed out on git.example.test",
		},
		{
			name:  "deadline without request",
			err:   context.DeadlineExceeded,
			cause: "operation timed out",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// given: a transport failure wrapped like provider requests wrap it

			// when: classifying the cause
			cause := diagnosticCause(scenario.err)

			// then: the failing host and reason are named without request details
			testastic.Equal(t, scenario.cause, cause)
		})
	}
}
