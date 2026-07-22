package httptrace

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

// Tracer logs sanitized metadata for HTTP attempts.
type Tracer struct {
	provider string
	attempts sync.Map
}

// New constructs an HTTP tracer for a provider.
func New(provider string) *Tracer {
	return &Tracer{provider: provider}
}

// RequestHook records retry attempt numbers from retryablehttp.
func (t *Tracer) RequestHook(_ retryablehttp.Logger, request *http.Request, retry int) {
	t.attempts.Store(request, retry+1)
}

// Interceptor wraps a transport with sanitized debug logging.
func (t *Tracer) Interceptor(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}

	return roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		started := time.Now()
		response, err := next.RoundTrip(request)
		attempt := t.requestAttempt(request)

		if slog.Default().Enabled(request.Context(), slog.LevelDebug) {
			status := 0

			var responseHeaders http.Header

			if response != nil {
				status = response.StatusCode
				responseHeaders = response.Header
			}

			slog.DebugContext(request.Context(), "http request completed",
				slog.String("provider", t.provider),
				slog.String("method", request.Method),
				slog.String("path", sanitizedPath(request)),
				slog.Int("status", status),
				slog.Int64("duration_ms", time.Since(started).Milliseconds()),
				slog.Int("attempt", attempt),
				slog.String("request_id", firstHeader(responseHeaders,
					"X-GitHub-Request-Id", "X-Request-Id", "X-VSS-E2EID", "X-TFS-Session")),
				slog.String("rate_limit_remaining", firstHeader(responseHeaders,
					"X-RateLimit-Remaining", "RateLimit-Remaining")),
				slog.String("rate_limit_reset", firstHeader(responseHeaders,
					"X-RateLimit-Reset", "RateLimit-Reset")),
				slog.String("retry_after", firstHeader(responseHeaders, "Retry-After")),
				slog.String("transport_error", transportErrorKind(err)),
			)
		}

		return response, err //nolint:wrapcheck // preserve concrete transport error types for retry policy
	})
}

func (t *Tracer) requestAttempt(request *http.Request) int {
	attempt := 1

	value, ok := t.attempts.LoadAndDelete(request)
	if !ok {
		return attempt
	}

	storedAttempt, ok := value.(int)
	if ok && storedAttempt > 0 {
		attempt = storedAttempt
	}

	return attempt
}

func sanitizedPath(request *http.Request) string {
	if request == nil || request.URL == nil {
		return ""
	}

	path := request.URL.EscapedPath()
	if path == "" {
		return "/"
	}

	return path
}

func firstHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := headers.Get(name); value != "" {
			return value
		}
	}

	return ""
}

func transportErrorKind(err error) string {
	if err == nil {
		return ""
	}

	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}

	return "network"
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
