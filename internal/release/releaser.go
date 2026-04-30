// Package release orchestrates the release process by coordinating
// commit parsing, version calculation, changelog generation, and
// VCS provider interactions.
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

var ErrInvalidReleaseAs = errors.New("invalid release-as footer")

var ErrConflictingReleaseAs = errors.New("conflicting release-as footers")

var ErrChangelogEntryNotFound = errors.New("changelog entry not found")

var ErrMultiplePendingReleasePRs = errors.New("multiple pending release PRs found")

var ErrUnknownTarget = errors.New("unknown target")

var ErrConflictingFileUpdate = errors.New("conflicting file update")

const (
	releaseBumpMajorOrder = 3
	releaseBumpMinorOrder = 2
	releaseBumpPatchOrder = 1
)

type Result struct {
	BaseBranch  string
	Plans       []TargetPlan
	PullRequest *provider.PullRequest
	Releases    []*provider.Release
}

type TargetPlan struct {
	ID              string
	Type            string
	Path            string
	CurrentVersion  string
	NextVersion     string
	NextTag         string
	BumpType        commit.BumpType
	CommitCount     int
	Changelog       string
	PRChangelog     string
	PRCompareRef    string
	Files           map[string]string
	IncludedTargets []string
	commitHashes    []string
}

type Releaser struct {
	cfg       *config.Config
	targets   map[string]config.ResolvedTarget
	history   versionHistoryProvider
	metadata  repoMetadataProvider
	prs       releasePRProvider
	overrides releasePRProvider
	files     releaseFileProvider
	publisher releasePublishingProvider
}

type versionStrategy struct {
	strategy version.Strategy
	prefix   string
}

func New(cfg *config.Config, deps releaserDependencies) (*Releaser, error) {
	targets, err := cfg.ResolvedTargets()
	if err != nil {
		return nil, fmt.Errorf("resolve release targets: %w", err)
	}

	targets, err = targetsForActiveChannel(cfg, targets)
	if err != nil {
		return nil, err
	}

	return &Releaser{
		cfg:       cfg,
		targets:   targets,
		history:   deps,
		metadata:  deps,
		prs:       deps,
		overrides: deps,
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
				"%w: prerelease channel %q supports semver targets only; target %q uses %q",
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

// Release performs the full release flow: analyze commits, calculate version, generate changelog, create PR.
func (r *Releaser) Release(ctx context.Context, dryRun bool) (*Result, error) {
	return r.ReleaseTargets(ctx, dryRun, nil)
}

// ReleaseTargets performs the release flow for all or selected targets.
func (r *Releaser) ReleaseTargets(ctx context.Context, dryRun bool, selectedTargetIDs []string) (*Result, error) {
	var finalizedReleases []*provider.Release

	if !dryRun {
		var err error

		finalizedReleases, err = r.finalizeMergedReleasePRs(ctx)
		if err != nil {
			if !errors.Is(err, provider.ErrNoPR) {
				return nil, err
			}
		}
	}

	for _, finalizedRelease := range finalizedReleases {
		slog.InfoContext(ctx, "finalized release", "tag", finalizedRelease.TagName, "url", finalizedRelease.URL)
	}

	result, err := newReleaseAnalyzer(r).analyze(ctx, selectedTargetIDs)
	if err != nil {
		return nil, err
	}

	result.Releases = finalizedReleases

	if len(result.Plans) == 0 {
		slog.InfoContext(ctx, "no releasable commits found")

		return result, nil
	}

	slog.InfoContext(ctx, "release analysis complete", "targets", len(result.Plans))

	if dryRun {
		return result, nil
	}

	workflow := newReleasePRWorkflow(r)

	pr, err := workflow.createOrUpdate(ctx, result)
	if err != nil {
		return nil, err
	}

	result.PullRequest = pr

	err = workflow.autoMerge(ctx, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Tag creates a release tag and VCS release from a merged release PR.
func (r *Releaser) Tag(ctx context.Context, tag, changelogBody string) (*Result, error) {
	release, err := newReleasePublisher(r).ensureReleaseForTag(ctx, tag, r.cfg.Branch, changelogBody, false)
	if err != nil {
		return nil, err
	}

	return &Result{
		Plans: []TargetPlan{{
			NextTag:   tag,
			Changelog: changelogBody,
		}},
		Releases: []*provider.Release{release},
	}, nil
}

func (r *Releaser) finalizeMergedReleasePRs(ctx context.Context) ([]*provider.Release, error) {
	return newReleasePublisher(r).finalizeMergedReleasePR(ctx)
}

func (r *Releaser) updateReleaseBranchFiles(ctx context.Context, branch string, result *Result) error {
	return newReleaseBranchUpdater(r).updateFiles(ctx, branch, result)
}

func (r *Releaser) strategyForTarget(target config.ResolvedTarget) versionStrategy {
	return versionStrategyForResolvedTarget(target)
}

func (r *Releaser) activePrereleaseIdentifier() string {
	channelName := strings.TrimSpace(r.cfg.ActiveChannel)
	if channelName == "" {
		return ""
	}

	channel, exists := r.cfg.Release.Channels[channelName]
	if !exists {
		return ""
	}

	return strings.TrimSpace(channel.Prerelease)
}

func (r *Releaser) isPrerelease() bool {
	return r.activePrereleaseIdentifier() != ""
}

func releaseBumpOrder(bumpType commit.BumpType) int {
	switch bumpType {
	case commit.BumpMajor:
		return releaseBumpMajorOrder
	case commit.BumpMinor:
		return releaseBumpMinorOrder
	case commit.BumpPatch:
		return releaseBumpPatchOrder
	default:
		return 0
	}
}

func multiplePendingReleasePRError(pendingPRs []*provider.PullRequest) error {
	prReferences := make([]string, 0, len(pendingPRs))

	for _, pendingPR := range pendingPRs {
		prReferences = append(prReferences, fmt.Sprintf("#%d %s", pendingPR.Number, pendingPR.URL))
	}

	return fmt.Errorf("%w: %s", ErrMultiplePendingReleasePRs, strings.Join(prReferences, ", "))
}
