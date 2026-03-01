//nolint:testpackage // This test validates unexported release behavior.
package release

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

type fileUpdate struct {
	branch  string
	path    string
	content string
	message string
}

type providerStub struct {
	files   map[string]string
	updates []fileUpdate

	updateFilesCalls    int
	updateFilesMessages []string

	latestRelease    *provider.Release
	latestReleaseErr error

	commits    []provider.CommitEntry
	commitsErr error

	commitsByRef      map[string][]provider.CommitEntry
	getCommitsSinceOf []string

	pullRequests map[string]*provider.PullRequest
	mergedPR     *provider.PullRequest
	openPending  []*provider.PullRequest

	createPRCalls int
	updatePRCalls int

	markPendingCalls []int
	markTaggedCalls  []int

	createdBranches []string

	createReleaseCalls int
}

func newProviderStub() *providerStub {
	return &providerStub{
		files:        make(map[string]string),
		pullRequests: make(map[string]*provider.PullRequest),
	}
}

func providerFileKey(branch, path string) string {
	return branch + ":" + path
}

func (p *providerStub) GetLatestRelease(context.Context) (*provider.Release, error) {
	if p.latestReleaseErr != nil {
		return nil, p.latestReleaseErr
	}

	if p.latestRelease == nil {
		return nil, provider.ErrNoRelease
	}

	return p.latestRelease, nil
}

func (p *providerStub) GetCommitsSince(_ context.Context, ref string) ([]provider.CommitEntry, error) {
	p.getCommitsSinceOf = append(p.getCommitsSinceOf, ref)

	if p.commitsErr != nil {
		return nil, p.commitsErr
	}

	if p.commitsByRef != nil {
		entries, exists := p.commitsByRef[ref]
		if !exists || len(entries) == 0 {
			return []provider.CommitEntry{}, nil
		}

		result := make([]provider.CommitEntry, len(entries))
		copy(result, entries)

		return result, nil
	}

	if len(p.commits) == 0 {
		return []provider.CommitEntry{}, nil
	}

	return p.commits, nil
}

func (p *providerStub) CreateReleasePR(
	_ context.Context,
	opts provider.ReleasePROptions,
) (*provider.PullRequest, error) {
	p.createPRCalls++

	number := p.createPRCalls

	pr := &provider.PullRequest{
		Number: number,
		Title:  opts.Title,
		Body:   opts.Body,
		URL:    fmt.Sprintf("https://example.com/pr/%d", number),
		Branch: opts.ReleaseBranch,
	}

	p.pullRequests[opts.ReleaseBranch] = pr

	return pr, nil
}

func (p *providerStub) UpdateReleasePR(context.Context, int, provider.ReleasePROptions) error {
	p.updatePRCalls++

	return nil
}

func (p *providerStub) FindReleasePR(_ context.Context, branch string) (*provider.PullRequest, error) {
	pr, exists := p.pullRequests[branch]
	if !exists {
		return nil, provider.ErrNoPR
	}

	return pr, nil
}

func (p *providerStub) FindOpenPendingReleasePRs(context.Context, string) ([]*provider.PullRequest, error) {
	if p.openPending != nil {
		return p.openPending, nil
	}

	pending := make([]*provider.PullRequest, 0, len(p.pullRequests))

	for _, pullRequest := range p.pullRequests {
		pending = append(pending, pullRequest)
	}

	return pending, nil
}

func (p *providerStub) FindMergedReleasePR(context.Context, string) (*provider.PullRequest, error) {
	if p.mergedPR == nil {
		return nil, provider.ErrNoPR
	}

	return p.mergedPR, nil
}

func (p *providerStub) CreateRelease(_ context.Context, opts provider.ReleaseOptions) (*provider.Release, error) {
	p.createReleaseCalls++

	release := &provider.Release{
		TagName: opts.TagName,
		Name:    opts.Name,
		Body:    opts.Body,
		URL:     "https://example.com/releases/" + opts.TagName,
	}

	p.latestRelease = release

	return release, nil
}

func (p *providerStub) MarkReleasePRPending(_ context.Context, number int) error {
	p.markPendingCalls = append(p.markPendingCalls, number)

	return nil
}

func (p *providerStub) MarkReleasePRTagged(_ context.Context, number int) error {
	p.markTaggedCalls = append(p.markTaggedCalls, number)

	return nil
}

