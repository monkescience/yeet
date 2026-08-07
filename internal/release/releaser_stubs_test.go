//nolint:testpackage // This test helper supports validation of unexported release behavior.
package release

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

// testReleaserDeps is the stub-side capability set: provider dependencies
// plus the version history that production wiring sources from the local
// checkout.
type testReleaserDeps interface {
	releaserDependencies
	versionHistoryProvider
}

func newTestReleaser(t *testing.T, cfg *config.Config, deps testReleaserDeps) *Releaser {
	t.Helper()

	if len(cfg.Targets) == 0 {
		cfg.Targets = map[string]config.Target{
			"default": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
			},
		}
	}

	r, err := New(t.Context(), cfg, deps, deps)
	if err != nil {
		t.Fatalf("New() returned unexpected error: %v", err)
	}

	return r
}

func testManifestBody(t *testing.T, tag, changelogFile string) string {
	t.Helper()

	marker, err := releaseManifestMarker(releaseManifest{
		BaseBranch: "main",
		Targets: []releaseManifestEntry{{
			ID:            "default",
			Type:          "path",
			Tag:           tag,
			ChangelogFile: changelogFile,
		}},
	})
	if err != nil {
		t.Fatalf("releaseManifestMarker returned unexpected error: %v", err)
	}

	return marker
}

func gitLabNormalizeYeetMarkers(body string) string {
	return strings.NewReplacer(
		"<!-- yeet-release-manifest", "<!--yeet-release-manifest",
		"\n-->", "-->",
		"<!-- BEGIN_YEET_RELEASE_NOTES -->", "<!--BEGIN_YEET_RELEASE_NOTES-->",
		"<!-- END_YEET_RELEASE_NOTES -->", "<!--END_YEET_RELEASE_NOTES-->",
	).Replace(body)
}

type fileUpdate struct {
	branch  string
	path    string
	content string
	exists  bool
	message string
}

type providerStub struct {
	*repoMetadataStub
	*versionHistoryStub
	*releasePRWorkflowStub
	*releaseFileStub
	*releasePublishingStub
}

func newProviderStub() *providerStub {
	history := &versionHistoryStub{
		commitsErrByRef: make(map[string]error),
	}

	stub := &providerStub{
		repoMetadataStub:   &repoMetadataStub{},
		versionHistoryStub: history,
		releasePRWorkflowStub: &releasePRWorkflowStub{
			pullRequests: make(map[string]*provider.PullRequest),
			mergePRSHA:   "merged-sha",
		},
		releaseFileStub: &releaseFileStub{
			files:             make(map[string]string),
			getFileCallsByKey: make(map[string]int),
		},
		releasePublishingStub: &releasePublishingStub{
			releasesByTag: make(map[string]*provider.Release),
			tags:          make(map[string]bool),
		},
	}

	history.publishing = stub.releasePublishingStub
	stub.history = history

	return stub
}

func providerFileKey(branch, path string) string {
	return branch + ":" + path
}

type repoMetadataStub struct {
	repoURL    string
	pathPrefix string
}

func (s *repoMetadataStub) RepoURL() string {
	return s.repoURL
}

func (s *repoMetadataStub) PathPrefix() string {
	return s.pathPrefix
}

func (s *repoMetadataStub) CompareURL(fromRef, toRef string) string {
	return fmt.Sprintf("%s%s/compare/%s...%s", s.repoURL, s.pathPrefix, fromRef, toRef)
}

type versionHistoryStub struct {
	tagList []string

	listTagsCalls            int
	getCommitsSinceRefsCalls int

	commits         []provider.CommitEntry
	commitsErr      error
	commitsErrByRef map[string]error

	commitsByRef               map[string][]provider.CommitEntry
	getCommitsSinceRefsOf      [][]string
	getCommitsSinceBranches    []string
	getCommitsSinceIncludePath []bool

	publishing *releasePublishingStub
}

func (s *versionHistoryStub) ListTags(context.Context) ([]string, error) {
	s.listTagsCalls++

	if len(s.tagList) == 0 {
		if s.publishing.latestRelease != nil {
			return []string{s.publishing.latestRelease.TagName}, nil
		}

		return nil, nil
	}

	refs := make([]string, len(s.tagList))
	copy(refs, s.tagList)

	return refs, nil
}

