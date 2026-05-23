package fixture

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/monkescience/testastic"
)

// WriteRepo initializes a fresh git repository under a temp directory with a
// single `origin` remote pointing at remoteURL. The returned path is the
// repository root, suitable for `testastic.WithRunWorkDir(...)`.
//
// The repository contains no commits and no working-tree files. It exists so
// that yeet's auto-detect path can read `.git/config` for the remote URL and
// fall back to env-supplied branch info via GITHUB_REF_NAME.
func WriteRepo(t *testing.T, remoteURL string) string {
	t.Helper()

	dir := t.TempDir()

	repository, err := git.PlainInit(dir, false)
	testastic.NoError(t, err)

	_, err = repository.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	testastic.NoError(t, err)

	return dir
}

// WriteRepoWithBranch initializes a fresh git repository with origin pointing at
// remoteURL and a checked-out branch containing one commit. The branch commit
// lets blackbox tests exercise yeet's local branch fallback instead of CI envs.
func WriteRepoWithBranch(t *testing.T, remoteURL string, branch string) string {
	t.Helper()

	dir := WriteRepo(t, remoteURL)

	repository, err := git.PlainOpen(dir)
	testastic.NoError(t, err)

	worktree, err := repository.Worktree()
	testastic.NoError(t, err)

	err = repository.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD,
		plumbing.NewBranchReferenceName(branch),
	))
	testastic.NoError(t, err)

	const filePerm = 0o600

	err = os.WriteFile(filepath.Join(dir, "README.md"), []byte("fixture repo\n"), filePerm)
	testastic.NoError(t, err)

	_, err = worktree.Add("README.md")
	testastic.NoError(t, err)

	_, err = worktree.Commit("chore: initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "yeet test",
			Email: "yeet@example.test",
			When:  time.Unix(0, 0),
		},
	})
	testastic.NoError(t, err)

	return dir
}

// AddInsteadOfRewrite adds a git `url.<base>.insteadOf` rewrite rule to repoDir.
func AddInsteadOfRewrite(t *testing.T, repoDir string, baseURL string, insteadOf string) {
	t.Helper()

	repository, err := git.PlainOpen(repoDir)
	testastic.NoError(t, err)

	repositoryConfig, err := repository.Config()
	testastic.NoError(t, err)

	if repositoryConfig.URLs == nil {
		repositoryConfig.URLs = make(map[string]*config.URL)
	}

	repositoryConfig.URLs[baseURL] = &config.URL{
		Name:       baseURL,
		InsteadOfs: []string{insteadOf},
	}

	err = repository.SetConfig(repositoryConfig)
	testastic.NoError(t, err)
}