func (p *providerStub) CreateBranch(_ context.Context, branch, _ string) error {
	p.createdBranches = append(p.createdBranches, branch)

	return nil
}

func (p *providerStub) GetFile(_ context.Context, branch, path string) (string, error) {
	content, exists := p.files[providerFileKey(branch, path)]
	if !exists {
		return "", provider.ErrFileNotFound
	}

	return content, nil
}

func (p *providerStub) UpdateFile(_ context.Context, branch, path, content, message string) error {
	p.files[providerFileKey(branch, path)] = content
	p.updates = append(p.updates, fileUpdate{
		branch:  branch,
		path:    path,
		content: content,
		message: message,
	})

	return nil
}

func (p *providerStub) UpdateFiles(
	_ context.Context,
	branch, base string,
	files map[string]string,
	message string,
) error {
	p.updateFilesCalls++
	p.updateFilesMessages = append(p.updateFilesMessages, message)

	branchPrefix := branch + ":"

	for key := range p.files {
		if strings.HasPrefix(key, branchPrefix) {
			delete(p.files, key)
		}
	}

	basePrefix := base + ":"

	for key, content := range p.files {
		if !strings.HasPrefix(key, basePrefix) {
			continue
		}

		path := strings.TrimPrefix(key, basePrefix)
		p.files[providerFileKey(branch, path)] = content
	}

	for path, content := range files {
		p.files[providerFileKey(branch, path)] = content
		p.updates = append(p.updates, fileUpdate{
			branch:  branch,
			path:    path,
			content: content,
			message: message,
		})
	}

	return nil
}

func (p *providerStub) RepoURL() string {
	return ""
}

func (p *providerStub) PathPrefix() string {
	return ""
}

func TestReleasePreviewBuildMetadata(t *testing.T) {
	t.Parallel()

	t.Run("semver appends short hash as build metadata", func(t *testing.T) {
		t.Parallel()

		// given: semver release with one patch commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: calculating a preview release
		result, err := r.Release(context.Background(), true, true, DefaultPreviewHashLength)

		// then: metadata suffix is appended and base version stays stable
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.4", result.BaseVersion)
		testastic.Equal(t, "1.2.4+abcdef1", result.NextVersion)
		testastic.Equal(t, "v1.2.4", result.BaseTag)
		testastic.Equal(t, "v1.2.4+abcdef1", result.NextTag)
	})

	t.Run("calver appends short hash as build metadata", func(t *testing.T) {
		t.Parallel()

		// given: calver release with one patch commit
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer

		stub := newProviderStub()
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: calculating a preview release
		result, err := r.Release(context.Background(), true, true, DefaultPreviewHashLength)

		// then: calver version also gets build metadata suffix
		testastic.NoError(t, err)
		testastic.True(t, strings.HasPrefix(result.NextVersion, result.BaseVersion+"+"))
		testastic.True(t, strings.HasSuffix(result.NextVersion, "+abcdef1"))
		testastic.Equal(t, "v"+result.BaseVersion, result.BaseTag)
		testastic.Equal(t, "v"+result.NextVersion, result.NextTag)
	})

	t.Run("preview hash length must be positive", func(t *testing.T) {
		t.Parallel()

		// given: semver release with one patch commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: preview hash length is invalid
		_, err := r.Release(context.Background(), true, true, 0)

		// then: validation error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidPreviewHashLength)
	})
}

func TestReleaseSemVerPreMajorBumps(t *testing.T) {
	t.Parallel()

	t.Run("breaking changes do not jump to 1.0.0", func(t *testing.T) {
		t.Parallel()

		// given: a pre-1.0.0 semver release with one breaking commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat(api)!: remove deprecated endpoint",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: version bumps to next minor instead of 1.0.0
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.CurrentVersion)
		testastic.Equal(t, "0.5.0", result.NextVersion)
		testastic.Equal(t, "v0.5.0", result.NextTag)
	})

	t.Run("feature commits bump patch before 1.0.0", func(t *testing.T) {
		t.Parallel()

		// given: a pre-1.0.0 semver release with one feature commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "feat: add export command",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: version bumps patch instead of minor
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.CurrentVersion)
		testastic.Equal(t, "0.4.3", result.NextVersion)
		testastic.Equal(t, "v0.4.3", result.NextTag)
	})
}

