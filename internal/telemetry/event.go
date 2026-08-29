package telemetry

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/release"
)

const eventType = "Yeet.Command.executed"

var (
	errEventPayloadTooLarge = errors.New("event payload is too large")
	errIngestRejected       = errors.New("ingest rejected event")
)

type wireEvent struct {
	AppID      string      `json:"appID"`
	ClientUser string      `json:"clientUser"`
	Type       string      `json:"type"`
	FloatValue float64     `json:"floatValue"`
	Payload    eventFields `json:"payload"`
}

type eventFields struct {
	EventDay                  string `json:"Yeet.eventDay"`
	Version                   string `json:"Yeet.version,omitempty"`
	OS                        string `json:"Yeet.os"`
	Arch                      string `json:"Yeet.arch"`
	Command                   string `json:"Yeet.command"`
	Outcome                   string `json:"Yeet.outcome"`
	FailureCategory           string `json:"Yeet.failure.category,omitempty"`
	ReleaseProvider           string `json:"Yeet.release.provider,omitempty"`
	ReleaseLayout             string `json:"Yeet.release.layout,omitempty"`
	ReleasePullRequestMode    string `json:"Yeet.release.pullRequestMode,omitempty"`
	ReleaseUnitCount          string `json:"Yeet.release.unitCount,omitempty"`
	ReleaseVersioning         string `json:"Yeet.release.versioning,omitempty"`
	ReleaseDryRun             string `json:"Yeet.release.dryRun,omitempty"`
	ReleaseChannelsConfigured string `json:"Yeet.release.channelsConfigured,omitempty"`
	ReleaseAutoMerge          string `json:"Yeet.release.autoMerge,omitempty"`
}

type releaseFields struct {
	provider           string
	layout             string
	pullRequestMode    string
	unitCount          string
	versioning         string
	dryRun             string
	channelsConfigured string
	autoMerge          string
}

func newEvent(
	appID string,
	version string,
	operatingSystem string,
	architecture string,
	command string,
	finished time.Time,
	started time.Time,
	commandErr error,
	profile *releaseFields,
) wireEvent {
	fields := eventFields{
		EventDay:        finished.UTC().Format(time.DateOnly),
		Version:         officialVersion(version),
		OS:              operatingSystem,
		Arch:            architecture,
		Command:         command,
		Outcome:         outcome(commandErr),
		FailureCategory: failureCategory(commandErr),
	}

	if profile != nil {
		fields.ReleaseProvider = profile.provider
		fields.ReleaseLayout = profile.layout
		fields.ReleasePullRequestMode = profile.pullRequestMode
		fields.ReleaseUnitCount = profile.unitCount
		fields.ReleaseVersioning = profile.versioning
		fields.ReleaseDryRun = profile.dryRun
		fields.ReleaseChannelsConfigured = profile.channelsConfigured
		fields.ReleaseAutoMerge = profile.autoMerge
	}

	return wireEvent{
		AppID:      appID,
		ClientUser: "",
		Type:       eventType,
		FloatValue: durationSeconds(finished.Sub(started)),
		Payload:    fields,
	}
}

const millisecondsPerSecond = 1000

func durationSeconds(duration time.Duration) float64 {
	return math.Round(duration.Seconds()*millisecondsPerSecond) / millisecondsPerSecond
}

func failureCategory(commandErr error) string {
	if commandErr == nil {
		return ""
	}

	if failure, ok := errors.AsType[*release.Failure](commandErr); ok {
		if failure.Kind() == release.FailureUnexpected && isNetworkError(commandErr) {
			return "network"
		}

		return string(failure.Kind())
	}

	switch {
	case errors.Is(commandErr, config.ErrExists):
		return "config_exists"
	case errors.Is(commandErr, config.ErrInvalidConfig):
		return "config_invalid"
	case isNetworkError(commandErr):
		return "network"
	default:
		return "unexpected"
	}
}

