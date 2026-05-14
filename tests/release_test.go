package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseDryRun(t *testing.T) {
	t.Run("gitlab happy path", func(t *testing.T) {
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/service",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
		})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(
				"GITLAB_TOKEN=test-token",
				"GITLAB_URL="+server.URL+"/api/v4",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("github happy path", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})
}

func TestReleaseCreatesPR(t *testing.T) {
	t.Run("github creates release pr", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseConfigErrors(t *testing.T) {
	t.Run("missing config file", func(t *testing.T) {
		tempDir := t.TempDir()

		result := binary.RunWithOptions(t,
			[]string{"release"},
			testastic.WithRunWorkDir(tempDir),
		)

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "configuration file not found")
		testastic.Contains(t, result.Stderr, "run `yeet init` or pass --config")
	})

	t.Run("malformed yaml", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".yeet.yaml")

		const filePerm = 0o600

		err := os.WriteFile(configPath, []byte("release: ["), filePerm)
		testastic.NoError(t, err)

		result := binary.Run(t, "release", "--config", configPath)

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "invalid configuration")
		testastic.Contains(t, result.Stderr, "parse config")
	})

	t.Run("missing github token", func(t *testing.T) {
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(
				"GITHUB_TOKEN=",
				"GH_TOKEN=",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "provider setup failed")
		testastic.Contains(t, result.Stderr, "GITHUB_TOKEN or GH_TOKEN")
	})

	t.Run("unsupported host without provider", func(t *testing.T) {
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Branch: "main",
			Host:   "code.company.com",
			Owner:  "platform",
			Repo:   "yeet",
		})

		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "repository resolution failed")
		testastic.Contains(t, result.Stderr, "unsupported remote host")
	})
}