func TestReleaseAsFooter(t *testing.T) {
	t.Parallel()

	t.Run("forces explicit version without releasable commit", func(t *testing.T) {
		t.Parallel()

		// given: a semver release with only a chore commit and Release-As footer
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: trigger stable release\n\nRelease-As: 1.0.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: explicit version override is used
		testastic.NoError(t, err)
		testastic.Equal(t, "0.4.2", result.CurrentVersion)
		testastic.Equal(t, "1.0.0", result.NextVersion)
		testastic.Equal(t, "v1.0.0", result.NextTag)
		testastic.Equal(t, commit.BumpMajor, result.BumpType)
	})

	t.Run("supports arbitrary semver override", func(t *testing.T) {
		t.Parallel()

		// given: a semver release with Release-As footer for minor update
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch issue\n\nRelease-As: 1.4.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: exact semver override is used
		testastic.NoError(t, err)
		testastic.Equal(t, "1.4.0", result.NextVersion)
		testastic.Equal(t, "v1.4.0", result.NextTag)
		testastic.Equal(t, commit.BumpMinor, result.BumpType)
	})

	t.Run("footer key matching is case-insensitive", func(t *testing.T) {
		t.Parallel()

		// given: a semver release with lowercase release-as footer key
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nrelease-as: 1.3.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: footer key is recognized regardless of casing
		testastic.NoError(t, err)
		testastic.Equal(t, "1.3.0", result.NextVersion)
		testastic.Equal(t, "v1.3.0", result.NextTag)
	})

	t.Run("rejects non-strict override value", func(t *testing.T) {
		t.Parallel()

		// given: a commit with semver missing patch segment
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: 1.3",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: non-strict semver values are rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("rejects v-prefixed override value", func(t *testing.T) {
		t.Parallel()

		// given: a commit with v-prefixed release-as value
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: v1.3.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: values must be strict semver without v-prefix
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("fails on conflicting override values", func(t *testing.T) {
		t.Parallel()

		// given: two commits with different Release-As values
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{
			{
				Hash:    "abcdef1234567890",
				Message: "chore: request release\n\nRelease-As: 1.0.0",
			},
			{
				Hash:    "1234567890abcdef",
				Message: "chore: request different release\n\nRelease-As: 1.1.0",
			},
		}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: conflict is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrConflictingReleaseAs)
	})

	t.Run("fails on invalid override value", func(t *testing.T) {
		t.Parallel()

		// given: a commit with malformed Release-As value
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.4.2"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: not-a-version",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: invalid value is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("fails when override is not greater than current version", func(t *testing.T) {
		t.Parallel()

		// given: a commit requesting the same version as current release
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: 1.2.3",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		_, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: non-incrementing override is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("ignores override for calver", func(t *testing.T) {
		t.Parallel()

		// given: a calver repo with only a Release-As chore commit
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer

		stub := newProviderStub()
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "chore: request release\n\nRelease-As: 1.0.0",
		}}

		r := New(cfg, stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true, false, DefaultPreviewHashLength)

		// then: release-as footer is ignored for calver
		testastic.NoError(t, err)
		testastic.Equal(t, commit.BumpNone, result.BumpType)
		testastic.Equal(t, "", result.NextVersion)
		testastic.Equal(t, "", result.NextTag)
	})
}

func TestReleasePreviewUsesStableBranch(t *testing.T) {
	t.Parallel()

	// given: semver release flow with preview enabled
	cfg := config.Default()

	stub := newProviderStub()
	stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
	stub.commits = []provider.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "fix: patch bug",
	}}

	r := New(cfg, stub)

	// when: creating the first release PR
	first, err := r.Release(context.Background(), false, true, DefaultPreviewHashLength)

	// then: release branch is stable and based on the target branch
	testastic.NoError(t, err)
	testastic.Equal(t, 1, stub.createPRCalls)
	testastic.Equal(t, 0, stub.updatePRCalls)
	testastic.Equal(t, 1, len(stub.createdBranches))
	testastic.Equal(t, "yeet/release-main", stub.createdBranches[0])

	// given: a new head commit changes preview hash
	stub.commits = []provider.CommitEntry{{
		Hash:    "1234567890abcdef",
		Message: "fix: patch bug",
	}}

	// when: running release again
	second, err := r.Release(context.Background(), false, true, DefaultPreviewHashLength)

	// then: same release branch/PR is reused
	testastic.NoError(t, err)
	testastic.Equal(t, 1, stub.createPRCalls)
	testastic.Equal(t, 1, stub.updatePRCalls)
	testastic.Equal(t, 2, len(stub.markPendingCalls))
	testastic.Equal(t, 1, len(stub.createdBranches))
	testastic.Equal(t, first.BaseTag, second.BaseTag)
	testastic.NotEqual(t, first.NextTag, second.NextTag)
}

