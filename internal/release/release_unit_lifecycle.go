package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

type releaseUnitLifecycle struct {
	core          *releaseCore
	text          *releaseText
	forge         releaseForge
	branchUpdater *releaseBranchUpdater
	publisher     *releasePublisher
	changelogs    *changelogFileCache
	labels        labelLifecycle
}

type releaseUnitBatchOutcome struct {
	units    []releaseUnitOutcome
	releases []FinalizedRelease
}

type releaseUnitOutcome struct {
	unit        string
	plans       []TargetPlan
	text        *RenderedRelease
	pullRequest *forge.PullRequest
	releases    []FinalizedRelease
	err         error
}

func newReleaseUnitLifecycle(
	core *releaseCore,
	text *releaseText,
	source releaseSource,
	forge releaseForge,
	publisher *releasePublisher,
) *releaseUnitLifecycle {
	return &releaseUnitLifecycle{
		core:          core,
		text:          text,
		forge:         forge,
		branchUpdater: newReleaseBranchUpdater(core, source, forge),
		publisher:     publisher,
		changelogs:    newChangelogFileCache(),
		labels:        newLabelLifecycle(core.cfg, forge),
	}
}

func (l *releaseUnitLifecycle) finalize(
	ctx context.Context,
	units []releaseUnit,
) (releaseUnitBatchOutcome, error) {
	var outcome releaseUnitBatchOutcome

	var err error

	if l.core.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
		outcome, err = l.finalizeIndependent(ctx, units)
	} else {
		outcome, err = l.finalizeCombined(ctx, units)
	}

	if err != nil {
		return outcome, err
	}

	for _, release := range outcome.releases {
		slog.InfoContext(ctx, "finalized release",
			slog.String("tag", release.Release.TagName),
			slog.String("url", release.Release.URL),
		)
	}

	return outcome, nil
}

func (l *releaseUnitLifecycle) preview(
	ctx context.Context,
	units []releaseUnit,
) (releaseUnitBatchOutcome, error) {
	outcome, err := l.renderUnits(ctx, units)
	if err != nil {
		return outcome, err
	}

	l.logReleaseAnalysis(ctx, units)

	return outcome, nil
}

func (l *releaseUnitLifecycle) renderUnits(
	ctx context.Context,
	units []releaseUnit,
) (releaseUnitBatchOutcome, error) {
	outcome := releaseUnitBatchOutcome{
		units: make([]releaseUnitOutcome, 0, len(units)),
	}

	for _, unit := range units {
		text, err := l.render(ctx, unit.Plans, unit.ReleaseBranch, unit.ID)
		if err != nil {
			return releaseUnitBatchOutcome{}, err
		}

		outcome.units = append(outcome.units, releaseUnitOutcome{
			unit:  unit.ID,
			plans: slices.Clone(unit.Plans),
			text:  text,
		})
	}

	return outcome, nil
}

func (l *releaseUnitLifecycle) apply(
	ctx context.Context,
	units []releaseUnit,
) (releaseUnitBatchOutcome, error) {
	outcome, err := l.renderUnits(ctx, units)
	if err != nil {
		return outcome, err
	}

	err = l.preflight(ctx, units)
	if err != nil {
		return outcome, err
	}

	l.logReleaseAnalysis(ctx, units)

	reconciliationErr := l.reconcile(ctx, units, &outcome)
	autoMergeErr := l.autoMergeUnits(ctx, units, &outcome)

	return outcome, errors.Join(reconciliationErr, autoMergeErr)
}

func (l *releaseUnitLifecycle) logReleaseAnalysis(ctx context.Context, units []releaseUnit) {
	targetCount := 0
	for _, unit := range units {
		targetCount += len(unit.Plans)
	}

	if targetCount == 0 {
		slog.InfoContext(ctx, "no releasable commits found")

		return
	}

	slog.InfoContext(ctx, "release analysis complete", slog.Int("targets", targetCount))
}

func (l *releaseUnitLifecycle) preflight(ctx context.Context, units []releaseUnit) error {
	err := l.validateNoIncompatibleOpenReleasePR(ctx)
	if err != nil {
		return err
	}

	return l.validateExistingUnitManifests(ctx, units)
}

