package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
)

const githubRefNameEnv = "GITHUB_REF_NAME"

var (
	errDetachedHead   = errors.New("git head is detached")
	errCINonBranchRef = errors.New("ci ref is not a branch")
)

func currentGitBranch(ctx context.Context) (string, error) {
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return "", fmt.Errorf("current git branch cancelled: %w", ctxErr)
	}

	githubRef := strings.TrimSpace(os.Getenv("GITHUB_REF"))
	if githubRef != "" {
		githubBranch, isBranch := strings.CutPrefix(githubRef, "refs/heads/")
		if !isBranch || githubBranch == "" {
			return "", fmt.Errorf("%w: %q", errCINonBranchRef, githubRef)
		}

		return githubBranch, nil
	}

	for _, envName := range []string{githubRefNameEnv, "CI_COMMIT_BRANCH", "BRANCH_NAME"} {
		branch := strings.TrimSpace(os.Getenv(envName))
		if branch != "" {
			return branch, nil
		}
	}

	azureRef := strings.TrimSpace(os.Getenv("BUILD_SOURCEBRANCH"))
	if azureRef != "" {
		azureBranch, isBranch := strings.CutPrefix(azureRef, "refs/heads/")
		if !isBranch || azureBranch == "" {
			return "", fmt.Errorf("%w: %q", errCINonBranchRef, azureRef)
		}

		return azureBranch, nil
	}

	repository, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return "", fmt.Errorf("open git repository: %w", err)
	}

	head, err := repository.Head()
	if err != nil {
		return "", fmt.Errorf("read git head: %w", err)
	}

	if !head.Name().IsBranch() {
		return "", fmt.Errorf("%w: %s", errDetachedHead, head.Hash())
	}

	return head.Name().Short(), nil
}