func TestReleaseAfterFinalizeMergedRelease(t *testing.T) {
	t.Parallel()

	const changelogBody = `# Changelog

## [v0.1.0](https://example.com/compare/v0.0.9...v0.1.0) (2026-03-01)

### Features

- add release flow (abc1234)
`

	t.Run("does not create PR when no commits exist after finalized tag", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR with no commits after its tag
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number: 3,
			URL:    "https://example.com/pr/3",
			Body:   "<!-- yeet-release-tag: v0.1.0 -->",
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]provider.CommitEntry{
			"v0.1.0": {},
		}

		r := New(cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: merged release is finalized and no new release PR is created
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.Release)(nil), result.Release)
		testastic.Equal(t, "v0.1.0", result.Release.TagName)
		testastic.Equal(t, commit.BumpNone, result.BumpType)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 1, len(stub.getCommitsSinceOf))
		testastic.Equal(t, "v0.1.0", stub.getCommitsSinceOf[0])
	})

	t.Run("creates PR when commits exist after finalized tag", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR and new commits after its tag
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v0.0.9"}
		stub.mergedPR = &provider.PullRequest{
			Number: 4,
			URL:    "https://example.com/pr/4",
			Body:   "<!-- yeet-release-tag: v0.1.0 -->",
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(changelogBody)
		stub.commitsByRef = map[string][]provider.CommitEntry{
			"v0.1.0": {{Hash: "abcdef1234567890", Message: "fix: patch after release"}},
		}

		r := New(cfg, stub)

		// when: running release end-to-end
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: merged release is finalized and a new release PR is created for fresh commits
		testastic.NoError(t, err)
		testastic.NotEqual(t, (*provider.Release)(nil), result.Release)
		testastic.Equal(t, "v0.1.0", result.Release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, stub.createPRCalls)
		testastic.NotEqual(t, (*provider.PullRequest)(nil), result.PullRequest)
		testastic.Equal(t, 1, len(stub.getCommitsSinceOf))
		testastic.Equal(t, "v0.1.0", stub.getCommitsSinceOf[0])
	})
}

func TestReleaseReusesSinglePendingPR(t *testing.T) {
	t.Parallel()

	// given: one open pending PR on a legacy release branch
	cfg := config.Default()

	stub := newProviderStub()
	stub.openPending = []*provider.PullRequest{{
		Number: 7,
		URL:    "https://example.com/pr/7",
		Branch: "yeet/release-v0.0.1",
	}}
	stub.commits = []provider.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "feat!: introduce breaking release flow",
	}}

	r := New(cfg, stub)

	// when: computing a new version while a pending PR already exists
	result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

	// then: pending PR is updated instead of creating a second release PR
	testastic.NoError(t, err)
	testastic.Equal(t, "0.1.0", result.BaseVersion)
	testastic.Equal(t, 0, stub.createPRCalls)
	testastic.Equal(t, 1, stub.updatePRCalls)
	testastic.Equal(t, 1, len(stub.markPendingCalls))
	testastic.Equal(t, "yeet/release-v0.0.1", result.PullRequest.Branch)
	testastic.True(t, strings.Contains(result.PullRequest.Body, "<!-- yeet-release-tag: v0.1.0 -->"))
}

func TestReleaseFailsOnMultiplePendingPRs(t *testing.T) {
	t.Parallel()

	// given: more than one open pending release PR
	cfg := config.Default()

	stub := newProviderStub()
	stub.openPending = []*provider.PullRequest{
		{Number: 1, URL: "https://example.com/pr/1", Branch: "yeet/release-v0.0.1"},
		{Number: 2, URL: "https://example.com/pr/2", Branch: "yeet/release-v0.1.0"},
	}
	stub.commits = []provider.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "fix: patch bug",
	}}

	r := New(cfg, stub)

	// when: attempting to create or update release PRs
	_, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

	// then: release fails fast with actionable pending PR details
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, ErrMultiplePendingReleasePRs)
	testastic.True(t, strings.Contains(err.Error(), "https://example.com/pr/1"))
	testastic.True(t, strings.Contains(err.Error(), "https://example.com/pr/2"))
	testastic.Equal(t, 0, stub.createPRCalls)
	testastic.Equal(t, 0, stub.updatePRCalls)
}

