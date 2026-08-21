package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
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
	Plans       []TargetPlan
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
	source    releaseSource
	prs       releasePRProvider
	files     releaseFileProvider
	publisher releasePublishingProvider
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
	releaseBranch string,
) (*releaseCore, error) {
	return newReleaseCoreAt(ctx, cfg, metadata, releaseBranch, time.Now())
}

func newReleaseCoreAt(
	ctx context.Context,
	cfg *config.Config,
	metadata repoMetadataProvider,
	releaseBranch string,
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

	targets, err = targetsForActiveChannel(cfg, targets)
	if err != nil {
		return nil, err
	}

	titles, err := newReleaseTitleTemplates(cfg.Release)
	if err != nil {
		return nil, err
	}

	return &releaseCore{
		cfg:           cfg,
		targets:       targets,
		metadata:      metadata,
		titles:        titles,
		releaseBranch: releaseBranch,
		releaseTime:   now.In(location),
	}, nil
}

// newReleaser wires a validated source to the provider-side capabilities.
// History and base-branch files are served by the local-git source, while every
// provider-side capability comes from provider.
func newReleaser(core *releaseCore, provider releaseProvider, source releaseSource) (*releaser, error) {
	if source == nil {
		return nil, errNilHistorySource
	}

	return &releaser{
		core:      core,
		source:    source,
		prs:       provider,
		files:     provider,
		publisher: provider,
	}, nil
}

func targetsForActiveChannel(
	cfg *config.Config,
	targets map[string]config.ResolvedTarget,
) (map[string]config.ResolvedTarget, error) {
	channelName := strings.TrimSpace(cfg.ActiveChannel)
	if channelName == "" {
		return targets, nil
	}

	channel, exists := cfg.Release.Channels[channelName]
	if !exists {
		return nil, fmt.Errorf("%w: unknown active release channel %q", config.ErrInvalidConfig, channelName)
	}

	channelChangelogPath := ""

	if strings.TrimSpace(channel.ChangelogFile) != "" {
		normalizedPath, err := config.NormalizeRepoFilePath(channel.ChangelogFile)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: release.channels.%s.changelog_file %v",
				config.ErrInvalidConfig,
				channelName,
				err,
			)
		}

		channelChangelogPath = normalizedPath
	}

	channelTargets := make(map[string]config.ResolvedTarget, len(targets))
	for targetID, target := range targets {
		if !versionStrategyForResolvedTarget(target).strategy.SupportsPrerelease() {
			return nil, fmt.Errorf(
				"%w: prerelease channel %q supports semver targets only. Target %q uses %q",
				config.ErrInvalidConfig,
				channelName,
				targetID,
				target.Versioning,
			)
		}

		if channelChangelogPath != "" && len(targets) == 1 {
			target.Changelog.File = channelChangelogPath
		} else {
			target.Changelog.File = channelChangelogFile(target.Changelog.File, channelName)
		}

		channelTargets[targetID] = target
	}

	return channelTargets, nil
}

func channelChangelogFile(changelogFile string, channelName string) string {
	dir, file := path.Split(changelogFile)
	ext := path.Ext(file)

	base := strings.TrimSuffix(file, ext)
	if base == "" {
		return changelogFile
	}

	return dir + base + "." + channelName + ext
}

func (r *releaser) releaseTargets(ctx context.Context, dryRun bool, selection releaseSelection) (*Result, error) {
	plans, analysisErr := analyze(ctx, r.core, r.source, selection, nil)
	if analysisErr == nil {
		validationErr := r.validateRenderedReleaseText(plans)
		if validationErr != nil {
			return nil, validationErr
		}
	}

	if dryRun {
		if analysisErr != nil {
			return nil, analysisErr
		}

		r.logReleaseAnalysis(ctx, plans)

		return &Result{BaseBranch: r.core.cfg.Branch, Plans: plans}, nil
	}

	plans, finalized, err := r.finalizeAndRefreshReleaseAnalysis(ctx, selection, plans, analysisErr)
	if err != nil {
		return nil, err
	}

	r.logReleaseAnalysis(ctx, plans)

	pullRequest, published, err := r.publishReleaseWave(ctx, plans)
	if err != nil {
		return nil, err
	}

	return &Result{
		BaseBranch:  r.core.cfg.Branch,
		Plans:       plans,
		PullRequest: pullRequest,
		Releases:    append(finalized, published...),
	}, nil
}

func (r *releaser) publishReleaseWave(
	ctx context.Context,
	plans []TargetPlan,
) (*forge.PullRequest, []FinalizedRelease, error) {
	if len(plans) == 0 {
		return nil, nil, nil
	}

	workflow := newReleasePRWorkflow(r.core, r.source, r.prs, r.files, r.publisher)

	pullRequest, err := workflow.createOrUpdate(ctx, plans)
	if err != nil {
		return nil, nil, err
	}

	published, err := workflow.autoMerge(ctx, pullRequest, plans)
	if err != nil {
		return nil, nil, err
	}

	return pullRequest, published, nil
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

	err = r.validateRenderedReleaseText(plans)
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

func (r *releaser) validateRenderedReleaseText(plans []TargetPlan) error {
	if len(plans) == 0 {
		return nil
	}

	_, err := r.core.releasePRTitle(plans)
	if err != nil {
		return err
	}

	_, err = r.core.releaseCommitSubject(plans)
	if err != nil {
		return err
	}

	for _, plan := range plans {
		_, err = r.core.releaseNameForPlan(plan)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *releaser) logReleaseAnalysis(ctx context.Context, plans []TargetPlan) {
	if len(plans) == 0 {
		slog.InfoContext(ctx, "no releasable commits found")

		return
	}

	slog.InfoContext(ctx, "release analysis complete", slog.Int("targets", len(plans)))
}

func (r *releaser) finalizeMergedReleasePRs(ctx context.Context) ([]FinalizedRelease, error) {
	return newReleasePublisher(r.core, r.publisher, r.source).finalizeMergedReleasePR(ctx)
}

func multiplePendingReleasePRError(pendingPRs []*forge.PullRequest) error {
	prReferences := make([]string, 0, len(pendingPRs))

	for _, pendingPR := range pendingPRs {
		prReferences = append(prReferences, fmt.Sprintf("#%d %s", pendingPR.Number, pendingPR.URL))
	}

	return fmt.Errorf("%w: %s", ErrMultiplePendingReleasePRs, strings.Join(prReferences, ", "))
}
