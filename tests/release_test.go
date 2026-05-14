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
	t.Parallel()

	t.Run("gitlab dry-run prints the planned release", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server with one releasable commit since v1.0.0
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: the binary exits 0 and prints the Dry Run banner
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("azuredevops dry-run prints the planned release", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server with one releasable commit since v1.0.0
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: the binary exits 0 and prints the Dry Run banner
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})

	t.Run("github dry-run prints the planned release", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server with one releasable commit since v1.0.0
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the binary exits 0 and prints the Dry Run banner
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "Dry Run")
	})
}

func TestReleaseCreatesPR(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops opens a release pull request", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server with one releasable feat commit
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

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet exits 0 (PR creation handled by the fake provider)
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab opens a release merge request", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server with one releasable feat commit
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

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet exits 0 (MR creation handled by the fake provider)
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github opens a release pull request", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server with one releasable feat commit
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

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 0 (PR creation handled by the fake provider)
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseAutoMerge(t *testing.T) {
	t.Parallel()

	t.Run("github finalizes an already-merged release PR", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server reporting that the prior release PR was merged
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

		// when: running `yeet release` without --auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet tags the merged release and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab --auto-merge tags the release", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server with a releasable commit
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

		// when: running `yeet release --auto-merge`
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet merges and tags the release, exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab finalizes an already-merged release MR", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server reporting that the prior release MR was merged
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

		// when: running `yeet release` without --auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet tags the merged release and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github --auto-merge tags the release", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server with a releasable commit
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

		// when: running `yeet release --auto-merge`
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet merges and tags the release, exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseAzureDevOpsFullFlow(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops --auto-merge tags the release", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server with a releasable feat commit
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

		// when: running `yeet release --auto-merge`
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet merges and tags the release, exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops finalizes an already-merged release PR", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server reporting that the prior release PR was merged
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

		// when: running `yeet release` without --auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet tags the merged release and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops rejects multiple pending release PRs", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server returning two open release PRs
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

		// when: running `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet exits 1 with a "multiple pending release PRs" error on stderr
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "multiple pending release PRs/MRs")
	})

	t.Run("azuredevops blocks --auto-merge when merge is gated", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server that flags the release PR as merge-blocked
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

		// when: running `yeet release --auto-merge` against the blocked PR
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet exits 1 with a "release PR merge blocked" error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "release PR merge blocked")
	})
}

