package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func (g *GitLab) GetLatestVersionRef(ctx context.Context) (string, error) {
	return latestVersionRefWithReleaseFallback(ctx,
		func(ctx context.Context) (string, error) {
			release, err := g.latestRelease(ctx)
			if err != nil {
				return "", err
			}

			return release.TagName, nil
		},
		g.ListTags,
	)
}

func (g *GitLab) ListTags(ctx context.Context) ([]string, error) {
	slog.DebugContext(ctx, "gitlab: listing tags")

	options := &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}
	tags := make([]string, 0)

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
			if name != "" {
				tags = append(tags, name)
			}

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "gitlab: listed tags", slog.Int("count", len(tags)))

	return tags, nil
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

func (g *GitLab) latestRelease(ctx context.Context) (*gitlab.Release, error) {
	releases, _, err := g.client.Releases.ListReleases(g.projectID, &gitlab.ListReleasesOptions{
		ListOptions: gitlab.ListOptions{PerPage: 1},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	if len(releases) == 0 {
		return nil, ErrNoRelease
	}

	return releases[0], nil
}
