package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

type releasePublisher struct {
	core       *releaseCore
	text       *releaseText
	publisher  releasePublishingProvider
	source     releaseSource
	changelogs *changelogFileCache
	labels     labelLifecycle
}

func newReleasePublisher(
	core *releaseCore,
	text *releaseText,
	publisher releasePublishingProvider,
	source releaseSource,
) *releasePublisher {
	return &releasePublisher{
		core:       core,
		text:       text,
		publisher:  publisher,
		source:     source,
		changelogs: newChangelogFileCache(),
		labels:     newLabelLifecycle(core.cfg, publisher),
	}
}

func (p *releasePublisher) finalizeMergedReleasePR(
	ctx context.Context,
	units ...releaseUnit,
) ([]FinalizedRelease, error) {
	unit := releaseUnit{
		ID:            combinedReleaseUnitID,
		ReleaseBranch: p.core.run.releaseBranch,
	}
	if len(units) > 0 {
		unit = units[0]
	}

	mergedPR, err := p.findMergedReleasePR(ctx, unit)
	if err != nil {
		return nil, err
	}

	return p.finalizeMergedPullRequest(ctx, mergedPR, unit)
}

func (p *releasePublisher) finalizeMergedPullRequest(
	ctx context.Context,
	mergedPR *forge.PullRequest,
	unit releaseUnit,
) ([]FinalizedRelease, error) {
	manifest, err := releaseManifestFromPullRequest(mergedPR)
	if err != nil {
		return nil, err
	}

	manifest, err = p.core.validateReleaseManifest(mergedPR, manifest, unit)
	if err != nil {
		return nil, err
	}

	releaseNames, err := p.releaseNamesForManifest(manifest)
	if err != nil {
		return nil, err
	}

	releaseRef, err := releaseRefForPullRequest(mergedPR)
	if err != nil {
		return nil, err
	}

	err = p.preflightReleasePRTagging(ctx)
	if err != nil {
		return nil, err
	}

	releases, err := p.publishManifestTargets(ctx, manifest, releaseNames, releaseRef)
	if err != nil {
		return releases, err
	}

	err = p.markReleasePRTagged(ctx, mergedPR)
	if err != nil {
		return releases, err
	}

	return releases, nil
}

func (p *releasePublisher) findMergedReleasePR(
	ctx context.Context,
	unit releaseUnit,
) (*forge.PullRequest, error) {
	r := p.core

	if r.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
		pullRequest, err := p.publisher.FindMergedReleasePR(
			ctx,
			r.run.baseBranch,
			r.cfg.Release.Labels.Pending,
			unit.ReleaseBranch,
		)
		if err != nil {
			return nil, fmt.Errorf("find independent merged release PR: %w", err)
		}

		return pullRequest, nil
	}

	pullRequest, err := p.publisher.FindMergedReleasePR(
		ctx,
		r.run.baseBranch,
		r.cfg.Release.Labels.Pending,
	)
	if err != nil {
		return nil, fmt.Errorf("find combined merged release PR: %w", err)
	}

	return pullRequest, nil
}

func (p *releasePublisher) releaseNamesForManifest(manifest releaseManifest) ([]string, error) {
	releaseNames := make([]string, len(manifest.Targets))

	for index, targetManifest := range manifest.Targets {
		name, err := p.text.nameForManifest(targetManifest)
		if err != nil {
			return nil, err
		}

		releaseNames[index] = name
	}

	return releaseNames, nil
}

func (p *releasePublisher) publishManifestTargets(
	ctx context.Context,
	manifest releaseManifest,
	releaseNames []string,
	releaseRef string,
) ([]FinalizedRelease, error) {
	releases := make([]FinalizedRelease, 0, len(manifest.Targets))

	for index, targetManifest := range manifest.Targets {
		releaseInfo, err := p.releaseForTag(
			ctx,
			targetManifest.Tag,
			releaseNames[index],
			targetManifest.ChangelogFile,
			releaseRef,
			manifest.Prerelease,
		)
		if err != nil {
			return releases, err
		}

		releases = append(releases, FinalizedRelease{
			TargetID:  targetManifest.ID,
			CommitSHA: releaseRef,
			Release:   releaseInfo,
		})
	}

	return releases, nil
}

func (p *releasePublisher) preflightReleasePRTagging(ctx context.Context) error {
	taggedLabel := p.core.cfg.Release.Labels.Tagged

	err := p.publisher.PreflightReleasePRTagging(ctx, taggedLabel)
	if err != nil {
		return fmt.Errorf("preflight release PR tagging: %w", err)
	}

	return nil
}

func (p *releasePublisher) ensureReleasesForPlans(
	ctx context.Context,
	plans []TargetPlan,
	releaseNames map[string]string,
	ref string,
) ([]FinalizedRelease, error) {
	releases := make([]FinalizedRelease, 0, len(plans))

	for _, plan := range plans {
		releaseBody := changelog.Render(plan.Entry)

		releaseInfo, err := p.ensureReleaseForTag(
			ctx,
			plan.NextTag,
			releaseNames[plan.ID],
			ref,
			releaseBody,
			p.core.run.isPrerelease(),
		)
		if err != nil {
			return releases, err
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
	tag, releaseName, changelogFile, ref string,
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

	return p.createReleaseForTag(ctx, tag, releaseName, ref, releaseBody, prerelease)
}

func (p *releasePublisher) createReleaseForTag(
	ctx context.Context,
	tag, releaseName, ref, releaseBody string,
	prerelease bool,
) (*forge.Release, error) {
	releaseInfo, err := p.publisher.CreateRelease(ctx, forge.ReleaseOptions{
		TagName:    tag,
		Ref:        ref,
		Name:       releaseName,
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
	tag, releaseName, ref, releaseBody string,
	prerelease bool,
) (*forge.Release, error) {
	existingRelease, exists, err := p.existingReleaseForTag(ctx, tag, ref)
	if err != nil {
		return nil, err
	}

	if exists {
		return existingRelease, nil
	}

	return p.createReleaseForTag(ctx, tag, releaseName, ref, releaseBody, prerelease)
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
	err := p.labels.published(ctx, pullRequest.Number)
	if err != nil {
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

	changelogBody, err := p.changelogs.get(r.run.baseBranch, changelogFile, func() (string, error) {
		content, getErr := p.source.GetFile(ctx, changelogFile)
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
