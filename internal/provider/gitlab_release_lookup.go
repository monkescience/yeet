package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

func (g *GitLab) GetReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	release, _, err := g.client.Releases.GetRelease(g.pid, tag, gitlab.WithContext(ctx))
	if err != nil {
		if errors.Is(err, gitlab.ErrNotFound) {
			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get release by tag %q: %w", tag, err)
	}

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
	releaseOptions := &gitlab.CreateReleaseOptions{
		TagName:     gitlab.Ptr(opts.TagName),
		Name:        gitlab.Ptr(opts.Name),
		Description: gitlab.Ptr(opts.Body),
	}

	if ref != "" {
		releaseOptions.Ref = gitlab.Ptr(ref)
	}

	release, _, err := g.client.Releases.CreateRelease(g.pid, releaseOptions, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

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
