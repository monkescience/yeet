package telemetry //nolint:testpackage // Tests exercise private event and delivery invariants.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/release"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestRecordingConfigPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		enabled     *bool
		environment []string
		configured  bool
		want        bool
	}{
		{name: "default enabled", want: true, configured: true},
		{name: "false global opt-out", environment: []string{"DO_NOT_TRACK=false"}, want: true, configured: true},
		{
			name: "repository disabled", enabled: new(false),
			want: false, configured: true,
		},
		{name: "repository enabled", enabled: new(true), want: true, configured: true},
		{
			name: "global opt-out wins", enabled: new(true),
			environment: []string{"DO_NOT_TRACK=yes"}, want: false, configured: true,
		},
		{name: "missing build configuration", enabled: new(true), want: false, configured: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := repositoryConfig(t, tt.enabled)

			manager := testManager(tt.environment, nil)
			if !tt.configured {
				manager.namespace = ""
			}

			_, enabled := manager.recordingConfig(t.Context(), path)

			testastic.Equal(t, tt.want, enabled)
		})
	}
}

func TestRecordingConfigFallsBackWhenRepositoryConfigIsUnavailable(t *testing.T) {
	t.Parallel()

	manager := testManager(nil, nil)

	_, enabled := manager.recordingConfig(t.Context(), filepath.Join(t.TempDir(), "missing.yaml"))

	testastic.True(t, enabled)
}

func TestRecordingConfigFailsClosedWhenRepositoryConfigIsInvalid(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), config.DefaultFile)
	err := os.WriteFile(path, []byte("telemetry:\n  enabled: maybe\n"), 0o600)
	testastic.NoError(t, err)

	manager := testManager(nil, nil)

	_, enabled := manager.recordingConfig(t.Context(), path)

	testastic.False(t, enabled)
}

func TestRecordInit(t *testing.T) {
	t.Parallel()

	var (
		requestCount atomic.Int32
		body         []byte
	)

	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestCount.Add(1)

		readBody, err := io.ReadAll(request.Body)
		testastic.NoError(t, err)

		body = readBody

		testastic.Equal(t, http.MethodPost, request.Method)
		testastic.Equal(t, "https://nom.telemetrydeck.com/v2/namespace/test-namespace/", request.URL.String())
		testastic.Equal(t, "application/json; charset=utf-8", request.Header.Get("Content-Type"))

		return response(http.StatusNoContent), nil
	})
	manager := testManager(nil, transport)
	path := repositoryConfig(t, new(true))
	started := time.Date(2026, time.August, 21, 11, 59, 54, 0, time.UTC)

	manager.RecordInit(t.Context(), started, path, nil)

	testastic.Equal(t, int32(1), requestCount.Load())
	testastic.AssertJSON(t, "testdata/init_event.expected.json", body)
}

func TestDeliveryIsBoundedAndBestEffort(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		requestCount.Add(1)

		return nil, errors.New("network unavailable")
	})
	manager := testManager(nil, transport)
	path := repositoryConfig(t, new(true))

	manager.RecordInit(t.Context(), manager.now(), path, nil)
	testastic.Equal(t, int32(1), requestCount.Load())
	testastic.Equal(t, deliveryTimeout, manager.client.Timeout)

	event := wireEvent{AppID: strings.Repeat("x", maxPayloadSize), Type: eventType}
	err := manager.deliver(t.Context(), event)
	testastic.ErrorIs(t, err, errEventPayloadTooLarge)
	testastic.Equal(t, int32(1), requestCount.Load())

	redirectErr := manager.client.CheckRedirect(&http.Request{}, nil)
	testastic.ErrorIs(t, redirectErr, http.ErrUseLastResponse)
}

func TestRecordingStopsBeforeDelivery(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		requestCount.Add(1)

		return response(http.StatusNoContent), nil
	})
	path := repositoryConfig(t, new(true))

	optedOut := testManager([]string{"DO_NOT_TRACK=on", "CI=true"}, transport)
	optedOut.RecordInit(t.Context(), optedOut.now(), path, nil)

	canceledContext, cancel := context.WithCancel(t.Context())
	cancel()
	testManager(nil, transport).RecordInit(canceledContext, time.Now(), path, nil)

	testastic.Equal(t, int32(0), requestCount.Load())
}

