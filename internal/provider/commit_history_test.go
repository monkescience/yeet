//nolint:testpackage // This test validates unexported commit history helpers.
package provider

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestCommitHistoryFromBoundaryPositions(t *testing.T) {
	t.Parallel()

	// given: one branch scan ordered newest-first and multiple boundary positions
	entries := []CommitEntry{
		{Hash: "newest", Paths: []string{"services/api/main.go"}},
		{Hash: "middle", Paths: []string{"apps/web/app.tsx"}},
		{Hash: "oldest", Paths: []string{"README.md"}},
	}
	positions := map[string]int{
		"tag-middle": 1,
		"tag-oldest": 2,
	}

	// when: building per-ref histories from one shared scan
	history := commitHistoryFromBoundaryPositions(
		[]string{"tag-middle", "tag-oldest", "missing", ""},
		entries,
		positions,
	)

	// then: each ref receives only commits newer than its own boundary
	testastic.SliceEqual(t, []string{"newest"}, commitHistoryHashes(history.EntriesByRef["tag-middle"]))
	testastic.SliceEqual(t, []string{"newest", "middle"}, commitHistoryHashes(history.EntriesByRef["tag-oldest"]))
	testastic.SliceEqual(t, []string{"newest", "middle", "oldest"}, commitHistoryHashes(history.EntriesByRef[""]))
	testastic.SliceEqual(t, []string{"missing"}, history.MissingRefs)
}

func TestCommitHistoryFromBoundaryPositionsClonesEntries(t *testing.T) {
	t.Parallel()

	// given: entries with path slices that will be shared into target histories
	entries := []CommitEntry{{Hash: "newest", Paths: []string{"before.go"}}}

	// when: building a history slice
	history := commitHistoryFromBoundaryPositions([]string{"tag"}, entries, map[string]int{"tag": 1})

	// then: callers cannot mutate the scanned entries through the returned history
	history.EntriesByRef["tag"][0].Paths[0] = "after.go"
	testastic.Equal(t, "before.go", entries[0].Paths[0])
}

func commitHistoryHashes(entries []CommitEntry) []string {
	hashes := make([]string, 0, len(entries))

	for _, entry := range entries {
		hashes = append(hashes, entry.Hash)
	}

	return hashes
}
