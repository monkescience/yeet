package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

func (g *GitLab) GetLatestVersionRef(ctx context.Context) (string, error) {
	release, err := g.latestRelease(ctx)
	if err == nil {
		return release.TagName, nil
	}

	if !errors.Is(err, ErrNoRelease) {
		return "", err
	}

	tags, err := g.ListTags(ctx)
	if err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "", ErrNoVersionRef
	}

	return tags[0], nil
}

func (g *GitLab) ListTags(ctx context.Context) ([]string, error) {
	options := &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}
	tags := make([]string, 0)

	for {
		pageTags, resp, err := g.client.Tags.ListTags(g.pid, options, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}

		for _, tag := range pageTags {
			name := strings.TrimSpace(tag.Name)
			if name == "" {
				continue
			}

			tags = append(tags, name)
		}

		if resp.NextPage == 0 {
			break
		}

		options.Page = resp.NextPage
	}

	return tags, nil
}

func (g *GitLab) GetCommitsSince(ctx context.Context, ref, branch string) ([]CommitEntry, error) {
	boundaryRef := strings.TrimSpace(ref)
	resolvedBoundaryID := boundaryRef
	branch = strings.TrimSpace(branch)

	if resolvedBoundaryID != "" {
		resolvedID, err := g.resolveCommitID(ctx, resolvedBoundaryID)
		if err != nil {
			return nil, fmt.Errorf("resolve ref %q: %w", boundaryRef, err)
		}

		resolvedBoundaryID = resolvedID
	}

	opts := &gitlab.ListCommitsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable page size
	}

	if branch != "" {
		opts.RefName = gitlab.Ptr(branch)
	}

	var entries []CommitEntry

	for {
		commits, resp, err := g.client.Commits.ListCommits(g.pid, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}

		for _, c := range commits {
			if resolvedBoundaryID != "" && c.ID == resolvedBoundaryID {
				return entries, nil
			}

			entries = append(entries, CommitEntry{
				Hash:    c.ID,
				Message: c.Message,
			})
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	if resolvedBoundaryID != "" {
		return nil, &CommitBoundaryNotFoundError{Ref: boundaryRef, Branch: branch}
	}

	return entries, nil
}

func (g *GitLab) latestRelease(ctx context.Context) (*gitlab.Release, error) {
	releases, _, err := g.client.Releases.ListReleases(g.pid, &gitlab.ListReleasesOptions{
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

func (g *GitLab) resolveCommitID(ctx context.Context, ref string) (string, error) {
	commit, _, err := g.client.Commits.GetCommit(g.pid, ref, nil, gitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("get commit for ref %q: %w", ref, err)
	}

	if commit.ID == "" {
		return "", fmt.Errorf("%w: ref %q", ErrEmptyCommitID, ref)
	}

	return commit.ID, nil
}
