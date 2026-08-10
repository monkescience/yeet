package provider //nolint:testpackage // validates the unexported Azure pagination helper directly

import (
	"context"
	"errors"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
)

var errPaginationTest = errors.New("pagination test failure")

func TestPaginateByCursor(t *testing.T) {
	t.Parallel()

	t.Run("follows the cursor until it is exhausted", func(t *testing.T) {
		t.Parallel()

		// given: two pages joined by a string cursor
		pages := map[string][]int{"": {1, 2}, "second": {3, 4}}
		next := map[string]string{"": "second", "second": ""}
		handled := make([]int, 0)

		// when: paginating from the zero cursor
		err := paginateByCursor(
			context.Background(),
			"test items",
			"",
			func(cursor string) ([]int, string, error) {
				return pages[cursor], next[cursor], nil
			},
			func(item int) (bool, error) {
				handled = append(handled, item)

				return false, nil
			},
		)

		// then: every page is handled in order
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []int{1, 2, 3, 4}, handled)
	})

	t.Run("stops without reading the rest of the page", func(t *testing.T) {
		t.Parallel()

		// given: a first page whose second item stops the walk
		handled := 0

		// when: the handler stops on the second item
		err := paginateByCursor(
			context.Background(),
			"test items",
			"",
			func(string) ([]int, string, error) {
				return []int{1, 2, 3}, "next", nil
			},
			func(item int) (bool, error) {
				handled++

				return item == 2, nil
			},
		)

		// then: no further item and no further page is fetched
		testastic.NoError(t, err)
		testastic.Equal(t, 2, handled)
	})

	t.Run("surfaces a handler error", func(t *testing.T) {
		t.Parallel()

		// given: a handler that rejects the first item
		// when: paginating a single page
		err := paginateByCursor(
			context.Background(),
			"test items",
			"",
			func(string) ([]int, string, error) {
				return []int{1}, "", nil
			},
			func(int) (bool, error) {
				return false, errPaginationTest
			},
		)

		// then: the handler error is returned unwrapped by the walk
		testastic.ErrorIs(t, err, errPaginationTest)
	})

	t.Run("surfaces a fetch error", func(t *testing.T) {
		t.Parallel()

		// given: a page fetch that fails
		// when: paginating from the zero cursor
		err := paginateByCursor(
			context.Background(),
			"test items",
			"",
			func(string) ([]int, string, error) {
				return nil, "", errPaginationTest
			},
			func(int) (bool, error) {
				return false, nil
			},
		)

		// then: the fetch error is returned unwrapped by the walk
		testastic.ErrorIs(t, err, errPaginationTest)
	})

	t.Run("stops a cancelled walk before fetching", func(t *testing.T) {
		t.Parallel()

		// given: an already cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		calls := 0

		// when: paginating under that context
		err := paginateByCursor(
			ctx,
			"test items",
			"",
			func(string) ([]int, string, error) {
				calls++

				return []int{1}, "", nil
			},
			func(int) (bool, error) {
				return false, nil
			},
		)

		// then: the cancellation is reported and no page is fetched
		testastic.ErrorIs(t, err, context.Canceled)
		testastic.Equal(t, 0, calls)
	})

	t.Run("rejects a cursor that never exhausts", func(t *testing.T) {
		t.Parallel()

		// given: an API that always announces another page
		calls := 0

		// when: paginating past the safety limit
		err := paginateByCursor(
			context.Background(),
			"test items",
			"",
			func(cursor string) ([]int, string, error) {
				calls++

				return []int{calls}, cursor + "x", nil
			},
			func(int) (bool, error) {
				return false, nil
			},
		)

		// then: the limit is enforced instead of looping forever
		testastic.ErrorIs(t, err, forge.ErrPaginationLimitExceeded)
		testastic.Equal(t, maxPaginationPages, calls)
	})
}

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
		testastic.ErrorIs(t, err, forge.ErrPaginationLimitExceeded)
		testastic.Equal(t, maxPaginationPages+1, calls)
		testastic.Equal(t, capacity, handled)
	})
}
