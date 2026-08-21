package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/release"
)

const (
	deliveryTimeout         = 300 * time.Millisecond
	maxPayloadSize          = 4 * 1024
	maxResponseBodySize     = 4 * 1024
	versionStrategyCapacity = 2
)

var (
	namespace string
	appID     string
)

type Manager struct {
	version   string
	namespace string
	appID     string
	environ   func() []string
	now       func() time.Time
	client    *http.Client
}

type options struct {
	version   string
	namespace string
	appID     string
	environ   func() []string
	now       func() time.Time
	transport http.RoundTripper
}

func New(version string) *Manager {
	return newManager(options{
		version:   version,
		namespace: namespace,
		appID:     appID,
		environ:   os.Environ,
		now:       time.Now,
		transport: http.DefaultTransport,
	})
}

func newManager(opts options) *Manager {
	client := &http.Client{
		Transport: opts.transport,
		Timeout:   deliveryTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Manager{
		version:   opts.version,
		namespace: strings.TrimSpace(opts.namespace),
		appID:     strings.TrimSpace(opts.appID),
		environ:   opts.environ,
		now:       opts.now,
		client:    client,
	}
}

func (m *Manager) RecordInit(
	ctx context.Context,
	started time.Time,
	configPath string,
	commandErr error,
) {
	_, enabled := m.recordingConfig(ctx, configPath)
	if !enabled {
		return
	}

	m.record(ctx, "init", started, commandErr, nil)
}

func (m *Manager) RecordRelease(
	ctx context.Context,
	started time.Time,
	configPath string,
	opts release.Options,
	commandErr error,
) {
	cfg, enabled := m.recordingConfig(ctx, configPath)
	if !enabled {
		return
	}

	profile := releaseProfile(cfg, opts)
	m.record(ctx, "release", started, commandErr, profile)
}

func (m *Manager) recordingConfig(ctx context.Context, configPath string) (*config.Config, bool) {
	if ctx.Err() != nil {
		return nil, false
	}

	if doNotTrack(m.environ()) {
		return nil, false
	}

	cfg, _, err := config.LoadResolvedQuiet(ctx, configPath)
	if err == nil && cfg.Telemetry.Enabled != nil {
		return cfg, *cfg.Telemetry.Enabled && m.configured()
	}

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false
	}

	return cfg, m.configured()
}

func (m *Manager) configured() bool {
	return m.namespace != "" && m.appID != ""
}

func (m *Manager) record(
	ctx context.Context,
	command string,
	started time.Time,
	commandErr error,
	profile *releaseFields,
) {
	event := newEvent(m.appID, m.version, runtime.GOOS, command, m.now(), started, commandErr, profile)

	err := m.deliver(ctx, event)
	if err != nil {
		slog.DebugContext(ctx, "telemetry delivery failed", slog.Any("error", err))
	}
}

func outcome(commandErr error) string {
	switch {
	case commandErr == nil:
		return "success"
	case errors.Is(commandErr, context.Canceled), errors.Is(commandErr, context.DeadlineExceeded):
		return "canceled"
	default:
		return "failure"
	}
}
