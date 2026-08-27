package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	BaseBranch  string
	Provider    config.ProviderType
	Plans       []TargetPlan
	Text        *RenderedRelease
	PullRequest *forge.PullRequest
	Releases    []FinalizedRelease
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
	plans, analysisErr := analyze(ctx, r.core, r.source, selection, nil)
	if dryRun {
		if analysisErr != nil {
			return nil, analysisErr
		}

		text, err := r.text.render(plans, r.core.run.releaseBranch, r.forge.MaxPRBodyLength())
		if err != nil {
			return nil, err
		}

		r.logReleaseAnalysis(ctx, plans)

		return &Result{BaseBranch: r.core.run.baseBranch, Plans: plans, Text: text}, nil
	}

	if analysisErr == nil {
		validationErr := r.text.validate(plans, r.forge.MaxPRBodyLength())
		if validationErr != nil {
			return nil, validationErr
		}
	}

	plans, finalized, err := r.finalizeAndRefreshReleaseAnalysis(ctx, selection, plans, analysisErr)
	if err != nil {
		return nil, err
	}

	r.logReleaseAnalysis(ctx, plans)

	pullRequest, text, published, err := r.publishReleaseWave(ctx, plans)
	if err != nil {
		return nil, err
	}

	return &Result{
		BaseBranch:  r.core.run.baseBranch,
		Plans:       plans,
		Text:        text,
		PullRequest: pullRequest,
		Releases:    append(finalized, published...),
	}, nil
}

func (r *releaser) publishReleaseWave(
	ctx context.Context,
	plans []TargetPlan,
) (*forge.PullRequest, *RenderedRelease, []FinalizedRelease, error) {
	if len(plans) == 0 {
		return nil, nil, nil, nil
	}

	workflow := newReleasePRWorkflow(r.core, r.text, r.source, r.forge, r.publisher)

	pullRequest, text, err := workflow.createOrUpdate(ctx, plans)
	if err != nil {
		return nil, nil, nil, err
	}

	published, err := workflow.autoMerge(ctx, pullRequest, plans, text.ReleaseNames)
	if err != nil {
		return nil, nil, nil, err
	}

	return pullRequest, text, published, nil
}

func (r *releaser) finalizeAndRefreshReleaseAnalysis(
	ctx context.Context,
	selection releaseSelection,
	plans []TargetPlan,
	analysisErr error,
) ([]TargetPlan, []FinalizedRelease, error) {
	finalized, err := r.finalizeMergedReleasePRs(ctx)
	if errors.Is(err, forge.ErrNoPR) {
		if analysisErr != nil {
			return nil, nil, analysisErr
		}

		return plans, nil, nil
	}

	if err != nil {
		return nil, nil, errors.Join(analysisErr, err)
	}

	plans, err = analyze(ctx, r.core, r.source, selection, publishedTagRefs(finalized))
	if err != nil {
		return nil, nil, err
	}

	err = r.text.validate(plans, r.forge.MaxPRBodyLength())
	if err != nil {
		return nil, nil, err
	}

	for _, finalizedRelease := range finalized {
		slog.InfoContext(ctx, "finalized release",
			slog.String("tag", finalizedRelease.Release.TagName),
			slog.String("url", finalizedRelease.Release.URL),
		)
	}

	return plans, finalized, nil
}

func (r *releaser) logReleaseAnalysis(ctx context.Context, plans []TargetPlan) {
	if len(plans) == 0 {
		slog.InfoContext(ctx, "no releasable commits found")

		return
	}

	slog.InfoContext(ctx, "release analysis complete", slog.Int("targets", len(plans)))
}

func (r *releaser) finalizeMergedReleasePRs(ctx context.Context) ([]FinalizedRelease, error) {
	return r.publisher.finalizeMergedReleasePR(ctx)
}

func multiplePendingReleasePRError(pendingPRs []*forge.PullRequest) error {
	prReferences := make([]string, 0, len(pendingPRs))

	for _, pendingPR := range pendingPRs {
		prReferences = append(prReferences, fmt.Sprintf("#%d %s", pendingPR.Number, pendingPR.URL))
	}

	return fmt.Errorf("%w: %s", ErrMultiplePendingReleasePRs, strings.Join(prReferences, ", "))
}
