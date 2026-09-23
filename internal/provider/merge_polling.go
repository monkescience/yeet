package provider

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/logattr"
)

const mergePollBackoffFactor = 2

type mergePolling struct {
	interval    time.Duration
	maxInterval time.Duration
	timeout     time.Duration
}

type MergePollingOption func(*mergePolling)

func WithMergePolling(initialInterval, maxInterval, timeout time.Duration) MergePollingOption {
	return func(polling *mergePolling) {
		if initialInterval > 0 {
			polling.interval = initialInterval
		}

		if maxInterval > 0 {
			polling.maxInterval = maxInterval
		}

		if timeout > 0 {
			polling.timeout = timeout
		}
	}
}

func newMergePolling(options ...MergePollingOption) mergePolling {
	defaults := config.Default().Release.MergePolling
	polling := mergePolling{
		interval:    defaults.InitialInterval,
		maxInterval: defaults.MaxInterval,
		timeout:     defaults.Timeout,
	}

	for _, option := range options {
		option(&polling)
	}

	return polling
}

func (p mergePolling) awaitMergedCommit(
	ctx context.Context,
	logger *slog.Logger,
	reference string,
	resolve func(context.Context) (string, error),
) (string, error) {
	waitCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	interval := p.interval
	deadline, _ := waitCtx.Deadline()

	for attempt := 0; ; attempt++ {
		if waitCtx.Err() != nil || !time.Now().Before(deadline) {
			return p.waitExpired(ctx, reference)
		}

		mergeSHA, err := resolve(waitCtx)
		if err != nil {
			if ctx.Err() == nil && waitCtx.Err() != nil {
				return "", p.notFinalizedFrom(reference, err)
			}

			return "", err
		}

		if mergeSHA != "" {
			return mergeSHA, nil
		}

		if attempt == 0 {
			logger.InfoContext(ctx, "waiting for merge to finalize", logattr.PullRequest(reference))
		}

		select {
		case <-waitCtx.Done():
			return p.waitExpired(ctx, reference)
		case <-time.After(interval):
		}

		interval = min(interval*mergePollBackoffFactor, p.maxInterval)
	}
}

func (p mergePolling) waitExpired(ctx context.Context, reference string) (string, error) {
	if ctx.Err() != nil {
		return "", fmt.Errorf("wait for %s: %w", reference, ctx.Err())
	}

	return "", p.notFinalizedResponsive(reference, context.DeadlineExceeded)
}

func (p mergePolling) notFinalizedFrom(reference string, cause error) error {
	return &MergeNotFinalizedError{
		reference: reference,
		timeout:   p.timeout,
		kind:      MergeTimeoutTransport,
		cause:     cause,
	}
}

func (p mergePolling) notFinalizedResponsive(reference string, cause error) error {
	return &MergeNotFinalizedError{
		reference: reference,
		timeout:   p.timeout,
		kind:      MergeTimeoutResponsive,
		cause:     cause,
	}
}

type MergeTimeoutKind string

const (
	MergeTimeoutTransport  MergeTimeoutKind = "transport"
	MergeTimeoutResponsive MergeTimeoutKind = "responsive"
)

type MergeNotFinalizedError struct {
	cause     error
	reference string
	timeout   time.Duration
	kind      MergeTimeoutKind
}

func (e *MergeNotFinalizedError) Reference() string { return e.reference }

func (e *MergeNotFinalizedError) Timeout() time.Duration { return e.timeout }

func (e *MergeNotFinalizedError) TimeoutKind() MergeTimeoutKind { return e.kind }

func (e *MergeNotFinalizedError) Error() string {
	return fmt.Sprintf("%s: %s after %s: %s", forge.ErrMergeNotFinalized, e.reference, e.timeout, e.cause)
}

func (e *MergeNotFinalizedError) Unwrap() []error {
	return []error{forge.ErrMergeNotFinalized, e.cause}
}
