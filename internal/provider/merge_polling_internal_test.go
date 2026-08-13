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

	defaults := config.Default().Release.MergePolling
	polling := newMergePolling()

	testastic.Equal(t, defaults.InitialInterval, polling.interval)
	testastic.Equal(t, defaults.MaxInterval, polling.maxInterval)
	testastic.Equal(t, defaults.Timeout, polling.timeout)

	polling = newMergePolling(WithMergePolling(time.Second, 7*time.Second, 3*time.Minute))

	testastic.Equal(t, time.Second, polling.interval)
	testastic.Equal(t, 7*time.Second, polling.maxInterval)
	testastic.Equal(t, 3*time.Minute, polling.timeout)
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
