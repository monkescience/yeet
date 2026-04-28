//nolint:testpackage // This test helper supports validation of unexported release behavior.
package release

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

func newTestReleaser(t *testing.T, cfg *config.Config, deps releaserDependencies) *Releaser {
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

	r, err := New(cfg, deps)
	if err != nil {
		t.Fatalf("New() returned unexpected error: %v", err)
	}

	return r
}

func testManifestBody(tag, changelogFile string) string {
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
		panic("testManifestBody: " + err.Error())
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
		},
		releaseFileStub: &releaseFileStub{
			files: make(map[string]string),
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

type versionHistoryStub struct {
	latestVersionRef    string
	latestVersionRefErr error
	tagList             []string

	commits         []provider.CommitEntry
	commitsErr      error
	commitsErrByRef map[string]error

	commitsByRef            map[string][]provider.CommitEntry
	getCommitsSinceOf       []string
	getCommitsSinceBranches []string

	publishing *releasePublishingStub
}

func (s *versionHistoryStub) GetLatestVersionRef(context.Context) (string, error) {
	if s.latestVersionRefErr != nil {
		return "", s.latestVersionRefErr
	}

	if s.latestVersionRef != "" {
		return s.latestVersionRef, nil
	}

	if s.publishing.latestRelease == nil {
		return "", provider.ErrNoVersionRef
	}

	return s.publishing.latestRelease.TagName, nil
}

func (s *versionHistoryStub) ListTags(context.Context) ([]string, error) {
	if len(s.tagList) == 0 {
		return nil, nil
	}

	refs := make([]string, len(s.tagList))
	copy(refs, s.tagList)

	return refs, nil
}

func (s *versionHistoryStub) GetCommitsSince(
	_ context.Context, ref, branch string, _ bool,
) ([]provider.CommitEntry, error) {
	s.getCommitsSinceOf = append(s.getCommitsSinceOf, ref)
	s.getCommitsSinceBranches = append(s.getCommitsSinceBranches, branch)

	if err, exists := s.commitsErrByRef[ref]; exists {
		return nil, err
	}

	if s.commitsErr != nil {
		return nil, s.commitsErr
	}

	if s.commitsByRef != nil {
		entries, exists := s.commitsByRef[ref]
		if !exists || len(entries) == 0 {
			return []provider.CommitEntry{}, nil
		}

		result := make([]provider.CommitEntry, len(entries))
		copy(result, entries)

		return result, nil
	}

	if len(s.commits) == 0 {
		return []provider.CommitEntry{}, nil
	}

	return s.commits, nil
}

type releasePRWorkflowStub struct {
	pullRequests map[string]*provider.PullRequest
	openPending  []*provider.PullRequest

	commitOverrideBodies map[string]string

	createPRCalls int
	updatePRCalls int

	markPendingCalls []int

	mergePRCalls   int
	mergePRNumbers []int
	mergePROptions []provider.MergeReleasePROptions
	mergePRErr     error

	createdBranches []string
}

func (s *releasePRWorkflowStub) CreateReleasePR(
	_ context.Context,
	opts provider.ReleasePROptions,
) (*provider.PullRequest, error) {
	s.createPRCalls++

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

func (s *releasePRWorkflowStub) UpdateReleasePR(context.Context, int, provider.ReleasePROptions) error {
	s.updatePRCalls++

	return nil
}

func (s *releasePRWorkflowStub) FindOpenPendingReleasePRs(context.Context, string) ([]*provider.PullRequest, error) {
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
) error {
	s.mergePRCalls++
	s.mergePRNumbers = append(s.mergePRNumbers, number)
	s.mergePROptions = append(s.mergePROptions, opts)

	if s.mergePRErr != nil {
		return s.mergePRErr
	}

	return nil
}

func (s *releasePRWorkflowStub) MarkReleasePRPending(_ context.Context, number int) error {
	s.markPendingCalls = append(s.markPendingCalls, number)

	return nil
}

func (s *releasePRWorkflowStub) CommitPullRequestBody(_ context.Context, hash string) (string, bool, error) {
	body, exists := s.commitOverrideBodies[hash]

	return body, exists, nil
}

func (s *releasePRWorkflowStub) CreateBranch(_ context.Context, branch, _ string) error {
	s.createdBranches = append(s.createdBranches, branch)

	return nil
}

type releaseFileStub struct {
	files   map[string]string
	updates []fileUpdate

	updateFilesCalls    int
	updateFilesMessages []string
}

func (s *releaseFileStub) GetFile(_ context.Context, branch, path string) (string, error) {
	content, exists := s.files[providerFileKey(branch, path)]
	if !exists {
		return "", provider.ErrFileNotFound
	}

	return content, nil
}

func (s *releaseFileStub) UpdateFiles(
	_ context.Context,
	branch, base string,
	files map[string]string,
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

	for path, content := range files {
		s.files[providerFileKey(branch, path)] = content
		s.updates = append(s.updates, fileUpdate{
			branch:  branch,
			path:    path,
			content: content,
			message: message,
		})
	}

	return nil
}

type releasePublishingStub struct {
	mergedPR *provider.PullRequest

	markTaggedCalls []int

	latestRelease *provider.Release
	releasesByTag map[string]*provider.Release
	tags          map[string]bool

	createReleaseCalls int
	createReleaseOpts  []provider.ReleaseOptions

	history *versionHistoryStub
}

func (s *releasePublishingStub) FindMergedReleasePR(context.Context, string) (*provider.PullRequest, error) {
	if s.mergedPR == nil {
		return nil, provider.ErrNoPR
	}

	return s.mergedPR, nil
}

func (s *releasePublishingStub) GetReleaseByTag(_ context.Context, tag string) (*provider.Release, error) {
	if releaseInfo, exists := s.releasesByTag[tag]; exists {
		return releaseInfo, nil
	}

	if s.latestRelease != nil && s.latestRelease.TagName == tag {
		return s.latestRelease, nil
	}

	return nil, provider.ErrNoRelease
}

func (s *releasePublishingStub) TagExists(_ context.Context, tag string) (bool, error) {
	if s.tags[tag] {
		return true, nil
	}

	if _, exists := s.releasesByTag[tag]; exists {
		return true, nil
	}

	if s.latestRelease != nil && s.latestRelease.TagName == tag {
		return true, nil
	}

	return false, nil
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

func (s *releasePublishingStub) MarkReleasePRTagged(_ context.Context, number int) error {
	s.markTaggedCalls = append(s.markTaggedCalls, number)

	return nil
}
