package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/monkescience/yeet/internal/provider"
)

type releasePRWorkflow struct {
	core          *releaseCore
	prs           releasePRProvider
	files         releaseFileProvider
	branchUpdater *releaseBranchUpdater
	publisher     *releasePublisher
}

func newReleasePRWorkflow(
	core *releaseCore,
	prs releasePRProvider,
	files releaseFileProvider,
	publisher releasePublishingProvider,
) *releasePRWorkflow {
	return &releasePRWorkflow{
		core:          core,
		prs:           prs,
		files:         files,
		branchUpdater: newReleaseBranchUpdater(core, files),
		publisher:     newReleasePublisher(core, publisher),
	}
}

func (w *releasePRWorkflow) createOrUpdate(ctx context.Context, result *Result) (*provider.PullRequest, error) {
	r := w.core

	pendingPRs, err := w.prs.FindOpenPendingReleasePRs(ctx, r.cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("find pending release PRs: %w", err)
	}

	if len(pendingPRs) > 1 {
		return nil, multiplePendingReleasePRError(pendingPRs)
	}

	if len(pendingPRs) == 1 {
		existing := pendingPRs[0]

		if err := w.preserveExistingChangelogEdits(ctx, existing.Branch, result); err != nil {
			return nil, err
		}

		prOpts, prErr := r.releasePROptions(ctx, result, existing.Branch, w.prs.MaxPRBodyLength())
		if prErr != nil {
			return nil, prErr
		}

		return w.updateExisting(ctx, existing, existing.Branch, prOpts, result)
	}

	releaseBranch := stableReleaseBranch(r.cfg.Branch)

	prOpts, err := r.releasePROptions(ctx, result, releaseBranch, w.prs.MaxPRBodyLength())
	if err != nil {
		return nil, err
	}

	return w.createNew(ctx, releaseBranch, prOpts, result)
}

func (w *releasePRWorkflow) preserveExistingChangelogEdits(
	ctx context.Context,
	releaseBranch string,
	result *Result,
) error {
	if result == nil {
		return nil
	}

	r := w.core

	for idx := range result.Plans {
		plan := &result.Plans[idx]

		target, exists := r.targets[plan.ID]
		if !exists {
			return fmt.Errorf("%w: %s", ErrUnknownTarget, plan.ID)
		}

		existingChangelog, err := w.files.GetFile(ctx, releaseBranch, target.Changelog.File)
		if err != nil {
			if errors.Is(err, provider.ErrFileNotFound) {
				continue
			}

			return fmt.Errorf("get release branch changelog file %s: %w", target.Changelog.File, err)
		}

		existingEntry, err := changelogEntryByTag(existingChangelog, plan.NextTag)
		if err != nil {
			if errors.Is(err, ErrChangelogEntryNotFound) {
				continue
			}

			return err
		}

		plan.Changelog = preserveManualChangelogSections(plan.Changelog, existingEntry)
		if plan.PRChangelog != "" {
			plan.PRChangelog = preserveManualChangelogSections(plan.PRChangelog, existingEntry)
		}
	}

	return nil
}

func (w *releasePRWorkflow) autoMerge(ctx context.Context, result *Result) error {
	r := w.core

	autoMergeEnabled := r.cfg.Release.AutoMerge || r.cfg.Release.AutoMergeForce
	if !autoMergeEnabled || result.PullRequest == nil {
		return nil
	}

	mergeOptions := provider.MergeReleasePROptions{
		Force:  r.cfg.Release.AutoMergeForce,
		Method: provider.MergeMethod(r.cfg.Release.AutoMergeMethod),
	}

	err := w.prs.MergeReleasePR(ctx, result.PullRequest.Number, mergeOptions)
	if err != nil {
		if mergeOptions.Force {
			return fmt.Errorf("force merge release PR: %w", err)
		}

		return fmt.Errorf("merge release PR: %w", err)
	}

	slog.InfoContext(ctx, "merged release PR", slog.String("url", result.PullRequest.URL))

	releaseInfos, err := w.publisher.ensureReleasesForResult(ctx, result, r.cfg.Branch)
	if err != nil {
		return err
	}

	if err := w.publisher.markReleasePRTagged(ctx, result.PullRequest); err != nil {
		return err
	}

	result.Releases = releaseInfos

	return nil
}

func (w *releasePRWorkflow) updateExisting(
	ctx context.Context,
	existing *provider.PullRequest,
	releaseBranch string,
	prOpts provider.ReleasePROptions,
	result *Result,
) (*provider.PullRequest, error) {
	slog.InfoContext(ctx, "updating existing release PR", slog.String("url", existing.URL))

	err := w.prs.UpdateReleasePR(ctx, existing.Number, prOpts)
	if err != nil {
		return nil, fmt.Errorf("update release PR: %w", err)
	}

	if err := w.branchUpdater.updateFiles(ctx, releaseBranch, result); err != nil {
		return nil, err
	}

	err = w.prs.MarkReleasePRPending(ctx, existing.Number)
	if err != nil {
		return nil, fmt.Errorf("mark release PR pending: %w", err)
	}

	existing.Title = prOpts.Title
	existing.Body = prOpts.Body

	return existing, nil
}

func (w *releasePRWorkflow) createNew(
	ctx context.Context,
	releaseBranch string,
	prOpts provider.ReleasePROptions,
	result *Result,
) (*provider.PullRequest, error) {
	r := w.core

	err := w.prs.CreateBranch(ctx, releaseBranch, r.cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("create release branch: %w", err)
	}

	if err := w.branchUpdater.updateFiles(ctx, releaseBranch, result); err != nil {
		return nil, err
	}

	pr, err := w.prs.CreateReleasePR(ctx, prOpts)
	if err != nil {
		return nil, fmt.Errorf("create release PR: %w", err)
	}

	err = w.prs.MarkReleasePRPending(ctx, pr.Number)
	if err != nil {
		return nil, fmt.Errorf("mark release PR pending: %w", err)
	}

	slog.InfoContext(ctx, "created release PR", slog.String("url", pr.URL))

	return pr, nil
}
