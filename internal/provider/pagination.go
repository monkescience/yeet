package provider

import (
	"context"
	"fmt"
)

// paginate iterates a paginated API up to maxPaginationPages times. fetch is
// called with the current page (0 for the first call, then whatever the prior
// fetch returned as nextPage, where 0 means there are no more pages). Each
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

// paginateAzureDevOps iterates a paginated ADO endpoint up to maxPaginationPages
// times using the continuation-token model (X-MS-ContinuationToken header +
// continuationToken query param). fetch is called with the current token ("" on
// the first call) and returns the page items plus the next-page token (empty
// when exhausted).
func paginateAzureDevOps[T any](
	ctx context.Context,
	resource string,
	fetch func(token string) (items []T, nextToken string, err error),
	handle func(T) (stop bool, err error),
) error {
	token := ""

	for range maxPaginationPages {
		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("paginate %s: %w", resource, err)
		}

		items, nextToken, err := fetch(token)
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

		if nextToken == "" {
			return nil
		}

		token = nextToken
	}

	return fmt.Errorf("%w: exceeded %d pages %s", ErrPaginationLimitExceeded, maxPaginationPages, resource)
}

// paginateAzureDevOpsBySkip iterates a paginated ADO endpoint that uses the
// $skip/$top offset model (e.g. GetCommits, which returns no continuation
// token). fetch is called with the current skip offset (0 on the first call)
// and returns the page items. Pagination stops when a page returns fewer than
// pageSize items. ctx is checked between pages, and items are handed to handle,
// which may signal early termination by returning stop=true.
func paginateAzureDevOpsBySkip[T any](
	ctx context.Context,
	resource string,
	pageSize int,
	fetch func(skip int) (items []T, err error),
	handle func(T) (stop bool, err error),
) error {
	skip := 0

	for range maxPaginationPages {
		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("paginate %s: %w", resource, err)
		}

		items, err := fetch(skip)
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

		if len(items) < pageSize {
			return nil
		}

		skip += len(items)
	}

	return fmt.Errorf("%w: exceeded %d pages %s", ErrPaginationLimitExceeded, maxPaginationPages, resource)
}