func (s *versionHistoryStub) GetCommitsSinceRefs(
	_ context.Context,
	refs []string,
	branch string,
	includePaths bool,
) (provider.CommitHistory, error) {
	s.getCommitsSinceRefsCalls++
	s.getCommitsSinceRefsOf = append(s.getCommitsSinceRefsOf, append([]string(nil), refs...))
	s.getCommitsSinceBranches = append(s.getCommitsSinceBranches, branch)
	s.getCommitsSinceIncludePath = append(s.getCommitsSinceIncludePath, includePaths)

	history := provider.CommitHistory{EntriesByRef: make(map[string][]provider.CommitEntry, len(refs))}

	if errors.Is(s.commitsErr, provider.ErrCommitBoundaryNotFound) {
		history.MissingRefs = append(history.MissingRefs, refs...)

		return history, nil
	}

	if s.commitsErr != nil {
		return provider.CommitHistory{}, s.commitsErr
	}

	for _, ref := range refs {
		if err, exists := s.commitsErrByRef[ref]; exists {
			if errors.Is(err, provider.ErrCommitBoundaryNotFound) {
				history.MissingRefs = append(history.MissingRefs, ref)

				continue
			}

			return provider.CommitHistory{}, err
		}

		entries := s.entriesForRef(ref)
		history.EntriesByRef[ref] = entries
	}

	return history, nil
}

// singleRefProbes returns the flat sequence of refs probed via single-ref
// GetCommitsSinceRefs calls (one ref per call). Multi-ref shared scans are
// excluded so callers can keep asserting on per-target boundary probes.
func (s *versionHistoryStub) singleRefProbes() []string {
	probes := make([]string, 0, len(s.getCommitsSinceRefsOf))

	for _, call := range s.getCommitsSinceRefsOf {
		if len(call) == 1 {
			probes = append(probes, call[0])
		}
	}

	return probes
}

func (s *versionHistoryStub) entriesForRef(ref string) []provider.CommitEntry {
	if s.commitsByRef != nil {
		entries, exists := s.commitsByRef[ref]
		if !exists || len(entries) == 0 {
			return []provider.CommitEntry{}
		}

		result := make([]provider.CommitEntry, len(entries))
		copy(result, entries)

		return result
	}

	if len(s.commits) == 0 {
		return []provider.CommitEntry{}
	}

	result := make([]provider.CommitEntry, len(s.commits))
	copy(result, s.commits)

	return result
}

type releasePRWorkflowStub struct {
	pullRequests map[string]*provider.PullRequest
	openPending  []*provider.PullRequest

	maxPRBodyLength int

	createPRCalls   int
	createPROptions []provider.ReleasePROptions
	updatePRCalls   int
	updatePROptions []provider.ReleasePROptions

	markPendingCalls  []int
	markPendingLabels []provider.ReleasePRLabels
	prepareLabelCalls []provider.ReleasePRLabels

	mergePRCalls   int
	mergePRNumbers []int
	mergePROptions []provider.MergeReleasePROptions
	mergePRSHA     string
	mergePRErr     error
}

func (s *releasePRWorkflowStub) CreateReleasePR(
	_ context.Context,
	opts provider.ReleasePROptions,
) (*provider.PullRequest, error) {
	s.createPRCalls++
	s.createPROptions = append(s.createPROptions, opts)

	number := s.createPRCalls

	pr := &provider.PullRequest{
		Number: number,
		Title:  opts.Title,
		Body:   opts.Body,
		URL:    fmt.Sprintf("https://example.com/pr/%d", number),
		Branch: opts.ReleaseBranch,
	}

	s.pullRequests[opts.ReleaseBranch] = pr

	return pr, nil
}

func (s *releasePRWorkflowStub) UpdateReleasePR(_ context.Context, _ int, opts provider.ReleasePROptions) error {
	s.updatePRCalls++
	s.updatePROptions = append(s.updatePROptions, opts)

	return nil
}

func (s *releasePRWorkflowStub) FindOpenPendingReleasePRs(
	context.Context,
	string,
	string,
) ([]*provider.PullRequest, error) {
	if s.openPending != nil {
		return s.openPending, nil
	}

	pending := make([]*provider.PullRequest, 0, len(s.pullRequests))

	for _, pullRequest := range s.pullRequests {
		pending = append(pending, pullRequest)
	}

	return pending, nil
}

func (s *releasePRWorkflowStub) MergeReleasePR(
	_ context.Context,
	number int,
	opts provider.MergeReleasePROptions,
) (string, error) {
	s.mergePRCalls++
	s.mergePRNumbers = append(s.mergePRNumbers, number)
	s.mergePROptions = append(s.mergePROptions, opts)

	if s.mergePRErr != nil {
		return "", s.mergePRErr
	}

	return s.mergePRSHA, nil
}

func (s *releasePRWorkflowStub) PrepareReleasePRLabels(
	_ context.Context,
	labels provider.ReleasePRLabels,
) error {
	s.prepareLabelCalls = append(s.prepareLabelCalls, labels)

	return nil
}

