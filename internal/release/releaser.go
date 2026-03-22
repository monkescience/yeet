// Package release orchestrates the release process by coordinating
// commit parsing, version calculation, changelog generation, and
// VCS provider interactions.
package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/version"
)

var ErrPreviewTagNotAllowed = errors.New("preview tags are not allowed")

var ErrInvalidReleaseAs = errors.New("invalid release-as footer")

var ErrConflictingReleaseAs = errors.New("conflicting release-as footers")

var ErrInvalidReleaseBranch = errors.New("invalid release branch")

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
	BaseBranch     string
	Plans          []TargetPlan
	CurrentVersion string
	NextVersion    string
	NextTag        string
	BumpType       commit.BumpType
	Changelog      string
	prChangelog    string
	PullRequest    *provider.PullRequest
	Release        *provider.Release
	Releases       []*provider.Release
	CommitCount    int
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

	return &Releaser{
		cfg:       cfg,
		targets:   targets,
		history:   deps,
		metadata:  deps,
		prs:       deps,
		files:     deps,
		publisher: deps,
	}, nil
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
	if len(finalizedReleases) > 0 {
		result.Release = finalizedReleases[0]
	}

	if len(result.Plans) == 0 {
		slog.InfoContext(ctx, "no releasable commits found")

		return result, nil
	}

	slog.InfoContext(ctx, "release analysis complete",
		"targets", len(result.Plans),
		"bump", result.BumpType,
		"commits", result.CommitCount,
	)

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
	if r.isPreviewTag(tag) {
		return nil, fmt.Errorf("%w: %s", ErrPreviewTagNotAllowed, tag)
	}

	release, err := newReleasePublisher(r).ensureReleaseForTag(ctx, tag, r.cfg.Branch, changelogBody)
	if err != nil {
		return nil, err
	}

	return &Result{
		NextTag:  tag,
		Release:  release,
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

func (r *Releaser) isPreviewTag(tag string) bool {
	for _, target := range r.targets {
		if isPreviewTag(tag, target.TagPrefix) {
			return true
		}
	}

	return isPreviewTag(tag, r.cfg.TagPrefix)
}

func (r *Releaser) setPrimaryPlan(result *Result) {
	result.BumpType = commit.BumpNone
	result.CommitCount = 0
	result.CurrentVersion = ""
	result.NextVersion = ""
	result.NextTag = ""
	result.Changelog = ""
	result.prChangelog = ""

	if len(result.Plans) == 0 {
		return
	}

	primaryPlan := result.Plans[0]
	result.CurrentVersion = primaryPlan.CurrentVersion
	result.NextVersion = primaryPlan.NextVersion
	result.NextTag = primaryPlan.NextTag
	result.Changelog = primaryPlan.Changelog
	result.prChangelog = primaryPlan.PRChangelog

	for _, plan := range result.Plans {
		if releaseBumpOrder(plan.BumpType) > releaseBumpOrder(result.BumpType) {
			result.BumpType = plan.BumpType
		}
	}

	result.CommitCount = aggregateCommitCount(result.Plans)
}

func aggregateCommitCount(plans []TargetPlan) int {
	commitHashes := make(map[string]struct{})
	commitCount := 0

	for _, plan := range plans {
		if len(plan.commitHashes) == 0 {
			commitCount += plan.CommitCount

			continue
		}

		for _, hash := range plan.commitHashes {
			if _, exists := commitHashes[hash]; exists {
				continue
			}

			commitHashes[hash] = struct{}{}
			commitCount++
		}
	}

	return commitCount
}

func (r *Releaser) resultPlans(result *Result) []TargetPlan {
	if len(result.Plans) > 0 {
		return result.Plans
	}

	if result.NextTag == "" && result.Changelog == "" {
		return nil
	}

	target := r.targets["default"]
	if target.ID == "" {
		ids := make([]string, 0, len(r.targets))
		for id := range r.targets {
			ids = append(ids, id)
		}

		sort.Strings(ids)

		if len(ids) > 0 {
			target = r.targets[ids[0]]
		}
	}

	return []TargetPlan{{
		ID:             target.ID,
		Type:           target.Type,
		Path:           target.Path,
		CurrentVersion: result.CurrentVersion,
		NextVersion:    result.NextVersion,
		NextTag:        result.NextTag,
		BumpType:       result.BumpType,
		CommitCount:    result.CommitCount,
		Changelog:      result.Changelog,
		PRChangelog:    result.prChangelog,
		Files: map[string]string{
			"changelog_file": target.Changelog.File,
		},
	}}
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
