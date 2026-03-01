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

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/version"
	"github.com/monkescience/yeet/internal/versionfile"
)

const (
	releaseBranchPrefix      = "yeet/release-"
	DefaultPreviewHashLength = 7
)

var ErrInvalidPreviewHashLength = errors.New("invalid preview hash length")

var ErrPreviewTagNotAllowed = errors.New("preview tags are not allowed")

var ErrInvalidReleaseAs = errors.New("invalid release-as footer")

var ErrConflictingReleaseAs = errors.New("conflicting release-as footers")

type Result struct {
	CurrentVersion string
	BaseVersion    string
	NextVersion    string
	BaseTag        string
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
func (r *Releaser) Release(ctx context.Context, dryRun, preview bool, previewHashLength int) (*Result, error) {
	result, err := r.analyze(ctx, preview, previewHashLength)
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
	if isPreviewTag(tag, r.strategy.prefix) {
		return nil, fmt.Errorf("%w: %s", ErrPreviewTagNotAllowed, tag)
	}

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

func (r *Releaser) analyze(ctx context.Context, preview bool, previewHashLength int) (*Result, error) {
	result := &Result{}

	if preview && previewHashLength <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidPreviewHashLength, previewHashLength)
	}

	currentVersion, ref, err := r.currentVersionFromLatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	result.CurrentVersion = currentVersion

	entries, err := r.provider.GetCommitsSince(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("get commits: %w", err)
	}

	commits := provider.ParseCommits(entries)
	result.CommitCount = len(commits)

	result.BumpType = commit.DetermineBump(commits)

	releaseAsVersion := ""

	if r.cfg.Versioning == config.VersioningSemver {
		releaseAsVersion, err = detectReleaseAs(commits)
		if err != nil {
			return nil, err
		}
	}

	current := result.CurrentVersion
	if current == "" {
		if sv, ok := r.strategy.strategy.(*version.SemVer); ok {
			current = sv.InitialVersion()
		}
	}

	nextVersion, bumpType, shouldRelease, err := r.resolveNextVersion(current, result.BumpType, releaseAsVersion)
	if err != nil {
		return nil, err
	}

	result.BumpType = bumpType

	if !shouldRelease {
		return result, nil
	}

	setVersionErr := r.setResultVersions(result, nextVersion, entries, preview, previewHashLength)
	if setVersionErr != nil {
		return nil, setVersionErr
	}

	result.Changelog = r.renderChangelog(result.NextTag, ref, commits)

	return result, nil
}

func (r *Releaser) currentVersionFromLatestRelease(ctx context.Context) (string, string, error) {
	latest, err := r.provider.GetLatestRelease(ctx)
	if err != nil {
		if errors.Is(err, provider.ErrNoRelease) {
			return "", "", nil
		}

		return "", "", fmt.Errorf("get latest release: %w", err)
	}

	currentVersion, err := r.strategy.strategy.Current(latest.TagName)
	if err != nil {
		return "", "", fmt.Errorf("parse current version: %w", err)
	}

	return currentVersion, latest.TagName, nil
}

func (r *Releaser) setResultVersions(
	result *Result,
	baseVersion string,
	entries []provider.CommitEntry,
	preview bool,
	previewHashLength int,
) error {
	result.BaseVersion = baseVersion
	result.BaseTag = r.strategy.prefix + baseVersion
	result.NextVersion = baseVersion

	if !preview {
		result.NextTag = result.BaseTag

		return nil
	}

	if len(entries) == 0 {
		return fmt.Errorf("%w: no commit hash available", ErrInvalidPreviewHashLength)
	}

	hash, err := shortHash(entries[0].Hash, previewHashLength)
	if err != nil {
		return err
	}

	result.NextVersion = baseVersion + "+" + hash
	result.NextTag = r.strategy.prefix + result.NextVersion

	return nil
}

func (r *Releaser) renderChangelog(nextTag, ref string, commits []commit.Commit) string {
	gen := &changelog.Generator{
		Sections:   r.cfg.Changelog.Sections,
		Include:    r.cfg.Changelog.Include,
		RepoURL:    r.provider.RepoURL(),
		PathPrefix: r.provider.PathPrefix(),
	}

	entry := gen.Generate(nextTag, ref, commits)

	return changelog.Render(entry)
}