func (s *releasePRWorkflowStub) MarkReleasePRPending(
	_ context.Context,
	number int,
	labels provider.ReleasePRLabels,
) error {
	s.markPendingCalls = append(s.markPendingCalls, number)
	s.markPendingLabels = append(s.markPendingLabels, labels)

	return nil
}

func (s *releasePRWorkflowStub) MaxPRBodyLength() int {
	return s.maxPRBodyLength
}

type releaseFileStub struct {
	files   map[string]string
	updates []fileUpdate

	updateFilesCalls    int
	updateFilesMessages []string
	getFileCalls        int
	getFileCallsByKey   map[string]int
}

func (s *releaseFileStub) GetFile(_ context.Context, branch, path string) (string, error) {
	s.getFileCalls++
	s.getFileCallsByKey[providerFileKey(branch, path)]++

	content, exists := s.files[providerFileKey(branch, path)]
	if !exists {
		return "", provider.ErrFileNotFound
	}

	return content, nil
}

func (s *releaseFileStub) UpdateFiles(
	_ context.Context,
	branch, base string,
	files map[string]provider.FileUpdate,
	message string,
) error {
	s.updateFilesCalls++
	s.updateFilesMessages = append(s.updateFilesMessages, message)

	branchPrefix := branch + ":"

	for key := range s.files {
		if strings.HasPrefix(key, branchPrefix) {
			delete(s.files, key)
		}
	}

	basePrefix := base + ":"

	for key, content := range s.files {
		if !strings.HasPrefix(key, basePrefix) {
			continue
		}

		path := strings.TrimPrefix(key, basePrefix)
		s.files[providerFileKey(branch, path)] = content
	}

	for path, update := range files {
		s.files[providerFileKey(branch, path)] = update.Content
		s.updates = append(s.updates, fileUpdate{
			branch:  branch,
			path:    path,
			content: update.Content,
			exists:  update.Exists,
			message: message,
		})
	}

	return nil
}

type releasePublishingStub struct {
	mergedPR          *provider.PullRequest
	mergedPRResponses []*provider.PullRequest
	findMergedPRCalls int

	markTaggedCalls  []int
	markTaggedLabels []provider.ReleasePRLabels

	latestRelease *provider.Release
	releasesByTag map[string]*provider.Release
	tags          map[string]bool

	getReleaseByTagCalls int
	createReleaseCalls   int
	createReleaseOpts    []provider.ReleaseOptions

	history *versionHistoryStub
}

func (s *releasePublishingStub) FindMergedReleasePR(
	context.Context,
	string,
	string,
) (*provider.PullRequest, error) {
	s.findMergedPRCalls++
	if len(s.mergedPRResponses) >= s.findMergedPRCalls {
		mergedPR := s.mergedPRResponses[s.findMergedPRCalls-1]
		if mergedPR == nil {
			return nil, provider.ErrNoPR
		}

		return mergedPR, nil
	}

	if s.mergedPR == nil {
		return nil, provider.ErrNoPR
	}

	return s.mergedPR, nil
}

func (s *releasePublishingStub) GetReleaseByTag(_ context.Context, tag string) (*provider.Release, error) {
	s.getReleaseByTagCalls++

	if releaseInfo, exists := s.releasesByTag[tag]; exists {
		return releaseInfo, nil
	}

	if s.latestRelease != nil && s.latestRelease.TagName == tag {
		return s.latestRelease, nil
	}

	return nil, provider.ErrNoRelease
}

func (s *releasePublishingStub) CreateRelease(
	_ context.Context,
	opts provider.ReleaseOptions,
) (*provider.Release, error) {
	s.createReleaseCalls++
	s.createReleaseOpts = append(s.createReleaseOpts, opts)

	release := &provider.Release{
		TagName: opts.TagName,
		Name:    opts.Name,
		Body:    opts.Body,
		URL:     "https://example.com/releases/" + opts.TagName,
	}

	s.latestRelease = release
	s.releasesByTag[opts.TagName] = release
	s.tags[opts.TagName] = true

	if !slices.Contains(s.history.tagList, opts.TagName) {
		s.history.tagList = append(s.history.tagList, opts.TagName)
	}

	return release, nil
}

func (s *releasePublishingStub) MarkReleasePRTagged(
	_ context.Context,
	number int,
	labels provider.ReleasePRLabels,
) error {
	s.markTaggedCalls = append(s.markTaggedCalls, number)
	s.markTaggedLabels = append(s.markTaggedLabels, labels)

	return nil
}
