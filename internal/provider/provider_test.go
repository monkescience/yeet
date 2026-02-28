package provider_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/provider"
)

func TestDetectFromRemote(t *testing.T) {
	t.Parallel()

	t.Run("github ssh", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub SSH remote URL
		url := "git@github.com:owner/repo.git"

		// when: detecting provider
		info, err := provider.DetectFromRemote(url)

		// then: GitHub is detected
		testastic.NoError(t, err)
		testastic.Equal(t, "github.com", info.Host)
		testastic.Equal(t, "owner", info.Owner)
		testastic.Equal(t, "repo", info.Repo)
		testastic.Equal(t, "github", info.ProviderType())
	})

	t.Run("github https", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub HTTPS remote URL
		url := "https://github.com/owner/repo.git"

		// when: detecting provider
		info, err := provider.DetectFromRemote(url)

		// then: GitHub is detected
		testastic.NoError(t, err)
		testastic.Equal(t, "github.com", info.Host)
		testastic.Equal(t, "owner", info.Owner)
		testastic.Equal(t, "repo", info.Repo)
		testastic.Equal(t, "github", info.ProviderType())
	})

	t.Run("github https without .git", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub HTTPS remote URL without .git suffix
		url := "https://github.com/owner/repo"

		// when: detecting provider
		info, err := provider.DetectFromRemote(url)

		// then: GitHub is detected
		testastic.NoError(t, err)
		testastic.Equal(t, "owner", info.Owner)
		testastic.Equal(t, "repo", info.Repo)
	})

	t.Run("gitlab ssh", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab SSH remote URL
		url := "git@gitlab.com:group/project.git"

		// when: detecting provider
		info, err := provider.DetectFromRemote(url)

		// then: GitLab is detected
		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab.com", info.Host)
		testastic.Equal(t, "group", info.Owner)
		testastic.Equal(t, "project", info.Repo)
		testastic.Equal(t, "gitlab", info.ProviderType())
	})

	t.Run("self-hosted gitlab", func(t *testing.T) {
		t.Parallel()

		// given: a self-hosted GitLab SSH remote URL
		url := "git@git.company.com:team/service.git"

		// when: detecting provider
		info, err := provider.DetectFromRemote(url)

		// then: defaults to GitLab for unknown hosts
		testastic.NoError(t, err)
		testastic.Equal(t, "git.company.com", info.Host)
		testastic.Equal(t, "team", info.Owner)
		testastic.Equal(t, "service", info.Repo)
		testastic.Equal(t, "gitlab", info.ProviderType())
	})

	t.Run("invalid url", func(t *testing.T) {
		t.Parallel()

		// given: an unparseable URL
		url := "not-a-valid-url"

		// when: detecting provider
		_, err := provider.DetectFromRemote(url)

		// then: error is returned
		testastic.Error(t, err)
	})
}

func TestParseCommits(t *testing.T) {
	t.Parallel()

	t.Run("parses commit entries", func(t *testing.T) {
		t.Parallel()

		// given: raw commit entries
		entries := []provider.CommitEntry{
			{Hash: "abc123", Message: "feat: add new feature"},
			{Hash: "def456", Message: "fix(auth): resolve token issue"},
			{Hash: "ghi789", Message: "not a conventional commit"},
		}

		// when: parsing commits
		commits := provider.ParseCommits(entries)

		// then: all commits are parsed
		testastic.Equal(t, 3, len(commits))
		testastic.Equal(t, "feat", commits[0].Type)
		testastic.Equal(t, "fix", commits[1].Type)
		testastic.Equal(t, "auth", commits[1].Scope)
		testastic.Equal(t, "", commits[2].Type)
	})
}

func TestParseCommitsEmpty(t *testing.T) {
	t.Parallel()

	// given: no commit entries
	var entries []provider.CommitEntry

	// when: parsing commits
	commits := provider.ParseCommits(entries)

	// then: empty result
	testastic.Equal(t, 0, len(commits))
}

// Compile-time interface checks.
var (
	_ provider.Provider = (*provider.GitHub)(nil)
	_ provider.Provider = (*provider.GitLab)(nil)
)

// Compile-time strategy checks.
var _ commit.BumpType = commit.BumpMajor
