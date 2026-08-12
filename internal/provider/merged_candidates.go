package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/monkescience/yeet/internal/forge"
)

// mergedCandidates tells resolveLatestMerged how to read one forge's merged
// pull request records.
type mergedCandidates[T any] struct {
	// mergedAt reports when a candidate merged and whether the forge said so.
	mergedAt func(T) (time.Time, bool)
	// hydrate re-reads a candidate whose listing omitted a merge time. It
	// reports false when the re-read proves the candidate never merged.
	hydrate func(context.Context, T) (T, bool, error)
	// reference names a candidate the way its forge does, for error messages.
	reference func(T) string
}

// resolveLatestMerged picks the candidate that merged last. Merge times are only
// re-read from the forge when several candidates compete, because a lone
// candidate needs no disambiguation. It returns errMergeTimeMissing when the
// forge cannot say when a competing candidate merged.
func resolveLatestMerged[T any](
	ctx context.Context,
	candidates []T,
	spec mergedCandidates[T],
) (T, error) {
	var zero T

	if len(candidates) == 0 {
		return zero, forge.ErrNoPR
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	merged := make([]T, 0, len(candidates))

	for _, candidate := range candidates {
		if _, known := spec.mergedAt(candidate); known {
			merged = append(merged, candidate)

			continue
		}

		full, stillMerged, err := spec.hydrate(ctx, candidate)
		if err != nil {
			return zero, err
		}

		if stillMerged {
			merged = append(merged, full)
		}
	}

	if len(merged) == 0 {
		return zero, forge.ErrNoPR
	}

	// A survivor that stands alone needs no merge time: nothing competes with it.
	if len(merged) == 1 {
		return merged[0], nil
	}

	for _, candidate := range merged {
		if _, known := spec.mergedAt(candidate); !known {
			return zero, fmt.Errorf("%w: %s", errMergeTimeMissing, spec.reference(candidate))
		}
	}

	best := merged[0]
	bestAt, _ := spec.mergedAt(best)

	for _, candidate := range merged[1:] {
		at, _ := spec.mergedAt(candidate)
		if at.After(bestAt) {
			best = candidate
			bestAt = at
		}
	}

	return best, nil
}