func (l *releaseUnitLifecycle) validateNoIncompatibleOpenReleasePR(ctx context.Context) error {
	if l.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return nil
	}

	configuredUnits, err := configuredReleaseUnits(l.core)
	if err != nil {
		return err
	}

	pullRequests, err := l.forge.FindOpenPendingReleasePRsForBase(
		ctx,
		l.core.run.baseBranch,
		l.core.cfg.Release.Labels.Pending,
	)
	if err != nil {
		return fmt.Errorf("find incompatible release PRs: %w", err)
	}

	errList := make([]error, 0)
	currentBranches := make(map[string]struct{}, len(configuredUnits))

	for _, unit := range configuredUnits {
		if unit.ID != combinedReleaseUnitID {
			currentBranches[unit.ReleaseBranch] = struct{}{}
		}
	}

	for _, pullRequest := range pullRequests {
		if _, current := currentBranches[strings.TrimSpace(pullRequest.Branch)]; current {
			manifest, manifestErr := releaseManifestFromPullRequest(pullRequest)
			if manifestErr != nil {
				continue
			}

			manifestUnitID := strings.TrimSpace(manifest.Unit)
			if manifestUnitID != "" && slices.ContainsFunc(configuredUnits, func(unit releaseUnit) bool {
				return unit.ID != combinedReleaseUnitID && unit.ID == manifestUnitID
			}) {
				continue
			}
		}

		unit, _, matchErr := matchReleaseUnit(pullRequest, configuredUnits)
		if matchErr == nil {
			matchErr = fmt.Errorf("%w: stale release unit %q", errInvalidReleaseManifest, unit.ID)
		}

		errList = append(errList, fmt.Errorf(
			"%w: incompatible release PR #%d %s: %v, merge it under the old configuration or close or relabel it",
			ErrMultiplePendingReleasePRs,
			pullRequest.Number,
			pullRequest.URL,
			matchErr,
		))
	}

	return errors.Join(errList...)
}

func (l *releaseUnitLifecycle) validateExistingUnitManifests(
	ctx context.Context,
	units []releaseUnit,
) error {
	if l.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return nil
	}

	errList := make([]error, 0)

	for _, unit := range units {
		pullRequests, err := l.findPendingPRs(ctx, unit)
		if err != nil {
			errList = append(errList, l.unitError(unit.ID, "manifest preflight", err))

			continue
		}

		if len(pullRequests) != 1 {
			continue
		}

		manifest, err := releaseManifestFromPullRequest(pullRequests[0])
		if err == nil {
			_, err = l.core.validateReleaseManifest(pullRequests[0], manifest, unit)
		}

		if err != nil {
			errList = append(errList, l.unitError(unit.ID, "manifest preflight", err))
		}
	}

	return errors.Join(errList...)
}

func (l *releaseUnitLifecycle) reconcile(
	ctx context.Context,
	units []releaseUnit,
	outcome *releaseUnitBatchOutcome,
) error {
	errs := make([]error, 0)

	for index, unit := range units {
		if l.core.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
			slog.InfoContext(ctx, "reconciling release unit",
				slog.String("unit", unit.ID),
				slog.String("phase", "reconcile"),
			)
		}

		pullRequest, text, err := l.createOrUpdate(ctx, unit)
		unitOutcome := &outcome.units[index]
		unitOutcome.pullRequest = pullRequest
		unitOutcome.plans = slices.Clone(unit.Plans)

		if text != nil {
			unitOutcome.text = text
		}

		if err == nil {
			continue
		}

		unitErr := l.unitError(unit.ID, "reconciliation", err)
		unitOutcome.err = unitErr
		errs = append(errs, unitErr)
	}

	return errors.Join(errs...)
}

