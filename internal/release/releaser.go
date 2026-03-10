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
	releaseTagMarkerPrefix   = "<!-- yeet-release-tag:"
	releaseTagMarkerSuffix   = "-->"
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

	result, err := r.analyze(ctx, preview, previewHashLength)
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

	pr, err := r.createOrUpdatePR(ctx, result)
	if err != nil {
		return nil, err
	}

	result.PullRequest = pr

	err = r.autoMergeReleasePR(ctx, result, preview)
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

	name := tag

	release, err := r.provider.CreateRelease(ctx, provider.ReleaseOptions{
		TagName: tag,
		Ref:     r.cfg.Branch,
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

func (r *Releaser) finalizeMergedReleasePR(ctx context.Context) (*provider.Release, error) {
	mergedPR, err := r.provider.FindMergedReleasePR(ctx, r.cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("find merged release PR: %w", err)
	}

	tag, err := releaseTagFromPullRequest(mergedPR)
	if err != nil {
		return nil, err
	}

	if isPreviewTag(tag, r.strategy.prefix) {
		return nil, fmt.Errorf("%w: %s", ErrPreviewTagNotAllowed, tag)
	}

	releaseInfo, err := r.releaseForTag(ctx, tag)
	if err != nil {
		return nil, err
	}

	err = r.markReleasePRTagged(ctx, mergedPR)
	if err != nil {
		return nil, err
	}

	return releaseInfo, nil
}

func (r *Releaser) releaseForTag(ctx context.Context, tag string) (*provider.Release, error) {
	existingRelease, exists, err := r.existingReleaseForTag(ctx, tag)
	if err != nil {
		return nil, err
	}

	if exists {
		return existingRelease, nil
	}

	releaseBody, err := r.releaseNotesFromChangelog(ctx, tag)
	if err != nil {
		return nil, err
	}

	return r.createReleaseForTag(ctx, tag, releaseBody)
}

func (r *Releaser) createReleaseForTag(ctx context.Context, tag, releaseBody string) (*provider.Release, error) {
	releaseInfo, err := r.provider.CreateRelease(ctx, provider.ReleaseOptions{
		TagName: tag,
		Ref:     r.cfg.Branch,
		Name:    tag,
		Body:    releaseBody,
	})
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	slog.InfoContext(ctx, "created release", "tag", tag, "url", releaseInfo.URL)

	return releaseInfo, nil
}

func (r *Releaser) ensureReleaseForTag(ctx context.Context, tag, releaseBody string) (*provider.Release, error) {
	existingRelease, exists, err := r.existingReleaseForTag(ctx, tag)
	if err != nil {
		return nil, err
	}

	if exists {
		return existingRelease, nil
	}

	return r.createReleaseForTag(ctx, tag, releaseBody)
}

func (r *Releaser) existingReleaseForTag(ctx context.Context, tag string) (*provider.Release, bool, error) {
	latest, latestErr := r.provider.GetLatestRelease(ctx)
	if latestErr != nil {
		if !errors.Is(latestErr, provider.ErrNoRelease) {
			return nil, false, fmt.Errorf("get latest release: %w", latestErr)
		}

		return nil, false, nil
	}

	if latest.TagName == tag {
		slog.InfoContext(ctx, "release already exists", "tag", tag)

		return latest, true, nil
	}

	return nil, false, nil
}

func (r *Releaser) markReleasePRTagged(ctx context.Context, pullRequest *provider.PullRequest) error {
	err := r.provider.MarkReleasePRTagged(ctx, pullRequest.Number)
	if err != nil {
		return fmt.Errorf("mark release PR tagged: %w", err)
	}

	slog.InfoContext(ctx, "marked release PR tagged", "url", pullRequest.URL)

	return nil
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

	entries, err := r.commitsSince(ctx, ref)
	if err != nil {
		return nil, err
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

	err = r.setResultVersions(result, nextVersion, entries, preview, previewHashLength)
	if err != nil {
		return nil, err
	}

	r.setResultChangelogs(result, ref, entries, commits)

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

func (r *Releaser) commitsSince(ctx context.Context, ref string) ([]provider.CommitEntry, error) {
	entries, err := r.provider.GetCommitsSince(ctx, ref, r.cfg.Branch)
	if err == nil {
		return entries, nil
	}

	if errors.Is(err, provider.ErrCommitBoundaryNotFound) {
		return nil, fmt.Errorf(
			"previous release ref %q is not reachable from release branch %q; "+
				"verify the latest tag/release and branch ancestry: %w",
			ref,
			r.cfg.Branch,
			err,
		)
	}

	return nil, fmt.Errorf("get commits from branch %q: %w", r.cfg.Branch, err)
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

func (r *Releaser) setResultChangelogs(
	result *Result,
	ref string,
	entries []provider.CommitEntry,
	commits []commit.Commit,
) {
	result.Changelog = r.renderChangelog(result.NextTag, ref, result.NextTag, commits)
	result.prChangelog = result.Changelog

	if ref != "" && len(entries) > 0 {
		result.prChangelog = r.renderChangelog(result.NextTag, ref, entries[0].Hash, commits)
	}
}

func (r *Releaser) renderChangelog(nextTag, ref, compareTarget string, commits []commit.Commit) string {
	gen := &changelog.Generator{
		Sections:   r.cfg.Changelog.Sections,
		Include:    r.cfg.Changelog.Include,
		RepoURL:    r.provider.RepoURL(),
		PathPrefix: r.provider.PathPrefix(),
	}

	entry := gen.Generate(nextTag, ref, commits)
	if ref != "" && compareTarget != "" {
		entry.CompareURL = compareURL(r.provider.RepoURL(), r.provider.PathPrefix(), ref, compareTarget)
	}

	return changelog.Render(entry)
}

func (r *Releaser) createOrUpdatePR(ctx context.Context, result *Result) (*provider.PullRequest, error) {
	pendingPRs, err := r.provider.FindOpenPendingReleasePRs(ctx, r.cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("find pending release PRs: %w", err)
	}

	if len(pendingPRs) > 1 {
		return nil, multiplePendingReleasePRError(pendingPRs)
	}

	releaseTag := releasePRTag(result)

	if len(pendingPRs) == 1 {
		existing := pendingPRs[0]
		prOpts := r.releasePROptions(result, existing.Branch, releaseTag)

		return r.updateExistingReleasePR(ctx, existing, existing.Branch, prOpts, result)
	}

	releaseBranch := stableReleaseBranch(r.cfg.Branch)
	prOpts := r.releasePROptions(result, releaseBranch, releaseTag)

	return r.createNewReleasePR(ctx, releaseBranch, prOpts, result)
}

func (r *Releaser) autoMergeReleasePR(ctx context.Context, result *Result, preview bool) error {
	autoMergeEnabled := r.cfg.Release.AutoMerge || r.cfg.Release.AutoMergeForce
	if preview || !autoMergeEnabled || result.PullRequest == nil {
		return nil
	}

	mergeOptions := provider.MergeReleasePROptions{
		Force:  r.cfg.Release.AutoMergeForce,
		Method: r.cfg.Release.AutoMergeMethod,
	}

	err := r.provider.MergeReleasePR(ctx, result.PullRequest.Number, mergeOptions)
	if err != nil {
		if mergeOptions.Force {
			return fmt.Errorf("force merge release PR: %w", err)
		}

		return fmt.Errorf("merge release PR: %w", err)
	}

	slog.InfoContext(
		ctx,
		"merged release PR",
		"url",
		result.PullRequest.URL,
		"force",
		mergeOptions.Force,
		"method",
		mergeOptions.Method,
	)

	releaseTag := releasePRTag(result)

	releaseInfo, err := r.ensureReleaseForTag(ctx, releaseTag, result.Changelog)
	if err != nil {
		return err
	}

	err = r.markReleasePRTagged(ctx, result.PullRequest)
	if err != nil {
		return err
	}

	result.Release = releaseInfo

	return nil
}

func (r *Releaser) releasePROptions(
	result *Result,
	releaseBranch, releaseTag string,
) provider.ReleasePROptions {
	prChangelog := result.Changelog
	if result.prChangelog != "" {
		prChangelog = result.prChangelog
	}

	return provider.ReleasePROptions{
		Title:         r.releaseSubject(result),
		Body:          r.releasePRBody(prChangelog, releaseTag),
		BaseBranch:    r.cfg.Branch,
		ReleaseBranch: releaseBranch,
		Files: map[string]string{
			r.cfg.Changelog.File: result.Changelog,
		},
	}
}

func (r *Releaser) updateExistingReleasePR(
	ctx context.Context,
	existing *provider.PullRequest,
	releaseBranch string,
	prOpts provider.ReleasePROptions,
	result *Result,
) (*provider.PullRequest, error) {
	slog.InfoContext(ctx, "updating existing release PR", "url", existing.URL)

	err := r.provider.UpdateReleasePR(ctx, existing.Number, prOpts)
	if err != nil {
		return nil, fmt.Errorf("update release PR: %w", err)
	}

	err = r.updateReleaseBranchFiles(ctx, releaseBranch, result)
	if err != nil {
		return nil, err
	}

	err = r.provider.MarkReleasePRPending(ctx, existing.Number)
	if err != nil {
		return nil, fmt.Errorf("mark release PR pending: %w", err)
	}

	existing.Title = prOpts.Title
	existing.Body = prOpts.Body

	return existing, nil
}

func (r *Releaser) createNewReleasePR(
	ctx context.Context,
	releaseBranch string,
	prOpts provider.ReleasePROptions,
	result *Result,
) (*provider.PullRequest, error) {
	err := r.provider.CreateBranch(ctx, releaseBranch, r.cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("create release branch: %w", err)
	}

	err = r.updateReleaseBranchFiles(ctx, releaseBranch, result)
	if err != nil {
		return nil, err
	}

	pr, err := r.provider.CreateReleasePR(ctx, prOpts)
	if err != nil {
		return nil, fmt.Errorf("create release PR: %w", err)
	}

	err = r.provider.MarkReleasePRPending(ctx, pr.Number)
	if err != nil {
		return nil, fmt.Errorf("mark release PR pending: %w", err)
	}

	slog.InfoContext(ctx, "created release PR", "url", pr.URL)

	return pr, nil
}

func (r *Releaser) updateReleaseBranchFiles(ctx context.Context, branch string, result *Result) error {
	changelogContent, err := r.releaseChangelogFileContent(ctx, result.Changelog)
	if err != nil {
		return err
	}

	files := map[string]string{
		r.cfg.Changelog.File: changelogContent,
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

	err = r.provider.UpdateFiles(ctx, branch, r.cfg.Branch, files, r.releaseSubject(result))
	if err != nil {
		return fmt.Errorf("update release branch files: %w", err)
	}

	return nil
}

func (r *Releaser) releaseChangelogFileContent(ctx context.Context, changelogEntry string) (string, error) {
	existing, err := r.provider.GetFile(ctx, r.cfg.Branch, r.cfg.Changelog.File)
	if err != nil {
		if errors.Is(err, provider.ErrFileNotFound) {
			return changelogEntry, nil
		}

		return "", fmt.Errorf("get changelog file %s: %w", r.cfg.Changelog.File, err)
	}

	return prependChangelogEntryPreservingStyle(existing, changelogEntry), nil
}

func prependChangelogEntryPreservingStyle(existing, changelogEntry string) string {
	if strings.TrimSpace(existing) == "" {
		return changelogEntry
	}

	if strings.HasPrefix(existing, "# ") {
		return changelog.Prepend(existing, changelogEntry)
	}

	return strings.TrimRight(changelogEntry, "\n") + "\n\n" + strings.TrimLeft(existing, "\n")
}

func compareURL(repoURL, pathPrefix, fromRef, toRef string) string {
	return fmt.Sprintf("%s%s/compare/%s...%s", repoURL, pathPrefix, fromRef, toRef)
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

func (r *Releaser) releasePRBody(changelogBody, releaseTag string) string {
	parts := make([]string, 0)

	if header := strings.TrimSpace(r.cfg.Release.PRBodyHeader); header != "" {
		parts = append(parts, header)
	}

	if body := strings.TrimSpace(changelogBody); body != "" {
		parts = append(parts, body)
	}

	if marker := releaseTagMarker(releaseTag); marker != "" {
		parts = append(parts, marker)
	}

	if footer := strings.TrimSpace(r.cfg.Release.PRBodyFooter); footer != "" {
		parts = append(parts, footer)
	}

	return strings.Join(parts, "\n\n")
}

func multiplePendingReleasePRError(pendingPRs []*provider.PullRequest) error {
	prReferences := make([]string, 0, len(pendingPRs))

	for _, pendingPR := range pendingPRs {
		prReferences = append(prReferences, fmt.Sprintf("#%d %s", pendingPR.Number, pendingPR.URL))
	}

	return fmt.Errorf("%w: %s", ErrMultiplePendingReleasePRs, strings.Join(prReferences, ", "))
}

func stableReleaseBranch(targetBranch string) string {
	return releaseBranchPrefix + targetBranch
}

func releasePRTag(result *Result) string {
	if result.BaseTag != "" {
		return result.BaseTag
	}

	return result.NextTag
}

func releaseTagMarker(releaseTag string) string {
	releaseTag = strings.TrimSpace(releaseTag)
	if releaseTag == "" {
		return ""
	}

	return fmt.Sprintf("%s %s %s", releaseTagMarkerPrefix, releaseTag, releaseTagMarkerSuffix)
}

func releaseTagFromPullRequest(pullRequest *provider.PullRequest) (string, error) {
	if releaseTag, ok := releaseTagFromBody(pullRequest.Body); ok {
		return releaseTag, nil
	}

	releaseTag, err := releaseTagFromBranch(pullRequest.Branch)
	if err != nil {
		return "", err
	}

	if looksLikeReleaseTag(releaseTag) {
		return releaseTag, nil
	}

	return "", fmt.Errorf(
		"%w: missing release tag marker in pull request #%d",
		ErrInvalidReleaseBranch,
		pullRequest.Number,
	)
}

func releaseTagFromBody(body string) (string, bool) {
	start := strings.Index(body, releaseTagMarkerPrefix)
	if start == -1 {
		return "", false
	}

	start += len(releaseTagMarkerPrefix)

	end := strings.Index(body[start:], releaseTagMarkerSuffix)
	if end == -1 {
		return "", false
	}

	releaseTag := strings.TrimSpace(body[start : start+end])
	if releaseTag == "" {
		return "", false
	}

	return releaseTag, true
}

func looksLikeReleaseTag(releaseTag string) bool {
	return strings.Contains(releaseTag, ".") && strings.ContainsAny(releaseTag, "0123456789")
}

func (r *Releaser) releaseNotesFromChangelog(ctx context.Context, tag string) (string, error) {
	changelogBody, err := r.provider.GetFile(ctx, r.cfg.Branch, r.cfg.Changelog.File)
	if err != nil {
		return "", fmt.Errorf("get changelog file %s: %w", r.cfg.Changelog.File, err)
	}

	entry, err := changelogEntryByTag(changelogBody, tag)
	if err != nil {
		return "", err
	}

	return entry, nil
}

func releaseTagFromBranch(branch string) (string, error) {
	if !strings.HasPrefix(branch, releaseBranchPrefix) {
		return "", fmt.Errorf("%w: %s", ErrInvalidReleaseBranch, branch)
	}

	tag := strings.TrimPrefix(branch, releaseBranchPrefix)
	if tag == "" {
		return "", fmt.Errorf("%w: %s", ErrInvalidReleaseBranch, branch)
	}

	return tag, nil
}

func changelogEntryByTag(changelogBody, tag string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(changelogBody, "\r\n", "\n"), "\n")

	start := -1

	for idx, line := range lines {
		if !strings.HasPrefix(line, "## ") {
			continue
		}

		headingTag, ok := headingTag(line)
		if !ok {
			continue
		}

		if headingTag == tag {
			start = idx

			break
		}
	}

	if start == -1 {
		return "", fmt.Errorf("%w: %s", ErrChangelogEntryNotFound, tag)
	}

	end := len(lines)

	for idx := start + 1; idx < len(lines); idx++ {
		if strings.HasPrefix(lines[idx], "## ") {
			end = idx

			break
		}
	}

	entry := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	if entry == "" {
		return "", fmt.Errorf("%w: %s", ErrChangelogEntryNotFound, tag)
	}

	return entry, nil
}

func headingTag(line string) (string, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	if rest == "" {
		return "", false
	}

	if strings.HasPrefix(rest, "[") {
		idx := strings.Index(rest, "]")
		if idx <= 1 {
			return "", false
		}

		return rest[1:idx], true
	}

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}

	return fields[0], true
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
