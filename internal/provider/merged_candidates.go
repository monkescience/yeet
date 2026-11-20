package provider

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/monkescience/yeet/internal/forge"
)

type mergedCandidates[T any] struct {
	mergedAt   func(T) (time.Time, bool)
	hydrate    func(context.Context, T) (T, bool, error)
	reference  func(T) string
	labelState func(T) releasePRLabelState
}

func mergeTimeMissingError(reference string) error {
	return fmt.Errorf("%w: %s", errMergeTimeMissing, reference)
}

func resolveMergedReleasePR[T any](
	ctx context.Context,
	trusted []T,
	spec mergedCandidates[T],
) (T, error) {
	var zero T

	pending := make([]T, 0, len(trusted))
	hasAdoptable := false

	for _, candidate := range trusted {
		switch spec.labelState(candidate) {
		case releasePRLabelsPending:
			pending = append(pending, candidate)
		case releasePRLabelsAdoptable:
			hasAdoptable = true
		case releasePRLabelsMismatched:
		}
	}

	if !hasAdoptable {
		return resolveLatestMerged(ctx, pending, spec)
	}

	latest, err := resolveLatestMerged(ctx, trusted, spec)
	if err != nil {
		return zero, err
	}

	if state := spec.labelState(latest); state == releasePRLabelsPending || state == releasePRLabelsAdoptable {
		return latest, nil
	}

	return resolveLatestMerged(ctx, pending, spec)
}

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

	if len(merged) == 1 {
		return merged[0], nil
	}

	for _, candidate := range merged {
		if _, known := spec.mergedAt(candidate); !known {
			return zero, mergeTimeMissingError(spec.reference(candidate))
		}
	}

	return slices.MaxFunc(merged, func(left, right T) int {
		leftAt, _ := spec.mergedAt(left)
		rightAt, _ := spec.mergedAt(right)

		return leftAt.Compare(rightAt)
	}), nil
}
