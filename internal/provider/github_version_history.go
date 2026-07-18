package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/google/go-github/v89/github"
)

func (g *GitHub) GetLatestVersionRef(ctx context.Context) (string, error) {
	return latestVersionRefWithReleaseFallback(ctx,
		func(ctx context.Context) (string, error) {
			release, err := g.latestRelease(ctx)
			if err != nil {
				return "", err
			}

			return release.GetTagName(), nil
		},
		g.ListTags,
	)
}

func (g *GitHub) ListTags(ctx context.Context) ([]string, error) {
	slog.DebugContext(ctx, "github: listing tags")

	options := &github.ListOptions{PerPage: 100} //nolint:mnd // reasonable API page size
	tags := make([]string, 0)

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
			if name != "" {
				tags = append(tags, name)
			}

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "github: listed tags", slog.Int("count", len(tags)))

	return tags, nil
}

func (g *GitHub) GetCommitsSinceRefs(
	ctx context.Context,
	refs []string,
	branch string,
	includePaths bool,
) (CommitHistory, error) {
	branch = strings.TrimSpace(branch)

	history, err := fetchCommitHistoryByRef(ctx, refs, g.maxConcurrentRequests,
		func(ctx context.Context, ref string) ([]CommitEntry, error) {
			return g.commitsSinceRef(ctx, ref, branch)
		},
	)
	if err != nil {
		return CommitHistory{}, err
	}

	if includePaths {
		if err := hydrateCommitHistoryPaths(ctx, history, g.maxConcurrentRequests, g.commitPaths); err != nil {
			return CommitHistory{}, err
		}
	}

	return history, nil
}

// commitsSinceRef returns the commits reachable from branch but not from ref,
// newest-first. It uses the compare endpoint so GitHub computes the graph range
// (ref...branch) server-side rather than walking the branch and slicing it,
// which over-includes commits on non-linear histories. ref == "" lists the
// whole branch history.
func (g *GitHub) commitsSinceRef(
	ctx context.Context,
	ref, branch string,
) ([]CommitEntry, error) {
	boundaryRef := strings.TrimSpace(ref)

	slog.DebugContext(ctx, "github: fetching commits",
		slog.String("branch", branch),
		slog.String("boundary_ref", boundaryRef),
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

	slog.DebugContext(ctx, "github: fetched commits", slog.Int("count", len(entries)))

	return entries, nil
}

// compareCommits returns the commits in base...head, newest-first. GitHub
// returns compare commits oldest-first. The unpaginated call caps at 250, but
// we page (PerPage 100) so the full comparison is retrieved up to
// total_commits. An unknown base ref answers 404, mapped to ErrRefNotFound so
// the batch records a missing ref.
func (g *GitHub) compareCommits(ctx context.Context, base, head string) ([]CommitEntry, error) {
	opts := &github.ListOptions{PerPage: 100} //nolint:mnd // reasonable page size

	entries := make([]CommitEntry, 0)
	total := 0
	status := ""

	err := paginate(ctx, "comparing commits",
		func(page int) ([]*github.RepositoryCommit, int, error) {
			opts.Page = page

			comparison, resp, err := g.client.Repositories.CompareCommits(ctx, g.repo.Owner, g.repo.Name, base, head, opts)
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusNotFound {
					return nil, 0, fmt.Errorf("%w: ref %q", ErrRefNotFound, base)
				}

				return nil, 0, fmt.Errorf("compare commits %q...%q: %w", base, head, err)
			}

			total = comparison.GetTotalCommits()
			status = comparison.GetStatus()

			return comparison.Commits, gitHubNextPage(resp), nil
		},
		func(c *github.RepositoryCommit) (bool, error) {
			entries = append(entries, CommitEntry{Hash: c.GetSHA(), Message: c.GetCommit().GetMessage()})

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	// "diverged"/"behind" mean base is not an ancestor of head, so head is not
	// reachable from the boundary. Surface it as a missing ref, matching the old
	// behavior where an off-branch boundary never appeared in the walk.
	if !gitHubBoundaryReachable(status) {
		return nil, fmt.Errorf("%w: ref %q", ErrRefNotFound, base)
	}

	// Pagination drains the full comparison, so total should equal what we
	// collected. A shortfall means pagination stopped early (the page-count
	// safety cap or a cancelled context), not an API-side 250 truncation.
	if total > len(entries) {
		slog.DebugContext(ctx, "github: compare pagination stopped before draining range",
			slog.String("base", base),
			slog.String("head", head),
			slog.Int("total_commits", total),
			slog.Int("returned", len(entries)),
		)
	}

	// GitHub returns base...head oldest-first, but callers expect newest-first.
	slices.Reverse(entries)

	return entries, nil
}

// gitHubBoundaryReachable reports whether a compare status means the boundary
// ref is an ancestor of the head. "ahead" is the normal "head has new commits"
// case and "identical" is "nothing new", and both are reachable. An empty
// status is treated as reachable so an unexpected payload does not block a release.
func gitHubBoundaryReachable(status string) bool {
	return status == "" || status == "ahead" || status == "identical"
}

// listBranchCommits walks the entire branch history newest-first. It backs the
// unbounded ("") ref, used when a target has no previous release to bound from.
func (g *GitHub) listBranchCommits(ctx context.Context, branch string) ([]CommitEntry, error) {
	opts := &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: 100}, //nolint:mnd // reasonable page size
	}

	if branch != "" {
		opts.SHA = branch
	}

	entries := make([]CommitEntry, 0)

	err := paginate(ctx, "listing commits",
		func(page int) ([]*github.RepositoryCommit, int, error) {
			opts.Page = page

			commits, resp, err := g.client.Repositories.ListCommits(ctx, g.repo.Owner, g.repo.Name, opts)
			if err != nil {
				return nil, 0, fmt.Errorf("list commits: %w", err)
			}

			return commits, gitHubNextPage(resp), nil
		},
		func(c *github.RepositoryCommit) (bool, error) {
			entries = append(entries, CommitEntry{Hash: c.GetSHA(), Message: c.GetCommit().GetMessage()})

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return entries, nil
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

func (g *GitHub) commitPaths(ctx context.Context, sha string) ([]string, error) {
	options := &github.ListOptions{PerPage: 100} //nolint:mnd // reasonable API page size
	paths := newPathSet()

	err := paginate(ctx, fmt.Sprintf("listing commit paths for %q", sha),
		func(page int) ([]*github.CommitFile, int, error) {
			options.Page = page

			commitDetails, resp, err := g.client.Repositories.GetCommit(ctx, g.repo.Owner, g.repo.Name, sha, options)
			if err != nil {
				return nil, 0, fmt.Errorf("get changed files for commit %q: %w", sha, err)
			}

			return commitDetails.Files, gitHubNextPage(resp), nil
		},
		func(changedFile *github.CommitFile) (bool, error) {
			paths.add(changedFile.GetFilename())
			paths.add(changedFile.GetPreviousFilename())

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return paths.paths, nil
}
