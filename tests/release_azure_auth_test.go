package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseAzureDevOpsAuth(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops uses AZURE_DEVOPS_SYSTEM_ACCESSTOKEN when set", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure server and SYSTEM_ACCESSTOKEN set (no PAT)
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

		// when: invoking `yeet release --dry-run` with SYSTEM_ACCESSTOKEN instead of EXT_PAT
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(
				"AZURE_DEVOPS_EXT_PAT=",
				"AZURE_DEVOPS_SYSTEM_ACCESSTOKEN=system-token",
				"AZURE_DEVOPS_URL="+server.URL,
				"GITHUB_REF_NAME=main",
			),
		)

		// then: yeet authenticates with the system token and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops requires AZURE_DEVOPS_SYSTEM_ACCESSTOKEN or EXT_PAT", func(t *testing.T) {
		t.Parallel()

		// given: an Azure config with neither token env var set
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		// when: invoking `yeet release` with both Azure tokens cleared
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(
				"AZURE_DEVOPS_EXT_PAT=",
				"AZURE_DEVOPS_SYSTEM_ACCESSTOKEN=",
				"GITHUB_REF_NAME=main",
			),
		)

		// then: yeet exits 1 and stderr names the missing env vars
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "AZURE_DEVOPS_SYSTEM_ACCESSTOKEN")
	})
}
