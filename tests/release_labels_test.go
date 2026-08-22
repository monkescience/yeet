package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseLabelConfigValidation(t *testing.T) {
	t.Parallel()

	scenarios := []string{
		"rejects_blank_pending_label",
		"rejects_pending_label_with_surrounding_whitespace",
		"rejects_pending_label_containing_a_comma",
		"rejects_reserved_lifecycle_label_filter_value",
		"rejects_matching_pending_and_tagged_labels",
		"rejects_lifecycle_label_colliding_with_the_yeet_label",
		"rejects_blank_extra_label",
		"rejects_extra_label_with_surrounding_whitespace",
		"rejects_extra_label_containing_a_comma",
		"rejects_extra_label_duplicating_a_lifecycle_label",
	}

	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			// given: the invalid release label configuration for this scenario

			// when: running a dry-run release with that configuration

			// then: the command rejects it with the expected diagnostic
			assertReleaseConfigRejected(t, scenario)
		})
	}
}

func assertReleaseConfigRejected(t *testing.T, scenario string) {
	t.Helper()

	dir := "testdata/release/" + scenario

	result := binary.RunWithOptions(t,
		[]string{"release", "--dry-run", "--config", absoluteTestFile(t, dir+"/input.yaml")},
		testastic.WithRunEnv("GITHUB_REF_NAME=main"),
	)

	testastic.Equal(t, 1, result.ExitCode)
	testastic.AssertFile(t, dir+"/stderr.expected.txt", result.Stderr)
}

func TestReleaseExtraLabels(t *testing.T) {
	t.Parallel()

	t.Run("github fails the release when an extra label does not exist", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub repository that knows none of the configured labels
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			Labels:   &fixture.LabelsOptions{Extra: []string{"needs-review"}},
		})

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet refuses to create the PR because yeet does not create extra labels
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_missing_extra_label/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github reuses lifecycle and extra labels that already exist", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub repository where every configured label already exists
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:          "testorg",
			Repo:           "testrepo",
			LatestTag:      "v1.0.0",
			BoundarySHA:    shas[0],
			BranchHeadSHA:  shas[1],
			ExistingLabels: []string{"yeet", "autorelease: pending", "autorelease: tagged", "needs-review"},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			Labels:   &fixture.LabelsOptions{Extra: []string{"needs-review"}},
		})

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 0 without recreating any label
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab fails the release when an extra label does not exist", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab project that knows none of the configured labels
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
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
			Labels:   &fixture.LabelsOptions{Extra: []string{"needs-review"}},
		})

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet refuses to create the MR because yeet does not create extra labels
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_missing_extra_label/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab reuses lifecycle and extra labels that already exist", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab project where every configured label already exists
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:        "group/service",
			LatestTag:      "v1.0.0",
			BoundarySHA:    shas[0],
			BranchHeadSHA:  shas[1],
			ExistingLabels: []string{"yeet", "autorelease: pending", "autorelease: tagged", "needs-review"},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
			Labels:   &fixture.LabelsOptions{Extra: []string{"needs-review"}},
		})

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet exits 0 without recreating any label
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab rejects an extra label sharing a scope with a lifecycle label", func(t *testing.T) {
		t.Parallel()

		// given: a scoped extra label colliding with the scoped pending label
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
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
			Labels: &fixture.LabelsOptions{
				Pending: "release::pending",
				Tagged:  "release::tagged",
				Extra:   []string{"release::extra"},
			},
		})

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet exits 1 because GitLab would drop one of the two scoped labels
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_scoped_label_conflict/stderr.expected.txt",
			result.Stderr,
		)
	})
}
