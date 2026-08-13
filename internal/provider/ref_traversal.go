package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/monkescience/yeet/internal/forge"
)

type pageFetcher[T any] func(ctx context.Context, handle func(T) (bool, error)) error

func foldTagRefs[T any](
	ctx context.Context,
	fetch pageFetcher[T],
	read func(T) (string, string, bool),
) ([]forge.TagRef, error) {
	refs := make([]forge.TagRef, 0)

	err := fetch(ctx, func(item T) (bool, error) {
		rawName, rawCommitSHA, ok := read(item)
		if !ok {
			return false, nil
		}

		name := strings.TrimSpace(rawName)
		if name == "" {
			return false, nil
		}

		commitSHA := strings.TrimSpace(rawCommitSHA)
		if commitSHA == "" {
			return false, fmt.Errorf("%w: tag %q", forge.ErrEmptyCommitSHA, name)
		}

		refs = append(refs, forge.TagRef{Name: name, CommitSHA: commitSHA})

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return refs, nil
}

// findRefByName resolves one ref by its full name. Azure DevOps ref filters
// match a prefix, so every page is read and the full name compared here, or a
// sibling such as "refs/heads/main2" answers for "refs/heads/main" (see ADR
// 0010).
func findRefByName(ctx context.Context, fetch pageFetcher[git.GitRef], want string) (git.GitRef, bool, error) {
	var found git.GitRef

	matched := false

	err := fetch(ctx, func(ref git.GitRef) (bool, error) {
		if derefString(ref.Name) != want {
			return false, nil
		}

		found = ref
		matched = true

		return true, nil
	})
	if err != nil {
		return git.GitRef{}, false, err
	}

	return found, matched, nil
}
