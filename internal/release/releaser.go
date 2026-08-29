package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/version"
)

var (
	errNilHistorySource          = errors.New("history source is required")
	errInvalidReleaseAs          = version.ErrInvalidReleaseAs
	errConflictingReleaseAs      = version.ErrConflictingReleaseAs
	ErrMultiplePendingReleasePRs = errors.New("multiple pending release PRs found")
	errUnknownTarget             = errors.New("unknown target")
	errConflictingFileUpdate     = errors.New("conflicting file update")
)

type Result struct {
	BaseBranch      string
	Provider        config.ProviderType
	PullRequestMode config.PullRequestMode
	Plans           []TargetPlan
	Units           []UnitResult
	Text            *RenderedRelease
	PullRequest     *forge.PullRequest
	Releases        []FinalizedRelease
}

type UnitResult struct {
	Unit        string
	Plans       []TargetPlan
	Text        *RenderedRelease
	PullRequest *forge.PullRequest
	Releases    []FinalizedRelease
	Error       error
}

// FinalizedRelease pairs a published release with the target it belongs to and
// the commit it was cut from, neither of which the forge's release object
// carries.
type FinalizedRelease struct {
	TargetID  string
	CommitSHA string
	Release   *forge.Release
}

type TargetPlan struct {
	ID              string
	Type            config.TargetType
	CurrentVersion  string
	NextVersion     string
	NextTag         string
	BumpType        commit.BumpType
	CommitCount     int
	Entry           changelog.Entry
	PREntry         changelog.Entry
	PRCompareRef    string
	ChangelogFile   string
	IncludedTargets []string
	commitHashes    []string
	previousRef     string
}

type releaser struct {
	core      *releaseCore
	text      *releaseText
	source    releaseSource
	forge     releaseForge
	publisher *releasePublisher
}

type versionStrategy struct {
	strategy version.Strategy
	prefix   string
}

// newReleaseCore resolves everything a run can determine from configuration
// alone, so target selection can be checked before any source is opened.
func newReleaseCore(
	ctx context.Context,
	cfg *config.Config,
	metadata repoMetadataProvider,
	run releaseRun,
) (*releaseCore, error) {
	return newReleaseCoreAt(ctx, cfg, metadata, run, time.Now())
}

func newReleaseCoreAt(
	ctx context.Context,
	cfg *config.Config,
	metadata repoMetadataProvider,
	run releaseRun,
	now time.Time,
) (*releaseCore, error) {
	location, err := cfg.TimeLocation()
	if err != nil {
		return nil, fmt.Errorf("resolve release timezone: %w", err)
	}

	targets, err := cfg.ResolvedTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve release targets: %w", err)
	}

	targets, err = run.withChannelChangelogs(targets)
	if err != nil {
		return nil, err
	}

	return &releaseCore{
		cfg:         cfg,
		run:         run,
		targets:     targets,
		metadata:    metadata,
		releaseTime: now.In(location),
	}, nil
}

func newReleaser(
	core *releaseCore,
	source releaseSource,
	forge releaseForge,
) (*releaser, error) {
	if source == nil {
		return nil, errNilHistorySource
	}

	text, err := newReleaseText(core.cfg, core.run, core.targets)
	if err != nil {
		return nil, err
	}

	return &releaser{
		core:      core,
		text:      text,
		source:    source,
		forge:     forge,
		publisher: newReleasePublisher(core, text, forge, source),
	}, nil
}

