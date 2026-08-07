package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/provider"
)

type releasePRWorkflow struct {
	core          *releaseCore
	prs           releasePRProvider
	files         releaseFileProvider
	branchUpdater *releaseBranchUpdater
	publisher     *releasePublisher
	changelogs    *changelogFileCache
}

func newReleasePRWorkflow(
	core *releaseCore,
	source releaseSource,
	prs releasePRProvider,
	files releaseFileProvider,
	publisher releasePublishingProvider,
) *releasePRWorkflow {
	return &releasePRWorkflow{
		core:          core,
		prs:           prs,
		files:         files,
		branchUpdater: newReleaseBranchUpdater(core, source, files),
		publisher:     newReleasePublisher(core, publisher, source),
		changelogs:    newChangelogFileCache(),
	}
}

func (w *releasePRWorkflow) createOrUpdate(ctx context.Context, result *Result) (*provider.PullRequest, error) {
	r := w.core

	pendingPRs, err := w.prs.FindOpenPendingReleasePRs(ctx, r.cfg.Branch, r.cfg.Release.Labels.Pending)
	if err != nil {
		return nil, fmt.Errorf("find pending release PRs: %w", err)
	}

	if len(pendingPRs) > 1 {
		return nil, multiplePendingReleasePRError(pendingPRs)
	}

	commitSubject, err := r.releaseCommitSubject(result)
	if err != nil {
		return nil, err
	}

	if len(pendingPRs) == 1 {
		existing := pendingPRs[0]

		if err := w.adoptUnlabeledReleasePR(ctx, existing); err != nil {
			return nil, err
		}

		if err := w.preserveExistingChangelogEdits(ctx, existing, result); err != nil {
			return nil, err
		}

		prOpts, prErr := r.releasePROptions(ctx, result, existing.Branch, w.prs.MaxPRBodyLength())
		if prErr != nil {
			return nil, prErr
		}

		return w.updateExisting(ctx, existing, existing.Branch, prOpts, commitSubject, result)
	}

	releaseBranch := stableReleaseBranch(r.cfg.Branch)

	prOpts, err := r.releasePROptions(ctx, result, releaseBranch, w.prs.MaxPRBodyLength())
	if err != nil {
		return nil, err
	}

	return w.createNew(ctx, releaseBranch, prOpts, commitSubject, result)
}

// adoptUnlabeledReleasePR recovers a release PR that was created but never
// labelled, which happens when a run is interrupted between CreateReleasePR and
// MarkReleasePRPending.
func (w *releasePRWorkflow) adoptUnlabeledReleasePR(ctx context.Context, existing *provider.PullRequest) error {
	if !existing.NeedsPendingLabel {
		return nil
	}

	slog.InfoContext(ctx, "adopting unlabelled release PR", slog.String("url", existing.URL))

	if err := w.prs.PrepareReleasePRLabels(ctx, w.core.releasePRLabels()); err != nil {
		return fmt.Errorf("prepare release PR labels: %w", err)
	}

	if err := w.prs.MarkReleasePRPending(ctx, existing.Number, w.core.releasePRLabels()); err != nil {
		return fmt.Errorf("mark release PR pending: %w", err)
	}

	existing.NeedsPendingLabel = false

	return nil
}

func (w *releasePRWorkflow) preserveExistingChangelogEdits(
	ctx context.Context,
	existing *provider.PullRequest,
	result *Result,
) error {
	if result == nil {
		return nil
	}

	r := w.core
	previousTags := make(map[string]string)
	previousChangelogFiles := make(map[string]string)

	manifest, hasManifest, err := releaseManifestFromBody(existing.Body)
	if err != nil {
		return fmt.Errorf("parse existing release PR manifest: %w", err)
	}

	if hasManifest {
		for _, targetManifest := range manifest.Targets {
			previousTags[targetManifest.ID] = targetManifest.Tag
			previousChangelogFiles[targetManifest.ID] = targetManifest.ChangelogFile
		}
	}

	for idx := range result.Plans {
		plan := &result.Plans[idx]

		target, exists := r.targets[plan.ID]
		if !exists {
			return fmt.Errorf("%w: %s", ErrUnknownTarget, plan.ID)
		}

		changelogFile := target.Changelog.File
		if previous := strings.TrimSpace(previousChangelogFiles[plan.ID]); previous != "" {
			changelogFile = previous
		}

		ownedHeadings := changelog.SectionHeadings(target.Changelog.Sections, target.Changelog.Include)

		if err := w.preserveTargetChangelogEdits(
			ctx,
			existing.Branch,
			changelogFile,
			previousTags[plan.ID],
			plan,
			ownedHeadings,
		); err != nil {
			return err
		}
	}

	return nil
}

