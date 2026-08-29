package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

type releasePRWorkflow struct {
	core          *releaseCore
	text          *releaseText
	prs           releasePRProvider
	files         releaseFileProvider
	branchUpdater *releaseBranchUpdater
	publisher     *releasePublisher
	changelogs    *changelogFileCache
	labels        labelLifecycle
}

func newReleasePRWorkflow(
	core *releaseCore,
	text *releaseText,
	source releaseSource,
	forge releaseForge,
	publisher *releasePublisher,
) *releasePRWorkflow {
	return &releasePRWorkflow{
		core:          core,
		text:          text,
		prs:           forge,
		files:         forge,
		branchUpdater: newReleaseBranchUpdater(core, source, forge),
		publisher:     publisher,
		changelogs:    newChangelogFileCache(),
		labels:        newLabelLifecycle(core.cfg, forge),
	}
}

func (w *releasePRWorkflow) createOrUpdate(
	ctx context.Context,
	plans []TargetPlan,
	units ...releaseUnit,
) (*forge.PullRequest, *RenderedRelease, error) {
	unit := releaseUnit{ID: combinedReleaseUnitID, ReleaseBranch: w.core.run.releaseBranch, Plans: plans}

	if len(units) > 0 {
		unit = units[0]
		plans = unit.Plans
	}

	pendingPRs, err := w.findPendingPRs(ctx, unit)
	if err != nil {
		return nil, nil, err
	}

	if len(pendingPRs) > 1 {
		return nil, nil, multiplePendingReleasePRError(pendingPRs)
	}

	if len(pendingPRs) == 1 {
		return w.refreshExisting(ctx, pendingPRs[0], unit)
	}

	releaseBranch := unit.ReleaseBranch

	rendered, err := w.render(ctx, plans, releaseBranch, unit.ID)
	if err != nil {
		return nil, nil, err
	}

	pullRequest, err := w.createNew(
		ctx,
		releaseBranch,
		rendered.PROptions,
		rendered.CommitSubject,
		plans,
	)

	return pullRequest, rendered, err
}

func (w *releasePRWorkflow) findPendingPRs(
	ctx context.Context,
	unit releaseUnit,
) ([]*forge.PullRequest, error) {
	r := w.core

	if r.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
		pullRequests, err := w.prs.FindOpenPendingReleasePRs(
			ctx,
			r.run.baseBranch,
			r.cfg.Release.Labels.Pending,
			unit.ReleaseBranch,
		)
		if err != nil {
			return nil, fmt.Errorf("find pending release PRs: %w", err)
		}

		return pullRequests, nil
	}

	pullRequests, err := w.prs.FindOpenPendingReleasePRs(
		ctx,
		r.run.baseBranch,
		r.cfg.Release.Labels.Pending,
	)
	if err != nil {
		return nil, fmt.Errorf("find pending release PRs: %w", err)
	}

	return pullRequests, nil
}

func (w *releasePRWorkflow) refreshExisting(
	ctx context.Context,
	existing *forge.PullRequest,
	unit releaseUnit,
) (*forge.PullRequest, *RenderedRelease, error) {
	if w.core.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
		manifest, err := releaseManifestFromPullRequest(existing)
		if err != nil {
			return nil, nil, err
		}

		_, err = w.core.validateReleaseManifest(existing, manifest, unit)
		if err != nil {
			return nil, nil, err
		}
	}

	err := w.adoptUnlabeledReleasePR(ctx, existing)
	if err != nil {
		return nil, nil, err
	}

	err = w.preserveExistingChangelogEdits(ctx, existing, unit.Plans)
	if err != nil {
		return nil, nil, err
	}

	rendered, err := w.render(ctx, unit.Plans, existing.Branch, unit.ID)
	if err != nil {
		return nil, nil, err
	}

	pullRequest, err := w.updateExisting(
		ctx,
		existing,
		existing.Branch,
		rendered.PROptions,
		rendered.CommitSubject,
		unit.Plans,
	)

	return pullRequest, rendered, err
}

func (w *releasePRWorkflow) render(
	ctx context.Context,
	plans []TargetPlan,
	releaseBranch string,
	unitIDs ...string,
) (*RenderedRelease, error) {
	rendered, err := w.text.render(plans, releaseBranch, w.prs.MaxPRBodyLength(), unitIDs...)
	if err != nil {
		return nil, err
	}

	if rendered.NotesOmitted {
		slog.WarnContext(ctx, "omitted release notes from PR body to fit provider limit",
			slog.Int("limit", rendered.bodyLimit),
			slog.Int("body_length", utf8.RuneCountInString(rendered.PROptions.Body)),
		)
	}

	return rendered, nil
}

// adoptUnlabeledReleasePR recovers a release PR that was created but never
// labelled, which happens when a run is interrupted between CreateReleasePR and
// MarkReleasePRPending.
func (w *releasePRWorkflow) adoptUnlabeledReleasePR(ctx context.Context, existing *forge.PullRequest) error {
	if !existing.NeedsPendingLabel {
		return nil
	}

	slog.InfoContext(ctx, "adopting unlabelled release PR", slog.String("url", existing.URL))

	err := w.labels.opened(ctx, existing.Number)
	if err != nil {
		return err
	}

	existing.NeedsPendingLabel = false

	return nil
}

func (w *releasePRWorkflow) preserveExistingChangelogEdits(
	ctx context.Context,
	existing *forge.PullRequest,
	plans []TargetPlan,
) error {
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

	for idx := range plans {
		plan := plans[idx]

		target, exists := r.targets[plan.ID]
		if !exists {
			return fmt.Errorf("%w: %s", errUnknownTarget, plan.ID)
		}

		changelogFile := target.Changelog.File
		if previous := strings.TrimSpace(previousChangelogFiles[plan.ID]); previous != "" {
			changelogFile = previous
		}

		edits, found, err := w.preserveTargetChangelogEdits(
			ctx,
			existing.Branch,
			changelogFile,
			previousTags[plan.ID],
			plan,
		)
		if err != nil {
			return err
		}

		if found {
			plans[idx].Entry = edits.Entry
			plans[idx].PREntry = edits.PREntry
		}
	}

	return nil
}

