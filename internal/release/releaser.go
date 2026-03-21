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
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/version"
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
		cfg:       cfg,
		history:   p,
		metadata:  p,
		prs:       p,
		files:     p,
		publisher: p,
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

func (r *Releaser) ensureReleaseForTag(ctx context.Context, tag, ref, releaseBody string) (*provider.Release, error) {
	return newReleasePublisher(r).ensureReleaseForTag(ctx, tag, ref, releaseBody)
}

func (r *Releaser) markReleasePRTagged(ctx context.Context, pullRequest *provider.PullRequest) error {
	return newReleasePublisher(r).markReleasePRTagged(ctx, pullRequest)
}

func (r *Releaser) analyze(ctx context.Context, preview bool, previewHashLength int) (*Result, error) {
	result := &Result{}

	if preview && previewHashLength <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidPreviewHashLength, previewHashLength)
	}

	currentVersion, ref, err := r.currentVersionFromReleaseHistory(ctx)
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

func (r *Releaser) currentVersionFromReleaseHistory(ctx context.Context) (string, string, error) {
	refs, err := r.versionHistoryRefs(ctx)
	if err != nil {
		return "", "", err
	}

	for _, ref := range refs {
		currentVersion, usable, useErr := r.currentVersionFromReachableRef(ctx, ref)
		if useErr != nil {
			return "", "", useErr
		}

		if usable {
			return currentVersion, ref, nil
		}
	}

	if len(refs) > 0 {
		return "", "", r.branchAncestryError(refs[0])
	}

	return "", "", nil
}

func (r *Releaser) versionHistoryRefs(ctx context.Context) ([]string, error) {
	refs := make([]string, 0)

	preferredRef, err := r.history.GetLatestVersionRef(ctx)
	if err != nil {
		if !errors.Is(err, provider.ErrNoVersionRef) {
			return nil, fmt.Errorf("get latest version ref: %w", err)
		}
	} else {
		refs = append(refs, preferredRef)
	}

	tags, err := r.history.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	refs = append(refs, tags...)

	return r.orderedVersionRefs(refs, ""), nil
}

func (r *Releaser) currentVersionFromReachableRef(ctx context.Context, ref string) (string, bool, error) {
	currentVersion, ok := r.currentVersionFromRef(ref)
	if !ok {
		return "", false, nil
	}

	reachable, err := r.refReachableFromBranch(ctx, ref)
	if err != nil {
		return "", false, err
	}

	if !reachable {
		return "", false, nil
	}

	return currentVersion, true, nil
}

func (r *Releaser) currentVersionFromRef(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || isPreviewTag(ref, r.strategy.prefix) {
		return "", false
	}

	currentVersion, err := r.strategy.strategy.Current(ref)
	if err != nil {
		return "", false
	}

	return currentVersion, true
}

func (r *Releaser) refReachableFromBranch(ctx context.Context, ref string) (bool, error) {
	_, err := r.history.GetCommitsSince(ctx, ref, r.cfg.Branch)
	if err != nil {
		if errors.Is(err, provider.ErrCommitBoundaryNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("validate version ref %q: %w", ref, err)
	}

	return true, nil
}

func (r *Releaser) orderedVersionRefs(refs []string, excludeRef string) []string {
	orderedRefs := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	excludeRef = strings.TrimSpace(excludeRef)

	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || ref == excludeRef {
			continue
		}

		if _, exists := seen[ref]; exists {
			continue
		}

		if _, ok := r.currentVersionFromRef(ref); !ok {
			continue
		}

		orderedRefs = append(orderedRefs, ref)
		seen[ref] = struct{}{}
	}

	sort.SliceStable(orderedRefs, func(i, j int) bool {
		return r.versionRefLess(orderedRefs[j], orderedRefs[i])
	})

	return orderedRefs
}

