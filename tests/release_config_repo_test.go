package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestReleaseRepositoryValidation(t *testing.T) {
	t.Parallel()

	t.Run("rejects blank repository.remote", func(t *testing.T) {
		t.Parallel()

		// given: a config that empties repository.remote
		configPath := absoluteTestFile(t, "testdata/release/rejects_blank_repository_remote/input.yaml")

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with a remote validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_blank_repository_remote/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects blank repository.host", func(t *testing.T) {
		t.Parallel()

		// given: a host set to whitespace
		configPath := absoluteTestFile(t, "testdata/release/rejects_blank_repository_host/input.yaml")

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr names the blank host
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_blank_repository_host/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects azuredevops config without project", func(t *testing.T) {
		t.Parallel()

		// given: an azure config missing project
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_azuredevops_config_without_project/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says project is required
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_azuredevops_config_without_project/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects owner without repo for github", func(t *testing.T) {
		t.Parallel()

		// given: github config with owner but no repo
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_owner_without_repo_for_github/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says owner+repo must be set together
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_owner_without_repo_for_github/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects mismatched project vs owner/repo", func(t *testing.T) {
		t.Parallel()

		// given: a config where project does not match owner/repo
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_mismatched_project_vs_owner/repo/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says project must match
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_mismatched_project_vs_owner/repo/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects github owner containing slash", func(t *testing.T) {
		t.Parallel()

		// given: a github config where owner contains '/'
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_github_owner_containing_slash/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says owner must not contain '/'
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_github_owner_containing_slash/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects target.path empty", func(t *testing.T) {
		t.Parallel()

		// given: a target with empty path
		configPath := absoluteTestFile(t, "testdata/release/rejects_target_path_empty/input.yaml")

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says path must not be empty
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_target_path_empty/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects target.path outside repository", func(t *testing.T) {
		t.Parallel()

		// given: a target path that escapes the repository root
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_target_path_outside_repository/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and requires repo-relative target paths
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_target_path_outside_repository/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects absolute target path", func(t *testing.T) {
		t.Parallel()

		// given: an absolute target path
		configPath := absoluteTestFile(t, "testdata/release/rejects_absolute_target_path/input.yaml")

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and requires repo-relative target paths
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_absolute_target_path/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects exclude path outside target path", func(t *testing.T) {
		t.Parallel()

		// given: a target excluding a sibling path it does not own
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_exclude_path_outside_target_path/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and requires excludes to be below their target path
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_exclude_path_outside_target_path/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects github project not in owner/repo form", func(t *testing.T) {
		t.Parallel()

		// given: a github config with a malformed project (too many segments)
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_github_project_not_in_owner/repo_form/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says project must be in owner/repo form
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_github_project_not_in_owner/repo_form/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects target.tag_prefix empty", func(t *testing.T) {
		t.Parallel()

		// given: a target with empty tag_prefix
		configPath := absoluteTestFile(t, "testdata/release/rejects_target_tag_prefix_empty/input.yaml")

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 (validation rejects empty tag_prefix or downstream fails)
		testastic.Equal(t, 1, result.ExitCode)
	})

	t.Run("rejects duplicate target tag prefixes", func(t *testing.T) {
		t.Parallel()

		// given: two targets that would try to publish the same tag names
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_duplicate_target_tag_prefixes/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and points at the duplicate tag prefix
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_duplicate_target_tag_prefixes/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects derived target including unknown target", func(t *testing.T) {
		t.Parallel()

		// given: a derived target that references a target that does not exist
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_derived_target_including_unknown_target/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the invalid include
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_derived_target_including_unknown_target/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects derived target including another derived target", func(t *testing.T) {
		t.Parallel()

		// given: a derived target graph that nests another derived target
		configPath := absoluteTestFile(
			t,
			"testdata/release/"+
				"rejects_derived_target_including_another_derived_target/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 because derived targets may only include path targets
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"rejects_derived_target_including_another_derived_target/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects overlapping direct path ownership", func(t *testing.T) {
		t.Parallel()

		// given: one target owns services and another owns a child path under it
		configPath := absoluteTestFile(
			t,
			"testdata/release/rejects_overlapping_direct_path_ownership/"+
				"input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 instead of allowing ambiguous target ownership
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/rejects_overlapping_direct_path_ownership/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("allows direct path overlap excluded by parent target", func(t *testing.T) {
		t.Parallel()

		// given: a parent target explicitly excludes the child target's path
		configPath := absoluteTestFile(
			t,
			"testdata/release/"+
				"allows_direct_path_overlap_excluded_by_parent_target/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: validation allows the disjoint ownership and later fails only because no token/server exists
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"allows_direct_path_overlap_excluded_by_parent_target/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects duplicate version file ownership across targets", func(t *testing.T) {
		t.Parallel()

		// given: two targets configured to edit the same version file
		configPath := absoluteTestFile(
			t,
			"testdata/release/"+
				"rejects_duplicate_version_file_ownership_across_targets/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 before one target can overwrite another target's file
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"rejects_duplicate_version_file_ownership_across_targets/stderr.expected.txt",
			result.Stderr,
		)
	})
}
