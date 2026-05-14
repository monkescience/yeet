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
	t.Parallel()

	t.Run("github https remote auto-detects owner/repo", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin is an https GitHub URL, plus a fake GitHub server
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

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: auto-detect resolves the GitHub coordinates and dry-run succeeds
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("github scp-style ssh remote auto-detects owner/repo", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin is an scp-style git@ URL
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

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: scp parser yields the same owner/repo as the https variant
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("gitlab nested-group remote auto-detects full project path", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin points at a nested GitLab group
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

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet preserves the full nested project path
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("azuredevops cloud https remote auto-detects org/project/repo", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin is an Azure DevOps cloud https URL
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

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: the Azure URL parser yields org/project/repo and dry-run succeeds
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("azuredevops ssh remote auto-detects org/project/repo", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin is the v3 ssh form
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

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: the v3 ssh parser resolves the same coordinates as the https form
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("azuredevops legacy visualstudio remote auto-detects coordinates", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin uses the legacy *.visualstudio.com host
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

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: the legacy host parser still maps to the modern coordinates
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})
}
