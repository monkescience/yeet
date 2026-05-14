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

	t.Run("gitlab finalizes merged pending release", func(t *testing.T) {
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:              "group/service",
			LatestTag:            "v1.0.0",
			BoundarySHA:          "boundary-sha",
			MergedPendingRelease: true,
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

func TestReleaseAzureDevOpsFullFlow(t *testing.T) {
	t.Run("azuredevops auto-merge tags the release", func(t *testing.T) {
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
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(
				"AZURE_DEVOPS_EXT_PAT=test-token",
				"AZURE_DEVOPS_URL="+server.URL,
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops finalizes merged pending release", func(t *testing.T) {
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:         "contoso",
			Project:              "platform",
			Repo:                 "yeet",
			LatestTag:            "v1.0.0",
			BoundarySHA:          "boundary-sha",
			MergedPendingRelease: true,
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

	t.Run("azuredevops multiple pending prs error", func(t *testing.T) {
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:    "contoso",
			Project:         "platform",
			Repo:            "yeet",
			LatestTag:       "v1.0.0",
			BoundarySHA:     "boundary-sha",
			MultipleOpenPRs: true,
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

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "multiple pending release PRs/MRs")
	})

	t.Run("azuredevops draft pr blocks auto-merge", func(t *testing.T) {
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v1.0.0",
			BoundarySHA:  "boundary-sha",
			MergeBlocked: true,
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
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(
				"AZURE_DEVOPS_EXT_PAT=test-token",
				"AZURE_DEVOPS_URL="+server.URL,
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "release PR merge blocked")
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

	t.Run("github calver initial release with no prior tag", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: initial feature"},
				{SHA: "boundary-sha", Message: "chore: bootstrap"},
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
	})

	t.Run("github calver year month day format dry run", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: ship feature"},
				{SHA: "boundary-sha", Message: "chore: bootstrap"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.MM.DD.MICRO",
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
	})

	t.Run("github calver isoweek dry run", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: ship feature"},
				{SHA: "boundary-sha", Message: "chore: bootstrap"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.WW.MICRO",
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

	t.Run("github updates array json pointer", func(t *testing.T) {
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
				"manifest.json": `{"packages":[{"name":"yeet","version":"1.0.0"}]}`,
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			VersionFiles: []fixture.VersionFileOptions{
				{Path: "manifest.json", Format: "json", JSONPointer: "/packages/0/version"},
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

	t.Run("github escaped json pointer with tilde", func(t *testing.T) {
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
				"escaped.json": `{"a~b":{"c/d":"1.0.0"}}`,
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			VersionFiles: []fixture.VersionFileOptions{
				{Path: "escaped.json", Format: "json", JSONPointer: "/a~0b/c~1d"},
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

func TestReleasePreMajor(t *testing.T) {
	t.Run("github pre-1.0 feat bumps minor", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v0.3.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v0.3.0"},
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
		testastic.Contains(t, result.Stdout, "v0.3.1")
	})
}

func TestReleaseNoChanges(t *testing.T) {
	t.Run("github reports no release needed", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "docs: tweak readme"},
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
	})
}

func TestReleaseMergeErrors(t *testing.T) {
	t.Run("github reports multiple pending prs", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:           "testorg",
			Repo:            "testrepo",
			LatestTag:       "v1.0.0",
			BoundarySHA:     "boundary-sha",
			MultipleOpenPRs: true,
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

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "multiple pending release PRs/MRs")
	})
}

func TestReleaseMultiTagHistory(t *testing.T) {
	t.Run("github semver orders multiple prior tags", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.2.0",
			ExtraTags:   []string{"v1.0.0", "v1.1.0", "v0.9.0"},
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.2.0"},
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
		testastic.Contains(t, result.Stdout, "v1.3.0")
	})

	t.Run("github calver orders multiple prior calver tags", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v2026.05.0",
			ExtraTags:   []string{"v2025.12.0", "v2026.01.0", "v2026.03.0"},
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release"},
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
	})
}

func TestReleaseMultiTargetPRBody(t *testing.T) {
	t.Run("github builds combined wave pr body for two path targets", func(t *testing.T) {
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

func TestReleaseDerivedTarget(t *testing.T) {
	t.Run("github derived root target aggregates included plans", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add api endpoint", Files: []string{"services/api/handler.go"}},
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
				{Name: "api", Path: "services/api", TagPrefix: "api-v"},
				{
					Name:         "root",
					Type:         "derived",
					Path:         ".",
					TagPrefix:    "v",
					ExcludePaths: []string{"services/api"},
					Includes:     []string{"api"},
				},
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

func TestReleaseMultiTargetCrossProvider(t *testing.T) {
	t.Run("gitlab multi-target resolves per-commit paths", func(t *testing.T) {
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/service",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "feat: add api endpoint", Files: []string{"api/handler.go"}},
				{SHA: "web-sha", Message: "feat: update web ui", Files: []string{"web/index.html"}},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0", Files: []string{"CHANGELOG.md"}},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
			Targets: []fixture.TargetOptions{
				{Name: "api", Path: "api/", TagPrefix: "api/v"},
				{Name: "web", Path: "web/", TagPrefix: "web/v"},
			},
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
	})

	t.Run("azuredevops multi-target resolves per-commit paths", func(t *testing.T) {
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v1.0.0",
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "feat: add api endpoint", Files: []string{"api/handler.go"}},
				{SHA: "web-sha", Message: "feat: update web ui", Files: []string{"web/index.html"}},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0", Files: []string{"CHANGELOG.md"}},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
			Targets: []fixture.TargetOptions{
				{Name: "api", Path: "api/", TagPrefix: "api/v"},
				{Name: "web", Path: "web/", TagPrefix: "web/v"},
			},
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
	})
}

func TestReleaseUpdatesExistingPR(t *testing.T) {
	t.Run("gitlab updates open release mr", func(t *testing.T) {
		manifest := "<!-- yeet-release-manifest\n" +
			`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}` +
			"\n-->"
		existingBody := "## release\n\n### Features\n\n* feat: add a thing\n\n" + manifest + "\n"

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               "boundary-sha",
			ExistingOpenReleasePRBody: existingBody,
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

	t.Run("azuredevops updates open release pr", func(t *testing.T) {
		manifest := "<!-- yeet-release-manifest\n" +
			`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}` +
			"\n-->"
		existingBody := "## release\n\n### Features\n\n* feat: add a thing\n\n" + manifest + "\n"

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:              "contoso",
			Project:                   "platform",
			Repo:                      "yeet",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               "boundary-sha",
			ExistingOpenReleasePRBody: existingBody,
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

	t.Run("github updates open release pr preserving manual section", func(t *testing.T) {
		manifest := "<!-- yeet-release-manifest\n" +
			`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}` +
			"\n-->"
		existingBody := "## release\n\n" +
			"## [v1.1.0](https://example.test/compare/v1.0.0...v1.1.0) (2026-01-01)\n\n" +
			"### Features\n\n* feat: add a thing\n\n" +
			"### Notes\n\nManual reviewer note that must survive an update.\n\n" +
			manifest + "\n"

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                     "testorg",
			Repo:                      "testrepo",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               "boundary-sha",
			ExistingOpenReleasePRBody: existingBody,
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

func TestReleaseAsFooter(t *testing.T) {
	t.Run("github release-as overrides computed version", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: tweak api\n\nRelease-As: 2.5.0"},
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
		testastic.Contains(t, result.Stdout, "v2.5.0")
	})

	t.Run("github release-as major version bump", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.2.3",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "fix: minor bump\n\nRelease-As: 3.0.0"},
				{SHA: "boundary-sha", Message: "chore: release v1.2.3"},
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
		testastic.Contains(t, result.Stdout, "v3.0.0")
	})
}

func TestReleaseCommitOverride(t *testing.T) {
	t.Run("github merged pr body override replaces commit message", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{
					SHA:     "head-sha",
					Message: "chore: squashed merge",
					AssociatedPRBody: "Some PR notes\n\n" +
						"BEGIN_COMMIT_OVERRIDE\n" +
						"feat: overridden first commit\n\n" +
						"fix: overridden second commit\n" +
						"END_COMMIT_OVERRIDE\n",
				},
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
		testastic.Contains(t, result.Stdout, "overridden first commit")
	})
}

func TestReleaseVersionFileErrors(t *testing.T) {
	t.Run("semver target rejects calver marker with suggestion", func(t *testing.T) {
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
				"VERSION.txt": "1.0.0 # x-yeet-month\n",
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

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "x-yeet-month")
	})

	t.Run("calver target rejects semver marker with suggestion", func(t *testing.T) {
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
				"VERSION.txt": "2025.11.1 # x-yeet-major\n",
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

		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "x-yeet-major")
	})

	t.Run("calver target updates month and micro markers", func(t *testing.T) {
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
				"BUILD.txt": "year: 2025  # x-yeet-year\nmonth: 11  # x-yeet-month\nmicro: 1  # x-yeet-micro\n",
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
			VersionFiles: []fixture.VersionFileOptions{{Path: "BUILD.txt"}},
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

func TestReleaseCLIFlagsOverrideConfig(t *testing.T) {
	t.Run("github cli flags override owner repo provider host", func(t *testing.T) {
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "flagorg",
			Repo:        "flagrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: cli override"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "configorg",
			Repo:     "configrepo",
		})

		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run", "--config", configPath,
				"--provider", "github", "--owner", "flagorg", "--repo", "flagrepo",
				"--host", "github.com",
				"--auto-merge-force",
				"--auto-merge-method", "squash",
			},
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab cli project flag clears owner repo", func(t *testing.T) {
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "flaggroup/svc",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "feat: cli override"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Owner:    "configorg",
			Repo:     "configrepo",
		})

		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run", "--config", configPath,
				"--provider", "gitlab", "--project", "flaggroup/svc",
			},
			testastic.WithRunEnv(
				"GITLAB_TOKEN=test-token",
				"GITLAB_URL="+server.URL+"/api/v4",
				"GITHUB_REF_NAME=main",
			),
		)

		testastic.Equal(t, 0, result.ExitCode)
	})
}
