package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseCommitOverrideCrossProvider(t *testing.T) {
	t.Parallel()

	t.Run("gitlab BEGIN_COMMIT_OVERRIDE block replaces squashed message", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout with a squashed merge whose commit message
		// wraps an override block
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: squashed merge\n\n" +
					"BEGIN_COMMIT_OVERRIDE\n" +
					"feat: overridden first commit\n\n" +
					"fix: overridden second commit\n" +
					"END_COMMIT_OVERRIDE\n"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: the changelog reflects the overridden subjects, not the squashed message
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"gitlab_b_e_g_i_n_c_o_m_m_i_t_o_v_e_r_r_i_d_e_block_replaces_squashed_message/"+
				"stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseNoChangesPerProvider(t *testing.T) {
	t.Parallel()

	t.Run("gitlab reports no release when no releasable commits exist", func(t *testing.T) {
		t.Parallel()

		// given: only a docs commit since the last release
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "docs: tweak readme"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet exits 0 with an empty plan
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops reports no release when no releasable commits exist", func(t *testing.T) {
		t.Parallel()

		// given: only a chore commit since the last release
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: housekeeping"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
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
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
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
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat!: redesign api\n\nBREAKING CHANGE: removed v1"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet plans v2.0.0 with breaking-changes section
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_bumps_major_on_breaking_change/"+
				"stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("azuredevops bumps major on breaking change", func(t *testing.T) {
		t.Parallel()

		// given: a feat! commit on Azure with a BREAKING CHANGE footer
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat!: redesign api\n\nBREAKING CHANGE: removed v1"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
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
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet emits the breaking-changes section and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"azuredevops_bumps_major_on_breaking_change/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseJSONPointerSkipPaths(t *testing.T) {
	t.Parallel()

	t.Run("github bumps a second array entry, skipping earlier objects", func(t *testing.T) {
		t.Parallel()

		// given: a JSON manifest where the target version is the third entry
		files := map[string]string{"manifest.json": readTestFile(
			t,
			"testdata/release/"+
				"github_bumps_a_second_array_entry__skipping_earlier_objects/"+
				"manifest.json",
		)}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet skips the earlier siblings and bumps the third element's version
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github rejects an invalid JSON pointer index", func(t *testing.T) {
		t.Parallel()

		// given: a JSON pointer that references a non-existent array index
		files := map[string]string{"manifest.json": readTestFile(
			t,
			"testdata/release/"+
				"github_rejects_an_invalid_j_s_o_n_pointer_index/manifest.json",
		)}

		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 with a JSON pointer not-found error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"github_rejects_an_invalid_j_s_o_n_pointer_index/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github surfaces malformed JSON in the version file", func(t *testing.T) {
		t.Parallel()

		// given: a malformed JSON file targeted by the version_files pointer
		files := map[string]string{"broken.json": readTestFile(
			t,
			"testdata/release/"+
				"github_surfaces_malformed_j_s_o_n_in_the_version_file/broken.json",
		)}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 with a JSON parsing error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"github_surfaces_malformed_j_s_o_n_in_the_version_file/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseJSONPointerProviders(t *testing.T) {
	t.Parallel()

	t.Run("gitlab bumps a JSON-pointer-targeted version", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab repo with package.json /version
		files := map[string]string{"package.json": readTestFile(
			t,
			"testdata/release/"+
				"gitlab_bumps_a_j_s_o_n_pointer_targeted_version/package.json",
		)}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet bumps the JSON pointer target and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops bumps a generic semver version file", func(t *testing.T) {
		t.Parallel()

		// given: an azure repo with VERSION.txt
		files := map[string]string{"VERSION.txt": "1.0.0 # x-yeet-version\n"}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
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
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet bumps the inline marker and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
