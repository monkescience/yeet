package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/version"
)

var (
	errNilHistorySource          = errors.New("history source is required")
	errInvalidReleaseAs          = errors.New("invalid release-as footer")
	errConflictingReleaseAs      = errors.New("conflicting release-as footers")
	errChangelogEntryNotFound    = errors.New("changelog entry not found")
	ErrMultiplePendingReleasePRs = errors.New("multiple pending release PRs found")
	errUnknownTarget             = errors.New("unknown target")
	errConflictingFileUpdate     = errors.New("conflicting file update")
)

type Result struct {
	BaseBranch  string
	Plans       []TargetPlan
	PullRequest *provider.PullRequest
	Releases    []*provider.Release
}

type TargetPlan struct {
	ID              string
	Type            config.TargetType
	CurrentVersion  string
	NextVersion     string
	NextTag         string
	BumpType        commit.BumpType
	CommitCount     int
	Changelog       string
	PRChangelog     string
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

// newReleaser constructs a releaser. History and base-branch files are served by the
// local-git source, while every provider-side capability comes from deps.
func newReleaser(
	ctx context.Context,
	cfg *config.Config,
	deps releaserDependencies,
	source releaseSource,
) (*releaser, error) {
	if source == nil {
		return nil, errNilHistorySource
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

	return &releaser{
		core:      &releaseCore{cfg: cfg, targets: targets, metadata: deps, titles: titles},
		source:    source,
		prs:       deps,
		files:     deps,
		publisher: deps,
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

	channelTargets := make(map[string]config.ResolvedTarget, len(targets))
	for targetID, target := range targets {
		if target.Versioning != config.VersioningSemver {
			return nil, fmt.Errorf(
				"%w: prerelease channel %q supports semver targets only. Target %q uses %q",
				config.ErrInvalidConfig,
				channelName,
				targetID,
				target.Versioning,
			)
		}

		if strings.TrimSpace(channel.ChangelogFile) != "" && len(targets) == 1 {
			target.Changelog.File = strings.TrimSpace(channel.ChangelogFile)
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

// validateTargets checks target selection without reading history or mutating
// provider state.
func (r *releaser) validateTargets(selectedTargetIDs []string) error {
	_, err := newReleaseAnalyzer(r.core, r.source).selectTargets(selectedTargetIDs)

	return err
}

func (r *releaser) releaseTargets(ctx context.Context, dryRun bool, selectedTargetIDs []string) (*Result, error) {
	analyzer := newReleaseAnalyzer(r.core, r.source)

	selection, err := analyzer.selectTargets(selectedTargetIDs)
	if err != nil {
		return nil, err
	}

	result, analysisErr := analyzer.analyze(ctx, selection)
	if analysisErr == nil {
		if err := r.validateRenderedReleaseTitles(result); err != nil {
			return nil, err
		}
	}

	if dryRun {
		if analysisErr != nil {
			return nil, analysisErr
		}

		r.logReleaseAnalysis(ctx, result)

		return result, nil
	}

	result, err = r.finalizeAndRefreshReleaseAnalysis(ctx, selection, result, analysisErr)
	if err != nil {
		return nil, err
	}

	r.logReleaseAnalysis(ctx, result)

	if len(result.Plans) == 0 {
		return result, nil
	}

	workflow := newReleasePRWorkflow(r.core, r.source, r.prs, r.files, r.publisher)

	pr, err := workflow.createOrUpdate(ctx, result)
	if err != nil {
		return nil, err
	}

	result.PullRequest = pr

	if err := workflow.autoMerge(ctx, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *releaser) finalizeAndRefreshReleaseAnalysis(
	ctx context.Context,
	selection releaseSelection,
	result *Result,
	analysisErr error,
) (*Result, error) {
	finalizedReleases, err := r.finalizeMergedReleasePRs(ctx)
	if errors.Is(err, provider.ErrNoPR) {
		if analysisErr != nil {
			return nil, analysisErr
		}

		return result, nil
	}

	if err != nil {
		return nil, errors.Join(analysisErr, err)
	}

	r.source.InvalidateTags()

	result, err = newReleaseAnalyzer(r.core, r.source).analyze(ctx, selection)
	if err != nil {
		return nil, err
	}

	if err := r.validateRenderedReleaseTitles(result); err != nil {
		return nil, err
	}

	for _, finalizedRelease := range finalizedReleases {
		slog.InfoContext(ctx, "finalized release",
			slog.String("tag", finalizedRelease.TagName),
			slog.String("url", finalizedRelease.URL),
		)
	}

	result.Releases = finalizedReleases

	return result, nil
}

func (r *releaser) validateRenderedReleaseTitles(result *Result) error {
	if len(result.Plans) == 0 {
		return nil
	}

	if _, err := r.core.releasePRTitle(result); err != nil {
		return err
	}

	if _, err := r.core.releaseCommitSubject(result); err != nil {
		return err
	}

	return nil
}

func (r *releaser) logReleaseAnalysis(ctx context.Context, result *Result) {
	if len(result.Plans) == 0 {
		slog.InfoContext(ctx, "no releasable commits found")

		return
	}

	slog.InfoContext(ctx, "release analysis complete", slog.Int("targets", len(result.Plans)))
}

func (r *releaser) finalizeMergedReleasePRs(ctx context.Context) ([]*provider.Release, error) {
	return newReleasePublisher(r.core, r.publisher, r.source).finalizeMergedReleasePR(ctx)
}

func multiplePendingReleasePRError(pendingPRs []*provider.PullRequest) error {
	prReferences := make([]string, 0, len(pendingPRs))

	for _, pendingPR := range pendingPRs {
		prReferences = append(prReferences, fmt.Sprintf("#%d %s", pendingPR.Number, pendingPR.URL))
	}

	return fmt.Errorf("%w: %s", ErrMultiplePendingReleasePRs, strings.Join(prReferences, ", "))
}
