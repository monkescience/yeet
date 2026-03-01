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

	latestRelease    *provider.Release
	latestReleaseErr error

	commits    []provider.CommitEntry
	commitsErr error

	pullRequests map[string]*provider.PullRequest

	createPRCalls int
	updatePRCalls int

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

func (p *providerStub) GetCommitsSince(context.Context, string) ([]provider.CommitEntry, error) {
	if p.commitsErr != nil {
		return nil, p.commitsErr
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

func (p *providerStub) CreateRelease(_ context.Context, _ provider.ReleaseOptions) (*provider.Release, error) {
	p.createReleaseCalls++

	return &provider.Release{}, nil
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

	// then: release branch is based on base tag
	testastic.NoError(t, err)
	testastic.Equal(t, 1, stub.createPRCalls)
	testastic.Equal(t, 0, stub.updatePRCalls)
	testastic.Equal(t, 1, len(stub.createdBranches))
	testastic.Equal(t, "yeet/release-v1.2.4", stub.createdBranches[0])

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
	testastic.Equal(t, 1, len(stub.createdBranches))
	testastic.Equal(t, first.BaseTag, second.BaseTag)
	testastic.NotEqual(t, first.NextTag, second.NextTag)
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
		stub.files[providerFileKey(branch, "VERSION.txt")] = "version=1.2.3 # x-yeet-version"

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
		stub.files[providerFileKey(branch, "VERSION.txt")] = "version=1.2.3"

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
