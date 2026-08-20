package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/monkescience/yeet/internal/forge"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func (g *GitLab) ListTagRefs(ctx context.Context) ([]forge.TagRef, error) {
	slog.DebugContext(ctx, "gitlab: listing tags")

	refs, err := foldTagRefs(ctx, g.tagPages, func(tag *gitlab.Tag) (string, string, bool) {
		if tag.Commit == nil {
			return tag.Name, "", true
		}

		return tag.Name, tag.Commit.ID, true
	})
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "gitlab: listed tags", slog.Int("count", len(refs)))

	return refs, nil
}

func (g *GitLab) tagPages(ctx context.Context, handle func(*gitlab.Tag) (bool, error)) error {
	options := &gitlab.ListTagsOptions{
		PerPage: gitLabPageSize,
	}

	return paginate(ctx, "listing tags",
		func(page int) ([]*gitlab.Tag, int, error) {
			options.Page = int64(page)

			pageTags, resp, err := g.client.Tags.ListTags(g.projectID, options, gitlab.WithContext(ctx))
			if err != nil {
				return nil, 0, fmt.Errorf("list tags: %w", err)
			}

			return pageTags, gitLabNextPage(resp), nil
		},
		handle,
	)
}

// GetBranchHead returns the commit SHA branch currently points at. The
// branches API only resolves branch names, so a tag with the same name cannot
// shadow it.
func (g *GitLab) GetBranchHead(ctx context.Context, branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("%w: empty branch", forge.ErrRefNotFound)
	}

	branchInfo, resp, err := g.client.Branches.GetBranch(g.projectID, branch, gitlab.WithContext(ctx))
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("%w: branch %q", forge.ErrRefNotFound, branch)
		}

		return "", fmt.Errorf("get branch head %q: %w", branch, err)
	}

	if branchInfo.Commit == nil || strings.TrimSpace(branchInfo.Commit.ID) == "" {
		return "", fmt.Errorf("%w: branch %q has no head commit", forge.ErrEmptyCommitSHA, branch)
	}

	return branchInfo.Commit.ID, nil
}
