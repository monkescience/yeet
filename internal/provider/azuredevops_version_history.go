package provider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

const azureDevOpsTagRefPrefix = "refs/tags/"

const azureDevOpsRefPageSize = 100

func (a *AzureDevOps) ListTagRefs(ctx context.Context) ([]TagRef, error) {
	slog.DebugContext(ctx, "azure devops: listing tags")

	refs := make([]TagRef, 0)

	err := a.paginateTagRefs(ctx, func(ref git.GitRef) (bool, error) {
		if ref.Name == nil {
			return false, nil
		}

		name := strings.TrimSpace(strings.TrimPrefix(*ref.Name, azureDevOpsTagRefPrefix))
		if name == "" {
			return false, nil
		}

		commitHash := strings.TrimSpace(derefString(ref.PeeledObjectId))
		if commitHash == "" {
			commitHash = strings.TrimSpace(derefString(ref.ObjectId))
		}

		if commitHash == "" {
			return false, fmt.Errorf("%w: tag %q", ErrEmptyCommitSHA, name)
		}

		refs = append(refs, TagRef{Name: name, CommitSHA: commitHash})

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "azure devops: listed tags", slog.Int("count", len(refs)))

	return refs, nil
}

func (a *AzureDevOps) paginateTagRefs(
	ctx context.Context,
	handle func(git.GitRef) (bool, error),
) error {
	gitClient, err := a.client(ctx)
	if err != nil {
		return err
	}

	filter := "tags/"
	pageSize := azureDevOpsRefPageSize
	peelTags := true

	return paginateAzureDevOps(ctx, "listing tag refs",
		func(token string) ([]git.GitRef, string, error) {
			args := git.GetRefsArgs{
				RepositoryId: &a.repo,
				Project:      &a.project,
				Filter:       &filter,
				PeelTags:     &peelTags,
				Top:          &pageSize,
			}

			if token != "" {
				args.ContinuationToken = new(token)
			}

			response, err := gitClient.GetRefs(ctx, args)
			if err != nil {
				return nil, "", fmt.Errorf("list tag refs: %w", err)
			}

			return response.Value, response.ContinuationToken, nil
		},
		handle,
	)
}

// GetBranchHead returns the commit SHA branch currently points at. GetRefs
// filters by prefix, so the exact ref name is matched explicitly to keep a
// sibling branch such as "main2" from answering for "main".
func (a *AzureDevOps) GetBranchHead(ctx context.Context, branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("%w: empty branch", ErrRefNotFound)
	}

	gitClient, err := a.client(ctx)
	if err != nil {
		return "", err
	}

	filter := "heads/" + branch
	wantName := "refs/heads/" + branch
	pageSize := azureDevOpsRefPageSize
	head := ""

	err = paginateAzureDevOps(ctx, "resolving branch head",
		func(token string) ([]git.GitRef, string, error) {
			args := git.GetRefsArgs{
				RepositoryId: &a.repo,
				Project:      &a.project,
				Filter:       &filter,
				Top:          &pageSize,
			}

			if token != "" {
				args.ContinuationToken = &token
			}

			response, err := gitClient.GetRefs(ctx, args)
			if err != nil {
				return nil, "", fmt.Errorf("list branch refs: %w", err)
			}

			return response.Value, response.ContinuationToken, nil
		},
		func(ref git.GitRef) (bool, error) {
			if derefString(ref.Name) != wantName {
				return false, nil
			}

			head = strings.TrimSpace(derefString(ref.ObjectId))

			return true, nil
		},
	)
	if err != nil {
		return "", err
	}

	if head == "" {
		return "", fmt.Errorf("%w: branch %q", ErrRefNotFound, branch)
	}

	return head, nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}
