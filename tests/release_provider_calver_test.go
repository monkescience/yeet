package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseGitLabCalVer(t *testing.T) {
	t.Parallel()

	t.Run("gitlab calver release creates a merge request with the next month/micro", func(t *testing.T) {
		t.Parallel()

		// given: a calver project on GitLab with a prior month tag
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/service",
			LatestTag:   "v2025.11.1",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v2025.11.1"},
			},
			Files: map[string]string{
				"VERSION.txt": "2025.11.1 # x-yeet-version\n",
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "gitlab",
			Branch:       "main",
			Host:         "gitlab.com",
			Project:      "group/service",
			Versioning:   "calver",
			CalVerFormat: "YYYY.0M.MICRO",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet plans the next calver and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseAzureCalVer(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops calver release creates a PR with the next month/micro", func(t *testing.T) {
		t.Parallel()

		// given: a calver project on Azure with a prior month tag
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v2025.11.1",
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v2025.11.1"},
			},
			Files: map[string]string{
				"VERSION.txt": "2025.11.1 # x-yeet-version\n",
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
			Versioning:   "calver",
			CalVerFormat: "YYYY.0M.MICRO",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet plans the next calver and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops calver auto-merge tags the release", func(t *testing.T) {
		t.Parallel()

		// given: a calver azuredevops project ready for auto-merge
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v2025.11.1",
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v2025.11.1"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
			Versioning:   "calver",
			CalVerFormat: "YYYY.0M.MICRO",
		})

		// when: invoking `yeet release --auto-merge`
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet plans, opens, merges, and tags the release, exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseExtraTagsCrossProvider(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops picks the highest among multiple tags", func(t *testing.T) {
		t.Parallel()

		// given: an Azure repo with several tags advertised
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v1.2.0",
			ExtraTags:    []string{"v1.0.0", "v1.1.0", "v0.9.0"},
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.2.0"},
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet plans v1.3.0 from the latest tag and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "1.3.0")
	})
}

func TestReleaseAzureAutoMergeForceRejectsDraft(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops --auto-merge-force does not bypass draft state", func(t *testing.T) {
		t.Parallel()

		// given: an Azure server that reports the PR as draft
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

		// when: invoking `yeet release --auto-merge --auto-merge-force` against a draft PR
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-force",
				"--config", configPath,
			},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: azure still rejects merging drafts and the binary surfaces the block
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "draft")
	})
}

func TestReleaseAzureAutoMergeTagsMultipleTargets(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops --auto-merge with multi-target tags both targets", func(t *testing.T) {
		t.Parallel()

		// given: an azure multi-target repo with commits in both api/ and web/
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "api/v1.0.0",
			ExtraTags:    []string{"web/v1.0.0"},
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "feat: api change", Files: []string{"api/handler.go"}},
				{SHA: "web-sha", Message: "feat: web change", Files: []string{"web/index.html"}},
				{SHA: "boundary-sha", Message: "chore: release", Files: []string{"CHANGELOG.md"}},
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

		// when: invoking `yeet release --auto-merge`
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet plans, opens, merges and tags both targets, exiting 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
