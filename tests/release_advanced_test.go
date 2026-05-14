package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseScalarVersionFile(t *testing.T) {
	t.Parallel()

	t.Run("github accepts scalar shorthand for version_files entries", func(t *testing.T) {
		t.Parallel()

		// given: version_files written as a list of scalar paths
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

		const configBody = `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
version_files:
  - VERSION.txt
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

		// then: yeet treats the scalar as a markers-format version file and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleasePerTargetOverrides(t *testing.T) {
	t.Parallel()

	t.Run("github honours per-target versioning and tag_prefix", func(t *testing.T) {
		t.Parallel()

		// given: two targets where one overrides versioning + tag_prefix
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "api-v1.0.0",
			ExtraTags:   []string{"web-v2025.05.0"},
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: api change", Files: []string{"api/handler.go"}},
				{SHA: "web-sha", Message: "feat: web change", Files: []string{"web/index.html"}},
				{SHA: "boundary-sha", Message: "chore: release", Files: []string{"CHANGELOG.md"}},
			},
		})

		const configBody = `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  api:
    type: path
    path: api/
    tag_prefix: api-v
  web:
    type: path
    path: web/
    tag_prefix: web-v
    versioning: calver
    calver:
      format: YYYY.0M.MICRO
`

		configPath := writeRawConfig(t, configBody)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans semver for api/ and calver for web/, exiting 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "api-v")
		testastic.Contains(t, result.Stdout, "web-v")
	})
}

func TestReleaseCustomBumpTypes(t *testing.T) {
	t.Parallel()

	t.Run("github classifies `chore` as patch when configured under bump_types", func(t *testing.T) {
		t.Parallel()

		// given: bump_types config that promotes `chore` to a patch bump
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "chore: minor housekeeping"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		const configBody = `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
bump_types:
  minor: [feat]
  patch: [fix, perf, chore]
changelog:
  include: [feat, fix, perf, chore]
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`

		configPath := writeRawConfig(t, configBody)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet bumps the patch version and emits a Miscellaneous Chores section
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "1.0.1")
		testastic.Contains(t, result.Stdout, "Miscellaneous Chores")
	})
}

func TestReleaseDerivedExclude(t *testing.T) {
	t.Parallel()

	t.Run("github derived target ignores path-target commits via exclude_paths", func(t *testing.T) {
		t.Parallel()

		// given: a derived `root` target excluding the path-target subtree
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: root change", Files: []string{"README.md"}},
				{SHA: "child-sha", Message: "feat: api change", Files: []string{"services/api/handler.go"}},
				{SHA: "boundary-sha", Message: "chore: release", Files: []string{"CHANGELOG.md"}},
			},
		})

		const configBody = `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  api:
    type: path
    path: services/api
    tag_prefix: api-v
  root:
    type: derived
    path: .
    tag_prefix: v
    includes:
      - api
    exclude_paths:
      - services/api
`

		configPath := writeRawConfig(t, configBody)

		// when: invoking `yeet release --dry-run --target root`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--target", "root", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans only the root target's commit, exiting 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "root")
		testastic.Contains(t, result.Stdout, "root change")
	})
}

func TestReleasePreMajorOverride(t *testing.T) {
	t.Parallel()

	t.Run("github respects pre_major_breaking_bumps_minor=false", func(t *testing.T) {
		t.Parallel()

		// given: a project still on 0.x with pre_major_breaking_bumps_minor disabled
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v0.3.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat!: breaking change\n\nBREAKING CHANGE: api"},
				{SHA: "boundary-sha", Message: "chore: release v0.3.0"},
			},
		})

		const configBody = `provider: github
branch: main
pre_major_breaking_bumps_minor: false
pre_major_features_bump_patch: false
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`

		configPath := writeRawConfig(t, configBody)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the breaking change bumps to v1.0.0 instead of v0.4.0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "1.0.0")
	})
}
