package release

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	source    releaseSource
	lifecycle *releaseUnitLifecycle
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

	publisher := newReleasePublisher(core, text, forge, source)

	return &releaser{
		core:      core,
		source:    source,
		lifecycle: newReleaseUnitLifecycle(core, text, source, forge, publisher),
	}, nil
}

func (r *releaser) releaseTargets(ctx context.Context, dryRun bool, selection releaseSelection) (*Result, error) {
	if dryRun {
		return r.releaseDryRun(ctx, selection)
	}

	configuredUnits, err := configuredReleaseUnits(r.core)
	if err != nil {
		return nil, err
	}

	finalization, err := r.lifecycle.finalize(ctx, configuredUnits)
	if errors.Is(err, forge.ErrNoPR) {
		err = nil
	}

	if err != nil {
		return r.resultForFinalizationError(finalization), err
	}

	plans, err := analyze(ctx, r.core, r.source, selection, publishedTagRefs(finalization.releases))
	if err != nil {
		return r.resultForFinalizationError(finalization), err
	}

	units, err := planReleaseUnits(r.core, plans)
	if err != nil {
		return nil, err
	}

	applied, err := r.lifecycle.apply(ctx, units)
	if err != nil && len(applied.units) == 0 {
		return nil, err
	}

	result := r.resultForBatches(plans, applied, finalization)
	if err != nil && r.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return nil, err
	}

	return result, err
}

func (r *releaser) releaseDryRun(
	ctx context.Context,
	selection releaseSelection,
) (*Result, error) {
	plans, err := analyze(ctx, r.core, r.source, selection, nil)
	if err != nil {
		return nil, err
	}

	units, err := planReleaseUnits(r.core, plans)
	if err != nil {
		return nil, err
	}

	preview, err := r.lifecycle.preview(ctx, units)
	if err != nil {
		return nil, err
	}

	return r.resultForBatches(plans, preview, releaseUnitBatchOutcome{}), nil
}

func (r *releaser) resultForBatches(
	plans []TargetPlan,
	applied releaseUnitBatchOutcome,
	finalized releaseUnitBatchOutcome,
) *Result {
	result := &Result{
		BaseBranch:      r.core.run.baseBranch,
		PullRequestMode: r.core.cfg.Release.PullRequestMode,
		Plans:           slices.Clone(plans),
		Units:           make([]UnitResult, 0, len(applied.units)),
	}

	for _, outcome := range applied.units {
		result.Units = append(result.Units, unitResultFromOutcome(outcome))
		r.replaceResultPlans(result, outcome.plans)
	}

	result.Releases = append(result.Releases, finalized.releases...)
	result.Releases = append(result.Releases, applied.releases...)
	r.mergeFinalizationOutcomes(result, finalized.units)
	r.setLegacyResultFields(result)

	return result
}

func unitResultFromOutcome(outcome releaseUnitOutcome) UnitResult {
	return UnitResult{
		Unit:        outcome.unit,
		Plans:       slices.Clone(outcome.plans),
		Text:        outcome.text,
		PullRequest: outcome.pullRequest,
		Releases:    slices.Clone(outcome.releases),
		Error:       outcome.err,
	}
}

func (r *releaser) resultForFinalizationError(outcome releaseUnitBatchOutcome) *Result {
	if r.core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return nil
	}

	result := &Result{
		BaseBranch:      r.core.run.baseBranch,
		PullRequestMode: r.core.cfg.Release.PullRequestMode,
		Units:           make([]UnitResult, 0, len(outcome.units)),
		Releases:        slices.Clone(outcome.releases),
	}

	for _, unit := range outcome.units {
		result.Units = append(result.Units, unitResultFromOutcome(unit))
	}

	return result
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

func (r *releaser) mergeFinalizationOutcomes(result *Result, finalized []releaseUnitOutcome) {
	for _, outcome := range finalized {
		if len(outcome.releases) == 0 && outcome.err == nil {
			continue
		}

		matched := false

		for index := range result.Units {
			if result.Units[index].Unit != outcome.unit {
				continue
			}

			result.Units[index].Releases = append(result.Units[index].Releases, outcome.releases...)
			result.Units[index].Error = errors.Join(result.Units[index].Error, outcome.err)
			matched = true

			break
		}

		if !matched {
			result.Units = append(result.Units, unitResultFromOutcome(outcome))
		}
	}
}