func (r *releaser) releaseTargets(ctx context.Context, dryRun bool, selection releaseSelection) (*Result, error) {
	if dryRun {
		plans, analysisErr := analyze(ctx, r.core, r.source, selection, nil)

		return r.releaseDryRun(ctx, plans, analysisErr)
	}

	plans, finalized, finalizationOutcomes, failureResult, err := r.finalizeAndAnalyze(ctx, selection)
	if err != nil {
		return failureResult, err
	}

	units, err := planReleaseUnits(r.core, plans)
	if err != nil {
		return nil, err
	}

	result, err := r.renderUnitResult(plans, units)
	if err != nil {
		return nil, err
	}

	result.Releases = append(result.Releases, finalized...)
	r.mergeFinalizationOutcomes(result, finalizationOutcomes)

	err = r.validateNoIncompatibleOpenReleasePR(ctx)
	if err != nil {
		return result, err
	}

	err = r.validateExistingUnitManifests(ctx, units)
	if err != nil {
		return result, err
	}

	for _, finalizedRelease := range finalized {
		slog.InfoContext(ctx, "finalized release",
			slog.String("tag", finalizedRelease.Release.TagName),
			slog.String("url", finalizedRelease.Release.URL),
		)
	}

	r.logReleaseAnalysis(ctx, plans)

	reconciliationErr := r.reconcileUnits(ctx, units, result)
	autoMergeErr := r.autoMergeUnits(ctx, units, result)

	combinedErr := errors.Join(reconciliationErr, autoMergeErr)
	if combinedErr != nil && r.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return nil, combinedErr
	}

	return result, combinedErr
}

func (r *releaser) finalizeAndAnalyze(
	ctx context.Context,
	selection releaseSelection,
) ([]TargetPlan, []FinalizedRelease, []UnitResult, *Result, error) {
	finalized, outcomes, err := r.finalizeMergedReleaseUnits(ctx)
	if errors.Is(err, forge.ErrNoPR) {
		err = nil
	}

	if err != nil {
		result := r.resultForError(outcomes)
		if result != nil {
			result.Releases = append(result.Releases, finalized...)
		}

		return nil, nil, nil, result, err
	}

	plans, err := analyze(ctx, r.core, r.source, selection, publishedTagRefs(finalized))
	if err != nil {
		result := r.resultForError(outcomes)
		if result != nil {
			result.Releases = append(result.Releases, finalized...)
		}

		return nil, nil, nil, result, err
	}

	return plans, finalized, outcomes, nil, nil
}

func (r *releaser) validateNoIncompatibleOpenReleasePR(ctx context.Context) error {
	if r.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return nil
	}

	configuredUnits, err := configuredReleaseUnits(r.core)
	if err != nil {
		return err
	}

	pullRequests, err := r.forge.FindOpenPendingReleasePRsForBase(
		ctx,
		r.core.run.baseBranch,
		r.core.cfg.Release.Labels.Pending,
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

func (r *releaser) validateExistingUnitManifests(ctx context.Context, units []releaseUnit) error {
	if r.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return nil
	}

	workflow := newReleasePRWorkflow(r.core, r.text, r.source, r.forge, r.publisher)
	errList := make([]error, 0)

	for _, unit := range units {
		pullRequests, err := workflow.findPendingPRs(ctx, unit)
		if err != nil {
			errList = append(errList, r.unitError(unit.ID, "manifest preflight", err))

			continue
		}

		if len(pullRequests) != 1 {
			continue
		}

		manifest, err := releaseManifestFromPullRequest(pullRequests[0])
		if err == nil {
			_, err = r.core.validateReleaseManifest(pullRequests[0], manifest, unit)
		}

		if err != nil {
			errList = append(errList, r.unitError(unit.ID, "manifest preflight", err))
		}
	}

	return errors.Join(errList...)
}

func (r *releaser) releaseDryRun(
	ctx context.Context,
	plans []TargetPlan,
	analysisErr error,
) (*Result, error) {
	if analysisErr != nil {
		return nil, analysisErr
	}

	units, err := planReleaseUnits(r.core, plans)
	if err != nil {
		return nil, err
	}

	result, err := r.renderUnitResult(plans, units)
	if err != nil {
		return nil, err
	}

	r.logReleaseAnalysis(ctx, plans)

	return result, nil
}

