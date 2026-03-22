package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v84/github"
	"golang.org/x/sync/errgroup"
)

func (g *GitHub) GetLatestVersionRef(ctx context.Context) (string, error) {
	release, err := g.latestRelease(ctx)
	if err == nil {
		return release.GetTagName(), nil
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

func (g *GitHub) ListTags(ctx context.Context) ([]string, error) {
	options := &github.ListOptions{PerPage: 100} //nolint:mnd // reasonable API page size
	tags := make([]string, 0)

	for {
		pageTags, resp, err := g.client.Repositories.ListTags(ctx, g.repo.Owner, g.repo.Name, options)
		if err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}

		for _, tag := range pageTags {
			name := strings.TrimSpace(tag.GetName())
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

const maxConcurrentPathFetches = 5

//nolint:funlen // Commit pagination and concurrent path fetching are clearer kept together.
func (g *GitHub) GetCommitsSince(ctx context.Context, ref, branch string, includePaths bool) ([]CommitEntry, error) {
	boundaryRef := strings.TrimSpace(ref)
	resolvedBoundarySHA := boundaryRef
	branch = strings.TrimSpace(branch)

	if resolvedBoundarySHA != "" {
		resolvedSHA, err := g.resolveCommitSHA(ctx, resolvedBoundarySHA)
		if err != nil {
			return nil, fmt.Errorf("resolve ref %q: %w", boundaryRef, err)
		}

		resolvedBoundarySHA = resolvedSHA
	}

	opts := &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: 100}, //nolint:mnd // reasonable page size
	}

	if branch != "" {
		opts.SHA = branch
	} else if resolvedBoundarySHA != "" {
		opts.SHA = "HEAD"
	}

	var entries []CommitEntry

	boundaryFound := false

	for {
		commits, resp, err := g.client.Repositories.ListCommits(ctx, g.repo.Owner, g.repo.Name, opts)
		if err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}

		for _, c := range commits {
			sha := c.GetSHA()

			if resolvedBoundarySHA != "" && sha == resolvedBoundarySHA {
				boundaryFound = true

				break
			}

			entries = append(entries, CommitEntry{
				Hash:    sha,
				Message: c.GetCommit().GetMessage(),
			})
		}

		if boundaryFound || resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	if resolvedBoundarySHA != "" && !boundaryFound {
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

func (g *GitHub) latestRelease(ctx context.Context) (*github.RepositoryRelease, error) {
	release, resp, err := g.client.Repositories.GetLatestRelease(ctx, g.repo.Owner, g.repo.Name)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get latest release: %w", err)
	}

	return release, nil
}

func (g *GitHub) resolveCommitSHA(ctx context.Context, ref string) (string, error) {
	commit, _, err := g.client.Repositories.GetCommit(ctx, g.repo.Owner, g.repo.Name, ref, nil)
	if err != nil {
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
	paths := make([]string, 0)
	seen := make(map[string]struct{})

	for {
		commitDetails, resp, err := g.client.Repositories.GetCommit(ctx, g.repo.Owner, g.repo.Name, sha, options)
		if err != nil {
			return nil, fmt.Errorf("get changed files for commit %q: %w", sha, err)
		}

		for _, changedFile := range commitDetails.Files {
			for _, candidatePath := range []string{changedFile.GetFilename(), changedFile.GetPreviousFilename()} {
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
