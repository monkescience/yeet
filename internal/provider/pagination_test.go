package provider //nolint:testpackage // validates the unexported Azure pagination helper directly

import (
	"context"
	"testing"

	"github.com/monkescience/testastic"
)

func TestPaginateAzureDevOpsBySkip(t *testing.T) {
	t.Parallel()

	const pageSize = 2

	t.Run("accepts exactly the page capacity", func(t *testing.T) {
		t.Parallel()

		// given: exactly one hundred full pages followed by an empty page
		capacity := maxPaginationPages * pageSize
		calls := 0
		handled := 0

		// when: paginating through every item at the safety limit
		err := paginateAzureDevOpsBySkip(
			context.Background(),
			"test items",
			pageSize,
			func(skip int) ([]int, error) {
				calls++

				if skip == capacity {
					return []int{}, nil
				}

				return []int{skip, skip + 1}, nil
			},
			func(int) (bool, error) {
				handled++

				return false, nil
			},
		)

		// then: one exhaustion probe succeeds after all capacity items are handled
		testastic.NoError(t, err)
		testastic.Equal(t, maxPaginationPages+1, calls)
		testastic.Equal(t, capacity, handled)
	})

	t.Run("rejects items beyond the page capacity", func(t *testing.T) {
		t.Parallel()

		// given: one item remains after one hundred full pages
		capacity := maxPaginationPages * pageSize
		calls := 0
		handled := 0

		// when: paginating beyond the safety limit
		err := paginateAzureDevOpsBySkip(
			context.Background(),
			"test items",
			pageSize,
			func(skip int) ([]int, error) {
				calls++

				if skip == capacity {
					return []int{skip}, nil
				}

				return []int{skip, skip + 1}, nil
			},
			func(int) (bool, error) {
				handled++

				return false, nil
			},
		)

		// then: the probe detects overflow without handling the extra item
		testastic.ErrorIs(t, err, ErrPaginationLimitExceeded)
		testastic.Equal(t, maxPaginationPages+1, calls)
		testastic.Equal(t, capacity, handled)
	})
}
