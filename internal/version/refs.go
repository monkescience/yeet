package version

import (
	"cmp"
	"slices"
	"strings"
)

type parsedRef[T any] struct {
	ref   string
	value T
}

func orderedRefs[T any](
	refs []string,
	parse func(string) (T, error),
	allowed func(T) bool,
	compare func(T, T) int,
) []string {
	parsed := make([]parsedRef[T], 0, len(refs))
	seen := make(map[string]struct{}, len(refs))

	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}

		if _, exists := seen[ref]; exists {
			continue
		}

		seen[ref] = struct{}{}

		value, err := parse(ref)
		if err != nil || !allowed(value) {
			continue
		}

		parsed = append(parsed, parsedRef[T]{ref: ref, value: value})
	}

	slices.SortFunc(parsed, func(left, right parsedRef[T]) int {
		if order := compare(right.value, left.value); order != 0 {
			return order
		}

		return cmp.Compare(right.ref, left.ref)
	})

	ordered := make([]string, len(parsed))
	for index, ref := range parsed {
		ordered[index] = ref.ref
	}

	return ordered
}
