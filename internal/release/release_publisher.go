package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/forge"
)

type releasePublisher struct {
	core       *releaseCore
	publisher  releasePublishingProvider
	source     releaseSource
	changelogs *changelogFileCache
	labels     labelLifecycle
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
		labels:     newLabelLifecycle(core, publisher),
	}
}

func (p *releasePublisher) finalizeMergedReleasePR(ctx context.Context) ([]FinalizedRelease, error) {
	r := p.core

	mergedPR, err := p.publisher.FindMergedReleasePR(ctx, r.cfg.Branch, r.cfg.Release.Labels.Pending)
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

	releaseRef, err := releaseRefForPullRequest(mergedPR)
	if err != nil {
		return nil, err
	}

	if err := p.preflightReleasePRTagging(ctx); err != nil {
		return nil, err
	}

	prerelease := manifest.Prerelease

	releases := make([]FinalizedRelease, 0, len(manifest.Targets))
	for _, targetManifest := range manifest.Targets {
		releaseInfo, releaseErr := p.releaseForTag(
			ctx,
			targetManifest.Tag,
			targetManifest.ChangelogFile,
			releaseRef,
			prerelease,
		)
		if releaseErr != nil {
			return nil, releaseErr
		}

		releases = append(releases, FinalizedRelease{
			TargetID:  targetManifest.ID,
			CommitSHA: releaseRef,
			Release:   releaseInfo,
		})
	}

	if err := p.markReleasePRTagged(ctx, mergedPR); err != nil {
		return nil, err
	}

	return releases, nil
}

func (p *releasePublisher) preflightReleasePRTagging(ctx context.Context) error {
	taggedLabel := p.core.cfg.Release.Labels.Tagged
	if err := p.publisher.PreflightReleasePRTagging(ctx, taggedLabel); err != nil {
		return fmt.Errorf("preflight release PR tagging: %w", err)
	}

	return nil
}

func (p *releasePublisher) ensureReleasesForPlans(
	ctx context.Context,
	plans []TargetPlan,
	ref string,
) ([]FinalizedRelease, error) {
	releases := make([]FinalizedRelease, 0, len(plans))

	for _, plan := range plans {
		releaseBody := changelog.Render(plan.Entry)

		releaseInfo, err := p.ensureReleaseForTag(ctx, plan.NextTag, ref, releaseBody, p.core.isPrerelease())
		if err != nil {
			return nil, err
		}

		releases = append(releases, FinalizedRelease{
			TargetID:  plan.ID,
			CommitSHA: ref,
			Release:   releaseInfo,
		})
	}

	return releases, nil
}

func (p *releasePublisher) releaseForTag(
	ctx context.Context,
	tag, changelogFile, ref string,
	prerelease bool,
) (*forge.Release, error) {
	existingRelease, exists, err := p.existingReleaseForTag(ctx, tag, ref)
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

	return p.createReleaseForTag(ctx, tag, ref, releaseBody, prerelease)
}

func (p *releasePublisher) createReleaseForTag(
	ctx context.Context,
	tag, ref, releaseBody string,
	prerelease bool,
) (*forge.Release, error) {
	releaseInfo, err := p.publisher.CreateRelease(ctx, forge.ReleaseOptions{
		TagName:    tag,
		Ref:        ref,
		Name:       tag,
		Body:       releaseBody,
		Prerelease: prerelease,
	})
	if err != nil {
		recovered, exists, recoveryErr := p.existingReleaseForTag(ctx, tag, ref)
		if recoveryErr == nil && exists {
			return recovered, nil
		}

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
) (*forge.Release, error) {
	existingRelease, exists, err := p.existingReleaseForTag(ctx, tag, ref)
	if err != nil {
		return nil, err
	}

	if exists {
		return existingRelease, nil
	}

	return p.createReleaseForTag(ctx, tag, ref, releaseBody, prerelease)
}

func (p *releasePublisher) existingReleaseForTag(
	ctx context.Context,
	tag, expectedCommitSHA string,
) (*forge.Release, bool, error) {
	releaseInfo, err := p.publisher.GetReleaseByTag(ctx, tag)
	if err != nil {
		if !errors.Is(err, forge.ErrNoRelease) {
			return nil, false, fmt.Errorf("get release by tag %q: %w", tag, err)
		}

		return nil, false, nil
	}

	actualCommitSHA := strings.TrimSpace(releaseInfo.CommitSHA)
	if actualCommitSHA == "" || !strings.EqualFold(actualCommitSHA, strings.TrimSpace(expectedCommitSHA)) {
		return nil, false, fmt.Errorf(
			"%w: tag %q resolves to %q, expected %q",
			forge.ErrReleaseTagMismatch,
			tag,
			actualCommitSHA,
			expectedCommitSHA,
		)
	}

	slog.InfoContext(ctx, "release already exists", slog.String("tag", tag))

	return releaseInfo, true, nil
}

func (p *releasePublisher) markReleasePRTagged(ctx context.Context, pullRequest *forge.PullRequest) error {
	if err := p.labels.published(ctx, pullRequest.Number); err != nil {
		return err
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

	entry, err := changelog.EntryByTag(changelogBody, tag)
	if err != nil {
		return "", fmt.Errorf("read changelog entry for %s: %w", tag, err)
	}

	return entry, nil
}