func (r *Releaser) versionRefLess(leftRef, rightRef string) bool {
	leftVersion, ok := r.currentVersionFromRef(leftRef)
	if !ok {
		return false
	}

	rightVersion, ok := r.currentVersionFromRef(rightRef)
	if !ok {
		return false
	}

	if r.cfg.Versioning == config.VersioningCalVer {
		return calVerVersionRefLess(leftVersion, rightVersion, leftRef, rightRef)
	}

	return semVerVersionRefLess(leftVersion, rightVersion, leftRef, rightRef)
}

func semVerVersionRefLess(leftVersion, rightVersion, leftRef, rightRef string) bool {
	leftSemver, err := semver.StrictNewVersion(leftVersion)
	if err != nil {
		return leftRef < rightRef
	}

	rightSemver, err := semver.StrictNewVersion(rightVersion)
	if err != nil {
		return leftRef < rightRef
	}

	if !leftSemver.Equal(rightSemver) {
		return leftSemver.LessThan(rightSemver)
	}

	return leftRef < rightRef
}

func calVerVersionRefLess(leftVersion, rightVersion, leftRef, rightRef string) bool {
	leftParts, err := parseCalVerVersion(leftVersion)
	if err != nil {
		return leftRef < rightRef
	}

	rightParts, err := parseCalVerVersion(rightVersion)
	if err != nil {
		return leftRef < rightRef
	}

	if leftParts[0] != rightParts[0] {
		return leftParts[0] < rightParts[0]
	}

	if leftParts[1] != rightParts[1] {
		return leftParts[1] < rightParts[1]
	}

	if leftParts[2] != rightParts[2] {
		return leftParts[2] < rightParts[2]
	}

	return leftRef < rightRef
}

func parseCalVerVersion(rawVersion string) ([3]int, error) {
	parts := strings.SplitN(strings.TrimSpace(rawVersion), ".", 3) //nolint:mnd // calver has 3 segments
	if len(parts) != 3 {                                           //nolint:mnd // calver has 3 segments
		return [3]int{}, fmt.Errorf("%w: %q", version.ErrInvalidVersion, rawVersion)
	}

	values := [3]int{}

	for idx, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return [3]int{}, fmt.Errorf("%w: %q: %w", version.ErrInvalidVersion, rawVersion, err)
		}

		values[idx] = value
	}

	return values, nil
}

func (r *Releaser) branchAncestryError(ref string) error {
	return fmt.Errorf(
		"previous release ref %q is not reachable from release branch %q; "+
			"verify the latest tag/release and branch ancestry: %w",
		ref,
		r.cfg.Branch,
		&provider.CommitBoundaryNotFoundError{Ref: ref, Branch: r.cfg.Branch},
	)
}

func (r *Releaser) commitsSince(ctx context.Context, ref string) ([]provider.CommitEntry, error) {
	entries, err := r.history.GetCommitsSince(ctx, ref, r.cfg.Branch)
	if err == nil {
		return entries, nil
	}

	if errors.Is(err, provider.ErrCommitBoundaryNotFound) {
		return nil, r.branchAncestryError(ref)
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
		RepoURL:    r.metadata.RepoURL(),
		PathPrefix: r.metadata.PathPrefix(),
	}

	entry := gen.Generate(nextTag, ref, commits)
	if ref != "" && compareTarget != "" {
		entry.CompareURL = compareURL(r.metadata.RepoURL(), r.metadata.PathPrefix(), ref, compareTarget)
	}

	return changelog.Render(entry)
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

func (r *Releaser) updateReleaseBranchFiles(ctx context.Context, branch string, result *Result) error {
	return newReleaseBranchUpdater(r).updateFiles(ctx, branch, result)
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

func releaseRefForPullRequest(pullRequest *provider.PullRequest, defaultRef string) string {
	mergeCommitSHA := strings.TrimSpace(pullRequest.MergeCommitSHA)
	if mergeCommitSHA != "" {
		return mergeCommitSHA
	}

	return strings.TrimSpace(defaultRef)
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
