package provider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/monkescience/yeet/internal/forge"
)

const azureDevOpsTagRefPrefix = "refs/tags/"

const azureDevOpsRefPageSize = 100

func (a *AzureDevOps) ListTagRefs(ctx context.Context) ([]forge.TagRef, error) {
	slog.DebugContext(ctx, "azure devops: listing tags")

	refs, err := foldTagRefs(
		ctx,
		a.refPages("listing tag refs", "tags/", true),
		readAzureDevOpsTagRef,
	)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "azure devops: listed tags", slog.Int("count", len(refs)))

	return refs, nil
}

// GetBranchHead returns the commit SHA branch currently points at.
func (a *AzureDevOps) GetBranchHead(ctx context.Context, branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("%w: empty branch", forge.ErrRefNotFound)
	}

	ref, found, err := findRefByName(
		ctx,
		a.refPages(fmt.Sprintf("resolving branch head %q", branch), "heads/"+branch, false),
		"refs/heads/"+branch,
	)
	if err != nil {
		return "", err
	}

	head := strings.TrimSpace(derefString(ref.ObjectId))
	if !found || head == "" {
		return "", fmt.Errorf("%w: branch %q", forge.ErrRefNotFound, branch)
	}

	return head, nil
}

func (a *AzureDevOps) refPages(resource, filter string, peelTags bool) pageFetcher[git.GitRef] {
	return func(ctx context.Context, handle func(git.GitRef) (bool, error)) error {
		gitClient, err := a.client(ctx)
		if err != nil {
			return err
		}

		pageSize := azureDevOpsRefPageSize

		return paginateAzureDevOps(ctx, resource,
			func(token string) ([]git.GitRef, string, error) {
				args := git.GetRefsArgs{
					RepositoryId: &a.repo,
					Project:      &a.project,
					Filter:       &filter,
					Top:          &pageSize,
				}

				if peelTags {
					args.PeelTags = &peelTags
				}

				if token != "" {
					args.ContinuationToken = &token
				}

				response, err := gitClient.GetRefs(ctx, args)
				if err != nil {
					return nil, "", fmt.Errorf("%s: %w", resource, err)
				}

				return response.Value, response.ContinuationToken, nil
			},
			handle,
		)
	}
}

func readAzureDevOpsTagRef(ref git.GitRef) (string, string, bool) {
	if ref.Name == nil {
		return "", "", false
	}

	commitSHA := strings.TrimSpace(derefString(ref.PeeledObjectId))
	if commitSHA == "" {
		commitSHA = derefString(ref.ObjectId)
	}

	return strings.TrimPrefix(*ref.Name, azureDevOpsTagRefPrefix), commitSHA, true
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}