func isNetworkError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error

	return errors.As(err, &netErr)
}

func officialVersion(raw string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), "v")

	version, err := semver.StrictNewVersion(trimmed)
	if err != nil {
		return ""
	}

	if strings.Contains(version.Prerelease(), "dirty") {
		return ""
	}

	return version.String()
}

func releaseProfile(cfg *config.Config, opts release.Options, result *release.Result) *releaseFields {
	if cfg == nil {
		return nil
	}

	provider := cfg.Provider
	if result != nil && result.Provider != "" {
		provider = result.Provider
	} else if opts.Provider != nil {
		provider = config.ProviderType(strings.TrimSpace(*opts.Provider))
	}

	return &releaseFields{
		provider:           allowedProvider(provider),
		layout:             releaseLayout(cfg),
		pullRequestMode:    allowedPullRequestMode(cfg.Release.PullRequestMode),
		unitCount:          releaseUnitCount(result),
		versioning:         releaseVersioning(cfg),
		dryRun:             boolString(opts.DryRun),
		channelsConfigured: boolString(len(cfg.Release.Channels) > 0),
		autoMerge:          releaseAutoMerge(cfg, opts),
	}
}

func allowedPullRequestMode(mode config.PullRequestMode) string {
	switch mode {
	case config.PullRequestModeCombined, config.PullRequestModeIndependent:
		return string(mode)
	default:
		return ""
	}
}

func releaseUnitCount(result *release.Result) string {
	if result == nil {
		return ""
	}

	return strconv.Itoa(len(result.Units))
}

func allowedProvider(provider config.ProviderType) string {
	switch provider {
	case config.ProviderAuto, config.ProviderGitHub, config.ProviderGitLab, config.ProviderAzureDevOps:
		return string(provider)
	default:
		return ""
	}
}

func releaseLayout(cfg *config.Config) string {
	if len(cfg.Targets) > 1 {
		return "monorepo"
	}

	return "single"
}

func releaseVersioning(cfg *config.Config) string {
	strategies := make(map[config.VersioningStrategy]struct{}, versionStrategyCapacity)

	for _, target := range cfg.Targets {
		strategy := target.Versioning
		if strategy == "" {
			strategy = cfg.Versioning
		}

		strategies[strategy] = struct{}{}
	}

	if len(strategies) > 1 {
		return "mixed"
	}

	for strategy := range strategies {
		switch strategy {
		case config.VersioningSemver, config.VersioningCalVer:
			return string(strategy)
		default:
			return ""
		}
	}

	return ""
}

func releaseAutoMerge(cfg *config.Config, opts release.Options) string {
	enabled := cfg.Release.AutoMerge
	forced := cfg.Release.AutoMergeForce

	if opts.AutoMerge != nil {
		enabled = *opts.AutoMerge
		if !enabled {
			forced = false
		}
	}

	if opts.AutoMergeForce != nil {
		forced = *opts.AutoMergeForce
	}

	if forced {
		return "force"
	}

	if enabled {
		return "normal"
	}

	return "off"
}

func boolString(value bool) string {
	if value {
		return "true"
	}

	return "false"
}

func (m *Manager) deliver(ctx context.Context, event wireEvent) error {
	payload, err := json.Marshal([]wireEvent{event})
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}

	if len(payload) > maxPayloadSize {
		return fmt.Errorf("%w: maximum is %d bytes", errEventPayloadTooLarge, maxPayloadSize)
	}

	endpoint := "https://nom.telemetrydeck.com/v2/namespace/" + url.PathEscape(m.namespace) + "/"

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")

	response, err := m.client.Do(request)
	if err != nil {
		return fmt.Errorf("send event: %w", err)
	}

	_, copyErr := io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodySize))
	closeErr := response.Body.Close()

	if copyErr != nil {
		return fmt.Errorf("discard response: %w", copyErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close response: %w", closeErr)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: HTTP status %d", errIngestRejected, response.StatusCode)
	}

	return nil
}
