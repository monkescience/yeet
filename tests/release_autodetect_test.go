package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

var autodetectHistory = []fixture.RepoCommit{
	{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
	{Message: "feat: add a thing"},
}

func TestReleaseAutoDetect(t *testing.T) {
	t.Parallel()

	t.Run("github https remote auto-detects owner/repo", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin is an https GitHub URL, plus a fake GitHub server
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/acme/repo.git", "main", autodetectHistory)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "acme",
			Repo:          "repo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: auto-detect resolves the GitHub coordinates and dry-run succeeds
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_https/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github scp-style ssh remote auto-detects owner/repo", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin is an scp-style git@ URL
		repoDir, shas := fixture.WriteRepoWithHistory(t, "git@github.com:acme/repo.git", "main", autodetectHistory)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "acme",
			Repo:          "repo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: scp parser yields the same owner/repo as the https variant
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_ssh/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github insteadOf remote rewrite auto-detects owner/repo and local branch", func(t *testing.T) {
		t.Parallel()

		// given: a git repo using a shorthand origin URL rewritten by url.insteadOf
		repoDir, shas := fixture.WriteRepoWithHistory(t, "gh:acme/repo.git", "main", autodetectHistory)
		fixture.AddInsteadOfRewrite(t, repoDir, "https://github.com/", "gh:")

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "acme",
			Repo:          "repo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		// when: running without CI branch env vars so yeet reads branch and remote config locally
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=",
				"CI_COMMIT_BRANCH=",
				"BRANCH_NAME=",
			),
		)

		// then: yeet rewrites gh: to github.com, detects acme/repo, and reads the local main branch
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_https/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("gitlab nested-group remote auto-detects full project path", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin points at a nested GitLab group
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/sub/repo.git", "main", autodetectHistory)

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/sub/repo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet preserves the full nested project path
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_nested_group/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("azuredevops cloud https remote auto-detects org/project/repo", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin is an Azure DevOps cloud https URL
		repoDir, shas := fixture.WriteRepoWithHistory(
			t, "https://dev.azure.com/contoso/platform/_git/yeet", "main", autodetectHistory)

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: the Azure URL parser yields org/project/repo and dry-run succeeds
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/azuredevops_https/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("azuredevops ssh remote auto-detects org/project/repo", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin is the v3 ssh form
		repoDir, shas := fixture.WriteRepoWithHistory(
			t, "git@ssh.dev.azure.com:v3/contoso/platform/yeet", "main", autodetectHistory)

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: the v3 ssh parser resolves the same coordinates as the https form
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/azuredevops_ssh/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("azuredevops legacy visualstudio remote auto-detects coordinates", func(t *testing.T) {
		t.Parallel()

		// given: a git repo whose origin uses the legacy *.visualstudio.com host
		repoDir, shas := fixture.WriteRepoWithHistory(
			t, "https://contoso.visualstudio.com/platform/_git/yeet", "main", autodetectHistory)

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		// when: running `yeet release --dry-run` from inside that repo
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: the legacy host parser still maps to the modern coordinates
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/azuredevops_visualstudio/stdout.expected.txt",
			result.Stdout,
		)
	})
}
