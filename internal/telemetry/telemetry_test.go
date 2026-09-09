package telemetry //nolint:testpackage // Tests exercise private event and delivery invariants.

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

			// given: repository and environment preferences with optional build configuration
			path := repositoryConfig(t, tt.enabled)

			manager := testManager(tt.environment, nil)
			if !tt.configured {
				manager.namespace = ""
			}

			// when: resolving whether recording is enabled
			_, enabled := manager.recordingConfig(t.Context(), path)

			// then: opt-out precedence and build availability determine recording
			testastic.Equal(t, tt.want, enabled)
		})
	}
}

func TestRecordingConfigFallsBackWhenRepositoryConfigIsUnavailable(t *testing.T) {
	t.Parallel()

	// given: a configured telemetry manager and a missing repository config
	manager := testManager(nil, nil)

	// when: resolving recording preferences
	_, enabled := manager.recordingConfig(t.Context(), filepath.Join(t.TempDir(), "missing.yaml"))

	// then: the default enabled preference applies
	testastic.True(t, enabled)
}

func TestRecordingConfigFailsClosedWhenRepositoryConfigIsInvalid(t *testing.T) {
	t.Parallel()

	// given: a repository config with an invalid telemetry preference
	path := filepath.Join(t.TempDir(), config.DefaultFile)
	err := os.WriteFile(path, []byte("telemetry:\n  enabled: maybe\n"), 0o600)
	testastic.NoError(t, err)

	manager := testManager(nil, nil)

	// when: resolving recording preferences
	_, enabled := manager.recordingConfig(t.Context(), path)

	// then: invalid configuration prevents recording
	testastic.False(t, enabled)
}

func TestRecordInit(t *testing.T) {
	t.Parallel()

	// given: enabled telemetry, a fixed clock, and a transport capturing the request
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

	// when: recording a successful initialization
	manager.RecordInit(t.Context(), started, path, nil)

	// then: one request contains the complete initialization event
	testastic.Equal(t, int32(1), requestCount.Load())
	testastic.AssertJSON(t, "testdata/init_event.expected.json", body)
}

func TestDeliveryIsBoundedAndBestEffort(t *testing.T) {
	t.Parallel()

	// given: enabled telemetry and a transport that always fails
	var requestCount atomic.Int32

	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		requestCount.Add(1)

		return nil, errors.New("network unavailable")
	})
	manager := testManager(nil, transport)
	path := repositoryConfig(t, new(true))

	// when: recording an initialization during a network failure
	manager.RecordInit(t.Context(), manager.now(), path, nil)

	// then: delivery makes one attempt with a bounded timeout
	testastic.Equal(t, int32(1), requestCount.Load())
	testastic.Equal(t, 3*time.Second, manager.client.Timeout)

	// when: delivering an event larger than the payload limit
	event := wireEvent{AppID: strings.Repeat("x", maxPayloadSize), Type: eventType}
	err := manager.deliver(t.Context(), event)

	// then: the payload is rejected before another network request
	testastic.ErrorIs(t, err, errEventPayloadTooLarge)
	testastic.Equal(t, int32(1), requestCount.Load())

	// when: the client receives a redirect
	redirectErr := manager.client.CheckRedirect(&http.Request{}, nil)

	// then: it returns the original response without following the redirect
	testastic.ErrorIs(t, redirectErr, http.ErrUseLastResponse)
}

func TestRecordingStopsBeforeDelivery(t *testing.T) {
	t.Parallel()

	// given: a capture transport, an opted-out manager, and a canceled context
	var requestCount atomic.Int32

	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		requestCount.Add(1)

		return response(http.StatusNoContent), nil
	})
	path := repositoryConfig(t, new(true))

	optedOut := testManager([]string{"DO_NOT_TRACK=on", "CI=true"}, transport)
	canceledContext, cancel := context.WithCancel(t.Context())
	cancel()

	// when: recording with either opt-out or cancellation
	optedOut.RecordInit(t.Context(), optedOut.now(), path, nil)
	testManager(nil, transport).RecordInit(canceledContext, time.Now(), path, nil)

	// then: neither recording reaches the transport
	testastic.Equal(t, int32(0), requestCount.Load())
}