func (w *releasePRWorkflow) preserveTargetChangelogEdits(
	ctx context.Context,
	branch, changelogFile, previousTag string,
	plan *TargetPlan,
	ownedHeadings map[string]struct{},
) error {
	existingChangelog, err := w.releaseBranchChangelog(ctx, branch, changelogFile)
	if err != nil {
		if errors.Is(err, provider.ErrFileNotFound) {
			return nil
		}

		return err
	}

	existingEntry, found, err := changelogEntryForRefresh(existingChangelog, plan.NextTag, previousTag, plan.previousRef)
	if err != nil {
		return err
	}

	if !found {
		return nil
	}

	plan.Changelog = preserveManualChangelogSections(plan.Changelog, existingEntry, ownedHeadings)
	if plan.PRChangelog != "" {
		plan.PRChangelog = preserveManualChangelogSections(plan.PRChangelog, existingEntry, ownedHeadings)
	}

	return nil
}

func (w *releasePRWorkflow) releaseBranchChangelog(ctx context.Context, branch, path string) (string, error) {
	return w.changelogs.get(branch, path, func() (string, error) {
		content, err := w.files.GetFile(ctx, branch, path)
		if err != nil {
			return "", fmt.Errorf("get release branch changelog file %s: %w", path, err)
		}

		return content, nil
	})
}

// changelogEntryForRefresh falls back to the manifest tag only while that tag
// is still an unpublished draft. Once it matches the released boundary its
// entry belongs to a shipped release and must not seed the next one.
func changelogEntryForRefresh(changelogBody, nextTag, previousTag, releasedRef string) (string, bool, error) {
	entry, err := changelogEntryByTag(changelogBody, nextTag)
	if err == nil {
		return entry, true, nil
	}

	if !errors.Is(err, ErrChangelogEntryNotFound) {
		return "", false, err
	}

	previousTag = strings.TrimSpace(previousTag)
	if previousTag == "" || previousTag == nextTag || previousTag == strings.TrimSpace(releasedRef) {
		return "", false, nil
	}

	entry, err = changelogEntryByTag(changelogBody, previousTag)
	if err == nil {
		return entry, true, nil
	}

	if errors.Is(err, ErrChangelogEntryNotFound) {
		return "", false, nil
	}

	return "", false, err
}

func (w *releasePRWorkflow) autoMerge(ctx context.Context, result *Result) error {
	r := w.core

	autoMergeEnabled := r.cfg.Release.AutoMerge || r.cfg.Release.AutoMergeForce
	if !autoMergeEnabled || result.PullRequest == nil {
		return nil
	}

	mergeOptions := provider.MergeReleasePROptions{
		BypassMergeChecks: r.cfg.Release.AutoMergeForce,
		Method:            provider.MergeMethod(r.cfg.Release.AutoMergeMethod),
	}

	if err := w.prs.PrepareReleasePRLabels(ctx, r.releasePRLifecycleLabels()); err != nil {
		return fmt.Errorf("prepare release PR labels: %w", err)
	}

	mergeSHA, err := w.prs.MergeReleasePR(ctx, result.PullRequest.Number, mergeOptions)
	if err != nil {
		if mergeOptions.BypassMergeChecks {
			return fmt.Errorf("force merge release PR: %w", err)
		}

		return fmt.Errorf("merge release PR: %w", err)
	}

	slog.InfoContext(ctx, "merged release PR", slog.String("url", result.PullRequest.URL))

	releaseInfos, err := w.publisher.ensureReleasesForResult(ctx, result, strings.TrimSpace(mergeSHA))
	if err != nil {
		return err
	}

	if err := w.publisher.markReleasePRTagged(ctx, result.PullRequest); err != nil {
		return err
	}

	result.Releases = append(result.Releases, releaseInfos...)

	return nil
}

func (w *releasePRWorkflow) updateExisting(
	ctx context.Context,
	existing *provider.PullRequest,
	releaseBranch string,
	prOpts provider.ReleasePROptions,
	commitSubject string,
	result *Result,
) (*provider.PullRequest, error) {
	slog.InfoContext(ctx, "updating existing release PR", slog.String("url", existing.URL))

	err := w.prs.UpdateReleasePR(ctx, existing.Number, prOpts)
	if err != nil {
		return nil, fmt.Errorf("update release PR: %w", err)
	}

	if err := w.branchUpdater.updateFiles(ctx, releaseBranch, result, commitSubject); err != nil {
		return nil, err
	}

	existing.Title = prOpts.Title
	existing.Body = prOpts.Body

	return existing, nil
}

func (w *releasePRWorkflow) createNew(
	ctx context.Context,
	releaseBranch string,
	prOpts provider.ReleasePROptions,
	commitSubject string,
	result *Result,
) (*provider.PullRequest, error) {
	if err := w.prs.PrepareReleasePRLabels(ctx, w.core.releasePRLabels()); err != nil {
		return nil, fmt.Errorf("prepare release PR labels: %w", err)
	}

	if err := w.branchUpdater.updateFiles(ctx, releaseBranch, result, commitSubject); err != nil {
		return nil, err
	}

	pr, err := w.prs.CreateReleasePR(ctx, prOpts)
	if err != nil {
		return nil, fmt.Errorf("create release PR: %w", err)
	}

	err = w.prs.MarkReleasePRPending(ctx, pr.Number, w.core.releasePRLabels())
	if err != nil {
		return nil, fmt.Errorf("mark release PR pending: %w", err)
	}

	slog.InfoContext(ctx, "created release PR", slog.String("url", pr.URL))

	return pr, nil
}
