package integration_test

import (
	"regexp"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseCalVerPrerelease(t *testing.T) {
	t.Parallel()

	// The tags are from an earlier period, so the next base is always this period at MICRO 1.
	testCases := []struct {
		name     string
		branch   string
		channel  bool
		tags     []string
		boundary string
		want     *regexp.Regexp
	}{
		{
			name: "github beta channel starts a prerelease after a stable tag", branch: "beta", channel: true,
			tags: []string{"v2025.05.3"}, boundary: "v2025.05.3",
			want: regexp.MustCompile(`2025\.05\.3 → \d{4}\.\d{2}\.1-beta\.1\b`),
		},
		{
			name: "github beta channel restarts the counter in a new period", branch: "beta", channel: true,
			tags: []string{"v2025.05.3", "v2025.05.4-beta.2"}, boundary: "v2025.05.4-beta.2",
			want: regexp.MustCompile(`2025\.05\.4-beta\.2 → \d{4}\.\d{2}\.1-beta\.1\b`),
		},
		{
			name: "github stable release ignores calver prerelease tags", branch: "main",
			tags: []string{"v2025.05.3", "v2025.05.4-beta.2"}, boundary: "v2025.05.3",
			want: regexp.MustCompile(`2025\.05\.3 → \d{4}\.\d{2}\.1\s`),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a calver project with a beta channel and tags from an earlier period
			commits := make([]fixture.RepoCommit, 0, len(testCase.tags)+1)
			tagSHAs := make(map[string]string, len(testCase.tags))

			for _, tag := range testCase.tags {
				commits = append(commits, fixture.RepoCommit{Message: "chore: release " + tag, Tag: tag})
			}

			commits = append(commits, fixture.RepoCommit{Message: "feat: add a thing"})

			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git",
				testCase.branch, commits)

			for index, tag := range testCase.tags {
				tagSHAs[tag] = shas[index]
			}

			server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner:         "testorg",
				Repo:          "testrepo",
				LatestTag:     testCase.tags[len(testCase.tags)-1],
				ExtraTags:     testCase.tags[:len(testCase.tags)-1],
				BoundarySHA:   tagSHAs[testCase.boundary],
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
				Channels: map[string]fixture.ChannelOptions{
					"beta": {Branch: "beta", Prerelease: "beta"},
				},
			})

			args := []string{"release", "--dry-run", "--config", configPath}
			if testCase.channel {
				args = append(args, "--channel", "beta")
			}

			// when: invoking `yeet release --dry-run`
			result := binary.RunWithOptions(t, args,
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(fixture.GitHubEnv(server, testCase.branch)...),
			)

			// then: yeet plans the calver version of this period and exits 0
			testastic.Equal(t, 0, result.ExitCode)
			testastic.True(t, testCase.want.MatchString(result.Stdout))
		})
	}
}
