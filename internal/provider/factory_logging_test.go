package provider //nolint:testpackage // validates unexported provider factory wiring directly

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

	gitLabProvider, err := createGitLabProvider(&RepositoryDescriptor{Project: "group/private"})
	testastic.NoError(t, err)

	// when: the provider lists repository tags
	_, err = gitLabProvider.ListTagRefs(t.Context())

	// then: the SDK request passes through the sanitized HTTP logger
	testastic.NoError(t, err)
	testastic.True(t, strings.Contains(logOutput.String(), `"msg":"http request completed"`))
	testastic.True(t, strings.Contains(logOutput.String(), `"provider":"gitlab"`))
	testastic.True(t, strings.Contains(logOutput.String(), `"request_id":"gitlab-request-123"`))
	testastic.False(t, strings.Contains(logOutput.String(), "fake-token"))
}
