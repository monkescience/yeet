package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func writeRawConfig(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, ".yeet.yaml")

	const filePerm = 0o600

	err := os.WriteFile(path, []byte(body), filePerm)
	testastic.NoError(t, err)

	return path
}

func TestReleaseConfigValidation(t *testing.T) {
	t.Parallel()

	t.Run("rejects invalid versioning value", func(t *testing.T) {
		t.Parallel()

		// given: a config whose versioning is neither semver nor calver
		configPath := writeRawConfig(t, `provider: github
branch: main
versioning: bogus
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with a versioning validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "versioning must be")
	})

	t.Run("rejects calver format without required tokens", func(t *testing.T) {
		t.Parallel()

		// given: a calver config whose format is invalid
		configPath := writeRawConfig(t, `provider: github
branch: main
versioning: calver
calver:
  format: NOPE
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr complains about the calver format
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "calver")
	})

	t.Run("rejects unknown commit type under bump_types", func(t *testing.T) {
		t.Parallel()

		// given: a bump_types entry naming an unsupported commit type
		configPath := writeRawConfig(t, `provider: github
branch: main
bump_types:
  minor: [""]
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with a bump_types validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "bump_types")
	})

	t.Run("rejects empty changelog include", func(t *testing.T) {
		t.Parallel()

		// given: a config that empties the changelog include list
		configPath := writeRawConfig(t, `provider: github
branch: main
changelog:
  include: []
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with a changelog.include validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "changelog.include")
	})

	t.Run("rejects channel with no branch", func(t *testing.T) {
		t.Parallel()

		// given: a release channel missing the branch field
		configPath := writeRawConfig(t, `provider: github
branch: main
release:
  channels:
    beta:
      prerelease: beta
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run --channel beta`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--channel", "beta", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=beta"),
		)

		// then: yeet exits 1 with a channel-branch validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "channel")
	})

	t.Run("rejects unknown auto_merge_method", func(t *testing.T) {
		t.Parallel()

		// given: a release config with an invalid auto_merge_method
		configPath := writeRawConfig(t, `provider: github
branch: main
release:
  auto_merge_method: wrongo
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the invalid method in the error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "auto_merge_method")
	})
}

func TestReleaseExplicitConfigDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("resolves config search root from a deep subdirectory", func(t *testing.T) {
		t.Parallel()

		// given: a config at the repo root and a yeet invocation from a nested dir
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

		root := t.TempDir()
		configPath := filepath.Join(root, ".yeet.yaml")

		const filePerm = 0o600

		configBody := `provider: github
branch: main
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

		err := os.WriteFile(configPath, []byte(configBody), filePerm)
		testastic.NoError(t, err)

		nested := filepath.Join(root, "a", "b", "c")
		err = os.MkdirAll(nested, 0o755)
		testastic.NoError(t, err)

		// when: invoking `yeet release --dry-run` from the nested dir without --config
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run"},
			testastic.WithRunWorkDir(nested),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet walks up to find .yeet.yaml at the root and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
