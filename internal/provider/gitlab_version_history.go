package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func (g *GitLab) ListTagRefs(ctx context.Context) ([]TagRef, error) {
	slog.DebugContext(ctx, "gitlab: listing tags")

	options := &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{PerPage: gitLabPageSize},
	}
	refs := make([]TagRef, 0)

	err := paginate(ctx, "listing tags",
		func(page int) ([]*gitlab.Tag, int, error) {
			options.Page = int64(page)

			pageTags, resp, err := g.client.Tags.ListTags(g.projectID, options, gitlab.WithContext(ctx))
			if err != nil {
				return nil, 0, fmt.Errorf("list tags: %w", err)
			}

			return pageTags, gitLabNextPage(resp), nil
		},
		func(tag *gitlab.Tag) (bool, error) {
			name := strings.TrimSpace(tag.Name)
			if name == "" {
				return false, nil
			}

			if tag.Commit == nil || strings.TrimSpace(tag.Commit.ID) == "" {
				return false, fmt.Errorf("%w: tag %q", ErrEmptyCommitSHA, name)
			}

			refs = append(refs, TagRef{Name: name, CommitSHA: strings.TrimSpace(tag.Commit.ID)})

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "gitlab: listed tags", slog.Int("count", len(refs)))

	return refs, nil
}

// GetBranchHead returns the commit SHA branch currently points at. The
// branches API only resolves branch names, so a tag with the same name cannot
// shadow it.
func (g *GitLab) GetBranchHead(ctx context.Context, branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("%w: empty branch", ErrRefNotFound)
	}

	branchInfo, resp, err := g.client.Branches.GetBranch(g.projectID, branch, gitlab.WithContext(ctx))
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("%w: branch %q", ErrRefNotFound, branch)
		}

		return "", fmt.Errorf("get branch head %q: %w", branch, err)
	}

	if branchInfo.Commit == nil || strings.TrimSpace(branchInfo.Commit.ID) == "" {
		return "", fmt.Errorf("%w: branch %q has no head commit", ErrEmptyCommitSHA, branch)
	}

	return branchInfo.Commit.ID, nil
}
