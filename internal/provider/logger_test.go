package provider_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"
)

func TestProviderInjectedLogger(t *testing.T) {
	t.Parallel()

	// given: two providers with independent log destinations and levels
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte("[]"))
		testastic.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	client, err := gitlab.NewClient("", gitlab.WithBaseURL(server.URL), gitlab.WithHTTPClient(server.Client()))
	testastic.NoError(t, err)

	var debugOutput, quietOutput bytes.Buffer

	debugLogger := slog.New(slog.NewTextHandler(&debugOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))
	quietLogger := slog.New(slog.NewTextHandler(&quietOutput, &slog.HandlerOptions{Level: slog.LevelWarn}))
	debugProvider := provider.NewGitLab(debugLogger.With(slog.String("instance", "debug")), client, "o/r")
	quietProvider := provider.NewGitLab(quietLogger, client, "o/r")

	// when: both providers list tags without changing the global logger
	_, err = debugProvider.ListTagRefs(t.Context())
	testastic.NoError(t, err)

	debugLogs := debugOutput.String()
	_, err = quietProvider.ListTagRefs(t.Context())
	testastic.NoError(t, err)

	// then: the injected logger retains caller context and provider context without cross-instance output
	testastic.Contains(t, debugLogs, `msg="listing tags" instance=debug provider=gitlab`)
	testastic.Contains(t, debugLogs, `msg="listed tags" instance=debug provider=gitlab count=0`)
	testastic.Equal(t, debugLogs, debugOutput.String())
	testastic.Equal(t, "", quietOutput.String())
}
