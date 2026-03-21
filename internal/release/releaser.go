// Package release orchestrates the release process by coordinating
// commit parsing, version calculation, changelog generation, and
// VCS provider interactions.
package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/version"
)

const (
	DefaultPreviewHashLength = 7
)

var ErrInvalidPreviewHashLength = errors.New("invalid preview hash length")

var ErrPreviewTagNotAllowed = errors.New("preview tags are not allowed")

var ErrInvalidReleaseAs = errors.New("invalid release-as footer")

var ErrConflictingReleaseAs = errors.New("conflicting release-as footers")

var ErrInvalidReleaseBranch = errors.New("invalid release branch")

var ErrChangelogEntryNotFound = errors.New("changelog entry not found")

var ErrMultiplePendingReleasePRs = errors.New("multiple pending release PRs found")

type Result struct {
	CurrentVersion string
	BaseVersion    string
	NextVersion    string
	BaseTag        string
	NextTag        string
	BumpType       commit.BumpType
	Changelog      string
	prChangelog    string
	PullRequest    *provider.PullRequest
	Release        *provider.Release
	CommitCount    int
}

type Releaser struct {
	cfg       *config.Config
	history   versionHistoryProvider
	metadata  repoMetadataProvider
	prs       releasePRProvider
	files     releaseFileProvider
	publisher releasePublishingProvider
	strategy  versionStrategy
}

type versionStrategy struct {
	strategy version.Strategy
	prefix   string
}

func New(cfg *config.Config, deps releaserDependencies) *Releaser {
	var strategy version.Strategy

	switch cfg.Versioning {
	case config.VersioningCalVer:
		strategy = &version.CalVer{
			Format: cfg.CalVer.Format,
			Prefix: cfg.TagPrefix,
		}
	default:
		strategy = &version.SemVer{
			Prefix: cfg.TagPrefix,
		}
	}

	return &Releaser{
		cfg:       cfg,
		history:   deps,
		metadata:  deps,
		prs:       deps,
		files:     deps,
		publisher: deps,
		strategy: versionStrategy{
			strategy: strategy,
			prefix:   cfg.TagPrefix,
		},
	}
}

// Release performs the full release flow: analyze commits, calculate version, generate changelog, create PR.
func (r *Releaser) Release(ctx context.Context, dryRun, preview bool, previewHashLength int) (*Result, error) {
	var finalizedRelease *provider.Release

	if !dryRun && !preview {
		var err error

		finalizedRelease, err = r.finalizeMergedReleasePR(ctx)
		if err != nil {
			if !errors.Is(err, provider.ErrNoPR) {
				return nil, err
			}
		}
	}

	if finalizedRelease != nil {
		slog.InfoContext(ctx, "finalized release", "tag", finalizedRelease.TagName, "url", finalizedRelease.URL)
	}

	result, err := newReleaseAnalyzer(r).analyze(ctx, preview, previewHashLength)
	if err != nil {
		return nil, err
	}

	result.Release = finalizedRelease

	if result.BumpType == commit.BumpNone {
		slog.InfoContext(ctx, "no releasable commits found")

		return result, nil
	}

	slog.InfoContext(ctx, "release analysis complete",
		"current", result.CurrentVersion,
		"next", result.NextVersion,
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

	err = workflow.autoMerge(ctx, result, preview)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Tag creates a release tag and VCS release from a merged release PR.
func (r *Releaser) Tag(ctx context.Context, tag, changelogBody string) (*Result, error) {
	if isPreviewTag(tag, r.strategy.prefix) {
		return nil, fmt.Errorf("%w: %s", ErrPreviewTagNotAllowed, tag)
	}

	release, err := newReleasePublisher(r).ensureReleaseForTag(ctx, tag, r.cfg.Branch, changelogBody)
	if err != nil {
		return nil, err
	}

	return &Result{
		NextTag: tag,
		Release: release,
	}, nil
}

func (r *Releaser) finalizeMergedReleasePR(ctx context.Context) (*provider.Release, error) {
	return newReleasePublisher(r).finalizeMergedReleasePR(ctx)
}

func (r *Releaser) updateReleaseBranchFiles(ctx context.Context, branch string, result *Result) error {
	return newReleaseBranchUpdater(r).updateFiles(ctx, branch, result)
}

func multiplePendingReleasePRError(pendingPRs []*provider.PullRequest) error {
	prReferences := make([]string, 0, len(pendingPRs))

	for _, pendingPR := range pendingPRs {
		prReferences = append(prReferences, fmt.Sprintf("#%d %s", pendingPR.Number, pendingPR.URL))
	}

	return fmt.Errorf("%w: %s", ErrMultiplePendingReleasePRs, strings.Join(prReferences, ", "))
}
