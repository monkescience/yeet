//nolint:testpackage // This test validates unexported commit history helpers.
package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/monkescience/testastic"
)

func TestFetchCommitHistoryByRef(t *testing.T) {
	t.Parallel()

	// given: a per-ref fetch returning distinct ranges, a missing ref, and an
	// unbounded ("") whole-history scan
	fetch := func(_ context.Context, ref string) ([]CommitEntry, error) {
		switch ref {
		case "v1.1.0":
			return []CommitEntry{{Hash: "newest"}}, nil
		case "v1.0.0":
			return []CommitEntry{{Hash: "newest"}, {Hash: "middle"}}, nil
		case "":
			return []CommitEntry{{Hash: "newest"}, {Hash: "middle"}, {Hash: "oldest"}}, nil
		case "gone":
			return nil, fmt.Errorf("%w: ref %q", ErrRefNotFound, ref)
		default:
			return nil, fmt.Errorf("unexpected ref %q", ref)
		}
	}

	// when: assembling history for the reachable, missing, and unbounded refs
	history, err := fetchCommitHistoryByRef(
		context.Background(),
		[]string{"v1.1.0", "v1.0.0", "gone", ""},
		4,
		fetch,
	)

	// then: each ref receives its own range, the missing ref is reported, and the
	// batch still succeeds
	testastic.NoError(t, err)
	testastic.SliceEqual(t, []string{"newest"}, commitHistoryHashes(history.EntriesByRef["v1.1.0"]))
	testastic.SliceEqual(t, []string{"newest", "middle"}, commitHistoryHashes(history.EntriesByRef["v1.0.0"]))
	testastic.SliceEqual(t, []string{"newest", "middle", "oldest"}, commitHistoryHashes(history.EntriesByRef[""]))
	testastic.SliceEqual(t, []string{"gone"}, history.MissingRefs)
}

func TestFetchCommitHistoryByRefPropagatesFetchErrors(t *testing.T) {
	t.Parallel()

	// given: a fetch that fails with a non-ErrRefNotFound error
	wantErr := errors.New("boom")
	fetch := func(_ context.Context, _ string) ([]CommitEntry, error) {
		return nil, wantErr
	}

	// when: assembling history
	_, err := fetchCommitHistoryByRef(context.Background(), []string{"v1.0.0"}, 4, fetch)

	// then: the error fails the whole batch rather than becoming a missing ref
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, wantErr)
}

func commitHistoryHashes(entries []CommitEntry) []string {
	hashes := make([]string, 0, len(entries))

	for _, entry := range entries {
		hashes = append(hashes, entry.Hash)
	}

	return hashes
}
