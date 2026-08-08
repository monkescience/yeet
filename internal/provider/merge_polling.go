package provider

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	defaultMergePollInterval    = 250 * time.Millisecond
	defaultMergePollMaxInterval = 5 * time.Second
	defaultMergeFinalizeTimeout = 2 * time.Minute
	mergePollBackoffFactor      = 2
)

type mergePolling struct {
	interval    time.Duration
	maxInterval time.Duration
	timeout     time.Duration
}

// MergePollingOption tunes how long a provider waits for a forge to finalize a
// merge it has already accepted.
type MergePollingOption func(*mergePolling)

// WithMergePolling sets a fixed poll interval and the overall wait budget.
func WithMergePolling(interval, timeout time.Duration) MergePollingOption {
	return func(polling *mergePolling) {
		if interval > 0 {
			polling.interval = interval
			polling.maxInterval = interval
		}

		if timeout > 0 {
			polling.timeout = timeout
		}
	}
}

func newMergePolling(options ...MergePollingOption) mergePolling {
	polling := mergePolling{
		interval:    defaultMergePollInterval,
		maxInterval: defaultMergePollMaxInterval,
		timeout:     defaultMergeFinalizeTimeout,
	}

	for _, option := range options {
		option(&polling)
	}

	return polling
}

// awaitMergedCommit calls resolve until it reports the commit an accepted merge
// produced on the base branch. resolve returns an empty SHA while the forge is
// still finalizing, and an error once the outcome is terminal. reference carries
// the forge's own wording so error text stays provider-specific.
func (p mergePolling) awaitMergedCommit(
	ctx context.Context,
	reference string,
	resolve func(context.Context) (string, error),
) (string, error) {
	waitCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	interval := p.interval

	for attempt := 0; ; attempt++ {
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
			slog.InfoContext(ctx, "waiting for merge to finalize", slog.String("pull_request", reference))
		}

		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return "", fmt.Errorf("wait for %s: %w", reference, ctx.Err())
			}

			return "", p.notFinalized(reference)
		case <-time.After(interval):
		}

		interval = min(interval*mergePollBackoffFactor, p.maxInterval)
	}
}

func (p mergePolling) notFinalized(reference string) error {
	return fmt.Errorf("%w: %s after %s", ErrMergeNotFinalized, reference, p.timeout)
}

// notFinalizedFrom keeps the cause that ended the wait alongside the sentinel,
// so a forge that went unreachable stays distinguishable from a slow one.
func (p mergePolling) notFinalizedFrom(reference string, cause error) error {
	return &mergeNotFinalizedError{reference: reference, timeout: p.timeout, cause: cause}
}

type mergeNotFinalizedError struct {
	cause     error
	reference string
	timeout   time.Duration
}

func (e *mergeNotFinalizedError) Error() string {
	return fmt.Sprintf("%s: %s after %s: %s", ErrMergeNotFinalized, e.reference, e.timeout, e.cause)
}

func (e *mergeNotFinalizedError) Unwrap() []error {
	return []error{ErrMergeNotFinalized, e.cause}
}
