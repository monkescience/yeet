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

// RepoCommit describes one commit in a scripted fixture repository. Files are
// written (or overwritten) relative to the repository root before committing.
// A non-empty Tag creates a lightweight tag on the commit.
type RepoCommit struct {
	Message string
	Files   map[string]string
	Tag     string
	// Branch, when non-empty, puts the commit on that side branch (created at
	// the current release-branch head when it does not exist yet). The release
	// branch is checked out again afterwards, so tags created here stay
	// unreachable from it.
	Branch string
}

// WriteRepoWithHistory initializes a repository on branch whose origin points
// at remoteURL and creates commits in order with fixed timestamps, so the
// resulting SHAs are deterministic across runs and machines. It returns the
// repository root and one SHA per commit, in input order. The head SHA is the
// last element.
func WriteRepoWithHistory(t *testing.T, remoteURL, branch string, commits []RepoCommit) (string, []string) {
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

	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	shas := make([]string, 0, len(commits))
	knownBranches := map[string]bool{}

	for _, commit := range commits {
		if commit.Branch != "" {
			err := worktree.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewBranchReferenceName(commit.Branch),
				Create: !knownBranches[commit.Branch],
			})
			testastic.NoError(t, err)

			knownBranches[commit.Branch] = true
		}

		files := commit.Files
		if len(files) == 0 {
			// go-git rejects empty commits by default; unspecified files fall
			// back to a marker file derived from the message so every commit
			// stays unique and deterministic.
			files = map[string]string{"history.txt": commit.Message + "\n"}
		}

		hash := commitFiles(t, dir, worktree, files, commit.Message, clock)
		shas = append(shas, hash.String())
		clock = clock.Add(time.Minute)

		if commit.Tag != "" {
			_, err := repository.CreateTag(commit.Tag, hash, nil)
			testastic.NoError(t, err)
		}

		if commit.Branch != "" {
			err := worktree.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewBranchReferenceName(branch),
			})
			testastic.NoError(t, err)
		}
	}

	return dir, shas
}

// WriteRepoWithTaggedHistory initializes a repository on branch with one
// commit tagged tag and one "feat: add local feature" commit on top. It
// returns the repository root and the head commit SHA so tests can align a
// fake provider's branch head with the local checkout.
func WriteRepoWithTaggedHistory(t *testing.T, remoteURL, branch, tag string) (string, string) {
	t.Helper()

	dir, shas := WriteRepoWithHistory(t, remoteURL, branch, []RepoCommit{
		{Message: "chore: initial commit", Files: map[string]string{"README.md": "fixture repo\n"}, Tag: tag},
		{Message: "feat: add local feature", Files: map[string]string{"feature.txt": "local feature\n"}},
	})

	return dir, shas[len(shas)-1]
}

func commitFiles(
	t *testing.T,
	dir string,
	worktree *git.Worktree,
	files map[string]string,
	message string,
	when time.Time,
) plumbing.Hash {
	t.Helper()

	const filePerm = 0o600

	const dirPerm = 0o750

	for path, content := range files {
		fullPath := filepath.Join(dir, path)

		err := os.MkdirAll(filepath.Dir(fullPath), dirPerm)
		testastic.NoError(t, err)

		err = os.WriteFile(fullPath, []byte(content), filePerm)
		testastic.NoError(t, err)

		_, err = worktree.Add(path)
		testastic.NoError(t, err)
	}

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