func TestReleaseSubjectFormatting(t *testing.T) {
	t.Parallel()

	t.Run("default subject omits branch and tag prefix", func(t *testing.T) {
		t.Parallel()

		// given: default config and one releasable commit
		cfg := config.Default()

		stub := newProviderStub()
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: PR title and commit subject use unscoped release subject
		testastic.NoError(t, err)
		testastic.Equal(t, "chore: release "+result.BaseVersion, result.PullRequest.Title)
		testastic.Equal(t, 1, stub.updateFilesCalls)
		testastic.Equal(t, "chore: release "+result.BaseVersion, stub.updateFilesMessages[0])
		testastic.Equal(t, 1, len(stub.markPendingCalls))
		testastic.True(t, strings.HasPrefix(result.PullRequest.Body, "## ٩(^ᴗ^)۶ release created\n\n"))
		testastic.True(t, strings.Contains(result.PullRequest.Body, "<!-- yeet-release-tag: "+result.BaseTag+" -->"))
		testastic.True(
			t,
			strings.HasSuffix(
				strings.TrimSpace(result.PullRequest.Body),
				"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
			),
		)
		testastic.False(
			t,
			strings.Contains(
				result.Changelog,
				"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
			),
		)

		releaseBranch := "yeet/release-main"
		testastic.Equal(t, result.Changelog, stub.files[providerFileKey(releaseBranch, cfg.Changelog.File)])
	})

	t.Run("optional branch scope uses stable base version in preview", func(t *testing.T) {
		t.Parallel()

		// given: branch scope enabled and preview release
		cfg := config.Default()
		cfg.Release.SubjectIncludeBranch = true

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3"}
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: creating a preview release PR
		result, err := r.Release(context.Background(), false, true, DefaultPreviewHashLength)

		// then: PR title and commit subject include branch and stable base version
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.4", result.BaseVersion)
		testastic.Equal(t, "1.2.4+abcdef1", result.NextVersion)
		testastic.Equal(t, "chore(main): release 1.2.4", result.PullRequest.Title)
		testastic.Equal(t, 1, stub.updateFilesCalls)
		testastic.Equal(t, "chore(main): release 1.2.4", stub.updateFilesMessages[0])
		testastic.True(t, strings.Contains(result.PullRequest.Body, "<!-- yeet-release-tag: v1.2.4 -->"))
	})

	t.Run("custom header and footer wrap PR body only", func(t *testing.T) {
		t.Parallel()

		// given: custom PR body header/footer and one releasable commit
		cfg := config.Default()
		cfg.Release.PRBodyHeader = "## Release checklist\n- [ ] smoke test"
		cfg.Release.PRBodyFooter = "Please review"

		stub := newProviderStub()
		stub.commits = []provider.CommitEntry{{
			Hash:    "abcdef1234567890",
			Message: "fix: patch bug",
		}}

		r := New(cfg, stub)

		// when: creating a release PR
		result, err := r.Release(context.Background(), false, false, DefaultPreviewHashLength)

		// then: PR body includes custom wrapper text while changelog content stays clean
		testastic.NoError(t, err)
		testastic.True(t, strings.HasPrefix(result.PullRequest.Body, cfg.Release.PRBodyHeader+"\n\n"))
		testastic.True(t, strings.HasSuffix(strings.TrimSpace(result.PullRequest.Body), cfg.Release.PRBodyFooter))
		testastic.False(
			t,
			strings.Contains(
				result.PullRequest.Body,
				"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
			),
		)
		testastic.False(t, strings.Contains(result.Changelog, cfg.Release.PRBodyHeader))
		testastic.False(t, strings.Contains(result.Changelog, cfg.Release.PRBodyFooter))
	})
}

