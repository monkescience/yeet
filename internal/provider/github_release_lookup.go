package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v85/github"
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

	err := g.ensureAnnotatedTag(ctx, opts.TagName, targetCommitish, opts.Body)
	if err != nil {
		return nil, err
	}

	releaseRequest := &github.RepositoryRelease{
		TagName:    new(opts.TagName),
		Name:       new(opts.Name),
		Body:       new(opts.Body),
		Prerelease: new(opts.Prerelease),
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

// ensureAnnotatedTag creates an annotated tag carrying the release body so the
// changelog lives in portable git data, mirroring release-please behavior. It
// is idempotent: if the tag ref already exists the call is a no-op.
func (g *GitHub) ensureAnnotatedTag(ctx context.Context, tagName, ref, message string) error {
	if strings.TrimSpace(tagName) == "" || strings.TrimSpace(ref) == "" {
		return nil
	}

	exists, err := g.TagExists(ctx, tagName)
	if err != nil {
		return fmt.Errorf("check tag %q: %w", tagName, err)
	}

	if exists {
		return nil
	}

	objectSHA, err := g.resolveCommitSHA(ctx, ref)
	if err != nil {
		return fmt.Errorf("resolve tag target %q: %w", ref, err)
	}

	tagger := g.resolveTaggerIdentity(ctx)

	tagObject, _, err := g.client.Git.CreateTag(ctx, g.repo.Owner, g.repo.Name, github.CreateTag{
		Tag:     tagName,
		Message: message,
		Object:  objectSHA,
		Type:    "commit",
		Tagger:  tagger,
	})
	if err != nil {
		return fmt.Errorf("create annotated tag %q: %w", tagName, err)
	}

	_, _, err = g.client.Git.CreateRef(ctx, g.repo.Owner, g.repo.Name, github.CreateRef{
		Ref: "refs/tags/" + tagName,
		SHA: tagObject.GetSHA(),
	})
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusUnprocessableEntity {
			return nil
		}

		return fmt.Errorf("create tag ref %q: %w", tagName, err)
	}

	return nil
}

func (g *GitHub) resolveTaggerIdentity(ctx context.Context) *github.CommitAuthor {
	g.taggerOnce.Do(func() {
		user, _, err := g.client.Users.Get(ctx, "")
		if err != nil || user == nil {
			g.taggerName = gitHubFallbackTaggerName
			g.taggerEmail = gitHubFallbackTaggerEmail

			return
		}

		name := strings.TrimSpace(user.GetName())
		if name == "" {
			name = strings.TrimSpace(user.GetLogin())
		}

		if name == "" {
			name = gitHubFallbackTaggerName
		}

		email := strings.TrimSpace(user.GetEmail())
		if email == "" {
			email = gitHubFallbackTaggerEmail
		}

		g.taggerName = name
		g.taggerEmail = email
	})

	now := time.Now().UTC()

	return &github.CommitAuthor{
		Name:  new(g.taggerName),
		Email: new(g.taggerEmail),
		Date:  &github.Timestamp{Time: now},
	}
}

func gitHubRelease(release *github.RepositoryRelease) *Release {
	return &Release{
		TagName: release.GetTagName(),
		Name:    release.GetName(),
		Body:    release.GetBody(),
		URL:     release.GetHTMLURL(),
	}
}
