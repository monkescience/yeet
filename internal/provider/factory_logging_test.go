package provider //nolint:testpackage // validates unexported provider factory wiring directly

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/monkescience/testastic"
)

func TestCreateGitLabProviderLogsHTTP(t *testing.T) {
	// given: a GitLab API and debug logging
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		testastic.Equal(t, "/api/v4/projects/group%2Fprivate/repository/tags", request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "gitlab-request-123")
		_, err := w.Write([]byte("[]"))
		testastic.NoError(t, err)
	}))
	defer server.Close()

	t.Setenv("GITLAB_TOKEN", "fake-token")
	t.Setenv("GITLAB_URL", server.URL+"/api/v4")

	var logOutput bytes.Buffer

	previousLogger := slog.Default()

	slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	gitLabProvider, err := create(&resolvedGitLabRepository{Project: "group/private"})
	testastic.NoError(t, err)

	// when: the provider lists repository tags
	_, err = gitLabProvider.ListTagRefs(t.Context())

	// then: the SDK request passes through the sanitized HTTP logger
	testastic.NoError(t, err)
	assertProviderTraceEvent(t, &logOutput, map[string]any{
		"level":                "DEBUG",
		"msg":                  "http request completed",
		"provider":             providerNameGitLab,
		"method":               http.MethodGet,
		"path":                 "/api/v4/projects/group%2Fprivate/repository/tags",
		"status":               float64(http.StatusOK),
		"attempt":              float64(1),
		"request_id":           "gitlab-request-123",
		"rate_limit_remaining": "",
		"rate_limit_reset":     "",
		"retry_after":          "",
		"transport_error":      "",
	})
	testastic.NotContains(t, logOutput.String(), "fake-token")
}
