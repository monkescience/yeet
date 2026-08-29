//nolint:testpackage // This test helper supports validation of unexported release behavior.
package release

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
)

type testReleaserDeps interface {
	repoMetadataProvider
	versionHistoryProvider
	releaseForge
}

type testSourceDeps interface {
	versionHistoryProvider
	releaseFileProvider
}

type testReleaseSource struct {
	versionHistoryProvider
	files  releaseFileProvider
	branch string
}

func (s testReleaseSource) GetFile(ctx context.Context, path string) (string, error) {
	return s.files.GetFile(ctx, s.branch, path)
}

func sourceFromTestDeps(branch string, deps testSourceDeps) releaseSource {
	return testReleaseSource{versionHistoryProvider: deps, files: deps, branch: branch}
}

func newStubReleaser(
	ctx context.Context,
	cfg *config.Config,
	deps testReleaserDeps,
	options ...Options,
) (*releaser, error) {
	return newStubReleaserWithSource(ctx, cfg, deps, deps, options...)
}

func newStubReleaserWithSource(
	ctx context.Context,
	cfg *config.Config,
	deps testReleaserDeps,
	source testSourceDeps,
	options ...Options,
) (*releaser, error) {
	resolvedOptions := Options{}
	if len(options) > 0 {
		resolvedOptions = options[0]
	}

	currentBranch := cfg.Branch
	if resolvedOptions.Channel != nil {
		channel := cfg.Release.Channels[strings.TrimSpace(*resolvedOptions.Channel)]
		currentBranch = channel.Branch
	}

	run, err := resolveRun(cfg, currentBranch, resolvedOptions)
	if err != nil {
		return nil, err
	}

	core, err := newReleaseCore(ctx, cfg, deps, run)
	if err != nil {
		return nil, err
	}

	if source == nil {
		return newReleaser(core, nil, deps)
	}

	return newReleaser(core, sourceFromTestDeps(run.baseBranch, source), deps)
}