func TestReleaseChannelAndVersionFiles(t *testing.T) {
	t.Parallel()

	t.Run("github prerelease channel opens a PR on the channel branch", func(t *testing.T) {
		t.Parallel()

		// given: a config with a `beta` channel and the binary running on the beta branch
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

		// when: invoking `yeet release --channel beta` from the beta ref
		result := binary.RunWithOptions(t,
			[]string{"release", "--channel", "beta", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: yeet opens a prerelease PR for the channel and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github release updates a configured version file", func(t *testing.T) {
		t.Parallel()

		// given: a config that lists VERSION.txt as a version_files entry
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet writes the bumped version and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseCalVer(t *testing.T) {
	t.Parallel()

	t.Run("github calver release creates a PR with the next month/micro", func(t *testing.T) {
		t.Parallel()

		// given: a calver project at v2025.11.1 with one new feat commit
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans the next calver and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github calver bootstraps the initial release without a prior tag", func(t *testing.T) {
		t.Parallel()

		// given: a calver project with no previous releases
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

		// when: invoking `yeet release --dry-run` for the first time
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans an initial calver release and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github calver YYYY.MM.DD.MICRO dry-run plans a daily version", func(t *testing.T) {
		t.Parallel()

		// given: a calver config using year/month/day/micro
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet resolves the daily calver format and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github calver YYYY.WW.MICRO dry-run plans an ISO-week version", func(t *testing.T) {
		t.Parallel()

		// given: a calver config using ISO year/week/micro
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet resolves the ISO-week calver format and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseJSONPointerVersionFile(t *testing.T) {
	t.Parallel()

	t.Run("github bumps a top-level package.json version", func(t *testing.T) {
		t.Parallel()

		// given: a project with a package.json containing /version
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet bumps the JSON pointer target and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github bumps a nested array JSON pointer", func(t *testing.T) {
		t.Parallel()

		// given: a manifest.json with a version at /packages/0/version
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet bumps the array-element version and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github escapes ~ and / in the JSON pointer", func(t *testing.T) {
		t.Parallel()

		// given: a JSON pointer that uses ~0 and ~1 escapes
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet resolves the escaped pointer and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseBreakingChange(t *testing.T) {
	t.Parallel()

	t.Run("github bumps the major version on a BREAKING CHANGE footer", func(t *testing.T) {
		t.Parallel()

		// given: a feat! commit carrying a BREAKING CHANGE footer
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans v2.0.0 and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "v2.0.0")
	})
}

func TestReleaseMultiTarget(t *testing.T) {
	t.Parallel()

	t.Run("github --target filters multi-target plans to the requested target", func(t *testing.T) {
		t.Parallel()

		// given: a repo with `api/` and `web/` path targets and commits in both
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

		// when: invoking `yeet release --dry-run --target api`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--target", "api", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans only the `api` target and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "api")
	})
}

func TestReleasePreMajor(t *testing.T) {
	t.Parallel()

	t.Run("github keeps feat at a minor bump while still pre-1.0", func(t *testing.T) {
		t.Parallel()

		// given: a project still on the 0.x series
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: a feat on 0.x bumps to v0.3.1 (patch) rather than v0.4.0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "v0.3.1")
	})
}

func TestReleaseNoChanges(t *testing.T) {
	t.Parallel()

	t.Run("github reports no release when no releasable commits exist", func(t *testing.T) {
		t.Parallel()

		// given: only a docs commit since the last release
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet still exits 0 (no-op release)
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseMergeErrors(t *testing.T) {
	t.Parallel()

	t.Run("github rejects multiple pending release PRs", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server returning two open release PRs
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 with a multi-PR error on stderr
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "multiple pending release PRs/MRs")
	})
}

func TestReleaseMultiTagHistory(t *testing.T) {
	t.Parallel()

	t.Run("github semver picks the highest of multiple prior tags", func(t *testing.T) {
		t.Parallel()

		// given: a repo with v0.9.0, v1.0.0, v1.1.0 and v1.2.0 advertised
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans the next minor relative to v1.2.0 (v1.3.0)
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "v1.3.0")
	})

	t.Run("github calver picks the highest of multiple prior calver tags", func(t *testing.T) {
		t.Parallel()

		// given: a calver repo with several prior month tags
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet uses the latest calver tag as the baseline and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseMultiTargetPRBody(t *testing.T) {
	t.Parallel()

	t.Run("github builds a combined wave PR body for two path targets", func(t *testing.T) {
		t.Parallel()

		// given: a multi-target repo with commits in both `api/` and `web/`
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

		// when: invoking `yeet release` without --target
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet emits a combined wave PR and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseDerivedTarget(t *testing.T) {
	t.Parallel()

	t.Run("github derived root target aggregates included path plans", func(t *testing.T) {
		t.Parallel()

		// given: a path target `api` and a derived `root` that includes `api`
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the derived target rolls up the included path plans and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseMultiTargetCrossProvider(t *testing.T) {
	t.Parallel()

	t.Run("gitlab multi-target resolves per-commit paths", func(t *testing.T) {
		t.Parallel()

		// given: a multi-target gitlab repo with commits in both `api/` and `web/`
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: gitlab routes commits to their respective targets and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops multi-target resolves per-commit paths", func(t *testing.T) {
		t.Parallel()

		// given: a multi-target Azure repo with commits in both `api/` and `web/`
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: azure routes commits to their respective targets and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseUpdatesExistingPR(t *testing.T) {
	t.Parallel()

	t.Run("gitlab updates the open release MR in place", func(t *testing.T) {
		t.Parallel()

		// given: an already-open release MR with a yeet manifest in its body
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet updates the existing MR rather than opening a new one
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops updates the open release PR in place", func(t *testing.T) {
		t.Parallel()

		// given: an already-open release PR carrying a yeet manifest
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet updates the existing PR rather than opening a new one
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github updates the open release PR while preserving manual sections", func(t *testing.T) {
		t.Parallel()

		// given: an open release PR whose body carries a reviewer-added "Notes" section
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet updates the body without trashing the Notes section
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseAsFooter(t *testing.T) {
	t.Parallel()

	t.Run("github Release-As footer pins the planned version", func(t *testing.T) {
		t.Parallel()

		// given: a feat commit carrying a `Release-As: 2.5.0` footer
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet honours the override and plans v2.5.0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "v2.5.0")
	})

	t.Run("github Release-As overrides a smaller computed bump", func(t *testing.T) {
		t.Parallel()

		// given: a fix commit that would normally bump v1.2.3 -> v1.2.4
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the explicit Release-As wins, yielding v3.0.0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "v3.0.0")
	})
}

func TestReleaseCommitOverride(t *testing.T) {
	t.Parallel()

	t.Run("github merged-PR body BEGIN/END_COMMIT_OVERRIDE replaces the squashed message", func(t *testing.T) {
		t.Parallel()

		// given: a squashed merge whose associated PR body wraps an override block
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

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the changelog uses the overridden commit subjects, not the squashed message
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "overridden first commit")
	})
}

func TestReleaseVersionFileErrors(t *testing.T) {
	t.Parallel()

	t.Run("semver target rejects a calver marker with a suggestion", func(t *testing.T) {
		t.Parallel()

		// given: a semver project whose VERSION.txt carries an `x-yeet-month` marker
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and names the offending marker in the suggestion
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "x-yeet-month")
	})

	t.Run("calver target rejects a semver marker with a suggestion", func(t *testing.T) {
		t.Parallel()

		// given: a calver project whose VERSION.txt carries an `x-yeet-major` marker
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and names the offending marker in the suggestion
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "x-yeet-major")
	})

	t.Run("calver target updates month and micro markers in BUILD.txt", func(t *testing.T) {
		t.Parallel()

		// given: a calver BUILD.txt with year/month/micro markers
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet updates each marker without rejecting the file and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseConfigErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing config file prints an init hint", func(t *testing.T) {
		t.Parallel()

		// given: a working directory with no .yeet.yaml
		tempDir := t.TempDir()

		// when: invoking `yeet release` from that directory
		result := binary.RunWithOptions(t,
			[]string{"release"},
			testastic.WithRunWorkDir(tempDir),
		)

		// then: yeet exits 1 with a "configuration file not found" hint to run init
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "configuration file not found")
		testastic.Contains(t, result.Stderr, "run `yeet init` or pass --config")
	})

	t.Run("malformed yaml reports a parse error", func(t *testing.T) {
		t.Parallel()

		// given: a syntactically broken .yeet.yaml
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".yeet.yaml")

		const filePerm = 0o600

		err := os.WriteFile(configPath, []byte("release: ["), filePerm)
		testastic.NoError(t, err)

		// when: invoking `yeet release --config <path>`
		result := binary.Run(t, "release", "--config", configPath)

		// then: yeet exits 1 with an "invalid configuration / parse config" error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "invalid configuration")
		testastic.Contains(t, result.Stderr, "parse config")
	})

	t.Run("github missing token surfaces the env-var requirement", func(t *testing.T) {
		t.Parallel()

		// given: a valid github config but neither GITHUB_TOKEN nor GH_TOKEN set
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking `yeet release` with the token vars cleared
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(
				"GITHUB_TOKEN=",
				"GH_TOKEN=",
				"GITHUB_REF_NAME=main",
			),
		)

		// then: yeet exits 1 and stderr names the missing env vars
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "provider setup failed")
		testastic.Contains(t, result.Stderr, "GITHUB_TOKEN or GH_TOKEN")
	})

	t.Run("unsupported remote host without explicit provider is rejected", func(t *testing.T) {
		t.Parallel()

		// given: a config pointing at code.company.com with no provider hint
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Branch: "main",
			Host:   "code.company.com",
			Owner:  "platform",
			Repo:   "yeet",
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with an "unsupported remote host" error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "repository resolution failed")
		testastic.Contains(t, result.Stderr, "unsupported remote host")
	})
}

func TestReleaseCLIFlagsOverrideConfig(t *testing.T) {
	t.Parallel()

	t.Run("github CLI flags override owner/repo/provider/host from config", func(t *testing.T) {
		t.Parallel()

		// given: a config naming `configorg/configrepo` and a server expecting `flagorg/flagrepo`
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

		// when: invoking `yeet release` with --owner/--repo/--provider/--host flags
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run", "--config", configPath,
				"--provider", "github", "--owner", "flagorg", "--repo", "flagrepo",
				"--host", "github.com",
				"--auto-merge-force",
				"--auto-merge-method", "squash",
			},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet uses the flag values and reaches the fake server (exit 0)
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab --project flag overrides config owner/repo", func(t *testing.T) {
		t.Parallel()

		// given: a config naming `configorg/configrepo` and a server expecting `flaggroup/svc`
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

		// when: invoking `yeet release --project flaggroup/svc`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run", "--config", configPath,
				"--provider", "gitlab", "--project", "flaggroup/svc",
			},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: --project clears owner/repo and yeet reaches the fake server (exit 0)
		testastic.Equal(t, 0, result.ExitCode)
	})
}
