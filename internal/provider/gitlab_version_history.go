package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

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

			pageTags, resp, err := g.client.Tags.ListTags(g.pid, options, gitlab.WithContext(ctx))
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

func (g *GitLab) GetCommitsSinceRefs(
	ctx context.Context,
	refs []string,
	branch string,
	includePaths bool,
) (CommitHistory, error) {
	branch = strings.TrimSpace(branch)

	return fetchCommitHistoryByRef(ctx, refs, g.maxConcurrentRequests,
		func(ctx context.Context, ref string) ([]CommitEntry, error) {
			return g.commitsSinceRef(ctx, ref, branch, includePaths)
		},
	)
}

// commitsSinceRef returns the commits reachable from branch but not from ref,
// newest-first. It uses the compare endpoint so GitLab computes the graph range
// server-side rather than walking the branch and slicing it, which
// over-includes commits on non-linear histories. ref == "" lists the whole
// branch history.
func (g *GitLab) commitsSinceRef(
	ctx context.Context,
	ref, branch string,
	includePaths bool,
) ([]CommitEntry, error) {
	boundaryRef := strings.TrimSpace(ref)

	slog.DebugContext(ctx, "gitlab: fetching commits",
		slog.String("branch", branch),
		slog.String("boundary_ref", boundaryRef),
		slog.Bool("include_paths", includePaths),
	)

	var (
		entries []CommitEntry
		err     error
	)

	if boundaryRef == "" {
		entries, err = g.listBranchCommits(ctx, branch)
	} else {
		entries, err = g.compareCommits(ctx, boundaryRef, branch)
	}

	if err != nil {
		return nil, err
	}

	if includePaths && len(entries) > 0 {
		err = hydrateCommitPaths(ctx, entries, g.maxConcurrentRequests, g.commitPaths)
		if err != nil {
			return nil, err
		}
	}

	slog.DebugContext(ctx, "gitlab: fetched commits", slog.Int("count", len(entries)))

	return entries, nil
}

// compareCommits returns the commits in from..to, newest-first. GitLab's
// compare endpoint does not page. Its commits array is always complete (the
// compare_timeout flag only warns that the diffs, which we do not read, may be
// truncated).
func (g *GitLab) compareCommits(ctx context.Context, from, to string) ([]CommitEntry, error) {
	boundary, resp, err := g.client.Commits.GetCommit(g.pid, from, nil, gitlab.WithContext(ctx))
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: ref %q", ErrRefNotFound, from)
		}

		return nil, fmt.Errorf("resolve compare boundary %q: %w", from, err)
	}

	refs := []string{from, to}

	mergeBase, resp, err := g.client.Repositories.MergeBase(g.pid, &gitlab.MergeBaseOptions{
		Ref: &refs,
	}, gitlab.WithContext(ctx))
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: ref %q", ErrRefNotFound, from)
		}

		return nil, fmt.Errorf("find merge base for %q and %q: %w", from, to, err)
	}

	if mergeBase.ID != boundary.ID {
		return nil, fmt.Errorf("%w: ref %q", ErrRefNotFound, from)
	}

	comparison, resp, err := g.client.Repositories.Compare(g.pid, &gitlab.CompareOptions{
		From: &from,
		To:   &to,
	}, gitlab.WithContext(ctx))
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: ref %q", ErrRefNotFound, from)
		}

		return nil, fmt.Errorf("compare commits %q..%q: %w", from, to, err)
	}

	commits := comparison.Commits

	// GitLab does not document the compare commit order, so sort newest-first by
	// committed date to give a stable, provider-consistent changelog order.
	slices.SortStableFunc(commits, func(a, b *gitlab.Commit) int {
		return gitLabCommitTime(b).Compare(gitLabCommitTime(a))
	})

	entries := make([]CommitEntry, 0, len(commits))
	for _, c := range commits {
		entries = append(entries, CommitEntry{Hash: c.ID, Message: c.Message})
	}

	return entries, nil
}

func gitLabCommitTime(c *gitlab.Commit) time.Time {
	if c.CommittedDate != nil {
		return *c.CommittedDate
	}

	return time.Time{}
}

// listBranchCommits walks the entire branch history newest-first. It backs the
// unbounded ("") ref, used when a target has no previous release to bound from.
func (g *GitLab) listBranchCommits(ctx context.Context, branch string) ([]CommitEntry, error) {
	opts := &gitlab.ListCommitsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable page size
	}

	if branch != "" {
		opts.RefName = new(branch)
	}

	entries := make([]CommitEntry, 0)

	err := paginate(ctx, "listing commits",
		func(page int) ([]*gitlab.Commit, int, error) {
			opts.Page = int64(page)

			commits, resp, err := g.client.Commits.ListCommits(g.pid, opts, gitlab.WithContext(ctx))
			if err != nil {
				return nil, 0, fmt.Errorf("list commits: %w", err)
			}

			return commits, gitLabNextPage(resp), nil
		},
		func(c *gitlab.Commit) (bool, error) {
			entries = append(entries, CommitEntry{Hash: c.ID, Message: c.Message})

			return false, nil
		},
	)
	if err != nil {
		return nil, err
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

func (g *GitLab) commitPaths(ctx context.Context, sha string) ([]string, error) {
	options := &gitlab.GetCommitDiffOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}
	paths := newPathSet()

	err := paginate(ctx, fmt.Sprintf("listing commit paths for %q", sha),
		func(page int) ([]*gitlab.Diff, int, error) {
			options.Page = int64(page)

			diffs, resp, err := g.client.Commits.GetCommitDiff(g.pid, sha, options, gitlab.WithContext(ctx))
			if err != nil {
				return nil, 0, fmt.Errorf("get changed files for commit %q: %w", sha, err)
			}

			return diffs, gitLabNextPage(resp), nil
		},
		func(diff *gitlab.Diff) (bool, error) {
			paths.add(diff.NewPath)
			paths.add(diff.OldPath)

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return paths.paths, nil
}