func (l *releaseUnitLifecycle) autoMergeUnits(
	ctx context.Context,
	units []releaseUnit,
	outcome *releaseUnitBatchOutcome,
) error {
	if !l.core.run.autoMerge.enabled {
		return nil
	}

	errs := make([]error, 0)

	for index, unit := range units {
		unitOutcome := &outcome.units[index]
		if unitOutcome.err != nil {
			continue
		}

		if l.core.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
			slog.InfoContext(ctx, "auto-merging release unit",
				slog.String("unit", unit.ID),
				slog.String("phase", "auto_merge"),
			)
		}

		published, err := l.autoMerge(
			ctx,
			unitOutcome.pullRequest,
			unit.Plans,
			unitOutcome.text.ReleaseNames,
		)
		unitOutcome.releases = append(unitOutcome.releases, published...)
		outcome.releases = append(outcome.releases, published...)

		if err == nil {
			continue
		}

		unitErr := l.unitError(unit.ID, "auto-merge", err)
		unitOutcome.err = unitErr
		errs = append(errs, unitErr)
	}

	return errors.Join(errs...)
}

func (l *releaseUnitLifecycle) finalizeCombined(
	ctx context.Context,
	units []releaseUnit,
) (releaseUnitBatchOutcome, error) {
	outcome := releaseUnitBatchOutcome{
		units: make([]releaseUnitOutcome, 0, len(units)),
	}
	errs := make([]error, 0)
	found := false

	for _, unit := range units {
		unitOutcome := releaseUnitOutcome{unit: unit.ID}

		releases, err := l.publisher.finalizeMergedReleasePR(ctx, unit)
		if errors.Is(err, forge.ErrNoPR) {
			outcome.units = append(outcome.units, unitOutcome)

			continue
		}

		found = true

		unitOutcome.releases = append(unitOutcome.releases, releases...)
		outcome.releases = append(outcome.releases, releases...)

		if err != nil {
			unitOutcome.err = err
			errs = append(errs, err)
		}

		outcome.units = append(outcome.units, unitOutcome)
	}

	if !found {
		return outcome, forge.ErrNoPR
	}

	return outcome, errors.Join(errs...)
}

func (l *releaseUnitLifecycle) finalizeIndependent(
	ctx context.Context,
	units []releaseUnit,
) (releaseUnitBatchOutcome, error) {
	outcome := releaseUnitBatchOutcome{
		units: make([]releaseUnitOutcome, len(units)),
	}
	branches := make([]string, 0, len(units))
	seenBranches := make(map[string]struct{}, len(units))

	for index, unit := range units {
		outcome.units[index].unit = unit.ID
		if _, seen := seenBranches[unit.ReleaseBranch]; seen {
			continue
		}

		seenBranches[unit.ReleaseBranch] = struct{}{}
		branches = append(branches, unit.ReleaseBranch)
	}

	pullRequests, err := l.forge.FindMergedReleasePRs(
		ctx,
		l.core.run.baseBranch,
		l.core.cfg.Release.Labels.Pending,
		branches...,
	)

	errList := make([]error, 0)
	if err != nil {
		errList = append(errList, fmt.Errorf("find independent merged release PRs: %w", err))
	}

	for _, pullRequest := range pullRequests {
		unit, unitIndex, matchErr := matchReleaseUnit(pullRequest, units)
		if matchErr != nil {
			errList = append(errList, matchErr)

			continue
		}

		slog.InfoContext(ctx, "finalizing release unit",
			slog.String("unit", unit.ID),
			slog.String("phase", "finalize"),
		)

		releases, finalizeErr := l.publisher.finalizeMergedPullRequest(ctx, pullRequest, unit)
		outcome.releases = append(outcome.releases, releases...)
		unitOutcome := &outcome.units[unitIndex]
		unitOutcome.releases = append(unitOutcome.releases, releases...)

		if finalizeErr == nil {
			continue
		}

		unitErr := l.unitError(unit.ID, "finalization", finalizeErr)
		unitOutcome.err = errors.Join(unitOutcome.err, unitErr)
		errList = append(errList, unitErr)
	}

	return outcome, errors.Join(errList...)
}

func (l *releaseUnitLifecycle) unitError(unitID, phase string, err error) error {
	if l.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return err
	}

	return fmt.Errorf("release unit %q %s: %w", unitID, phase, err)
}

