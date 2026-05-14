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

	t.Run("azuredevops happy path", func(t *testing.T) {
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

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(
				"AZURE_DEVOPS_EXT_PAT=test-token",
				"AZURE_DEVOPS_URL="+server.URL,
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
	t.Run("azuredevops creates pull request", func(t *testing.T) {
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

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(
				"AZURE_DEVOPS_EXT_PAT=test-token",
				"AZURE_DEVOPS_URL="+server.URL,
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab creates merge request", func(t *testing.T) {
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
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(
				"GITLAB_TOKEN=test-token",
				"GITLAB_URL="+server.URL+"/api/v4",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
	})

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

func TestReleaseAutoMerge(t *testing.T) {
	t.Run("github finalizes merged pending release", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                "testorg",
			Repo:                 "testrepo",
			LatestTag:            "v1.0.0",
			BoundarySHA:          "boundary-sha",
			MergedPendingRelease: true,
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

	t.Run("gitlab auto-merge tags the release", func(t *testing.T) {
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
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(
				"GITLAB_TOKEN=test-token",
				"GITLAB_URL="+server.URL+"/api/v4",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github auto-merge tags the release", func(t *testing.T) {
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
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseChannelAndVersionFiles(t *testing.T) {
	t.Run("github prerelease channel creates pr", func(t *testing.T) {
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
			Channels: map[string]fixture.ChannelOptions{
				"beta": {Branch: "beta", Prerelease: "beta"},
			},
		})

		result := binary.RunWithOptions(t,
			[]string{"release", "--channel", "beta", "--config", configPath},
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=beta",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github version files release creates pr", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
			Files: map[string]string{
				"VERSION.txt": "1.0.0 # x-yeet-version\n",
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
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

func TestReleaseCalVer(t *testing.T) {
	t.Run("github calver release creates pr", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v2025.11.1",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v2025.11.1"},
			},
			Files: map[string]string{
				"VERSION.txt": "2025.11.1 # x-yeet-version\n",
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.0M.MICRO",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
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

func TestReleaseJSONPointerVersionFile(t *testing.T) {
	t.Run("github updates package json", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
			Files: map[string]string{
				"package.json": `{"name":"yeet","version":"1.0.0"}`,
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			VersionFiles: []fixture.VersionFileOptions{
				{Path: "package.json", Format: "json", JSONPointer: "/version"},
			},
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

func TestReleaseBreakingChange(t *testing.T) {
	t.Run("github major bump on breaking change", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat!: redesign api\n\nBREAKING CHANGE: removed v1 endpoints"},
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
		testastic.Contains(t, result.Stdout, "v2.0.0")
	})
}

func TestReleaseMultiTarget(t *testing.T) {
	t.Run("github filters to one target", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add api endpoint", Files: []string{"api/handler.go"}},
				{SHA: "web-sha", Message: "feat: update web ui", Files: []string{"web/index.html"}},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0", Files: []string{"CHANGELOG.md"}},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			Targets: []fixture.TargetOptions{
				{Name: "api", Path: "api/", TagPrefix: "api/v"},
				{Name: "web", Path: "web/", TagPrefix: "web/v"},
			},
		})

		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--target", "api", "--config", configPath},
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "api")
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
