package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v6"
	gitconfig "github.com/go-git/go-git/v6/config"
)

var (
	ErrGitRemoteNotFound = errors.New("git remote not found")
	ErrGitRemoteHasNoURL = errors.New("git remote has no url")
	ErrGitRemoteURLBlank = errors.New("git remote url is blank")
)

func gitRemoteURL(_ context.Context, remote string) (string, error) {
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
