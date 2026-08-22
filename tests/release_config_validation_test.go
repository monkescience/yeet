package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseConfigValidation(t *testing.T) {
	t.Parallel()

	t.Run("rejects invalid versioning value", func(t *testing.T) {
		t.Parallel()

		// given: a config whose versioning is neither semver nor calver
		configPath := absoluteTestFile(t, "testdata/release/rejects_invalid_versioning_value/input.yaml")

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with a versioning validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_invalid_versioning_value/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects calver format without required tokens", func(t *testing.T) {
		t.Parallel()

		// given: a calver config whose format is invalid
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_calver_format_without_required_tokens/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr complains about the calver format
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_calver_format_without_required_tokens/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects unknown commit type under bump_types", func(t *testing.T) {
		t.Parallel()

		// given: a bump_types entry naming an unsupported commit type
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_unknown_commit_type_under_bump_types/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with a bump_types validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_unknown_commit_type_under_bump_types/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects empty changelog include", func(t *testing.T) {
		t.Parallel()

		// given: a config that empties the changelog include list
		configPath := absoluteTestFile(t, "testdata/release/rejects_empty_changelog_include/input.yaml")

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with a changelog.include validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_empty_changelog_include/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects channel with no branch", func(t *testing.T) {
		t.Parallel()

		// given: a release channel missing the branch field
		configPath := absoluteTestFile(t, "testdata/release/rejects_channel_with_no_branch/input.yaml")

		// when: invoking `yeet release --dry-run --channel beta`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--channel", "beta", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=beta"),
		)

		// then: yeet exits 1 with a channel-branch validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_channel_with_no_branch/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects reserved stable release channel", func(t *testing.T) {
		t.Parallel()

		// given: a release channel using the reserved stable name
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_reserved_stable_release_channel/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and explains that stable is reserved
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_reserved_stable_release_channel/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects duplicate release channel branches", func(t *testing.T) {
		t.Parallel()

		// given: two prerelease channels pointing at the same branch
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_duplicate_release_channel_branches/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the duplicate branch
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_duplicate_release_channel_branches/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects duplicate release channel prerelease identifiers", func(t *testing.T) {
		t.Parallel()

		// given: two prerelease channels that would publish the same semver identifier
		configPath := absoluteTestFile(
			t,
			"testdata/release/"+
				"rejects_duplicate_release_channel_prerelease_identifiers/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the duplicate prerelease identifier
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"rejects_duplicate_release_channel_prerelease_identifiers/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects invalid release channel prerelease identifier", func(t *testing.T) {
		t.Parallel()

		// given: a prerelease identifier that semver cannot encode
		configPath := absoluteTestFile(
			t,
			"testdata/release/"+
				"rejects_invalid_release_channel_prerelease_identifier/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and reports an invalid semver prerelease identifier
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"rejects_invalid_release_channel_prerelease_identifier/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects channel branch matching stable branch", func(t *testing.T) {
		t.Parallel()

		// given: a prerelease channel pointed at the stable release branch
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_channel_branch_matching_stable_branch/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the stable-branch duplication
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_channel_branch_matching_stable_branch/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects calver target with pre-major flags", func(t *testing.T) {
		t.Parallel()

		// given: a calver target with semver-only pre-major behavior configured
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_calver_target_with_pre_major_flags/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 instead of accepting a no-op semver-only setting
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_calver_target_with_pre_major_flags/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects unsupported version file format", func(t *testing.T) {
		t.Parallel()

		// given: a version file with an unknown format
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_unsupported_version_file_format/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the unsupported version file format
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_unsupported_version_file_format/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects json version file without json pointer", func(t *testing.T) {
		t.Parallel()

		// given: a JSON version file without a pointer to the version string
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_json_version_file_without_json_pointer/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and requires json_pointer for JSON files
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_json_version_file_without_json_pointer/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects malformed json pointer escape", func(t *testing.T) {
		t.Parallel()

		// given: a JSON pointer containing an escape sequence not allowed by RFC 6901
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_malformed_json_pointer_escape/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and reports the bad JSON pointer escape
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_malformed_json_pointer_escape/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects unknown auto_merge_method", func(t *testing.T) {
		t.Parallel()

		// given: a release config with an invalid auto_merge_method
		configPath := absoluteTestFile(t, "testdata/release/rejects_unknown_auto_merge_method/input.yaml")

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the invalid method in the error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_unknown_auto_merge_method/stderr.expected.txt",
			result.Stderr,
		)
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

		configBody, err := os.ReadFile(absoluteTestFile(
			t,
			"testdata/release/"+
				"resolves_config_search_root_from_a_deep_subdirectory/input.yaml",
		))
		testastic.NoError(t, err)

		err = os.WriteFile(configPath, configBody, filePerm)
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
