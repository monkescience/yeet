package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func (g *GitLab) GetReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	slog.DebugContext(ctx, "gitlab: looking up release by tag", slog.String("tag", tag))

	release, _, err := g.client.Releases.GetRelease(g.pid, tag, gitlab.WithContext(ctx))
	if err != nil {
		if errors.Is(err, gitlab.ErrNotFound) {
			slog.DebugContext(ctx, "gitlab: release not found", slog.String("tag", tag))

			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get release by tag %q: %w", tag, err)
	}

	slog.DebugContext(ctx, "gitlab: release found",
		slog.String("tag", tag),
		slog.String("url", release.Links.Self),
	)

	return gitLabRelease(release), nil
}

func (g *GitLab) TagExists(ctx context.Context, tag string) (bool, error) {
	_, _, err := g.client.Tags.GetTag(g.pid, tag, gitlab.WithContext(ctx))
	if err != nil {
		if errors.Is(err, gitlab.ErrNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("get tag %q: %w", tag, err)
	}

	return true, nil
}

func (g *GitLab) CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error) {
	ref := strings.TrimSpace(opts.Ref)

	slog.DebugContext(ctx, "gitlab: creating release",
		slog.String("tag", opts.TagName),
		slog.String("ref", ref),
	)

	releaseOptions := &gitlab.CreateReleaseOptions{
		TagName:     new(opts.TagName),
		Name:        new(opts.Name),
		Description: new(opts.Body),
		TagMessage:  new(opts.Body),
	}

	if ref != "" {
		releaseOptions.Ref = new(ref)
	}

	release, _, err := g.client.Releases.CreateRelease(g.pid, releaseOptions, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	slog.DebugContext(ctx, "gitlab: created release",
		slog.String("tag", release.TagName),
		slog.String("url", release.Links.Self),
	)

	return gitLabRelease(release), nil
}

func gitLabRelease(release *gitlab.Release) *Release {
	return &Release{
		TagName: release.TagName,
		Name:    release.Name,
		Body:    release.Description,
		URL:     release.Links.Self,
	}
}
