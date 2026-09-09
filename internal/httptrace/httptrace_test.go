package httptrace_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/httptrace"
)

func TestTracer(t *testing.T) {
	t.Run("logs a sanitized response summary", func(t *testing.T) {
		// given: debug logging and a request containing private transport data
		var logOutput bytes.Buffer

		previousLogger := slog.Default()

		slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() {
			slog.SetDefault(previousLogger)
		})

		request, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"https://api.github.com/repos/acme/private/pulls?access_token=fake-sensitive&ref=private",
			strings.NewReader("private request body"),
		)
		testastic.NoError(t, err)
		request.Header.Set("Authorization", "Bearer fake-sensitive")

		trace := httptrace.New("github")
		trace.RequestHook(nil, request, 1)
		transport := trace.Interceptor(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusCreated,
				Header: http.Header{
					"X-Github-Request-Id":   []string{"request-123"},
					"X-Ratelimit-Remaining": []string{"4999"},
				},
				Body:    io.NopCloser(strings.NewReader("private response body")),
				Request: req,
			}, nil
		}))

		// when: the HTTP attempt completes
		response, err := transport.RoundTrip(request)
		testastic.NoError(t, err)

		_ = response.Body.Close()

		// then: the log keeps diagnostic metadata and excludes private transport data
		assertTraceEvent(t, logOutput.Bytes(), map[string]any{
			"level":                "DEBUG",
			"msg":                  "http request completed",
			"provider":             "github",
			"method":               http.MethodPost,
			"path":                 "/repos/acme/private/pulls",
			"status":               float64(http.StatusCreated),
			"attempt":              float64(2),
			"request_id":           "request-123",
			"rate_limit_remaining": "4999",
			"rate_limit_reset":     "",
			"retry_after":          "",
			"transport_error":      "",
		})

		for _, private := range []string{"fake-sensitive", "private request body", "private response body"} {
			testastic.NotContains(t, logOutput.String(), private)
		}
	})

	t.Run("classifies a transport failure without logging its details", func(t *testing.T) {
		// given: debug logging and a timed out request containing a private query
		var logOutput bytes.Buffer

		previousLogger := slog.Default()

		slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() {
			slog.SetDefault(previousLogger)
		})

		request, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"https://api.github.com/repos/acme/private?token=fake-sensitive",
			nil,
		)
		testastic.NoError(t, err)

		trace := httptrace.New("github")
		transport := trace.Interceptor(roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}))

		// when: the HTTP attempt times out
		response, err := transport.RoundTrip(request)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}

		// then: the failure category is logged without the raw error or query
		testastic.ErrorIs(t, err, context.DeadlineExceeded)
		assertTraceEvent(t, logOutput.Bytes(), map[string]any{
			"level":                "DEBUG",
			"msg":                  "http request completed",
			"provider":             "github",
			"method":               http.MethodGet,
			"path":                 "/repos/acme/private",
			"status":               float64(0),
			"attempt":              float64(1),
			"request_id":           "",
			"rate_limit_remaining": "",
			"rate_limit_reset":     "",
			"retry_after":          "",
			"transport_error":      "timeout",
		})
		testastic.NotContains(t, logOutput.String(), "fake-sensitive")
	})

	t.Run("is silent when debug logging is disabled", func(t *testing.T) {
		// given: the default info logging level
		var logOutput bytes.Buffer

		previousLogger := slog.Default()

		slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelInfo})))
		t.Cleanup(func() {
			slog.SetDefault(previousLogger)
		})

		request, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"https://gitlab.com/api/v4/projects/1",
			nil,
		)
		testastic.NoError(t, err)

		trace := httptrace.New("gitlab")
		transport := trace.Interceptor(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("[]")),
				Request:    req,
			}, nil
		}))

		// when: the HTTP attempt completes
		response, err := transport.RoundTrip(request)
		testastic.NoError(t, err)

		_ = response.Body.Close()

		// then: no HTTP summary is emitted
		testastic.Equal(t, "", logOutput.String())
	})
}

func assertTraceEvent(t *testing.T, data []byte, expected map[string]any) {
	t.Helper()

	var event map[string]any

	err := json.Unmarshal(data, &event)
	testastic.NoError(t, err)

	timestamp, ok := event["time"].(string)
	testastic.True(t, ok)

	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	testastic.NoError(t, err)
	testastic.False(t, parsed.IsZero())

	duration, ok := event["duration_ms"].(float64)
	testastic.True(t, ok)
	testastic.GreaterOrEqual(t, duration, float64(0))
	testastic.Equal(t, math.Trunc(duration), duration)

	delete(event, "time")
	delete(event, "duration_ms")
	testastic.DeepEqual(t, expected, event)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