func matchReleaseUnit(
	pullRequest *forge.PullRequest,
	units []releaseUnit,
) (releaseUnit, int, error) {
	manifest, err := releaseManifestFromPullRequest(pullRequest)
	if err != nil {
		return releaseUnit{}, -1, err
	}

	unitID := strings.TrimSpace(manifest.Unit)
	if unitID == "" {
		unitID = combinedReleaseUnitID
	}

	for index, unit := range units {
		if unit.ID == unitID && unit.ReleaseBranch == strings.TrimSpace(pullRequest.Branch) {
			return unit, index, nil
		}
	}

	return releaseUnit{}, -1, fmt.Errorf(
		"%w: merged pull request branch %q and unit %q do not match the configured release layout",
		errInvalidReleaseManifest,
		pullRequest.Branch,
		unitID,
	)
}

func multiplePendingReleasePRError(pendingPRs []*forge.PullRequest) error {
	prReferences := make([]string, 0, len(pendingPRs))

	for _, pendingPR := range pendingPRs {
		prReferences = append(prReferences, fmt.Sprintf("#%d %s", pendingPR.Number, pendingPR.URL))
	}

	return fmt.Errorf("%w: %s", ErrMultiplePendingReleasePRs, strings.Join(prReferences, ", "))
}

func (l *releaseUnitLifecycle) createOrUpdate(
	ctx context.Context,
	unit releaseUnit,
) (*forge.PullRequest, *RenderedRelease, error) {
	plans := unit.Plans

	pendingPRs, err := l.findPendingPRs(ctx, unit)
	if err != nil {
		return nil, nil, err
	}

	if len(pendingPRs) > 1 {
		return nil, nil, multiplePendingReleasePRError(pendingPRs)
	}

	if len(pendingPRs) == 1 {
		return l.refreshExisting(ctx, pendingPRs[0], unit)
	}

	releaseBranch := unit.ReleaseBranch

	rendered, err := l.render(ctx, plans, releaseBranch, unit.ID)
	if err != nil {
		return nil, nil, err
	}

	pullRequest, err := l.createNew(
		ctx,
		releaseBranch,
		rendered.PROptions,
		rendered.CommitSubject,
		plans,
	)

	return pullRequest, rendered, err
}

func (l *releaseUnitLifecycle) findPendingPRs(
	ctx context.Context,
	unit releaseUnit,
) ([]*forge.PullRequest, error) {
	r := l.core

	if r.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
		pullRequests, err := l.forge.FindOpenPendingReleasePRs(
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

	pullRequests, err := l.forge.FindOpenPendingReleasePRs(
		ctx,
		r.run.baseBranch,
		r.cfg.Release.Labels.Pending,
	)
	if err != nil {
		return nil, fmt.Errorf("find pending release PRs: %w", err)
	}

	return pullRequests, nil
}

func (l *releaseUnitLifecycle) refreshExisting(
	ctx context.Context,
	existing *forge.PullRequest,
	unit releaseUnit,
) (*forge.PullRequest, *RenderedRelease, error) {
	if l.core.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
		manifest, err := releaseManifestFromPullRequest(existing)
		if err != nil {
			return nil, nil, err
		}

		_, err = l.core.validateReleaseManifest(existing, manifest, unit)
		if err != nil {
			return nil, nil, err
		}
	}

	err := l.adoptUnlabeledReleasePR(ctx, existing)
	if err != nil {
		return nil, nil, err
	}

	err = l.preserveExistingChangelogEdits(ctx, existing, unit.Plans)
	if err != nil {
		return nil, nil, err
	}

	rendered, err := l.render(ctx, unit.Plans, existing.Branch, unit.ID)
	if err != nil {
		return nil, nil, err
	}

	pullRequest, err := l.updateExisting(
		ctx,
		existing,
		existing.Branch,
		rendered.PROptions,
		rendered.CommitSubject,
		unit.Plans,
	)

	return pullRequest, rendered, err
}

