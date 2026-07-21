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

		// given: version_files written as a list of scalar paths and a local
		// checkout with one releasable commit since v1.0.0
		files := map[string]string{"VERSION.txt": "1.0.0 # x-yeet-version\n"}
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

		const configBody = `provider: github
branch: main
repository:
  github:
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
			testastic.WithRunWorkDir(repoDir),
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

		// given: two targets where one overrides versioning + tag_prefix, with
		// both prior tags present in the local checkout
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release api", Tag: "api-v1.0.0", Files: map[string]string{"CHANGELOG.md": "api release\n"}},
				{Message: "chore: release web", Tag: "web-v2025.05.0", Files: map[string]string{"CHANGELOG.md": "web release\n"}},
				{Message: "feat: api change", Files: map[string]string{"api/handler.go": "package api\n"}},
				{Message: "feat: web change", Files: map[string]string{"web/index.html": "<html></html>\n"}},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "api-v1.0.0",
			ExtraTags:     []string{"web-v2025.05.0"},
			BoundarySHA:   shas[0],
			TagSHAs:       map[string]string{"api-v1.0.0": shas[0], "web-v2025.05.0": shas[1]},
			BranchHeadSHA: shas[3],
		})

		const configBody = `provider: github
branch: main
repository:
  github:
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
			testastic.WithRunWorkDir(repoDir),
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
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: minor housekeeping"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		const configBody = `provider: github
branch: main
repository:
  github:
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
			testastic.WithRunWorkDir(repoDir),
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
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release", Tag: "v1.0.0", Files: map[string]string{"CHANGELOG.md": "release\n"}},
				{Message: "feat: api change", Files: map[string]string{"services/api/handler.go": "package api\n"}},
				{Message: "feat: root change", Files: map[string]string{"README.md": "root change\n"}},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[2],
		})

		const configBody = `provider: github
branch: main
repository:
  github:
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
			testastic.WithRunWorkDir(repoDir),
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
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v0.3.0", Tag: "v0.3.0"},
				{Message: "feat!: breaking change\n\nBREAKING CHANGE: api"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v0.3.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		const configBody = `provider: github
branch: main
pre_major_breaking_bumps_minor: false
pre_major_features_bump_patch: false
repository:
  github:
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the breaking change bumps to v1.0.0 instead of v0.4.0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "1.0.0")
	})
}
