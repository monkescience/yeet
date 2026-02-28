//nolint:testpackage // This test validates unexported release branch update behavior.
package release

import (
	"context"
	"testing"

	"github.com/monkescience/testastic"
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
}

func newProviderStub() *providerStub {
	return &providerStub{files: make(map[string]string)}
}

func providerFileKey(branch, path string) string {
	return branch + ":" + path
}

func (p *providerStub) GetLatestRelease(context.Context) (*provider.Release, error) {
	return nil, provider.ErrNoRelease
}

func (p *providerStub) GetCommitsSince(context.Context, string) ([]provider.CommitEntry, error) {
	return nil, nil
}

func (p *providerStub) CreateReleasePR(context.Context, provider.ReleasePROptions) (*provider.PullRequest, error) {
	return &provider.PullRequest{}, nil
}

func (p *providerStub) UpdateReleasePR(context.Context, int, provider.ReleasePROptions) error {
	return nil
}

func (p *providerStub) FindReleasePR(context.Context, string) (*provider.PullRequest, error) {
	return nil, provider.ErrNoPR
}

func (p *providerStub) CreateRelease(context.Context, provider.ReleaseOptions) (*provider.Release, error) {
	return &provider.Release{}, nil
}

func (p *providerStub) CreateBranch(context.Context, string, string) error {
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
