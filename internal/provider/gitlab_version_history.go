package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/sync/errgroup"
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

	for range maxPaginationPages {
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
			return tags, nil
		}

		options.Page = resp.NextPage
	}

	return nil, fmt.Errorf("%w: exceeded %d pages listing tags", ErrPaginationLimitExceeded, maxPaginationPages)
}

//nolint:funlen // Commit pagination and concurrent path fetching are clearer kept together.
func (g *GitLab) GetCommitsSince(ctx context.Context, ref, branch string, includePaths bool) ([]CommitEntry, error) {
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
		opts.RefName = new(branch)
	}

	var entries []CommitEntry

	boundaryFound := false

	for range maxPaginationPages {
		commits, resp, err := g.client.Commits.ListCommits(g.pid, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}

		for _, c := range commits {
			if resolvedBoundaryID != "" && c.ID == resolvedBoundaryID {
				boundaryFound = true

				break
			}

			entries = append(entries, CommitEntry{
				Hash:    c.ID,
				Message: c.Message,
			})
		}

		if boundaryFound || resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	if resolvedBoundaryID != "" && !boundaryFound {
		return nil, &CommitBoundaryNotFoundError{Ref: boundaryRef, Branch: branch}
	}

	if includePaths && len(entries) > 0 {
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(maxConcurrentPathFetches)

		for idx := range entries {
			eg.Go(func() error {
				paths, err := g.commitPaths(egCtx, entries[idx].Hash)
				if err != nil {
					return err
				}

				entries[idx].Paths = paths

				return nil
			})
		}

		err := eg.Wait()
		if err != nil {
			return nil, fmt.Errorf("fetch commit paths: %w", err)
		}
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

func (g *GitLab) commitPaths(ctx context.Context, sha string) ([]string, error) {
	options := &gitlab.GetCommitDiffOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}
	paths := make([]string, 0)
	seen := make(map[string]struct{})

	for {
		diffs, resp, err := g.client.Commits.GetCommitDiff(g.pid, sha, options, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("get changed files for commit %q: %w", sha, err)
		}

		for _, diff := range diffs {
			for _, candidatePath := range []string{diff.NewPath, diff.OldPath} {
				normalizedPath := strings.TrimSpace(candidatePath)
				if normalizedPath == "" {
					continue
				}

				if _, exists := seen[normalizedPath]; exists {
					continue
				}

				seen[normalizedPath] = struct{}{}
				paths = append(paths, normalizedPath)
			}
		}

		if resp == nil || resp.NextPage == 0 {
			break
		}

		options.Page = resp.NextPage
	}

	return paths, nil
}
