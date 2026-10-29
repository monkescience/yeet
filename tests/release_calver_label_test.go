package integration_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseCalVerLabel(t *testing.T) {
	t.Parallel()

	// The tags are from an earlier period, so the next version is always this period at MICRO 1.
	testCases := []struct {
		name  string
		label string
		tags  []string
		want  *regexp.Regexp
	}{
		{
			name: "github labeled release follows a labeled tag", label: "beta",
			tags: []string{"v2025.05.2", "v2025.05.3-beta"},
			want: regexp.MustCompile(`2025\.05\.3-beta → \d{4}\.\d{2}\.1-beta\s`),
		},
		{
			name: "github release without the label follows a labeled tag",
			tags: []string{"v2025.05.3-beta", "v2025.05.4-beta"},
			want: regexp.MustCompile(`2025\.05\.4-beta → \d{4}\.\d{2}\.1\s`),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a calver project on the main branch with labeled tags from an earlier period
			commits := make([]fixture.RepoCommit, 0, len(testCase.tags)+1)
			tagSHAs := make(map[string]string, len(testCase.tags))

			for _, tag := range testCase.tags {
				commits = append(commits, fixture.RepoCommit{Message: "chore: release " + tag, Tag: tag})
			}

			commits = append(commits, fixture.RepoCommit{Message: "feat: add a thing"})

			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main", commits)

			for index, tag := range testCase.tags {
				tagSHAs[tag] = shas[index]
			}

			latest := testCase.tags[len(testCase.tags)-1]

			server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner:         "testorg",
				Repo:          "testrepo",
				LatestTag:     latest,
				ExtraTags:     testCase.tags[:len(testCase.tags)-1],
				BoundarySHA:   tagSHAs[latest],
				TagSHAs:       tagSHAs,
				BranchHeadSHA: shas[len(shas)-1],
			})

			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider:     "github",
				Branch:       "main",
				Host:         "github.com",
				Owner:        "testorg",
				Repo:         "testrepo",
				Versioning:   "calver",
				CalVerFormat: "YYYY.0M.MICRO",
				CalVerLabel:  testCase.label,
			})

			// when: invoking `yeet release --dry-run`
			result := binary.RunWithOptions(t, []string{"release", "--dry-run", "--config", configPath},
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
			)

			// then: yeet continues MICRO from the labeled tag and adds the configured label
			testastic.Equal(t, 0, result.ExitCode)
			testastic.True(t, testCase.want.MatchString(result.Stdout))
		})
	}

	t.Run("github rejects a calver Release-As value with a label", func(t *testing.T) {
		t.Parallel()

		// given: a labeled calver project and a commit whose Release-As footer includes the label
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2025.05.3-beta", Tag: "v2025.05.3-beta"},
				{Message: "feat: pin\n\nRelease-As: 2025.05.9-beta"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2025.05.3-beta",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.0M.MICRO",
			CalVerLabel:  "beta",
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t, []string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and the hint tells to remove the label
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr,
			"invalid release version override: requested version must not include a label")
		testastic.Contains(t, result.Stderr, `hint="remove the label from the version, calver.label adds it"`)
	})

	t.Run("rejects a dotted label", func(t *testing.T) {
		t.Parallel()

		// given: a calver config with a label that reads like a prerelease counter
		repoDir, _ := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{{Message: "feat: add a thing"}})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.0M.MICRO",
			CalVerLabel:  "beta.1",
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t, []string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
		)

		// then: yeet exits 1 and names the label
		testastic.Equal(t, 1, result.ExitCode)
		testastic.True(t, strings.Contains(result.Stderr, "calver.label"))
	})
}
