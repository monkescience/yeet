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
	core       *releaseCore
	publisher  releasePublishingProvider
	source     releaseSource
	changelogs *changelogFileCache
}

func newReleasePublisher(
	core *releaseCore,
	publisher releasePublishingProvider,
	source releaseSource,
) *releasePublisher {
	return &releasePublisher{
		core:       core,
		publisher:  publisher,
		source:     source,
		changelogs: newChangelogFileCache(),
	}
}

func (p *releasePublisher) finalizeMergedReleasePR(ctx context.Context) ([]*provider.Release, error) {
	r := p.core

	mergedPR, err := p.publisher.FindMergedReleasePR(ctx, r.cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("find merged release PR: %w", err)
	}

	manifest, err := releaseManifestFromPullRequest(mergedPR)
	if err != nil {
		return nil, err
	}

	if err := r.validateReleaseManifest(mergedPR, manifest); err != nil {
		return nil, err
	}

	prerelease := manifest.Prerelease

	releases := make([]*provider.Release, 0, len(manifest.Targets))
	for _, targetManifest := range manifest.Targets {
		releaseInfo, releaseErr := p.releaseForTag(
			ctx,
			targetManifest.Tag,
			targetManifest.ChangelogFile,
			releaseRefForPullRequest(mergedPR, r.cfg.Branch),
			prerelease,
		)
		if releaseErr != nil {
			return nil, releaseErr
		}

		releases = append(releases, releaseInfo)
	}

	if err := p.markReleasePRTagged(ctx, mergedPR); err != nil {
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

		releaseInfo, err := p.ensureReleaseForTag(ctx, plan.NextTag, ref, releaseBody, p.core.isPrerelease())
		if err != nil {
			return nil, err
		}

		releases = append(releases, releaseInfo)
	}

	return releases, nil
}

func (p *releasePublisher) releaseForTag(
	ctx context.Context,
	tag, changelogFile, ref string,
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

	return p.createReleaseForUnreleasedTag(ctx, tag, ref, releaseBody, prerelease)
}

func (p *releasePublisher) createReleaseForTag(
	ctx context.Context,
	tag, ref, releaseBody string,
	prerelease bool,
) (*provider.Release, error) {
	releaseInfo, err := p.publisher.CreateRelease(ctx, provider.ReleaseOptions{
		TagName:    tag,
		Ref:        ref,
		Name:       tag,
		Body:       releaseBody,
		Prerelease: prerelease,
	})
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	slog.InfoContext(ctx, "created release",
		slog.String("tag", tag),
		slog.String("url", releaseInfo.URL),
	)

	return releaseInfo, nil
}

func (p *releasePublisher) ensureReleaseForTag(
	ctx context.Context,
	tag, ref, releaseBody string,
	prerelease bool,
) (*provider.Release, error) {
	existingRelease, exists, err := p.existingReleaseForTag(ctx, tag)
	if err != nil {
		return nil, err
	}

	if exists {
		return existingRelease, nil
	}

	return p.createReleaseForUnreleasedTag(ctx, tag, ref, releaseBody, prerelease)
}

func (p *releasePublisher) createReleaseForUnreleasedTag(
	ctx context.Context,
	tag, ref, releaseBody string,
	prerelease bool,
) (*provider.Release, error) {
	r := p.core

	creationRef := strings.TrimSpace(ref)
	if creationRef == "" {
		creationRef = r.cfg.Branch
	}

	return p.createReleaseForTag(ctx, tag, creationRef, releaseBody, prerelease)
}

func (p *releasePublisher) existingReleaseForTag(ctx context.Context, tag string) (*provider.Release, bool, error) {
	releaseInfo, err := p.publisher.GetReleaseByTag(ctx, tag)
	if err != nil {
		if !errors.Is(err, provider.ErrNoRelease) {
			return nil, false, fmt.Errorf("get release by tag %q: %w", tag, err)
		}

		return nil, false, nil
	}

	slog.InfoContext(ctx, "release already exists", slog.String("tag", tag))

	return releaseInfo, true, nil
}

func (p *releasePublisher) markReleasePRTagged(ctx context.Context, pullRequest *provider.PullRequest) error {
	err := p.publisher.MarkReleasePRTagged(ctx, pullRequest.Number)
	if err != nil {
		return fmt.Errorf("mark release PR tagged: %w", err)
	}

	slog.InfoContext(ctx, "marked release PR tagged", slog.String("url", pullRequest.URL))

	return nil
}

func (p *releasePublisher) releaseNotesFromChangelog(
	ctx context.Context,
	changelogFile string,
	tag string,
) (string, error) {
	r := p.core

	changelogBody, err := p.changelogs.get(r.cfg.Branch, changelogFile, func() (string, error) {
		content, getErr := p.source.GetFile(ctx, r.cfg.Branch, changelogFile)
		if getErr != nil {
			return "", fmt.Errorf("get changelog file %s: %w", changelogFile, getErr)
		}

		return content, nil
	})
	if err != nil {
		return "", err
	}

	entry, err := changelogEntryByTag(changelogBody, tag)
	if err != nil {
		return "", err
	}

	return entry, nil
}
