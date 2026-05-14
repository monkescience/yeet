package fixture

import (
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/monkescience/testastic"
)

// WriteRepo initializes a fresh git repository under a temp directory with a
// single `origin` remote pointing at remoteURL. The returned path is the
// repository root, suitable for `testastic.WithRunWorkDir(...)`.
//
// The repository contains no commits and no working-tree files; it exists so
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
