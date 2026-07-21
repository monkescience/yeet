package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/go-github/v89/github"
)

func (g *GitHub) GetLatestReleaseRef(ctx context.Context) (string, error) {
	release, err := g.latestRelease(ctx)
	if err != nil {
		return "", err
	}

	return release.GetTagName(), nil
}

func (g *GitHub) ListTagRefs(ctx context.Context) ([]TagRef, error) {
	slog.DebugContext(ctx, "github: listing tags")

	options := &github.ListOptions{PerPage: gitHubPageSize}
	refs := make([]TagRef, 0)

	err := paginate(ctx, "listing tags",
		func(page int) ([]*github.RepositoryTag, int, error) {
			options.Page = page

			pageTags, resp, err := g.client.Repositories.ListTags(ctx, g.repo.Owner, g.repo.Name, options)
			if err != nil {
				return nil, 0, fmt.Errorf("list tags: %w", err)
			}

			return pageTags, gitHubNextPage(resp), nil
		},
		func(tag *github.RepositoryTag) (bool, error) {
			name := strings.TrimSpace(tag.GetName())
			if name == "" {
				return false, nil
			}

			commitHash := strings.TrimSpace(tag.GetCommit().GetSHA())
			if commitHash == "" {
				return false, fmt.Errorf("%w: tag %q", ErrEmptyCommitSHA, name)
			}

			refs = append(refs, TagRef{Name: name, CommitSHA: commitHash})

			return false, nil
		},
	)
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
		return "", fmt.Errorf("%w: empty branch", ErrRefNotFound)
	}

	sha, err := g.resolveCommitSHA(ctx, "heads/"+branch)
	if err != nil {
		return "", fmt.Errorf("get branch head %q: %w", branch, err)
	}

	return sha, nil
}

func (g *GitHub) latestRelease(ctx context.Context) (*github.RepositoryRelease, error) {
	slog.DebugContext(ctx, "github: looking up latest release")

	release, resp, err := g.client.Repositories.GetLatestRelease(ctx, g.repo.Owner, g.repo.Name)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			slog.DebugContext(ctx, "github: no latest release",
				slog.Int("status", resp.StatusCode),
			)

			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get latest release: %w", err)
	}

	slog.DebugContext(ctx, "github: latest release",
		slog.String("tag", release.GetTagName()),
		slog.String("url", release.GetHTMLURL()),
	)

	return release, nil
}

func (g *GitHub) resolveCommitSHA(ctx context.Context, ref string) (string, error) {
	commit, resp, err := g.client.Repositories.GetCommit(ctx, g.repo.Owner, g.repo.Name, ref, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("%w: ref %q", ErrRefNotFound, ref)
		}

		return "", fmt.Errorf("get commit for ref %q: %w", ref, err)
	}

	sha := commit.GetSHA()
	if sha == "" {
		return "", fmt.Errorf("%w: ref %q", ErrEmptyCommitSHA, ref)
	}

	return sha, nil
}
