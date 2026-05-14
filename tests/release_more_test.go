package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseCommitOverrideCrossProvider(t *testing.T) {
	t.Parallel()

	t.Run("gitlab BEGIN_COMMIT_OVERRIDE block replaces squashed message", func(t *testing.T) {
		t.Parallel()

		// given: a squashed merge whose MR description wraps an override block
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/service",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{
					SHA:     "head-sha",
					Message: "chore: squashed merge",
					AssociatedPRBody: "Some MR notes\n\n" +
						"BEGIN_COMMIT_OVERRIDE\n" +
						"feat: overridden first commit\n\n" +
						"fix: overridden second commit\n" +
						"END_COMMIT_OVERRIDE\n",
				},
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

		// then: the changelog reflects the overridden subjects, not the squashed message
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "overridden first commit")
		testastic.Contains(t, result.Stdout, "overridden second commit")
	})
}

func TestReleaseNoChangesPerProvider(t *testing.T) {
	t.Parallel()

	t.Run("gitlab reports no release when no releasable commits exist", func(t *testing.T) {
		t.Parallel()

		// given: only a docs commit since the last release
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/service",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "docs: tweak readme"},
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

		// then: yeet exits 0 with an empty plan
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops reports no release when no releasable commits exist", func(t *testing.T) {
		t.Parallel()

		// given: only a chore commit since the last release
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v1.0.0",
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "chore: housekeeping"},
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

		// then: yeet exits 0 with an empty plan
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseBreakingChangePerProvider(t *testing.T) {
	t.Parallel()

	t.Run("gitlab bumps major on breaking change", func(t *testing.T) {
		t.Parallel()

		// given: a feat! commit on GitLab with a BREAKING CHANGE footer
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/service",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "feat!: redesign api\n\nBREAKING CHANGE: removed v1"},
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

		// then: yeet plans v2.0.0 with breaking-changes section
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "2.0.0")
		testastic.Contains(t, result.Stdout, "BREAKING CHANGES")
	})

	t.Run("azuredevops bumps major on breaking change", func(t *testing.T) {
		t.Parallel()

		// given: a feat! commit on Azure with a BREAKING CHANGE footer
		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization: "contoso",
			Project:      "platform",
			Repo:         "yeet",
			LatestTag:    "v1.0.0",
			BoundarySHA:  "boundary-sha",
			Commits: []fakeprovider.AzureCommit{
				{SHA: "head-sha", Message: "feat!: redesign api\n\nBREAKING CHANGE: removed v1"},
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

		// then: yeet emits the breaking-changes section and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "BREAKING CHANGES")
	})
}

func TestReleaseJSONPointerSkipPaths(t *testing.T) {
	t.Parallel()

	t.Run("github bumps a second array entry, skipping earlier objects", func(t *testing.T) {
		t.Parallel()

		// given: a JSON manifest where the target version is the third entry
		manifest := `{
  "packages": [
    {"name": "sibling-a", "version": "9.9.9", "deps": {"x":1, "y":[2,3]}},
    {"name": "sibling-b", "version": "8.8.8", "tags": ["one", "two"]},
    {"name": "yeet", "version": "1.0.0"}
  ]
}`

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
			Files: map[string]string{"manifest.json": manifest},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			VersionFiles: []fixture.VersionFileOptions{
				{Path: "manifest.json", Format: "json", JSONPointer: "/packages/2/version"},
			},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet skips the earlier siblings and bumps the third element's version
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github rejects an invalid JSON pointer index", func(t *testing.T) {
		t.Parallel()

		// given: a JSON pointer that references a non-existent array index
		manifest := `{"packages":[{"version":"1.0.0"}]}`

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
			Files: map[string]string{"manifest.json": manifest},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			VersionFiles: []fixture.VersionFileOptions{
				{Path: "manifest.json", Format: "json", JSONPointer: "/packages/5/version"},
			},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 with a JSON pointer not-found error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "json")
	})

	t.Run("github surfaces malformed JSON in the version file", func(t *testing.T) {
		t.Parallel()

		// given: a malformed JSON file targeted by the version_files pointer
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
			Files: map[string]string{"broken.json": `{"version": "1.0.0"`},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			VersionFiles: []fixture.VersionFileOptions{
				{Path: "broken.json", Format: "json", JSONPointer: "/version"},
			},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 with a JSON parsing error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "json")
	})
}

func TestReleaseJSONPointerProviders(t *testing.T) {
	t.Parallel()

	t.Run("gitlab bumps a JSON-pointer-targeted version", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab repo with package.json /version
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/service",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
			Files: map[string]string{
				"package.json": `{"name":"yeet","version":"1.0.0"}`,
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
			VersionFiles: []fixture.VersionFileOptions{
				{Path: "package.json", Format: "json", JSONPointer: "/version"},
			},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet bumps the JSON pointer target and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops bumps a generic semver version file", func(t *testing.T) {
		t.Parallel()

		// given: an azure repo with VERSION.txt
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
			Files: map[string]string{
				"VERSION.txt": "1.0.0 # x-yeet-version\n",
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet bumps the inline marker and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