func (r *releaser) renderUnitResult(plans []TargetPlan, units []releaseUnit) (*Result, error) {
	result := &Result{
		BaseBranch:      r.core.run.baseBranch,
		PullRequestMode: r.core.cfg.Release.PullRequestMode,
		Plans:           plans,
	}
	result.Units = make([]UnitResult, 0, len(units))

	for _, unit := range units {
		text, err := r.text.render(
			unit.Plans,
			unit.ReleaseBranch,
			r.forge.MaxPRBodyLength(),
			unit.ID,
		)
		if err != nil {
			return nil, err
		}

		result.Units = append(result.Units, UnitResult{
			Unit:  unit.ID,
			Plans: slices.Clone(unit.Plans),
			Text:  text,
		})
	}

	r.setLegacyResultFields(result)

	return result, nil
}

func (r *releaser) reconcileUnits(ctx context.Context, units []releaseUnit, result *Result) error {
	workflow := newReleasePRWorkflow(r.core, r.text, r.source, r.forge, r.publisher)
	errs := make([]error, 0)

	for index, unit := range units {
		if r.core.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
			slog.InfoContext(ctx, "reconciling release unit",
				slog.String("unit", unit.ID),
				slog.String("phase", "reconcile"),
			)
		}

		pullRequest, text, err := workflow.createOrUpdate(ctx, unit.Plans, unit)
		result.Units[index].PullRequest = pullRequest
		result.Units[index].Plans = slices.Clone(unit.Plans)
		r.replaceResultPlans(result, unit.Plans)

		if text != nil {
			result.Units[index].Text = text
		}

		if err == nil {
			continue
		}

		unitErr := r.unitError(unit.ID, "reconciliation", err)
		result.Units[index].Error = unitErr
		errs = append(errs, unitErr)
	}

	r.setLegacyResultFields(result)

	return errors.Join(errs...)
}

func (r *releaser) autoMergeUnits(ctx context.Context, units []releaseUnit, result *Result) error {
	if !r.core.run.autoMerge.enabled {
		return nil
	}

	workflow := newReleasePRWorkflow(r.core, r.text, r.source, r.forge, r.publisher)
	errs := make([]error, 0)

	for index, unit := range units {
		outcome := &result.Units[index]
		if outcome.Error != nil {
			continue
		}

		if r.core.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
			slog.InfoContext(ctx, "auto-merging release unit",
				slog.String("unit", unit.ID),
				slog.String("phase", "auto_merge"),
			)
		}

		published, err := workflow.autoMerge(
			ctx,
			outcome.PullRequest,
			unit.Plans,
			outcome.Text.ReleaseNames,
		)
		outcome.Releases = append(outcome.Releases, published...)
		result.Releases = append(result.Releases, published...)

		if err == nil {
			continue
		}

		unitErr := r.unitError(unit.ID, "auto-merge", err)
		outcome.Error = unitErr
		errs = append(errs, unitErr)
	}

	return errors.Join(errs...)
}

func (r *releaser) logReleaseAnalysis(ctx context.Context, plans []TargetPlan) {
	if len(plans) == 0 {
		slog.InfoContext(ctx, "no releasable commits found")

		return
	}

	slog.InfoContext(ctx, "release analysis complete", slog.Int("targets", len(plans)))
}

func (r *releaser) finalizeMergedReleasePRs(ctx context.Context) ([]FinalizedRelease, error) {
	finalized, _, err := r.finalizeMergedReleaseUnits(ctx)

	return finalized, err
}

func (r *releaser) finalizeMergedReleaseUnits(
	ctx context.Context,
) ([]FinalizedRelease, []UnitResult, error) {
	units, err := configuredReleaseUnits(r.core)
	if err != nil {
		return nil, nil, err
	}

	if r.core.cfg.Release.PullRequestMode == config.PullRequestModeIndependent {
		return r.finalizeIndependentReleaseUnits(ctx, units)
	}

	finalized := make([]FinalizedRelease, 0)
	outcomes := make([]UnitResult, 0, len(units))
	errs := make([]error, 0)
	found := false

	for _, unit := range units {
		releases, finalizeErr := r.publisher.finalizeMergedReleasePR(ctx, unit)
		if errors.Is(finalizeErr, forge.ErrNoPR) {
			outcomes = append(outcomes, UnitResult{Unit: unit.ID})

			continue
		}

		found = true

		if finalizeErr != nil {
			unitErr := r.unitError(unit.ID, "finalization", finalizeErr)
			outcomes = append(outcomes, UnitResult{Unit: unit.ID, Error: unitErr})
			errs = append(errs, unitErr)

			continue
		}

		finalized = append(finalized, releases...)
		outcomes = append(outcomes, UnitResult{Unit: unit.ID, Releases: releases})
	}

	if !found {
		return nil, outcomes, forge.ErrNoPR
	}

	return finalized, outcomes, errors.Join(errs...)
}

