package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/go-github/v89/github"
	"github.com/monkescience/yeet/internal/forge"
)

func (g *GitHub) ListTagRefs(ctx context.Context) ([]forge.TagRef, error) {
	slog.DebugContext(ctx, "github: listing tags")

	refs, err := foldTagRefs(ctx, g.tagPages, func(tag *github.RepositoryTag) (string, string, bool) {
		return tag.GetName(), tag.GetCommit().GetSHA(), true
	})
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "github: listed tags", slog.Int("count", len(refs)))

	return refs, nil
}

// GetBranchHead returns the commit SHA branch currently points at. The
// "heads/" prefix pins the lookup to the branch namespace so a tag with the
// same name cannot shadow it.
func (g *GitHub) GetBranchHead(ctx context.Context, branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("%w: empty branch", forge.ErrRefNotFound)
	}

	sha, err := g.resolveCommitSHA(ctx, "heads/"+branch)
	if err != nil {
		return "", fmt.Errorf("get branch head %q: %w", branch, err)
	}

	return sha, nil
}

func (g *GitHub) tagPages(ctx context.Context, handle func(*github.RepositoryTag) (bool, error)) error {
	options := &github.ListOptions{PerPage: gitHubPageSize}

	return paginate(ctx, "listing tags",
		func(page int) ([]*github.RepositoryTag, int, error) {
			options.Page = page

			pageTags, resp, err := g.client.Repositories.ListTags(ctx, g.repo.Owner, g.repo.Name, options)
			if err != nil {
				return nil, 0, fmt.Errorf("list tags: %w", err)
			}

			return pageTags, gitHubNextPage(resp), nil
		},
		handle,
	)
}

func (g *GitHub) resolveCommitSHA(ctx context.Context, ref string) (string, error) {
	commit, resp, err := g.client.Repositories.GetCommit(ctx, g.repo.Owner, g.repo.Name, ref, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("%w: ref %q", forge.ErrRefNotFound, ref)
		}

		return "", fmt.Errorf("get commit for ref %q: %w", ref, err)
	}

	sha := commit.GetSHA()
	if sha == "" {
		return "", fmt.Errorf("%w: ref %q", forge.ErrEmptyCommitSHA, ref)
	}

	return sha, nil
}