func newTestReleaser(
	t *testing.T,
	cfg *config.Config,
	deps testReleaserDeps,
	options ...Options,
) *releaser {
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

	r, err := newStubReleaser(t.Context(), cfg, deps, options...)
	testastic.NoError(t, err)

	if err != nil {
		t.FailNow()
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
	testastic.NoError(t, err)

	if err != nil {
		t.FailNow()
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

func pendingPhaseOnly() []forge.ReleasePRPhase {
	return []forge.ReleasePRPhase{forge.ReleasePRPhasePending}
}

func taggedPhaseOnly() []forge.ReleasePRPhase {
	return []forge.ReleasePRPhase{forge.ReleasePRPhaseTagged}
}

type callSequence struct {
	calls []string
}

func (s *callSequence) record(name string) {
	if s == nil {
		return
	}

	s.calls = append(s.calls, name)
}

type providerStub struct {
	sequence *callSequence

	*repoMetadataStub
	*versionHistoryStub
	*releasePRWorkflowStub
	*releaseFileStub
	*releasePublishingStub
}

// SetReleasePRLabels resolves the embedded method conflict while preserving phase-specific call recording.
func (s *providerStub) SetReleasePRLabels(
	ctx context.Context,
	number int,
	labels forge.ReleasePRLabels,
	phase forge.ReleasePRPhase,
) error {
	if phase == forge.ReleasePRPhaseTagged {
		return s.releasePublishingStub.SetReleasePRLabels(ctx, number, labels, phase)
	}

	return s.releasePRWorkflowStub.SetReleasePRLabels(ctx, number, labels, phase)
}

func newProviderStub() *providerStub {
	sequence := &callSequence{}

	stub := &providerStub{
		sequence:         sequence,
		repoMetadataStub: &repoMetadataStub{},
		versionHistoryStub: &versionHistoryStub{
			commitsErrByRef: make(map[string]error),
		},
		releasePRWorkflowStub: &releasePRWorkflowStub{
			pullRequests: make(map[string]*forge.PullRequest),
			mergePRSHA:   "merged-sha",
			sequence:     sequence,
		},
		releaseFileStub: &releaseFileStub{
			files:             make(map[string]string),
			getFileCallsByKey: make(map[string]int),
			sequence:          sequence,
		},
		releasePublishingStub: &releasePublishingStub{
			releasesByTag: make(map[string]*forge.Release),
			tags:          make(map[string]bool),
		},
	}

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

	commits         []history.CommitEntry
	commitsErr      error
	commitsErrByRef map[string]error

	commitsByRef               map[string][]history.CommitEntry
	getCommitsSinceRefsOf      [][]string
	getCommitsSinceIncludePath []bool
}

func (s *versionHistoryStub) ListTags(context.Context) ([]string, error) {
	s.listTagsCalls++

	return slices.Clone(s.tagList), nil
}

func (s *versionHistoryStub) GetCommitsSinceRefs(
	_ context.Context,
	refs []string,
	includePaths bool,
	_ []forge.TagRef,
) (history.CommitHistory, error) {
	s.getCommitsSinceRefsCalls++
	s.getCommitsSinceRefsOf = append(s.getCommitsSinceRefsOf, append([]string(nil), refs...))
	s.getCommitsSinceIncludePath = append(s.getCommitsSinceIncludePath, includePaths)

	scanned := history.CommitHistory{EntriesByRef: make(map[string][]history.CommitEntry, len(refs))}

	if errors.Is(s.commitsErr, forge.ErrCommitBoundaryNotFound) {
		scanned.MissingRefs = append(scanned.MissingRefs, refs...)

		return scanned, nil
	}

	if s.commitsErr != nil {
		return history.CommitHistory{}, s.commitsErr
	}

	for _, ref := range refs {
		if err, exists := s.commitsErrByRef[ref]; exists {
			if errors.Is(err, forge.ErrCommitBoundaryNotFound) {
				scanned.MissingRefs = append(scanned.MissingRefs, ref)

				continue
			}

			return history.CommitHistory{}, err
		}

		entries := s.entriesForRef(ref)
		scanned.EntriesByRef[ref] = entries
	}

	return scanned, nil
}

func (s *versionHistoryStub) singleRefProbes() []string {
	probes := make([]string, 0, len(s.getCommitsSinceRefsOf))

	for _, call := range s.getCommitsSinceRefsOf {
		if len(call) == 1 {
			probes = append(probes, call[0])
		}
	}

	return probes
}

func (s *versionHistoryStub) entriesForRef(ref string) []history.CommitEntry {
	if s.commitsByRef != nil {
		entries, exists := s.commitsByRef[ref]
		if !exists || len(entries) == 0 {
			return []history.CommitEntry{}
		}

		result := make([]history.CommitEntry, len(entries))
		copy(result, entries)

		return result
	}

	if len(s.commits) == 0 {
		return []history.CommitEntry{}
	}

	result := make([]history.CommitEntry, len(s.commits))
	copy(result, s.commits)

	return result
}

type releasePRWorkflowStub struct {
	pullRequests map[string]*forge.PullRequest
	openPending  []*forge.PullRequest

	maxPRBodyLength int

	createPRCalls     int
	createPROptions   []forge.ReleasePROptions
	createPRErrByCall map[int]error
	updatePRCalls     int
	updatePROptions   []forge.ReleasePROptions

	markPendingCalls  []int
	markPendingLabels []forge.ReleasePRLabels
	setLabelPhases    []forge.ReleasePRPhase

	mergePRCalls       int
	mergePRNumbers     []int
	mergePROptions     []forge.MergeReleasePROptions
	mergePRSHA         string
	mergePRErr         error
	mergePRErrByNumber map[int]error

	sequence *callSequence
}

func (s *releasePRWorkflowStub) CreateReleasePR(
	_ context.Context,
	opts forge.ReleasePROptions,
) (*forge.PullRequest, error) {
	s.createPRCalls++
	s.createPROptions = append(s.createPROptions, opts)

	err := s.createPRErrByCall[s.createPRCalls]
	if err != nil {
		return nil, err
	}

	number := s.createPRCalls

	pr := &forge.PullRequest{
		Number: number,
		Title:  opts.Title,
		Body:   opts.Body,
		URL:    fmt.Sprintf("https://example.com/pr/%d", number),
		Branch: opts.ReleaseBranch,
	}

	s.pullRequests[opts.ReleaseBranch] = pr

	return pr, nil
}

func (s *releasePRWorkflowStub) UpdateReleasePR(_ context.Context, _ int, opts forge.ReleasePROptions) error {
	s.sequence.record("UpdateReleasePR")

	s.updatePRCalls++
	s.updatePROptions = append(s.updatePROptions, opts)

	return nil
}

func (s *releasePRWorkflowStub) FindOpenPendingReleasePRs(
	_ context.Context,
	_ string,
	_ string,
	expectedBranches ...string,
) ([]*forge.PullRequest, error) {
	if s.openPending != nil {
		if len(expectedBranches) > 0 {
			pending := make([]*forge.PullRequest, 0)

			for _, pullRequest := range s.openPending {
				if pullRequest.Branch == expectedBranches[0] {
					pending = append(pending, pullRequest)
				}
			}

			return pending, nil
		}

		return s.openPending, nil
	}

	pending := make([]*forge.PullRequest, 0, len(s.pullRequests))

	for _, pullRequest := range s.pullRequests {
		if len(expectedBranches) > 0 && pullRequest.Branch != expectedBranches[0] {
			continue
		}

		pending = append(pending, pullRequest)
	}

	return pending, nil
}

func (s *releasePRWorkflowStub) FindOpenPendingReleasePRsForBase(
	context.Context,
	string,
	string,
) ([]*forge.PullRequest, error) {
	return s.openPending, nil
}

func (s *releasePRWorkflowStub) MergeReleasePR(
	_ context.Context,
	number int,
	opts forge.MergeReleasePROptions,
) (string, error) {
	s.mergePRCalls++
	s.mergePRNumbers = append(s.mergePRNumbers, number)
	s.mergePROptions = append(s.mergePROptions, opts)

	if s.mergePRErr != nil {
		return "", s.mergePRErr
	}

	err := s.mergePRErrByNumber[number]
	if err != nil {
		return "", err
	}

	return s.mergePRSHA, nil
}

func (s *releasePRWorkflowStub) SetReleasePRLabels(
	_ context.Context,
	number int,
	labels forge.ReleasePRLabels,
	phase forge.ReleasePRPhase,
) error {
	s.setLabelPhases = append(s.setLabelPhases, phase)
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

	sequence *callSequence
}

func (s *releaseFileStub) GetFile(_ context.Context, branch, path string) (string, error) {
	s.getFileCalls++
	s.getFileCallsByKey[providerFileKey(branch, path)]++

	content, exists := s.files[providerFileKey(branch, path)]
	if !exists {
		return "", forge.ErrFileNotFound
	}

	return content, nil
}

func (s *releaseFileStub) UpdateFiles(
	_ context.Context,
	branch, base string,
	files map[string]forge.FileUpdate,
	message string,
) error {
	s.sequence.record("UpdateFiles")

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
	mergedPR           *forge.PullRequest
	mergedPRResponses  []*forge.PullRequest
	mergedPRErrByCall  map[int]error
	findMergedPRCalls  int
	findMergedPRsCalls int
	findMergedPRsErr   error
	preflightErr       error
	preflightCalls     []string

	markTaggedCalls  []int
	markTaggedLabels []forge.ReleasePRLabels
	setLabelPhases   []forge.ReleasePRPhase

	latestRelease *forge.Release
	releasesByTag map[string]*forge.Release
	tags          map[string]bool

	getReleaseByTagCalls   int
	createReleaseCalls     int
	createReleaseOpts      []forge.ReleaseOptions
	createReleaseErr       error
	createReleaseErrByCall map[int]error
	releaseOnCreateError   *forge.Release
}

func (s *releasePublishingStub) PreflightReleasePRTagging(
	_ context.Context,
	taggedLabel string,
) error {
	s.preflightCalls = append(s.preflightCalls, taggedLabel)

	return s.preflightErr
}

func (s *releasePublishingStub) FindMergedReleasePR(
	context.Context,
	string,
	string,
	...string,
) (*forge.PullRequest, error) {
	s.findMergedPRCalls++

	err := s.mergedPRErrByCall[s.findMergedPRCalls]
	if err != nil {
		return nil, err
	}

	if len(s.mergedPRResponses) >= s.findMergedPRCalls {
		mergedPR := s.mergedPRResponses[s.findMergedPRCalls-1]
		if mergedPR == nil {
			return nil, forge.ErrNoPR
		}

		return mergedPR, nil
	}

	if s.mergedPR == nil {
		return nil, forge.ErrNoPR
	}

	return s.mergedPR, nil
}

func (s *releasePublishingStub) FindMergedReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel string,
	expectedBranches ...string,
) ([]*forge.PullRequest, error) {
	s.findMergedPRsCalls++
	if s.findMergedPRsErr != nil {
		return nil, s.findMergedPRsErr
	}

	branches := expectedBranches
	if len(branches) == 0 {
		branches = []string{""}
	}

	merged := make([]*forge.PullRequest, 0, len(branches))
	errList := make([]error, 0)

	for range branches {
		pr, err := s.FindMergedReleasePR(ctx, baseBranch, pendingLabel)
		if errors.Is(err, forge.ErrNoPR) {
			continue
		}

		if err != nil {
			errList = append(errList, err)

			continue
		}

		merged = append(merged, pr)
	}

	return merged, errors.Join(errList...)
}

func (s *releasePublishingStub) GetReleaseByTag(_ context.Context, tag string) (*forge.Release, error) {
	s.getReleaseByTagCalls++

	if releaseInfo, exists := s.releasesByTag[tag]; exists {
		return releaseInfo, nil
	}

	if s.latestRelease != nil && s.latestRelease.TagName == tag {
		return s.latestRelease, nil
	}

	return nil, forge.ErrNoRelease
}

func (s *releasePublishingStub) CreateRelease(
	_ context.Context,
	opts forge.ReleaseOptions,
) (*forge.Release, error) {
	s.createReleaseCalls++
	s.createReleaseOpts = append(s.createReleaseOpts, opts)

	err := s.createReleaseErrByCall[s.createReleaseCalls]
	if err != nil {
		return nil, err
	}

	if s.createReleaseErr != nil {
		if s.releaseOnCreateError != nil {
			s.releasesByTag[opts.TagName] = s.releaseOnCreateError
		}

		return nil, s.createReleaseErr
	}

	release := &forge.Release{
		TagName:   opts.TagName,
		CommitSHA: opts.Ref,
		Name:      opts.Name,
		Body:      opts.Body,
		URL:       "https://example.com/releases/" + opts.TagName,
	}

	s.latestRelease = release
	s.releasesByTag[opts.TagName] = release
	s.tags[opts.TagName] = true

	return release, nil
}

func (s *releasePublishingStub) SetReleasePRLabels(
	_ context.Context,
	number int,
	labels forge.ReleasePRLabels,
	phase forge.ReleasePRPhase,
) error {
	s.setLabelPhases = append(s.setLabelPhases, phase)
	s.markTaggedCalls = append(s.markTaggedCalls, number)
	s.markTaggedLabels = append(s.markTaggedLabels, labels)

	return nil
}
