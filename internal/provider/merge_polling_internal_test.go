package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monkescience/testastic"
)

var errMergePollingProbe = errors.New("forge is unreachable")

func TestAwaitMergedCommitReportsTheCauseThatEndedTheWait(t *testing.T) {
	t.Parallel()

	// given: a resolve call that fails only once the wait budget is spent
	polling := newMergePolling(WithMergePolling(time.Millisecond, 10*time.Millisecond))

	resolve := func(pollCtx context.Context) (string, error) {
		<-pollCtx.Done()

		return "", errMergePollingProbe
	}

	// when: the driver waits for the merge to finalize
	_, err := polling.awaitMergedCommit(context.Background(), "pull request #42", resolve)

	// then: a real failure stays distinguishable from a slow forge
	testastic.ErrorIs(t, err, ErrMergeNotFinalized)
	testastic.ErrorIs(t, err, errMergePollingProbe)
}
