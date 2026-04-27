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

func (p *releasePublisher) finalizeMergedReleasePR(ctx context.Context) ([]*provider.Release, error) {
	r := p.releaser

	mergedPR, err := r.publisher.FindMergedReleasePR(ctx, r.cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("find merged release PR: %w", err)
	}

	manifest, err := releaseManifestFromPullRequest(mergedPR)
	if err != nil {
		return nil, err
	}

	releaseNotes := releaseNotesFromPullRequest(mergedPR)
	prerelease := manifest.Prerelease || r.isPrerelease()

	releases := make([]*provider.Release, 0, len(manifest.Targets))
	for _, targetManifest := range manifest.Targets {
		releaseInfo, releaseErr := p.releaseForTag(
			ctx,
			targetManifest.Tag,
			targetManifest.ChangelogFile,
			releaseRefForPullRequest(mergedPR, r.cfg.Branch),
			releaseNotes,
			prerelease,
		)
		if releaseErr != nil {
			return nil, releaseErr
		}

		releases = append(releases, releaseInfo)
	}

	err = p.markReleasePRTagged(ctx, mergedPR)
	if err != nil {
		return nil, err
	}

	return releases, nil
}

func (p *releasePublisher) ensureReleasesForResult(
	ctx context.Context,
	result *Result,
	ref string,
) ([]*provider.Release, error) {
	releases := make([]*provider.Release, 0, len(result.Plans))

	for _, plan := range result.Plans {
		releaseBody := plan.Changelog

		releaseInfo, err := p.ensureReleaseForTag(ctx, plan.NextTag, ref, releaseBody, p.releaser.isPrerelease())
		if err != nil {
			return nil, err
		}

		releases = append(releases, releaseInfo)
	}

	return releases, nil
}

func (p *releasePublisher) releaseForTag(
	ctx context.Context,
	tag, changelogFile, ref, releaseNotes string,
	prerelease bool,
) (*provider.Release, error) {
	existingRelease, exists, err := p.existingReleaseForTag(ctx, tag)
	if err != nil {
		return nil, err
	}

	if exists {
		return existingRelease, nil
	}

	releaseBody, err := p.releaseNotesFromChangelog(ctx, changelogFile, tag)
	if err != nil {
		return nil, err
	}

	releaseBody = insertReleaseNotes(releaseBody, releaseNotes)

	return p.ensureReleaseForTag(ctx, tag, ref, releaseBody, prerelease)
}

func (p *releasePublisher) createReleaseForTag(
	ctx context.Context,
	tag, ref, releaseBody string,
	prerelease bool,
) (*provider.Release, error) {
	r := p.releaser

	releaseInfo, err := r.publisher.CreateRelease(ctx, provider.ReleaseOptions{
		TagName:    tag,
		Ref:        ref,
		Name:       tag,
		Body:       releaseBody,
		Prerelease: prerelease,
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
	prerelease bool,
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
		return p.createReleaseForTag(ctx, tag, "", releaseBody, prerelease)
	}

	creationRef := strings.TrimSpace(ref)
	if creationRef == "" {
		creationRef = r.cfg.Branch
	}

	return p.createReleaseForTag(ctx, tag, creationRef, releaseBody, prerelease)
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

func (p *releasePublisher) releaseNotesFromChangelog(
	ctx context.Context,
	changelogFile string,
	tag string,
) (string, error) {
	r := p.releaser

	changelogBody, err := r.publisher.GetFile(ctx, r.cfg.Branch, changelogFile)
	if err != nil {
		return "", fmt.Errorf("get changelog file %s: %w", changelogFile, err)
	}

	entry, err := changelogEntryByTag(changelogBody, tag)
	if err != nil {
		return "", err
	}

	return entry, nil
}
