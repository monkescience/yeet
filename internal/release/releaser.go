// Package release orchestrates the release process by coordinating
// commit parsing, version calculation, changelog generation, and
// VCS provider interactions.
package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/version"
	"github.com/monkescience/yeet/internal/versionfile"
)

const releaseBranchPrefix = "yeet/release-"

type Result struct {
	CurrentVersion string
	NextVersion    string
	NextTag        string
	BumpType       commit.BumpType
	Changelog      string
	PullRequest    *provider.PullRequest
	Release        *provider.Release
	CommitCount    int
}

type Releaser struct {
	cfg      *config.Config
	provider provider.Provider
	strategy versionStrategy
}

type versionStrategy struct {
	strategy version.Strategy
	prefix   string
}

func New(cfg *config.Config, p provider.Provider) *Releaser {
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
		cfg:      cfg,
		provider: p,
		strategy: versionStrategy{
			strategy: strategy,
			prefix:   cfg.TagPrefix,
		},
	}
}

// Release performs the full release flow: analyze commits, calculate version, generate changelog, create PR.
func (r *Releaser) Release(ctx context.Context, dryRun bool) (*Result, error) {
	result, err := r.analyze(ctx)
	if err != nil {
		return nil, err
	}

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

	pr, err := r.createOrUpdatePR(ctx, result)
	if err != nil {
		return nil, err
	}

	result.PullRequest = pr

	return result, nil
}

// Tag creates a release tag and VCS release from a merged release PR.
func (r *Releaser) Tag(ctx context.Context, tag, changelogBody string) (*Result, error) {
	name := tag

	release, err := r.provider.CreateRelease(ctx, provider.ReleaseOptions{
		TagName: tag,
		Name:    name,
		Body:    changelogBody,
	})
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	return &Result{
		NextTag: tag,
		Release: release,
	}, nil
}

func (r *Releaser) analyze(ctx context.Context) (*Result, error) {
	result := &Result{}

	latest, err := r.provider.GetLatestRelease(ctx)
	if err != nil && !errors.Is(err, provider.ErrNoRelease) {
		return nil, fmt.Errorf("get latest release: %w", err)
	}

	ref := ""

	if err == nil {
		currentVersion, verErr := r.strategy.strategy.Current(latest.TagName)
		if verErr != nil {
			return nil, fmt.Errorf("parse current version: %w", verErr)
		}

		result.CurrentVersion = currentVersion
		ref = latest.TagName
	}

	entries, err := r.provider.GetCommitsSince(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("get commits: %w", err)
	}

	commits := provider.ParseCommits(entries)
	result.CommitCount = len(commits)

	result.BumpType = commit.DetermineBump(commits)

	if result.BumpType == commit.BumpNone {
		return result, nil
	}

	current := result.CurrentVersion
	if current == "" {
		if sv, ok := r.strategy.strategy.(*version.SemVer); ok {
			current = sv.InitialVersion()
		}
	}

	nextVersion, err := r.strategy.strategy.Next(current, result.BumpType)
	if err != nil {
		return nil, fmt.Errorf("calculate next version: %w", err)
	}

	result.NextVersion = nextVersion
	result.NextTag = r.strategy.prefix + nextVersion

	gen := &changelog.Generator{
		Sections:   r.cfg.Changelog.Sections,
		Include:    r.cfg.Changelog.Include,
		RepoURL:    r.provider.RepoURL(),
		PathPrefix: r.provider.PathPrefix(),
	}

	entry := gen.Generate(result.NextTag, ref, commits)
	result.Changelog = changelog.Render(entry)

	return result, nil
}

func (r *Releaser) createOrUpdatePR(ctx context.Context, result *Result) (*provider.PullRequest, error) {
	releaseBranch := releaseBranchPrefix + result.NextTag

	prOpts := provider.ReleasePROptions{
		Title:         "chore: release " + result.NextTag,
		Body:          result.Changelog,
		BaseBranch:    r.cfg.Branch,
		ReleaseBranch: releaseBranch,
		Files: map[string]string{
			r.cfg.Changelog.File: result.Changelog,
		},
	}

	existing, err := r.provider.FindReleasePR(ctx, releaseBranch)
	if err != nil && !errors.Is(err, provider.ErrNoPR) {
		return nil, fmt.Errorf("find release PR: %w", err)
	}

	if err == nil {
		slog.InfoContext(ctx, "updating existing release PR", "url", existing.URL)

		updateErr := r.provider.UpdateReleasePR(ctx, existing.Number, prOpts)
		if updateErr != nil {
			return nil, fmt.Errorf("update release PR: %w", updateErr)
		}

		updateErr = r.updateReleaseBranchFiles(ctx, releaseBranch, result)
		if updateErr != nil {
			return nil, updateErr
		}

		existing.Title = prOpts.Title
		existing.Body = prOpts.Body

		return existing, nil
	}

	branchErr := r.provider.CreateBranch(ctx, releaseBranch, r.cfg.Branch)
	if branchErr != nil {
		return nil, fmt.Errorf("create release branch: %w", branchErr)
	}

	filesErr := r.updateReleaseBranchFiles(ctx, releaseBranch, result)
	if filesErr != nil {
		return nil, filesErr
	}

	pr, err := r.provider.CreateReleasePR(ctx, prOpts)
	if err != nil {
		return nil, fmt.Errorf("create release PR: %w", err)
	}

	slog.InfoContext(ctx, "created release PR", "url", pr.URL)

	return pr, nil
}

func (r *Releaser) updateReleaseBranchFiles(ctx context.Context, branch string, result *Result) error {
	err := r.provider.UpdateFile(
		ctx,
		branch,
		r.cfg.Changelog.File,
		result.Changelog,
		"chore: update changelog for "+result.NextTag,
	)
	if err != nil {
		return fmt.Errorf("update changelog file: %w", err)
	}

	for _, path := range r.cfg.VersionFiles {
		content, fileErr := r.provider.GetFile(ctx, branch, path)
		if fileErr != nil {
			return fmt.Errorf("get version file %s: %w", path, fileErr)
		}

		updatedContent, changed := versionfile.ApplyGenericMarkers(content, result.NextVersion)
		if !changed {
			slog.InfoContext(ctx, "skipping version file without yeet markers", "path", path)

			continue
		}

		updateErr := r.provider.UpdateFile(
			ctx,
			branch,
			path,
			updatedContent,
			"chore: update version markers for "+result.NextTag,
		)
		if updateErr != nil {
			return fmt.Errorf("update version file %s: %w", path, updateErr)
		}
	}

	return nil
}
