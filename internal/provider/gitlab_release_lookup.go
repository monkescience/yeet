package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/monkescience/yeet/internal/forge"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func (g *GitLab) GetReleaseByTag(ctx context.Context, tag string) (*forge.Release, error) {
	slog.DebugContext(ctx, "gitlab: looking up release by tag", slog.String("tag", tag))

	release, _, err := g.client.Releases.GetRelease(g.projectID, tag, gitlab.WithContext(ctx))
	if err != nil {
		if errors.Is(err, gitlab.ErrNotFound) {
			slog.DebugContext(ctx, "gitlab: release not found", slog.String("tag", tag))

			return nil, forge.ErrNoRelease
		}

		return nil, fmt.Errorf("get release by tag %q: %w", tag, err)
	}

	slog.DebugContext(ctx, "gitlab: release found",
		slog.String("tag", tag),
		slog.String("url", release.Links.Self),
	)

	return gitLabRelease(release, release.Commit.ID), nil
}

func (g *GitLab) CreateRelease(ctx context.Context, opts forge.ReleaseOptions) (*forge.Release, error) {
	ref := opts.Ref
	if !isFullCommitSHA(ref) {
		return nil, fmt.Errorf("create release: %w: %q", forge.ErrInvalidCommitSHA, ref)
	}

	slog.DebugContext(ctx, "gitlab: creating release",
		slog.String("tag", opts.TagName),
		slog.String("ref", ref),
	)

	releaseOptions := &gitlab.CreateReleaseOptions{
		TagName:     new(opts.TagName),
		Name:        new(opts.Name),
		Description: new(opts.Body),
		TagMessage:  new(opts.Body),
		Ref:         new(ref),
	}

	release, _, err := g.client.Releases.CreateRelease(g.projectID, releaseOptions, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	slog.DebugContext(ctx, "gitlab: created release",
		slog.String("tag", release.TagName),
		slog.String("url", release.Links.Self),
	)

	commitSHA := release.Commit.ID

	err = validateReleaseTagCommit(opts.TagName, commitSHA, ref)
	if err != nil {
		return nil, err
	}

	return gitLabRelease(release, commitSHA), nil
}

func gitLabRelease(release *gitlab.Release, commitSHA string) *forge.Release {
	return &forge.Release{
		TagName:   release.TagName,
		CommitSHA: commitSHA,
		Name:      release.Name,
		Body:      release.Description,
		URL:       release.Links.Self,
	}
}
