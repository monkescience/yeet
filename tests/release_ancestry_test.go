package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseUnreachableAncestor(t *testing.T) {
	t.Parallel()

	t.Run("github surfaces an unreachable boundary as a branch-ancestry error", func(t *testing.T) {
		t.Parallel()

		// given: a LatestTag whose boundary SHA never appears in the branch commits
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "ghost-sha-not-on-branch",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "second-sha", Message: "feat: another"},
				{SHA: "third-sha", Message: "chore: setup"},
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

		// then: yeet exits 1 with a "not reachable" branch ancestry error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "not reachable")
	})
}

func TestReleaseChannelChangelogFile(t *testing.T) {
	t.Parallel()

	t.Run("github prerelease writes to the channel's changelog_file", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel that points its changelog_file to a separate path
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
				"CHANGELOG-beta.md": "# Beta Changelog\n",
			},
		})

		const configBody = `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
release:
  channels:
    beta:
      branch: beta
      prerelease: beta
      changelog_file: CHANGELOG-beta.md
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`

		configPath := writeRawConfig(t, configBody)

		// when: invoking `yeet release --channel beta` on the beta branch
		result := binary.RunWithOptions(t,
			[]string{"release", "--channel", "beta", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: yeet writes to the per-channel changelog and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleasePRBodyHeaderFooter(t *testing.T) {
	t.Parallel()

	t.Run("github uses configured pr_body_header and pr_body_footer", func(t *testing.T) {
		t.Parallel()

		// given: a release config with custom pr_body_header and pr_body_footer values
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

		const configBody = `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
release:
  pr_body_header: "## Custom Release Header"
  pr_body_footer: "_yeet release footer_"
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`

		configPath := writeRawConfig(t, configBody)

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet opens the PR with the custom header/footer and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github subject_include_branch shapes PR title", func(t *testing.T) {
		t.Parallel()

		// given: a release config that opts into branch-included PR subjects
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

		const configBody = `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
release:
  subject_include_branch: true
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`

		configPath := writeRawConfig(t, configBody)

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet opens the PR with the branch-included subject and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
