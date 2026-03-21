package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/provider"
)

type releasePublisher struct {
	releaser *Releaser
}

func newReleasePublisher(releaser *Releaser) *releasePublisher {
	return &releasePublisher{releaser: releaser}
}

func (p *releasePublisher) finalizeMergedReleasePR(ctx context.Context) (*provider.Release, error) {
	r := p.releaser

	mergedPR, err := r.publisher.FindMergedReleasePR(ctx, r.cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("find merged release PR: %w", err)
	}

	tag, err := releaseTagFromPullRequest(mergedPR)
	if err != nil {
		return nil, err
	}

	if isPreviewTag(tag, r.strategy.prefix) {
		return nil, fmt.Errorf("%w: %s", ErrPreviewTagNotAllowed, tag)
	}

	releaseInfo, err := p.releaseForTag(ctx, tag, releaseRefForPullRequest(mergedPR, r.cfg.Branch))
	if err != nil {
		return nil, err
	}

	err = p.markReleasePRTagged(ctx, mergedPR)
	if err != nil {
		return nil, err
	}

	return releaseInfo, nil
}

func (p *releasePublisher) releaseForTag(ctx context.Context, tag, ref string) (*provider.Release, error) {
	existingRelease, exists, err := p.existingReleaseForTag(ctx, tag)
	if err != nil {
		return nil, err
	}

	if exists {
		return existingRelease, nil
	}

	releaseBody, err := p.releaseNotesFromChangelog(ctx, tag)
	if err != nil {
		return nil, err
	}

	return p.ensureReleaseForTag(ctx, tag, ref, releaseBody)
}

func (p *releasePublisher) createReleaseForTag(
	ctx context.Context,
	tag, ref, releaseBody string,
) (*provider.Release, error) {
	r := p.releaser

	releaseInfo, err := r.publisher.CreateRelease(ctx, provider.ReleaseOptions{
		TagName: tag,
		Ref:     ref,
		Name:    tag,
		Body:    releaseBody,
	})
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	slog.InfoContext(ctx, "created release", "tag", tag, "url", releaseInfo.URL)

	return releaseInfo, nil
}

func (p *releasePublisher) ensureReleaseForTag(
	ctx context.Context,
	tag, ref, releaseBody string,
) (*provider.Release, error) {
	r := p.releaser

	existingRelease, exists, err := p.existingReleaseForTag(ctx, tag)
	if err != nil {
		return nil, err
	}

	if exists {
		return existingRelease, nil
	}

	tagExists, err := r.publisher.TagExists(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("check tag %q: %w", tag, err)
	}

	if tagExists {
		return p.createReleaseForTag(ctx, tag, "", releaseBody)
	}

	creationRef := strings.TrimSpace(ref)
	if creationRef == "" {
		creationRef = r.cfg.Branch
	}

	return p.createReleaseForTag(ctx, tag, creationRef, releaseBody)
}

func (p *releasePublisher) existingReleaseForTag(ctx context.Context, tag string) (*provider.Release, bool, error) {
	r := p.releaser

	releaseInfo, err := r.publisher.GetReleaseByTag(ctx, tag)
	if err != nil {
		if !errors.Is(err, provider.ErrNoRelease) {
			return nil, false, fmt.Errorf("get release by tag %q: %w", tag, err)
		}

		return nil, false, nil
	}

	slog.InfoContext(ctx, "release already exists", "tag", tag)

	return releaseInfo, true, nil
}

func (p *releasePublisher) markReleasePRTagged(ctx context.Context, pullRequest *provider.PullRequest) error {
	r := p.releaser

	err := r.publisher.MarkReleasePRTagged(ctx, pullRequest.Number)
	if err != nil {
		return fmt.Errorf("mark release PR tagged: %w", err)
	}

	slog.InfoContext(ctx, "marked release PR tagged", "url", pullRequest.URL)

	return nil
}

func (p *releasePublisher) releaseNotesFromChangelog(ctx context.Context, tag string) (string, error) {
	r := p.releaser

	changelogBody, err := r.publisher.GetFile(ctx, r.cfg.Branch, r.cfg.Changelog.File)
	if err != nil {
		return "", fmt.Errorf("get changelog file %s: %w", r.cfg.Changelog.File, err)
	}

	entry, err := changelogEntryByTag(changelogBody, tag)
	if err != nil {
		return "", err
	}

	return entry, nil
}
