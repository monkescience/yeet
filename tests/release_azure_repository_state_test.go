package integration_test

import (
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseAzureRepositoryState(t *testing.T) {
	t.Parallel()

	t.Run("creates a missing release branch before writing files", func(t *testing.T) {
		t.Parallel()

		// given: an Azure repository whose release branch does not exist.
		repoDir, shas := fixture.WriteRepoWithHistory(
			t,
			"https://dev.azure.com/contoso/platform/_git/yeet",
			"main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			},
		)

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:         "contoso",
			Project:              "platform",
			Repo:                 "yeet",
			LatestTag:            "v1.0.0",
			BoundarySHA:          shas[0],
			BranchHeadSHA:        shas[1],
			ReleaseBranchMissing: true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
		})

		// when: creating a release pull request.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet creates the release branch and completes successfully.
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
	})

	t.Run("resets a stale release branch before writing files", func(t *testing.T) {
		t.Parallel()

		// given: an Azure release branch that is behind the base branch.
		repoDir, shas := fixture.WriteRepoWithHistory(
			t,
			"https://dev.azure.com/contoso/platform/_git/yeet",
			"main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			},
		)

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:         "contoso",
			Project:              "platform",
			Repo:                 "yeet",
			LatestTag:            "v1.0.0",
			BoundarySHA:          shas[0],
			BranchHeadSHA:        shas[1],
			ReleaseBranchHeadSHA: "7374616c6572656c656173656272616e63687368",
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
		})

		// when: refreshing the release pull request.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet resets the branch before writing release files.
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
	})

	t.Run("does not recreate an existing annotated release", func(t *testing.T) {
		t.Parallel()

		// given: a merged Azure release pull request whose annotated tag already exists.
		repoDir, shas := fixture.WriteRepoWithHistory(
			t,
			"https://dev.azure.com/contoso/platform/_git/yeet",
			"main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "docs: no new release"},
			},
		)

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:         "contoso",
			Project:              "platform",
			Repo:                 "yeet",
			LatestTag:            "v1.0.0",
			BoundarySHA:          shas[0],
			BranchHeadSHA:        shas[1],
			MergedPendingRelease: true,
			ExistingReleaseTag:   "v1.1.0",
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
		})

		// when: finalizing the merged release again.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet treats the existing release as an idempotent success.
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
	})

	t.Run("creates a changelog that is missing from the base branch", func(t *testing.T) {
		t.Parallel()

		// given: an Azure repository without the configured changelog file.
		repoDir, shas := fixture.WriteRepoWithHistory(
			t,
			"https://dev.azure.com/contoso/platform/_git/yeet",
			"main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			},
		)

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := absoluteTestFile(
			t,
			"testdata/release/missing_changelog/input.yaml",
		)
		// when: creating a release pull request.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet creates the missing changelog and completes successfully.
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
	})

	for _, test := range []struct {
		name    string
		status  string
		problem string
		flag    string
	}{
		{name: "policy", status: "rejectedByPolicy", problem: "branch update was rejected by policy"},
		{
			name: "policy verbose", status: "rejectedByPolicy",
			problem: "branch update was rejected by policy", flag: "--verbose",
		},
		{name: "policy quiet", status: "rejectedByPolicy", problem: "branch update was rejected by policy", flag: "--quiet"},
		{name: "force push", status: "forcePushRequired", problem: "branch update requires force-push permission"},
		{name: "write permission", status: "writePermissionRequired", problem: "branch update requires write permission"},
		{name: "create permission", status: "createBranchPermissionRequired", problem: "branch creation requires permission"},
		{name: "stale branch", status: "staleOldObjectId", problem: "branch changed before the update completed"},
		{name: "locked", status: "locked", problem: "branch is locked"},
		{name: "missing status", problem: "provider rejected the branch update"},
		{name: "unknown status", status: "private-sdk-status", problem: "provider rejected the branch update"},
	} {
		t.Run("reports a rejected release branch reset/"+test.name, func(t *testing.T) {
			t.Parallel()

			// given: Azure rejects the attempted release branch reset.
			repoDir, shas := fixture.WriteRepoWithHistory(
				t,
				"https://dev.azure.com/contoso/platform/_git/yeet",
				"main",
				[]fixture.RepoCommit{
					{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
					{Message: "feat: add a thing"},
				},
			)

			server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
				Organization:         "contoso",
				Project:              "platform",
				Repo:                 "yeet",
				LatestTag:            "v1.0.0",
				BoundarySHA:          shas[0],
				BranchHeadSHA:        shas[1],
				ReleaseBranchHeadSHA: "7374616c6572656c656173656272616e63687368",
				RefUpdateFailure:     "private-sdk-response",
				RefUpdateStatus:      test.status,
			})

			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider:     "azuredevops",
				Branch:       "main",
				Host:         "dev.azure.com",
				Organization: "contoso",
				Project:      "platform",
				Repo:         "yeet",
			})

			args := []string{"release", "--config", configPath}
			if test.flag != "" {
				args = append(args, test.flag)
			}

			// when: refreshing the release pull request.
			result := binary.RunWithOptions(
				t,
				args,
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
			)

			// then: the diagnostic identifies the branch and safe reason, and the response text stays in debug records.
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.Contains(t, result.Stderr,
				"ERROR could not update release branch: "+test.problem+
					" provider=azuredevops branch=yeet/release-main")
			testastic.NotContains(t, withoutDebugDiagnostics(result.Stderr), "private-sdk-response")
			testastic.NotContains(t, result.Stderr, "private-sdk-status")

			if test.status != "rejectedByPolicy" {
				return
			}

			for line := range strings.SplitSeq(result.Stderr, "\n") {
				if strings.HasPrefix(line, "ERROR ") {
					testastic.AssertFile(t, "testdata/release/rejected_branch_reset/stderr.expected.txt", line+"\n")
				}
			}
		})
	}

	t.Run("resolves configured reviewers before creating the pull request", func(t *testing.T) {
		t.Parallel()

		// given: an Azure release configured with a resolvable reviewer.
		repoDir, shas := fixture.WriteRepoWithHistory(
			t,
			"https://dev.azure.com/contoso/platform/_git/yeet",
			"main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			},
		)

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Reviewers: map[string]string{
				"alice@example.test": "11111111-1111-1111-1111-111111111111",
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			Reviewers:    []string{"alice@example.test"},
		})

		// when: creating the release pull request.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet resolves the reviewer and creates the pull request.
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
	})
}

func TestReleaseGitHubRepositoryState(t *testing.T) {
	t.Parallel()

	// given: a GitHub repository whose release branch does not exist.
	repoDir, shas := fixture.WriteRepoWithHistory(
		t,
		"https://github.com/testorg/testrepo.git",
		"main",
		[]fixture.RepoCommit{
			{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
			{Message: "feat: add a thing"},
		},
	)

	server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
		Owner:                "testorg",
		Repo:                 "testrepo",
		LatestTag:            "v1.0.0",
		BoundarySHA:          shas[0],
		BranchHeadSHA:        shas[1],
		ReleaseBranchMissing: true,
	})

	configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
		Provider: "github",
		Branch:   "main",
		Host:     "github.com",
		Owner:    "testorg",
		Repo:     "testrepo",
	})

	// when: creating a release pull request.
	result := binary.RunWithOptions(
		t,
		[]string{"release", "--config", configPath},
		testastic.WithRunWorkDir(repoDir),
		testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
	)

	// then: yeet creates the missing release branch and completes successfully.
	testastic.Equal(t, 0, result.ExitCode)
	testastic.Equal(t, "", result.Stdout)
}