type changelogEdits struct {
	Entry   changelog.Entry
	PREntry changelog.Entry
}

// preserveTargetChangelogEdits merges whatever a human already wrote into the
// release branch's changelog back over a plan's generated entries. It reports
// false when the branch holds nothing to carry forward.
func (w *releasePRWorkflow) preserveTargetChangelogEdits(
	ctx context.Context,
	branch, changelogFile, previousTag string,
	plan TargetPlan,
) (changelogEdits, bool, error) {
	existingChangelog, err := w.releaseBranchChangelog(ctx, branch, changelogFile)
	if err != nil {
		if errors.Is(err, forge.ErrFileNotFound) {
			return changelogEdits{}, false, nil
		}

		return changelogEdits{}, false, err
	}

	existingEntry, found, err := changelogEntryForRefresh(existingChangelog, plan.NextTag, previousTag, plan.previousRef)
	if err != nil {
		return changelogEdits{}, false, err
	}

	if !found {
		return changelogEdits{}, false, nil
	}

	foreign := changelog.ParseEntry(existingEntry)

	return changelogEdits{
		Entry:   changelog.Merge(plan.Entry, foreign),
		PREntry: changelog.Merge(plan.PREntry, foreign),
	}, true, nil
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
	entry, err := changelog.EntryByTag(changelogBody, nextTag)
	if err == nil {
		return entry, true, nil
	}

	if !errors.Is(err, changelog.ErrEntryNotFound) {
		return "", false, fmt.Errorf("read changelog entry for %s: %w", nextTag, err)
	}

	previousTag = strings.TrimSpace(previousTag)
	if previousTag == "" || previousTag == nextTag || previousTag == strings.TrimSpace(releasedRef) {
		return "", false, nil
	}

	entry, err = changelog.EntryByTag(changelogBody, previousTag)
	if err == nil {
		return entry, true, nil
	}

	if errors.Is(err, changelog.ErrEntryNotFound) {
		return "", false, nil
	}

	return "", false, fmt.Errorf("read changelog entry for %s: %w", previousTag, err)
}

func (w *releasePRWorkflow) autoMerge(
	ctx context.Context,
	pullRequest *forge.PullRequest,
	plans []TargetPlan,
	releaseNames map[string]string,
) ([]FinalizedRelease, error) {
	r := w.core

	if !r.run.autoMerge.enabled || pullRequest == nil {
		return nil, nil
	}

	mergeOptions := forge.MergeReleasePROptions{
		BypassMergeChecks: r.run.autoMerge.force,
		Method:            forge.MergeMethod(r.run.autoMerge.method),
		ReleaseBranch:     pullRequest.Branch,
	}

	err := w.publisher.preflightReleasePRTagging(ctx)
	if err != nil {
		return nil, err
	}

	mergeSHA, err := w.prs.MergeReleasePR(ctx, pullRequest.Number, mergeOptions)
	if err != nil {
		if mergeOptions.BypassMergeChecks {
			return nil, fmt.Errorf("force merge release PR: %w", err)
		}

		return nil, fmt.Errorf("merge release PR: %w", err)
	}

	slog.InfoContext(ctx, "merged release PR", slog.String("url", pullRequest.URL))

	releases, err := w.publisher.ensureReleasesForPlans(ctx, plans, releaseNames, strings.TrimSpace(mergeSHA))
	if err != nil {
		return releases, err
	}

	err = w.publisher.markReleasePRTagged(ctx, pullRequest)
	if err != nil {
		return releases, err
	}

	return releases, nil
}

func (w *releasePRWorkflow) updateExisting(
	ctx context.Context,
	existing *forge.PullRequest,
	releaseBranch string,
	prOpts forge.ReleasePROptions,
	commitSubject string,
	plans []TargetPlan,
) (*forge.PullRequest, error) {
	slog.InfoContext(ctx, "updating existing release PR", slog.String("url", existing.URL))

	// The branch is written before the body, because the manifest marker in the
	// body is authoritative for finalization. A failure between the two then
	// leaves newer content under an older manifest, so a merge inside the window
	// publishes the older tag and the next run re-plans the newer version from
	// it. The reverse order advertises tags and files the branch does not carry,
	// which does not self-heal.
	err := w.branchUpdater.updateFiles(ctx, releaseBranch, plans, commitSubject)
	if err != nil {
		return nil, err
	}

	err = w.prs.UpdateReleasePR(ctx, existing.Number, prOpts)
	if err != nil {
		return nil, fmt.Errorf("update release PR: %w", err)
	}

	existing.Title = prOpts.Title
	existing.Body = prOpts.Body

	return existing, nil
}

func (w *releasePRWorkflow) createNew(
	ctx context.Context,
	releaseBranch string,
	prOpts forge.ReleasePROptions,
	commitSubject string,
	plans []TargetPlan,
) (*forge.PullRequest, error) {
	err := w.branchUpdater.updateFiles(ctx, releaseBranch, plans, commitSubject)
	if err != nil {
		return nil, err
	}

	pr, err := w.prs.CreateReleasePR(ctx, prOpts)
	if err != nil {
		return nil, fmt.Errorf("create release PR: %w", err)
	}

	err = w.labels.opened(ctx, pr.Number)
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "created release PR", slog.String("url", pr.URL))

	return pr, nil
}
