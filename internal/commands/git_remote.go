package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	gitconfig "github.com/go-git/go-git/v6/config"
)

var (
	ErrGitRemoteNotFound = errors.New("git remote not found")
	ErrGitRemoteHasNoURL = errors.New("git remote has no url")
	ErrGitRemoteURLBlank = errors.New("git remote url is blank")
	ErrDetachedHead      = errors.New("git head is detached")
)

func getGitRemoteURL(_ context.Context, remote string) (string, error) {
	repository, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return "", fmt.Errorf("open git repository: %w", err)
	}

	repositoryConfig, err := repository.Config()
	if err != nil {
		return "", fmt.Errorf("read git config: %w", err)
	}

	remoteConfig, exists := repositoryConfig.Remotes[remote]
	if !exists {
		return "", fmt.Errorf("%w: %q", ErrGitRemoteNotFound, remote)
	}

	if len(remoteConfig.URLs) == 0 {
		return "", fmt.Errorf("%w: %q", ErrGitRemoteHasNoURL, remote)
	}

	remoteURL := strings.TrimSpace(remoteConfig.URLs[0])
	if remoteURL == "" {
		return "", fmt.Errorf("%w: %q", ErrGitRemoteURLBlank, remote)
	}

	return rewriteGitRemoteURL(remoteURL, repositoryConfig), nil
}

func currentGitBranch(ctx context.Context) (string, error) {
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return "", fmt.Errorf("current git branch cancelled: %w", ctxErr)
	}

	for _, envName := range []string{"GITHUB_REF_NAME", "CI_COMMIT_BRANCH", "BRANCH_NAME"} {
		branch := strings.TrimSpace(os.Getenv(envName))
		if branch != "" {
			return branch, nil
		}
	}

	azureBranch := strings.TrimPrefix(strings.TrimSpace(os.Getenv("BUILD_SOURCEBRANCH")), "refs/heads/")
	if azureBranch != "" {
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
		return "", fmt.Errorf("%w: %s", ErrDetachedHead, head.Hash())
	}

	return head.Name().Short(), nil
}

func rewriteGitRemoteURL(remoteURL string, repositoryConfig *gitconfig.Config) string {
	if repositoryConfig == nil {
		return remoteURL
	}

	rewrittenURL := remoteURL
	longestMatchLength := 0

	for _, rule := range repositoryConfig.URLs {
		for _, insteadOfValue := range rule.InsteadOfs {
			insteadOf := strings.TrimSpace(insteadOfValue)
			if insteadOf == "" || !strings.HasPrefix(remoteURL, insteadOf) {
				continue
			}

			if len(insteadOf) <= longestMatchLength {
				continue
			}

			rewrittenURL = rule.ApplyInsteadOf(remoteURL)
			longestMatchLength = len(insteadOf)
		}
	}

	return rewrittenURL
}
