package release //nolint:testpackage // validates unexported CI and Git branch resolution directly

import (
	"context"
	"testing"

	"github.com/monkescience/testastic"
)

func TestCurrentGitBranch(t *testing.T) {
	t.Run("uses Azure Pipelines full source branch", func(t *testing.T) {
		// given: an Azure Pipelines checkout where Git HEAD cannot provide the branch
		t.Chdir(t.TempDir())
		clearCurrentBranchEnv(t)
		t.Setenv("BUILD_SOURCEBRANCH", " refs/heads/release/2026 ")

		// when: resolving the current branch
		branch, err := currentGitBranch(context.Background())

		// then: the heads prefix is removed without losing nested branch segments
		testastic.NoError(t, err)
		testastic.Equal(t, "release/2026", branch)
	})

	for name, testCase := range map[string]struct {
		ref       string
		wantError string
	}{
		"rejects Azure Pipelines pull request ref": {
			ref:       "refs/pull/123/merge",
			wantError: `ci ref is not a branch: "refs/pull/123/merge"`,
		},
		"rejects Azure Pipelines tag ref": {
			ref:       "refs/tags/v1.2.3",
			wantError: `ci ref is not a branch: "refs/tags/v1.2.3"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given: an Azure Pipelines source ref that is not a branch
			t.Chdir(t.TempDir())
			clearCurrentBranchEnv(t)
			t.Setenv("BUILD_SOURCEBRANCH", testCase.ref)

			// when: resolving the current branch
			branch, err := currentGitBranch(context.Background())

			// then: the ref is rejected instead of being returned as a branch
			testastic.Equal(t, "", branch)
			testastic.Error(t, err)
			testastic.Equal(t, testCase.wantError, err.Error())
		})
	}

	for name, refs := range map[string]struct {
		full      string
		name      string
		wantError string
	}{
		"rejects GitHub Actions pull request ref": {
			full:      "refs/pull/123/merge",
			name:      "123/merge",
			wantError: `ci ref is not a branch: "refs/pull/123/merge"`,
		},
		"rejects GitHub Actions tag ref": {
			full:      "refs/tags/v1.2.3",
			name:      "v1.2.3",
			wantError: `ci ref is not a branch: "refs/tags/v1.2.3"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given: a GitHub Actions ref that is not a branch
			t.Chdir(t.TempDir())
			clearCurrentBranchEnv(t)
			t.Setenv("GITHUB_REF", refs.full)
			t.Setenv("GITHUB_REF_NAME", refs.name)

			// when: resolving the current branch
			branch, err := currentGitBranch(context.Background())

			// then: the ref is rejected instead of its short name being returned as a branch
			testastic.Equal(t, "", branch)
			testastic.Error(t, err)
			testastic.Equal(t, refs.wantError, err.Error())
		})
	}
}

func clearCurrentBranchEnv(t *testing.T) {
	t.Helper()

	for _, envName := range []string{
		"GITHUB_REF",
		githubRefNameEnv,
		"CI_COMMIT_BRANCH",
		"BRANCH_NAME",
		"BUILD_SOURCEBRANCH",
	} {
		t.Setenv(envName, "")
	}
}