func TestEventFields(t *testing.T) {
	t.Parallel()

	t.Run("duration buckets", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			duration time.Duration
			want     string
		}{
			{duration: 999 * time.Millisecond, want: "under_1s"},
			{duration: time.Second, want: "1s_to_5s"},
			{duration: 5 * time.Second, want: "5s_to_30s"},
			{duration: 30 * time.Second, want: "30s_to_120s"},
			{duration: 120 * time.Second, want: "over_120s"},
		}

		for _, tt := range tests {
			testastic.Equal(t, tt.want, durationBucket(tt.duration))
		}
	})

	t.Run("official versions", func(t *testing.T) {
		t.Parallel()

		testastic.Equal(t, "1.4.0", officialVersion("v1.4.0"))
		testastic.Equal(t, "1.4.0-beta.1", officialVersion("1.4.0-beta.1"))
		testastic.Equal(t, "", officialVersion("dev"))
		testastic.Equal(t, "", officialVersion("1.4.0-dirty"))
	})

	t.Run("outcomes", func(t *testing.T) {
		t.Parallel()

		testastic.Equal(t, "success", outcome(nil))
		testastic.Equal(t, "failure", outcome(errors.New("failed")))
		testastic.Equal(t, "canceled", outcome(context.Canceled))
		testastic.Equal(t, "canceled", outcome(context.DeadlineExceeded))
	})

	t.Run("release profile", func(t *testing.T) {
		t.Parallel()

		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Targets = map[string]config.Target{
			"api": {Versioning: config.VersioningSemver},
			"web": {Versioning: config.VersioningCalVer},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta"},
		}
		cfg.Release.AutoMerge = true
		finished := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
		profile := releaseProfile(cfg, release.Options{DryRun: true})
		event := newEvent("test-app", "v1.4.0", "linux", "release", finished, finished.Add(-6*time.Second), nil, profile)

		testastic.AssertJSON(t, "testdata/release_event.expected.json", []wireEvent{event})
	})
}

func TestPayloadExcludesProhibitedData(t *testing.T) {
	t.Parallel()

	event := newEvent(
		"test-app",
		"v1.4.0",
		"linux",
		"release",
		time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.August, 21, 11, 59, 59, 0, time.UTC),
		errors.New("secret repository path"),
		nil,
	)

	testastic.Equal(t, "", event.ClientUser)
	testastic.Equal(t, "failure", event.Payload.Outcome)
	testastic.False(t, strings.Contains(event.Type, "secret repository path"))
	testastic.False(t, strings.Contains(event.AppID, "secret repository path"))
	encoded, err := json.Marshal(event)
	testastic.NoError(t, err)

	data := string(encoded)
	for _, prohibited := range []string{"sessionID", "secret repository path", "repository", "branch", "targetCount"} {
		testastic.False(t, strings.Contains(data, prohibited))
	}
}

func repositoryConfig(t *testing.T, enabled *bool) string {
	t.Helper()

	telemetry := ""
	if enabled != nil {
		telemetry = "telemetry:\n  enabled: " + boolString(*enabled) + "\n"
	}

	path := filepath.Join(t.TempDir(), config.DefaultFile)
	contents := telemetry + "targets:\n  app:\n    type: path\n    path: .\n    tag_prefix: v\n"
	err := os.WriteFile(path, []byte(contents), 0o600)
	testastic.NoError(t, err)

	return path
}

func testManager(environment []string, transport http.RoundTripper) *Manager {
	if transport == nil {
		transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusNoContent), nil
		})
	}

	return newManager(options{
		version:   "v1.4.0",
		namespace: "test-namespace",
		appID:     "test-app",
		environ: func() []string {
			return append([]string(nil), environment...)
		},
		now: func() time.Time {
			return time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
		},
		transport: transport,
	})
}

func response(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}
}
