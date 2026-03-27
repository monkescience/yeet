package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v84/github"
)

func (g *GitHub) GetReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	release, resp, err := g.client.Repositories.GetReleaseByTag(ctx, g.repo.Owner, g.repo.Name, tag)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get release by tag %q: %w", tag, err)
	}

	return gitHubRelease(release), nil
}

func (g *GitHub) TagExists(ctx context.Context, tag string) (bool, error) {
	_, resp, err := g.client.Git.GetRef(ctx, g.repo.Owner, g.repo.Name, "tags/"+tag)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return false, nil
		}

		return false, fmt.Errorf("get tag ref %q: %w", tag, err)
	}

	return true, nil
}

func (g *GitHub) CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error) {
	targetCommitish := strings.TrimSpace(opts.Ref)
	releaseRequest := &github.RepositoryRelease{
		TagName: new(opts.TagName),
		Name:    new(opts.Name),
		Body:    new(opts.Body),
	}

	if targetCommitish != "" {
		releaseRequest.TargetCommitish = new(targetCommitish)
	}

	rel, _, err := g.client.Repositories.CreateRelease(
		ctx, g.repo.Owner, g.repo.Name, releaseRequest,
	)
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	return gitHubRelease(rel), nil
}

func gitHubRelease(release *github.RepositoryRelease) *Release {
	return &Release{
		TagName: release.GetTagName(),
		Name:    release.GetName(),
		Body:    release.GetBody(),
		URL:     release.GetHTMLURL(),
	}
}