func (l *releaseUnitLifecycle) render(
	ctx context.Context,
	plans []TargetPlan,
	releaseBranch string,
	unitID string,
) (*RenderedRelease, error) {
	rendered, err := l.text.render(plans, releaseBranch, l.forge.MaxPRBodyLength(), unitID)
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
func (l *releaseUnitLifecycle) adoptUnlabeledReleasePR(ctx context.Context, existing *forge.PullRequest) error {
	if !existing.NeedsPendingLabel {
		return nil
	}

	slog.InfoContext(ctx, "adopting unlabelled release PR", slog.String("url", existing.URL))

	err := l.labels.opened(ctx, existing.Number)
	if err != nil {
		return err
	}

	existing.NeedsPendingLabel = false

	return nil
}

func (l *releaseUnitLifecycle) preserveExistingChangelogEdits(
	ctx context.Context,
	existing *forge.PullRequest,
	plans []TargetPlan,
) error {
	r := l.core
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

		edits, found, err := l.preserveTargetChangelogEdits(
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
func (l *releaseUnitLifecycle) preserveTargetChangelogEdits(
	ctx context.Context,
	branch, changelogFile, previousTag string,
	plan TargetPlan,
) (changelogEdits, bool, error) {
	existingChangelog, err := l.releaseBranchChangelog(ctx, branch, changelogFile)
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

func (l *releaseUnitLifecycle) releaseBranchChangelog(ctx context.Context, branch, path string) (string, error) {
	return l.changelogs.get(branch, path, func() (string, error) {
		content, err := l.forge.GetFile(ctx, branch, path)
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

func (l *releaseUnitLifecycle) autoMerge(
	ctx context.Context,
	pullRequest *forge.PullRequest,
	plans []TargetPlan,
	releaseNames map[string]string,
) ([]FinalizedRelease, error) {
	r := l.core

	if !r.run.autoMerge.enabled || pullRequest == nil {
		return nil, nil
	}

	mergeOptions := forge.MergeReleasePROptions{
		BypassMergeChecks: r.run.autoMerge.force,
		BaseBranch:        r.run.baseBranch,
		Method:            forge.MergeMethod(r.run.autoMerge.method),
		ReleaseBranch:     pullRequest.Branch,
	}

	err := l.publisher.preflightReleasePRTagging(ctx)
	if err != nil {
		return nil, err
	}

	mergeSHA, err := l.forge.MergeReleasePR(ctx, pullRequest.Number, mergeOptions)
	if err != nil {
		if mergeOptions.BypassMergeChecks {
			return nil, fmt.Errorf("force merge release PR: %w", err)
		}

		return nil, fmt.Errorf("merge release PR: %w", err)
	}

	slog.InfoContext(ctx, "merged release PR", slog.String("url", pullRequest.URL))

	releases, err := l.publisher.ensureReleasesForPlans(ctx, plans, releaseNames, strings.TrimSpace(mergeSHA))
	if err != nil {
		return releases, err
	}

	err = l.publisher.markReleasePRTagged(ctx, pullRequest)
	if err != nil {
		return releases, err
	}

	return releases, nil
}

func (l *releaseUnitLifecycle) updateExisting(
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
	err := l.branchUpdater.updateFiles(ctx, releaseBranch, plans, commitSubject)
	if err != nil {
		return nil, err
	}

	err = l.forge.UpdateReleasePR(ctx, existing.Number, prOpts)
	if err != nil {
		return nil, fmt.Errorf("update release PR: %w", err)
	}

	existing.Title = prOpts.Title
	existing.Body = prOpts.Body

	return existing, nil
}

func (l *releaseUnitLifecycle) createNew(
	ctx context.Context,
	releaseBranch string,
	prOpts forge.ReleasePROptions,
	commitSubject string,
	plans []TargetPlan,
) (*forge.PullRequest, error) {
	err := l.branchUpdater.updateFiles(ctx, releaseBranch, plans, commitSubject)
	if err != nil {
		return nil, err
	}

	pr, err := l.forge.CreateReleasePR(ctx, prOpts)
	if err != nil {
		return nil, fmt.Errorf("create release PR: %w", err)
	}

	err = l.labels.opened(ctx, pr.Number)
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "created release PR", slog.String("url", pr.URL))

	return pr, nil
}