func (r *releaser) finalizeIndependentReleaseUnits(
	ctx context.Context,
	units []releaseUnit,
) ([]FinalizedRelease, []UnitResult, error) {
	outcomes := make([]UnitResult, len(units))
	branches := make([]string, 0, len(units))
	seenBranches := make(map[string]struct{}, len(units))

	for index, unit := range units {
		outcomes[index].Unit = unit.ID
		if _, seen := seenBranches[unit.ReleaseBranch]; seen {
			continue
		}

		seenBranches[unit.ReleaseBranch] = struct{}{}
		branches = append(branches, unit.ReleaseBranch)
	}

	pullRequests, err := r.forge.FindMergedReleasePRs(
		ctx,
		r.core.run.baseBranch,
		r.core.cfg.Release.Labels.Pending,
		branches...,
	)
	finalized := make([]FinalizedRelease, 0)

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

		releases, finalizeErr := r.publisher.finalizeMergedPullRequest(ctx, pullRequest, unit)
		finalized = append(finalized, releases...)
		outcomes[unitIndex].Releases = append(outcomes[unitIndex].Releases, releases...)

		if finalizeErr == nil {
			continue
		}

		unitErr := r.unitError(unit.ID, "finalization", finalizeErr)
		outcomes[unitIndex].Error = errors.Join(outcomes[unitIndex].Error, unitErr)
		errList = append(errList, unitErr)
	}

	return finalized, outcomes, errors.Join(errList...)
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

func (r *releaser) unitError(unitID, phase string, err error) error {
	if r.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return err
	}

	return fmt.Errorf("release unit %q %s: %w", unitID, phase, err)
}

func (r *releaser) replaceResultPlans(result *Result, plans []TargetPlan) {
	byID := make(map[string]TargetPlan, len(plans))
	for _, plan := range plans {
		byID[plan.ID] = plan
	}

	for index, plan := range result.Plans {
		if replacement, exists := byID[plan.ID]; exists {
			result.Plans[index] = replacement
		}
	}
}

func (r *releaser) setLegacyResultFields(result *Result) {
	if len(result.Units) != 1 {
		result.Text = nil
		result.PullRequest = nil

		return
	}

	result.Text = result.Units[0].Text
	result.PullRequest = result.Units[0].PullRequest
}

func (r *releaser) mergeFinalizationOutcomes(result *Result, finalized []UnitResult) {
	for _, outcome := range finalized {
		if len(outcome.Releases) == 0 && outcome.Error == nil {
			continue
		}

		matched := false

		for index := range result.Units {
			if result.Units[index].Unit != outcome.Unit {
				continue
			}

			result.Units[index].Releases = append(result.Units[index].Releases, outcome.Releases...)
			matched = true

			break
		}

		if !matched {
			result.Units = append(result.Units, outcome)
		}
	}
}

func (r *releaser) resultForError(units []UnitResult) *Result {
	if r.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return nil
	}

	return &Result{
		BaseBranch:      r.core.run.baseBranch,
		PullRequestMode: r.core.cfg.Release.PullRequestMode,
		Units:           units,
	}
}

func multiplePendingReleasePRError(pendingPRs []*forge.PullRequest) error {
	prReferences := make([]string, 0, len(pendingPRs))

	for _, pendingPR := range pendingPRs {
		prReferences = append(prReferences, fmt.Sprintf("#%d %s", pendingPR.Number, pendingPR.URL))
	}

	return fmt.Errorf("%w: %s", ErrMultiplePendingReleasePRs, strings.Join(prReferences, ", "))
}