func (r *Releaser) createOrUpdatePR(ctx context.Context, result *Result) (*provider.PullRequest, error) {
	releaseBranchTag := result.BaseTag
	if releaseBranchTag == "" {
		releaseBranchTag = result.NextTag
	}

	releaseBranch := releaseBranchPrefix + releaseBranchTag

	prOpts := provider.ReleasePROptions{
		Title:         r.releaseSubject(result),
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
	files := map[string]string{
		r.cfg.Changelog.File: result.Changelog,
	}

	for _, path := range r.cfg.VersionFiles {
		content, fileErr := r.provider.GetFile(ctx, r.cfg.Branch, path)
		if fileErr != nil {
			return fmt.Errorf("get version file %s: %w", path, fileErr)
		}

		updatedContent, changed := versionfile.ApplyGenericMarkers(content, result.NextVersion)
		if !changed {
			slog.InfoContext(ctx, "skipping version file without yeet markers", "path", path)

			continue
		}

		files[path] = updatedContent
	}

	err := r.provider.UpdateFiles(ctx, branch, r.cfg.Branch, files, r.releaseSubject(result))
	if err != nil {
		return fmt.Errorf("update release branch files: %w", err)
	}

	return nil
}

func (r *Releaser) releaseSubject(result *Result) string {
	version := result.BaseVersion
	if version == "" {
		version = result.NextVersion
	}

	if r.cfg.Release.SubjectIncludeBranch {
		return fmt.Sprintf("chore(%s): release %s", r.cfg.Branch, version)
	}

	return "chore: release " + version
}

func shortHash(hash string, length int) (string, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "", fmt.Errorf("%w: empty commit hash", ErrInvalidPreviewHashLength)
	}

	if length <= 0 {
		return "", fmt.Errorf("%w: got %d", ErrInvalidPreviewHashLength, length)
	}

	if len(hash) <= length {
		return hash, nil
	}

	return hash[:length], nil
}

func (r *Releaser) resolveNextVersion(
	current string,
	bump commit.BumpType,
	releaseAsVersion string,
) (string, commit.BumpType, bool, error) {
	if releaseAsVersion != "" && r.cfg.Versioning == config.VersioningSemver {
		nextVersion, overrideBump, err := applyReleaseAs(current, releaseAsVersion)
		if err != nil {
			return "", commit.BumpNone, false, err
		}

		return nextVersion, overrideBump, true, nil
	}

	if bump == commit.BumpNone {
		return "", bump, false, nil
	}

	nextVersion, err := r.strategy.strategy.Next(current, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next version: %w", err)
	}

	return nextVersion, bump, true, nil
}

func detectReleaseAs(commits []commit.Commit) (string, error) {
	releaseAsVersion := ""

	for _, c := range commits {
		for _, footer := range c.Footers {
			if !strings.EqualFold(strings.TrimSpace(footer.Key), "Release-As") {
				continue
			}

			candidate := strings.TrimSpace(footer.Value)
			if candidate == "" {
				return "", fmt.Errorf("%w: empty value", ErrInvalidReleaseAs)
			}

			normalizedCandidate, err := normalizeReleaseAsValue(candidate)
			if err != nil {
				return "", err
			}

			if releaseAsVersion == "" {
				releaseAsVersion = normalizedCandidate

				continue
			}

			if releaseAsVersion != normalizedCandidate {
				return "", fmt.Errorf("%w: %q and %q", ErrConflictingReleaseAs, releaseAsVersion, normalizedCandidate)
			}
		}
	}

	return releaseAsVersion, nil
}

func normalizeReleaseAsValue(releaseAsVersion string) (string, error) {
	v, err := semver.StrictNewVersion(releaseAsVersion)
	if err != nil {
		return "", fmt.Errorf("%w: invalid version %q: %w", ErrInvalidReleaseAs, releaseAsVersion, err)
	}

	return v.String(), nil
}

func applyReleaseAs(current, releaseAsVersion string) (string, commit.BumpType, error) {
	targetVersion, err := semver.StrictNewVersion(releaseAsVersion)
	if err != nil {
		return "", commit.BumpNone, fmt.Errorf("%w: invalid version %q: %w", ErrInvalidReleaseAs, releaseAsVersion, err)
	}

	if targetVersion.Prerelease() != "" || targetVersion.Metadata() != "" {
		return "", commit.BumpNone, fmt.Errorf("%w: %q must be a stable version", ErrInvalidReleaseAs, releaseAsVersion)
	}

	currentVersion, err := semver.StrictNewVersion(current)
	if err != nil {
		return "", commit.BumpNone, fmt.Errorf("%w: parse current version %q: %w", ErrInvalidReleaseAs, current, err)
	}

	if !targetVersion.GreaterThan(currentVersion) {
		return "", commit.BumpNone, fmt.Errorf(
			"%w: %s must be greater than current version %s",
			ErrInvalidReleaseAs,
			targetVersion.String(),
			currentVersion.String(),
		)
	}

	bump := inferSemverBump(currentVersion, targetVersion)

	return targetVersion.String(), bump, nil
}

func inferSemverBump(currentVersion, targetVersion *semver.Version) commit.BumpType {
	if targetVersion.Major() > currentVersion.Major() {
		return commit.BumpMajor
	}

	if targetVersion.Minor() > currentVersion.Minor() {
		return commit.BumpMinor
	}

	return commit.BumpPatch
}

func isPreviewTag(tag, prefix string) bool {
	versionPart := strings.TrimPrefix(tag, prefix)
	if versionPart == tag {
		for idx := range len(tag) {
			if tag[idx] >= '0' && tag[idx] <= '9' {
				versionPart = tag[idx:]

				break
			}
		}
	}

	return strings.Contains(versionPart, "+") || strings.Contains(versionPart, "-")
}