func TestReleasePRBody(t *testing.T) {
	t.Parallel()

	t.Run("defaults include generated header and footer", func(t *testing.T) {
		t.Parallel()

		// given: releaser with default config
		r := New(config.Default(), newProviderStub())
		changelogBody := "## v1.2.4 (2026-03-01)\n\n### Bug Fixes\n\n- patch issue (abc1234)\n"

		// when: building PR body
		body := r.releasePRBody(changelogBody, "v1.2.4")

		// then: changelog is wrapped by default header and footer notes
		testastic.Equal(
			t,
			"## ٩(^ᴗ^)۶ release created\n\n"+
				strings.TrimSpace(changelogBody)+
				"\n\n<!-- yeet-release-tag: v1.2.4 -->"+
				"\n\n_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
			body,
		)
	})

	t.Run("custom header and footer surround changelog", func(t *testing.T) {
		t.Parallel()

		// given: releaser with custom PR body wrapper text
		cfg := config.Default()
		cfg.Release.PRBodyHeader = "Header"
		cfg.Release.PRBodyFooter = "Footer"

		r := New(cfg, newProviderStub())

		// when: building PR body
		body := r.releasePRBody("## v1.2.4", "v1.2.4")

		// then: body contains header, changelog, and footer in order
		testastic.Equal(t, "Header\n\n## v1.2.4\n\n<!-- yeet-release-tag: v1.2.4 -->\n\nFooter", body)
	})

	t.Run("empty wrapper fields keep changelog only", func(t *testing.T) {
		t.Parallel()

		// given: releaser with both wrapper fields disabled
		cfg := config.Default()
		cfg.Release.PRBodyHeader = ""
		cfg.Release.PRBodyFooter = ""

		r := New(cfg, newProviderStub())

		// when: building PR body
		body := r.releasePRBody("## v1.2.4\n", "v1.2.4")

		// then: body is the changelog without extra sections
		testastic.Equal(t, "## v1.2.4\n\n<!-- yeet-release-tag: v1.2.4 -->", body)
	})
}

func TestFinalizeMergedReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("creates release from latest changelog entry and marks PR tagged", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR and changelog entry on main
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 42,
			URL:    "https://example.com/pr/42",
			Body:   "<!-- yeet-release-tag: v1.2.3 -->",
			Branch: "yeet/release-main",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)

## [v1.2.2](https://example.com/compare/v1.2.1...v1.2.2) (2026-02-20)

### Bug Fixes

- fix bug (def5678)
`)

		r := New(cfg, stub)

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: release is created from matching changelog entry and PR is marked tagged
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 42, stub.markTaggedCalls[0])
		testastic.True(t, strings.Contains(release.Body, "## [v1.2.3]"))
		testastic.False(t, strings.Contains(release.Body, "## [v1.2.2]"))
	})

	t.Run("falls back to legacy release branch tag without marker", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR from legacy versioned branch naming
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 33,
			URL:    "https://example.com/pr/33",
			Branch: "yeet/release-v1.2.3",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature (abc1234)
`)

		r := New(cfg, stub)

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: fallback branch tag is used
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 33, stub.markTaggedCalls[0])
	})

	t.Run("skips creation when latest release already exists", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR already tagged in provider releases
		cfg := config.Default()

		stub := newProviderStub()
		stub.latestRelease = &provider.Release{TagName: "v1.2.3", URL: "https://example.com/releases/v1.2.3"}
		stub.mergedPR = &provider.PullRequest{
			Number: 9,
			URL:    "https://example.com/pr/9",
			Body:   "<!-- yeet-release-tag: v1.2.3 -->",
			Branch: "yeet/release-main",
		}

		r := New(cfg, stub)

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: existing release is reused and PR is still marked tagged
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 1, len(stub.markTaggedCalls))
		testastic.Equal(t, 9, stub.markTaggedCalls[0])
	})

	t.Run("returns no-pr error when no merged pending release PR exists", func(t *testing.T) {
		t.Parallel()

		// given: no merged pending release PR
		r := New(config.Default(), newProviderStub())

		// when: finalizing merged release PR
		release, err := r.finalizeMergedReleasePR(context.Background())

		// then: nothing is finalized
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrNoPR)
		testastic.Equal(t, (*provider.Release)(nil), release)
	})

	t.Run("fails when stable branch PR has no release marker", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR on stable branch without marker
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 25,
			URL:    "https://example.com/pr/25",
			Branch: "yeet/release-main",
		}

		r := New(cfg, stub)

		// when: finalizing merged release PR
		_, err := r.finalizeMergedReleasePR(context.Background())

		// then: marker requirement is enforced for stable branch naming
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseBranch)
		testastic.Equal(t, 0, stub.createReleaseCalls)
	})

	t.Run("fails when matching changelog entry is missing", func(t *testing.T) {
		t.Parallel()

		// given: merged pending release PR but changelog lacks target tag entry
		cfg := config.Default()

		stub := newProviderStub()
		stub.mergedPR = &provider.PullRequest{
			Number: 12,
			URL:    "https://example.com/pr/12",
			Branch: "yeet/release-v1.2.3",
		}
		stub.files[providerFileKey(cfg.Branch, cfg.Changelog.File)] = "# Changelog\n\n## v1.2.2 (2026-02-20)"

		r := New(cfg, stub)

		// when: finalizing merged release PR
		_, err := r.finalizeMergedReleasePR(context.Background())

		// then: missing entry is reported
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrChangelogEntryNotFound)
	})
}

