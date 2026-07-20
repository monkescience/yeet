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
  github:
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
  github:
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
  github:
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
  github:
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
  github:
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

	t.Run("rejects reserved stable release channel", func(t *testing.T) {
		t.Parallel()

		// given: a release channel using the reserved stable name
		configPath := writeRawConfig(t, `provider: github
branch: main
release:
  channels:
    stable:
      branch: stable
      prerelease: stable
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
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and explains that stable is reserved
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "reserved name stable")
	})

	t.Run("rejects duplicate release channel branches", func(t *testing.T) {
		t.Parallel()

		// given: two prerelease channels pointing at the same branch
		configPath := writeRawConfig(t, `provider: github
branch: main
release:
  channels:
    beta:
      branch: prerelease
      prerelease: beta
    rc:
      branch: prerelease
      prerelease: rc
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
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the duplicate branch
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "duplicates release.channels")
	})

	t.Run("rejects duplicate release channel prerelease identifiers", func(t *testing.T) {
		t.Parallel()

		// given: two prerelease channels that would publish the same semver identifier
		configPath := writeRawConfig(t, `provider: github
branch: main
release:
  channels:
    beta:
      branch: beta
      prerelease: next
    rc:
      branch: rc
      prerelease: next
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
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the duplicate prerelease identifier
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "prerelease")
		testastic.Contains(t, result.Stderr, "duplicates")
	})

	t.Run("rejects invalid release channel prerelease identifier", func(t *testing.T) {
		t.Parallel()

		// given: a prerelease identifier that semver cannot encode
		configPath := writeRawConfig(t, `provider: github
branch: main
release:
  channels:
    beta:
      branch: beta
      prerelease: bad value
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
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and reports an invalid semver prerelease identifier
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "invalid semver prerelease identifier")
	})

	t.Run("rejects channel branch matching stable branch", func(t *testing.T) {
		t.Parallel()

		// given: a prerelease channel pointed at the stable release branch
		configPath := writeRawConfig(t, `provider: github
branch: main
release:
  channels:
    beta:
      branch: main
      prerelease: beta
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
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the stable-branch duplication
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "duplicates stable branch")
	})

	t.Run("rejects calver target with pre-major flags", func(t *testing.T) {
		t.Parallel()

		// given: a calver target with semver-only pre-major behavior configured
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
    versioning: calver
    pre_major_breaking_bumps_minor: true
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 instead of accepting a no-op semver-only setting
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "has no effect with calver")
	})

	t.Run("rejects unsupported version file format", func(t *testing.T) {
		t.Parallel()

		// given: a version file with an unknown format
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  github:
    host: github.com
    owner: testorg
    repo: testrepo
version_files:
  - path: package.json
    format: toml
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

		// then: yeet exits 1 and names the unsupported version file format
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "version_files")
		testastic.Contains(t, result.Stderr, "format")
	})

	t.Run("rejects json version file without json pointer", func(t *testing.T) {
		t.Parallel()

		// given: a JSON version file without a pointer to the version string
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  github:
    host: github.com
    owner: testorg
    repo: testrepo
version_files:
  - path: package.json
    format: json
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

		// then: yeet exits 1 and requires json_pointer for JSON files
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "json_pointer")
	})

	t.Run("rejects malformed json pointer escape", func(t *testing.T) {
		t.Parallel()

		// given: a JSON pointer containing an escape sequence not allowed by RFC 6901
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  github:
    host: github.com
    owner: testorg
    repo: testrepo
version_files:
  - path: package.json
    format: json
    json_pointer: /packages/~2/version
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

		// then: yeet exits 1 and reports the bad JSON pointer escape
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "invalid escape")
	})

	t.Run("rejects unknown auto_merge_method", func(t *testing.T) {
		t.Parallel()

		// given: a release config with an invalid auto_merge_method
		configPath := writeRawConfig(t, `provider: github
branch: main
release:
  auto_merge_method: wrongo
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
		root, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
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

		configPath := filepath.Join(root, ".yeet.yaml")

		const filePerm = 0o600

		configBody := `provider: github
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