func TestEventFields(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		duration time.Duration
		expected float64
	}{
		{1500 * time.Millisecond, 1.5},
		{1234567 * time.Nanosecond, 0.001},
		{6 * time.Second, 6.0},
	} {
		t.Run("duration seconds/"+test.duration.String(), func(t *testing.T) {
			t.Parallel()

			// given: an elapsed duration with millisecond or finer precision
			// when: converting it into telemetry seconds
			seconds := durationSeconds(test.duration)

			// then: seconds retain millisecond precision and truncate finer units
			testastic.Equal(t, test.expected, seconds)
		})
	}

	for _, test := range []struct {
		version  string
		expected string
	}{
		{"v1.4.0", "1.4.0"},
		{"1.4.0-beta.1", "1.4.0-beta.1"},
		{"dev", ""},
		{"1.4.0-dirty", ""},
	} {
		t.Run("official versions/"+test.version, func(t *testing.T) {
			t.Parallel()

			// given: an official or development build version
			// when: normalizing it for telemetry
			version := officialVersion(test.version)

			// then: only official versions are reported, without the tag prefix
			testastic.Equal(t, test.expected, version)
		})
	}

	for _, test := range []struct {
		err      error
		expected string
	}{
		{nil, "success"},
		{errors.New("failed"), "failure"},
	} {
		t.Run("outcomes/"+test.expected, func(t *testing.T) {
			t.Parallel()

			// given: a command result with or without an error
			// when: classifying the outcome
			result := outcome(test.err)

			// then: only error presence determines success or failure
			testastic.Equal(t, test.expected, result)
		})
	}

	for name, test := range map[string]struct {
		configuredEnabled bool
		configuredForce   bool
		options           release.Options
		expected          string
	}{
		"configured off": {
			expected: "off",
		},
		"configured normal": {
			configuredEnabled: true,
			expected:          "normal",
		},
		"configured force": {
			configuredForce: true,
			expected:        "force",
		},
		"explicit enable overrides configured off": {
			options:  release.Options{AutoMerge: new(true)},
			expected: "normal",
		},
		"explicit false clears configured force": {
			configuredEnabled: true,
			configuredForce:   true,
			options:           release.Options{AutoMerge: new(false)},
			expected:          "off",
		},
		"explicit force false retains configured normal": {
			configuredEnabled: true,
			configuredForce:   true,
			options:           release.Options{AutoMergeForce: new(false)},
			expected:          "normal",
		},
		"explicit force overrides explicit false": {
			configuredEnabled: true,
			configuredForce:   true,
			options: release.Options{
				AutoMerge:      new(false),
				AutoMergeForce: new(true),
			},
			expected: "force",
		},
	} {
		t.Run("auto merge mode/"+name, func(t *testing.T) {
			t.Parallel()

			// given: configured auto-merge behavior and optional explicit flags
			cfg := config.Default()
			cfg.Release.AutoMerge = test.configuredEnabled
			cfg.Release.AutoMergeForce = test.configuredForce

			// when: building the release telemetry profile
			profile := releaseProfile(cfg, test.options, nil)

			// then: the field reports the release package's resolved mode
			testastic.Equal(t, test.expected, profile.autoMerge)
		})
	}

	t.Run("failure categories", func(t *testing.T) {
		t.Parallel()

		_, missingConfigErr := release.Run(t.Context(), filepath.Join(t.TempDir(), "missing.yaml"), release.Options{})
		for _, test := range []struct {
			name     string
			err      error
			expected string
		}{
			{"success", nil, ""},
			{"existing config", fmt.Errorf("init: %w", config.ErrExists), "config_exists"},
			{"invalid config", fmt.Errorf("load: %w", config.ErrInvalidConfig), "config_invalid"},
			{"network error", &url.Error{Op: "Post", URL: "https://api.invalid", Err: errors.New("refused")}, "network"},
			{"unexpected error", errors.New("boom"), "unexpected"},
			{"missing config", fmt.Errorf("release failed: %w", missingConfigErr), "config_missing"},
		} {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				// given: a command error, possibly wrapped with context, or success
				// when: selecting a telemetry failure category
				category := failureCategory(test.err)

				// then: the category describes the cause without exposing error details
				testastic.Equal(t, test.expected, category)
			})
		}
	})

	t.Run("release profile", func(t *testing.T) {
		t.Parallel()

		// given: a release configuration and its resolved provider
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
		started := finished.Add(-6 * time.Second)

		// when: building the release telemetry profile
		profile := releaseProfile(
			cfg,
			release.Options{DryRun: true},
			&release.Result{Provider: config.ProviderGitHub},
		)
		event := newEvent("test-app", "v1.4.0", "linux", "arm64", "release", finished, started, nil, profile)

		// then: the complete event describes the release with the fixed clock and platform
		testastic.AssertJSON(t, "testdata/release_event.expected.json", []wireEvent{event})
	})

	t.Run("resolved provider", func(t *testing.T) {
		t.Parallel()

		// given: a release configuration and its resolved provider
		cfg := config.Default()

		// when: building the release telemetry profile
		profile := releaseProfile(cfg, release.Options{}, &release.Result{Provider: config.ProviderGitLab})

		// then: the actual provider overrides the unresolved configuration
		testastic.Equal(t, string(config.ProviderGitLab), profile.provider)
	})
}

func TestPayloadExcludesProhibitedData(t *testing.T) {
	t.Parallel()

	// given: a command failure containing private repository details
	// when: constructing its telemetry event
	event := newEvent(
		"test-app",
		"v1.4.0",
		"linux",
		"arm64",
		"release",
		time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.August, 21, 11, 59, 59, 0, time.UTC),
		errors.New("secret repository path"),
		nil,
	)

	// then: the event reports failure without a user identity or private error details
	testastic.Equal(t, "", event.ClientUser)
	testastic.Equal(t, "failure", event.Payload.Outcome)
	testastic.NotContains(t, event.Type, "secret repository path")
	testastic.NotContains(t, event.AppID, "secret repository path")

	// when: encoding the event for delivery
	encoded, err := json.Marshal(event)
	testastic.NoError(t, err)

	// then: no prohibited identifier or repository metadata appears anywhere in the payload
	data := string(encoded)
	for _, prohibited := range []string{"sessionID", "secret repository path", "repository", "branch", "targetCount"} {
		testastic.NotContains(t, data, prohibited)
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
