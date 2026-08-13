package provider

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/monkescience/yeet/internal/forge"
)

type mergedCandidates[T any] struct {
	mergedAt func(T) (time.Time, bool)
	// hydrate re-reads a candidate whose listing omitted a merge time. It
	// reports false when the re-read proves the candidate never merged, and
	// mergeTimeMissingError when the re-read withholds the merge time of a
	// candidate the forge still calls merged.
	hydrate   func(context.Context, T) (T, bool, error)
	reference func(T) string
}

// mergeTimeMissingError reports a candidate the forge calls merged without
// saying when, which leaves competing candidates impossible to order.
func mergeTimeMissingError(reference string) error {
	return fmt.Errorf("%w: %s", errMergeTimeMissing, reference)
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
			return zero, mergeTimeMissingError(spec.reference(candidate))
		}
	}

	// MaxFunc keeps the first of several candidates that merged at the same time.
	return slices.MaxFunc(merged, func(left, right T) int {
		leftAt, _ := spec.mergedAt(left)
		rightAt, _ := spec.mergedAt(right)

		return leftAt.Compare(rightAt)
	}), nil
}
