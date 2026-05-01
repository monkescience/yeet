package provider

import (
	"context"
	"fmt"
)

// paginate iterates a paginated API up to maxPaginationPages times. fetch is
// called with the current page (0 for the first call, then whatever the prior
// fetch returned as nextPage; 0 nextPage means there are no more pages). Each
// returned item is handed to handle, which may signal early termination by
// returning stop=true. ctx is checked between pages. Hitting the page cap
// returns ErrPaginationLimitExceeded wrapped with the resource description.
func paginate[T any](
	ctx context.Context,
	resource string,
	fetch func(page int) (items []T, nextPage int, err error),
	handle func(T) (stop bool, err error),
) error {
	page := 0

	for range maxPaginationPages {
		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("paginate %s: %w", resource, err)
		}

		items, nextPage, err := fetch(page)
		if err != nil {
			return err
		}

		for _, item := range items {
			stop, err := handle(item)
			if err != nil {
				return err
			}

			if stop {
				return nil
			}
		}

		if nextPage == 0 {
			return nil
		}

		page = nextPage
	}

	return fmt.Errorf("%w: exceeded %d pages %s", ErrPaginationLimitExceeded, maxPaginationPages, resource)
}