func TestChangelogEntryByTag(t *testing.T) {
	t.Parallel()

	t.Run("extracts linked heading entry", func(t *testing.T) {
		t.Parallel()

		// given: a changelog containing linked version headings
		changelog := strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature

## [v1.2.2](https://example.com/compare/v1.2.1...v1.2.2) (2026-02-20)

### Bug Fixes

- patch
`)

		// when: extracting entry for v1.2.3
		entry, err := changelogEntryByTag(changelog, "v1.2.3")

		// then: only matching section is returned
		testastic.NoError(t, err)
		testastic.True(t, strings.HasPrefix(entry, "## [v1.2.3]"))
		testastic.False(t, strings.Contains(entry, "## [v1.2.2]"))
	})

	t.Run("extracts plain heading entry", func(t *testing.T) {
		t.Parallel()

		// given: a changelog with plain version heading
		changelog := "# Changelog\n\n## v1.2.3 (2026-03-01)\n\n### Features\n\n- add feature\n"

		// when: extracting entry for v1.2.3
		entry, err := changelogEntryByTag(changelog, "v1.2.3")

		// then: plain heading entry is returned
		testastic.NoError(t, err)
		testastic.True(t, strings.HasPrefix(entry, "## v1.2.3"))
	})

	t.Run("returns error for missing tag", func(t *testing.T) {
		t.Parallel()

		// given: a changelog without requested tag
		changelog := "# Changelog\n\n## v1.2.2 (2026-02-20)\n"

		// when: extracting entry for missing tag
		_, err := changelogEntryByTag(changelog, "v1.2.3")

		// then: not found error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrChangelogEntryNotFound)
	})
}

func TestReleaseTagFromBranch(t *testing.T) {
	t.Parallel()

	t.Run("parses tag from release branch", func(t *testing.T) {
		t.Parallel()

		// given: a valid release branch name
		branch := "yeet/release-v1.2.3"

		// when: parsing tag from branch
		tag, err := releaseTagFromBranch(branch)

		// then: release tag is returned
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", tag)
	})

	t.Run("rejects invalid release branch", func(t *testing.T) {
		t.Parallel()

		// given: a non-release branch name
		branch := "feature/something"

		// when: parsing tag from branch
		_, err := releaseTagFromBranch(branch)

		// then: branch format error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseBranch)
	})
}

func TestTagRejectsPreviewTags(t *testing.T) {
	t.Parallel()

	t.Run("rejects build metadata tags", func(t *testing.T) {
		t.Parallel()

		// given: a releaser
		cfg := config.Default()
		stub := newProviderStub()
		r := New(cfg, stub)

		// when: trying to create a preview tag
		_, err := r.Tag(context.Background(), "v1.2.3+abc1234", "")

		// then: tag creation is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrPreviewTagNotAllowed)
		testastic.Equal(t, 0, stub.createReleaseCalls)
	})

	t.Run("rejects prerelease tags", func(t *testing.T) {
		t.Parallel()

		// given: a releaser
		cfg := config.Default()
		stub := newProviderStub()
		r := New(cfg, stub)

		// when: trying to create a prerelease tag
		_, err := r.Tag(context.Background(), "v1.2.3-rc.1", "")

		// then: tag creation is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrPreviewTagNotAllowed)
		testastic.Equal(t, 0, stub.createReleaseCalls)
	})

	t.Run("allows stable tags with hyphenated prefix", func(t *testing.T) {
		t.Parallel()

		// given: a releaser with a hyphenated tag prefix
		cfg := config.Default()
		cfg.TagPrefix = "release-"

		stub := newProviderStub()
		r := New(cfg, stub)

		// when: creating a stable tag
		_, err := r.Tag(context.Background(), "release-1.2.3", "")

		// then: tag is accepted
		testastic.NoError(t, err)
		testastic.Equal(t, 1, stub.createReleaseCalls)
	})
}

func TestUpdateReleaseBranchFiles(t *testing.T) {
	t.Parallel()

	t.Run("updates configured version files", func(t *testing.T) {
		t.Parallel()

		// given: releaser with one configured version file containing yeet markers
		cfg := config.Default()
		cfg.VersionFiles = []string{"VERSION.txt"}

		stub := newProviderStub()
		branch := "yeet/release-v1.2.4"
		stub.files[providerFileKey(cfg.Branch, "VERSION.txt")] = "version=1.2.3 # x-yeet-version"

		r := New(cfg, stub)

		result := &Result{
			NextVersion: "1.2.4",
			NextTag:     "v1.2.4",
			Changelog:   "## v1.2.4 (2026-03-01)\n",
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: changelog and version file are updated
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(stub.updates))
		testastic.Equal(t, "version=1.2.4 # x-yeet-version", stub.files[providerFileKey(branch, "VERSION.txt")])
	})

	t.Run("skips version files without yeet markers", func(t *testing.T) {
		t.Parallel()

		// given: releaser with one configured version file without markers
		cfg := config.Default()
		cfg.VersionFiles = []string{"VERSION.txt"}

		stub := newProviderStub()
		branch := "yeet/release-v1.2.4"
		stub.files[providerFileKey(cfg.Branch, "VERSION.txt")] = "version=1.2.3"

		r := New(cfg, stub)

		result := &Result{
			NextVersion: "1.2.4",
			NextTag:     "v1.2.4",
			Changelog:   "## v1.2.4 (2026-03-01)\n",
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: only changelog is updated
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(stub.updates))
		testastic.Equal(t, "version=1.2.3", stub.files[providerFileKey(branch, "VERSION.txt")])
	})

	t.Run("prepends changelog entry and preserves existing history style", func(t *testing.T) {
		t.Parallel()

		// given: existing changelog without top header and a new release entry
		cfg := config.Default()

		stub := newProviderStub()
		branch := "yeet/release-v0.1.1"
		changelogPath := providerFileKey(cfg.Branch, cfg.Changelog.File)
		stub.files[changelogPath] = strings.TrimSpace(`## [v0.1.0](https://example.com/compare/v0.0.9...v0.1.0) (2026-03-01)

### Features

- first release entry (abc1234)
`)

		r := New(cfg, stub)

		result := &Result{
			NextVersion: "0.1.1",
			NextTag:     "v0.1.1",
			Changelog: strings.TrimSpace(`## [v0.1.1](https://example.com/compare/v0.1.0...v0.1.1) (2026-03-02)

### Bug Fixes

- follow-up fix (def5678)
`),
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), branch, result)

		// then: new entry is prepended without replacing old release content
		testastic.NoError(t, err)

		updated := stub.files[providerFileKey(branch, cfg.Changelog.File)]
		testastic.False(t, strings.HasPrefix(updated, "# Changelog"))
		testastic.True(t, strings.Contains(updated, "## [v0.1.1]"))
		testastic.True(t, strings.Contains(updated, "## [v0.1.0]"))

		newEntryIndex := strings.Index(updated, "## [v0.1.1]")
		oldEntryIndex := strings.Index(updated, "## [v0.1.0]")

		testastic.True(t, newEntryIndex >= 0)
		testastic.True(t, oldEntryIndex >= 0)
		testastic.True(t, newEntryIndex < oldEntryIndex)
	})

	t.Run("fails when configured version file is missing", func(t *testing.T) {
		t.Parallel()

		// given: releaser with a missing configured version file
		cfg := config.Default()
		cfg.VersionFiles = []string{"VERSION.txt"}

		r := New(cfg, newProviderStub())

		result := &Result{
			NextVersion: "1.2.4",
			NextTag:     "v1.2.4",
			Changelog:   "## v1.2.4 (2026-03-01)\n",
		}

		// when: updating release branch files
		err := r.updateReleaseBranchFiles(context.Background(), "yeet/release-v1.2.4", result)

		// then: missing file error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrFileNotFound)
	})
}
