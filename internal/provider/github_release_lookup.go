package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v89/github"
)

func (g *GitHub) GetReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	slog.DebugContext(ctx, "github: looking up release by tag", slog.String("tag", tag))

	release, resp, err := g.client.Repositories.GetReleaseByTag(ctx, g.repo.Owner, g.repo.Name, tag)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			slog.DebugContext(ctx, "github: release not found",
				slog.String("tag", tag),
				slog.Int("status", resp.StatusCode),
			)

			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get release by tag %q: %w", tag, err)
	}

	slog.DebugContext(ctx, "github: release found",
		slog.String("tag", tag),
		slog.String("url", release.GetHTMLURL()),
	)

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

	slog.DebugContext(ctx, "github: creating release",
		slog.String("tag", opts.TagName),
		slog.String("target_commitish", targetCommitish),
		slog.Bool("prerelease", opts.Prerelease),
	)

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

	slog.DebugContext(ctx, "github: created release",
		slog.String("tag", rel.GetTagName()),
		slog.String("url", rel.GetHTMLURL()),
	)

	return gitHubRelease(rel), nil
}

// Creates an annotated tag carrying the release body so the changelog lives in
// portable git data, mirroring release-please behavior. Idempotent: if the tag
// ref already exists the call is a no-op.
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
