package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v84/github"
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

func (g *GitHub) GetCommitsSince(ctx context.Context, ref, branch string) ([]CommitEntry, error) {
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

	for {
		commits, resp, err := g.client.Repositories.ListCommits(ctx, g.repo.Owner, g.repo.Name, opts)
		if err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}

		for _, c := range commits {
			sha := c.GetSHA()

			if resolvedBoundarySHA != "" && sha == resolvedBoundarySHA {
				return entries, nil
			}

			entries = append(entries, CommitEntry{
				Hash:    sha,
				Message: c.GetCommit().GetMessage(),
			})
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	if resolvedBoundarySHA != "" {
		return nil, &CommitBoundaryNotFoundError{Ref: boundaryRef, Branch: branch}
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
