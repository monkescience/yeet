package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

// TestReleaseAutoDetect exercises the auto-detection path: yeet reads the
// `origin` remote URL from the local git config and resolves provider, host,
// and coordinates from it.
//
// Each subtest spins up a fake provider server, points the matching
// PROVIDER_URL env var at it, and runs yeet from a CWD that is the test repo.
// A minimal config without repository.owner/repo/project forces the binary
// down the auto-detect branch covered by internal/cli/helpers.go and
// internal/provider/provider.go URL parsers.
func TestReleaseAutoDetect(t *testing.T) {
	t.Run("github https remote", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "acme",
			Repo:        "repo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		repoDir := fixture.WriteRepo(t, "https://github.com/acme/repo.git")
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("github scp-like ssh remote", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "acme",
			Repo:        "repo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		repoDir := fixture.WriteRepo(t, "git@github.com:acme/repo.git")
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("gitlab nested group remote", func(t *testing.T) {
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/sub/repo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		repoDir := fixture.WriteRepo(t, "https://gitlab.com/group/sub/repo.git")
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"GITLAB_TOKEN=test-token",
				"GITLAB_URL="+server.URL+"/api/v4",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("azuredevops cloud https remote", func(t *testing.T) {
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v1.0.0",
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		repoDir := fixture.WriteRepo(t, "https://dev.azure.com/contoso/platform/_git/yeet")
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"AZURE_DEVOPS_EXT_PAT=test-token",
				"AZURE_DEVOPS_URL="+server.URL,
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("azuredevops ssh remote", func(t *testing.T) {
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v1.0.0",
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		repoDir := fixture.WriteRepo(t, "git@ssh.dev.azure.com:v3/contoso/platform/yeet")
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"AZURE_DEVOPS_EXT_PAT=test-token",
				"AZURE_DEVOPS_URL="+server.URL,
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("azuredevops legacy visualstudio remote", func(t *testing.T) {
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v1.0.0",
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		repoDir := fixture.WriteRepo(t, "https://contoso.visualstudio.com/platform/_git/yeet")
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"AZURE_DEVOPS_EXT_PAT=test-token",
				"AZURE_DEVOPS_URL="+server.URL,
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})
}
