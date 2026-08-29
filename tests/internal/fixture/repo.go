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

// WriteRepo creates an empty temporary repository with remoteURL as origin.
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

// RepoCommit describes one commit in a scripted fixture repository.
type RepoCommit struct {
	Message string
	Files   map[string]string
	Tag     string
	// Branch creates the commit on a side branch, then restores the release branch.
	Branch string
}

// WriteRepoWithHistory creates deterministic commits and returns their SHAs in order.
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
			// go-git rejects empty commits, so use the message as deterministic content.
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

// WriteRepoWithTaggedHistory creates a tagged base and one feature commit.
func WriteRepoWithTaggedHistory(t *testing.T, remoteURL, branch, tag string) (string, string, string) {
	t.Helper()

	dir, shas := WriteRepoWithHistory(t, remoteURL, branch, []RepoCommit{
		{Message: "chore: initial commit", Files: map[string]string{"README.md": "fixture repo\n"}, Tag: tag},
		{Message: "feat: add local feature", Files: map[string]string{"feature.txt": "local feature\n"}},
	})

	return dir, shas[0], shas[len(shas)-1]
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

	repositoryConfig.URLs = append(repositoryConfig.URLs, &config.URL{
		Name:       baseURL,
		InsteadOfs: []string{insteadOf},
	})

	err = repository.SetConfig(repositoryConfig)
	testastic.NoError(t, err)
}
