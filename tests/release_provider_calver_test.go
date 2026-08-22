package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseGitLabCalVer(t *testing.T) {
	t.Parallel()

	t.Run("gitlab calver release creates a merge request with the next month/micro", func(t *testing.T) {
		t.Parallel()

		// given: a calver project on GitLab with a prior month tag
		files := map[string]string{"VERSION.txt": "2025.11.1 # x-yeet-version\n"}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2025.11.1", Tag: "v2025.11.1"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
			LatestTag:     "v2025.11.1",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
			testastic.WithRunWorkDir(repoDir),
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
		files := map[string]string{"VERSION.txt": "2025.11.1 # x-yeet-version\n"}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2025.11.1", Tag: "v2025.11.1"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v2025.11.1",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet plans the next calver and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops calver auto-merge tags the release", func(t *testing.T) {
		t.Parallel()

		// given: a calver azuredevops project ready for auto-merge
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2025.11.1", Tag: "v2025.11.1"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v2025.11.1",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
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
			testastic.WithRunWorkDir(repoDir),
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

		// given: an Azure repo with several tags advertised, each tagging its own
		// local commit so highest-tag selection works against real ancestry
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v0.9.0", Tag: "v0.9.0"},
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: release v1.1.0", Tag: "v1.1.0"},
				{Message: "chore: release v1.2.0", Tag: "v1.2.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v1.2.0",
			ExtraTags:    []string{"v1.0.0", "v1.1.0", "v0.9.0"},
			BoundarySHA:  shas[3],
			TagSHAs: map[string]string{
				"v0.9.0": shas[0], "v1.0.0": shas[1], "v1.1.0": shas[2], "v1.2.0": shas[3],
			},
			BranchHeadSHA: shas[4],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet plans v1.3.0 from the latest tag and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"azuredevops_picks_the_highest_among_multiple_tags/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseAzureAutoMergeForceRejectsDraft(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops --auto-merge-force does not bypass draft state", func(t *testing.T) {
		t.Parallel()

		// given: an Azure server that reports the PR as draft
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			MergeBlocked:  true,
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: azure still rejects merging drafts and the binary surfaces the block
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"azuredevops___auto_merge_force_does_not_bypass_draft_state/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseAzureAutoMergeTagsMultipleTargets(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops --auto-merge with multi-target tags both targets", func(t *testing.T) {
		t.Parallel()

		// given: an azure multi-target repo with commits in both api/ and web/
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{
					Message: "chore: release api v1.0.0",
					Tag:     "api/v1.0.0",
					Files:   map[string]string{"api/handler.go": "package api\n"},
				},
				{
					Message: "chore: release web v1.0.0",
					Tag:     "web/v1.0.0",
					Files:   map[string]string{"web/index.html": "<html></html>\n"},
				},
				{Message: "feat: api change", Files: map[string]string{"api/handler.go": "package api // v2\n"}},
				{Message: "feat: web change", Files: map[string]string{"web/index.html": "<html>v2</html>\n"}},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "api/v1.0.0",
			ExtraTags:     []string{"web/v1.0.0"},
			BoundarySHA:   shas[0],
			TagSHAs:       map[string]string{"api/v1.0.0": shas[0], "web/v1.0.0": shas[1]},
			BranchHeadSHA: shas[3],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet plans, opens, merges and tags both targets, exiting 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
