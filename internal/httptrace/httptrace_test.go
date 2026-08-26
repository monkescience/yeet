package httptrace_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

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
		var event struct {
			Message            string `json:"msg"`
			Provider           string `json:"provider"`
			Method             string `json:"method"`
			Path               string `json:"path"`
			Status             int    `json:"status"`
			Attempt            int    `json:"attempt"`
			RequestID          string `json:"request_id"`
			RateLimitRemaining string `json:"rate_limit_remaining"`
		}

		err = json.Unmarshal(bytes.TrimSpace(logOutput.Bytes()), &event)
		testastic.NoError(t, err)
		testastic.Equal(t, "http request completed", event.Message)
		testastic.Equal(t, "github", event.Provider)
		testastic.Equal(t, http.MethodPost, event.Method)
		testastic.Equal(t, "/repos/acme/private/pulls", event.Path)
		testastic.Equal(t, http.StatusCreated, event.Status)
		testastic.Equal(t, 2, event.Attempt)
		testastic.Equal(t, "request-123", event.RequestID)
		testastic.Equal(t, "4999", event.RateLimitRemaining)
		testastic.False(t, strings.Contains(logOutput.String(), "fake-sensitive"))
		testastic.False(t, strings.Contains(logOutput.String(), "private request body"))
		testastic.False(t, strings.Contains(logOutput.String(), "private response body"))
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
		testastic.True(t, strings.Contains(logOutput.String(), `"transport_error":"timeout"`))
		testastic.False(t, strings.Contains(logOutput.String(), "fake-sensitive"))
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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
