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

// WriteRepoWithTaggedHistory initializes a repository on branch with one
// commit tagged tag and one "feat: add local feature" commit on top. It
// returns the repository root and the head commit SHA so tests can align a
// fake provider's branch head with the local checkout.
func WriteRepoWithTaggedHistory(t *testing.T, remoteURL, branch, tag string) (string, string) {
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

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tagged := commitFile(t, dir, worktree, "README.md", "fixture repo\n", "chore: initial commit", base)

	_, err = repository.CreateTag(tag, tagged, nil)
	testastic.NoError(t, err)

	head := commitFile(
		t, dir, worktree,
		"feature.txt", "local feature\n",
		"feat: add local feature",
		base.Add(time.Minute),
	)

	return dir, head.String()
}

func commitFile(
	t *testing.T,
	dir string,
	worktree *git.Worktree,
	path, content, message string,
	when time.Time,
) plumbing.Hash {
	t.Helper()

	const filePerm = 0o600

	err := os.WriteFile(filepath.Join(dir, path), []byte(content), filePerm)
	testastic.NoError(t, err)

	_, err = worktree.Add(path)
	testastic.NoError(t, err)

	signature := &object.Signature{Name: "yeet test", Email: "yeet@example.test", When: when}

	hash, err := worktree.Commit(message, &git.CommitOptions{Author: signature, Committer: signature})
	testastic.NoError(t, err)

	return hash
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
