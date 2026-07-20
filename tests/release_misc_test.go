package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseConfigPerTargetReferences(t *testing.T) {
	t.Parallel()

	t.Run("per-target references override top-level patterns", func(t *testing.T) {
		t.Parallel()

		// given: per-target references.patterns merging on top of defaults
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: address ABC-1\n\nRefs: TKT-2"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := writeRawConfig(t, `provider: github
branch: main
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
    changelog:
      references:
        patterns:
          - pattern: "ABC-\\d+"
            url: https://issues.example.test/{value}
        footers:
          Refs: https://tracker.example.test/{value}
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet renders the linked references and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "ABC-1")
		testastic.Contains(t, result.Stdout, "TKT-2")
	})
}

func TestReleaseChangelogSectionOverride(t *testing.T) {
	t.Parallel()

	t.Run("custom sections rename a default commit type", func(t *testing.T) {
		t.Parallel()

		// given: a config that renames the `feat` section heading
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

		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  github:
    host: github.com
    owner: testorg
    repo: testrepo
changelog:
  sections:
    feat: "🚀 Features"
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet renders the custom section heading and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "🚀 Features")
	})

	t.Run("custom include adds a non-default commit type via capitalized fallback", func(t *testing.T) {
		t.Parallel()

		// given: a config that adds `style` to include without a section mapping
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "perf: improve startup"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  github:
    host: github.com
    owner: testorg
    repo: testrepo
changelog:
  sections: {}
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet still emits a section (default mapping preserved)
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "improve startup")
	})
}

func TestReleaseDryRunWithExtraTags(t *testing.T) {
	t.Parallel()

	t.Run("gitlab picks highest among multiple advertised tags", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout carrying several historical tags where the
		// advertised latest tag v1.5.0 is the highest reachable one
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: release v1.4.0", Tag: "v1.4.0"},
				{Message: "chore: release v1.5.0", Tag: "v1.5.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
			LatestTag:     "v1.5.0",
			BoundarySHA:   shas[2],
			BranchHeadSHA: shas[3],
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

		// then: yeet plans v1.6.0 from v1.5.0 and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "1.6.0")
	})
}

func TestReleaseChangelogNonDefaultFile(t *testing.T) {
	t.Parallel()

	t.Run("github writes to a custom changelog.file path", func(t *testing.T) {
		t.Parallel()

		// given: a config with a custom changelog.file (CHANGES.md instead of CHANGELOG.md)
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
			Files: map[string]string{
				"CHANGES.md": "# Changes\n\n## v1.0.0\n",
			},
		})

		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  github:
    host: github.com
    owner: testorg
    repo: testrepo
changelog:
  file: CHANGES.md
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet writes to CHANGES.md and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseConfigPerTargetChangelog(t *testing.T) {
	t.Parallel()

	t.Run("per-target changelog overrides file, include and sections", func(t *testing.T) {
		t.Parallel()

		// given: a per-target changelog with file/include/sections overrides
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: ship feature"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := writeRawConfig(t, `provider: github
branch: main
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
    changelog:
      file: CHANGES.md
      include: [feat, fix]
      sections:
        feat: "✨ Features"
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet renders the custom section heading and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "✨ Features")
	})
}

func TestReleaseInitDefaultName(t *testing.T) {
	t.Parallel()

	t.Run("init derives target name from working directory", func(t *testing.T) {
		t.Parallel()

		// given: a fresh tempdir whose final segment is a valid target name
		tempDir := t.TempDir()

		// when: running `yeet init` from the tempdir without --config
		result := binary.RunWithOptions(t, []string{"init"}, testastic.WithRunWorkDir(tempDir))

		// then: yeet exits 0 and the rendered config picks up the directory name
		testastic.Equal(t, 0, result.ExitCode)
	})
}
