package provider

import (
	"context"
	"fmt"

	"github.com/monkescience/yeet/internal/forge"
)

// paginate walks page-number APIs until fetch returns nextPage zero or handle stops.
func paginate[T any](
	ctx context.Context,
	resource string,
	fetch func(page int) (items []T, nextPage int, err error),
	handle func(T) (stop bool, err error),
) error {
	return paginateByCursor(ctx, resource, 0, fetch, handle)
}

// paginateAzureDevOps walks Azure DevOps continuation-token APIs.
func paginateAzureDevOps[T any](
	ctx context.Context,
	resource string,
	fetch func(token string) (items []T, nextToken string, err error),
	handle func(T) (stop bool, err error),
) error {
	return paginateByCursor(ctx, resource, "", fetch, handle)
}

func paginateByCursor[T any, C comparable](
	ctx context.Context,
	resource string,
	cursor C,
	fetch func(C) (items []T, nextCursor C, err error),
	handle func(T) (stop bool, err error),
) error {
	var exhausted C

	for range maxPaginationPages {
		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("paginate %s: %w", resource, err)
		}

		items, nextCursor, err := fetch(cursor)
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

		if nextCursor == exhausted {
			return nil
		}

		cursor = nextCursor
	}

	return paginationLimitExceeded(resource)
}

// paginateAzureDevOpsBySkip walks Azure DevOps $skip/$top APIs. After the page
// limit, one empty probe distinguishes exact capacity from overflow.
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

	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("paginate %s: %w", resource, err)
	}

	items, err := fetch(skip)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		return nil
	}

	return paginationLimitExceeded(resource)
}

func paginationLimitExceeded(resource string) error {
	return fmt.Errorf("%w: exceeded %d pages %s", forge.ErrPaginationLimitExceeded, maxPaginationPages, resource)
}
