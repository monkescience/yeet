package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

var errMergePollingProbe = errors.New("forge is unreachable")

func TestMergePollingSettings(t *testing.T) {
	t.Parallel()

	// given: the default release polling settings
	defaults := config.Default().Release.MergePolling

	// when: constructing polling without overrides
	polling := newMergePolling()

	// then: every default polling value is preserved
	testastic.Equal(t, mergePolling{
		interval:    defaults.InitialInterval,
		maxInterval: defaults.MaxInterval,
		timeout:     defaults.Timeout,
	}, polling)

	// when: constructing polling with explicit overrides
	polling = newMergePolling(WithMergePolling(time.Second, 7*time.Second, 3*time.Minute))

	// then: every polling value is replaced together
	testastic.Equal(t, mergePolling{
		interval:    time.Second,
		maxInterval: 7 * time.Second,
		timeout:     3 * time.Minute,
	}, polling)
}

func TestAwaitMergedCommitReportsTheCauseThatEndedTheWait(t *testing.T) {
	t.Parallel()

	// given: a resolve call that fails only once the wait budget is spent
	polling := newMergePolling(WithMergePolling(time.Millisecond, time.Millisecond, 10*time.Millisecond))

	resolve := func(pollCtx context.Context) (string, error) {
		<-pollCtx.Done()

		return "", errMergePollingProbe
	}

	// when: the driver waits for the merge to finalize
	_, err := polling.awaitMergedCommit(context.Background(), "pull request #42", resolve)

	// then: a real failure stays distinguishable from a slow forge
	testastic.ErrorIs(t, err, forge.ErrMergeNotFinalized)
	testastic.ErrorIs(t, err, errMergePollingProbe)
}
