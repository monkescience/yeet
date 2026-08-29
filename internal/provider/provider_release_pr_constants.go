package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/forge"
)

const defaultReleaseBranchPrefix = "yeet/release-"

func releaseBranchName(configuredBranch, baseBranch string) string {
	configuredBranch = strings.TrimSpace(configuredBranch)
	if configuredBranch != "" {
		return configuredBranch
	}

	return defaultReleaseBranchPrefix + strings.TrimSpace(baseBranch)
}

func isExpectedReleaseBranch(sourceBranch, baseBranch, configuredBranch string) bool {
	return strings.TrimSpace(sourceBranch) == releaseBranchName(configuredBranch, baseBranch)
}

func expectedReleaseBranch(configuredBranch, baseBranch string, expectedBranches []string) string {
	if len(expectedBranches) > 0 && strings.TrimSpace(expectedBranches[0]) != "" {
		return strings.TrimSpace(expectedBranches[0])
	}

	return releaseBranchName(configuredBranch, baseBranch)
}

func mergeExpectedReleaseBranch(configuredBranch, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}

	return configuredBranch
}

func collectExpectedBranchPRs(
	branches []string,
	find func(string) ([]*forge.PullRequest, error),
) ([]*forge.PullRequest, error) {
	collected := make([]*forge.PullRequest, 0)

	for _, branch := range branches {
		pullRequests, err := find(branch)
		if err != nil {
			return nil, err
		}

		collected = append(collected, pullRequests...)
	}

	return collected, nil
}

func collectExpectedBranchMergedPRs(
	branches []string,
	find func(string) (*forge.PullRequest, error),
) ([]*forge.PullRequest, error) {
	collected := make([]*forge.PullRequest, 0, len(branches))
	errList := make([]error, 0)

	for _, branch := range branches {
		pullRequest, err := find(branch)
		if errors.Is(err, forge.ErrNoPR) {
			continue
		}

		if err != nil {
			errList = append(errList, fmt.Errorf("find merged release request for branch %q: %w", branch, err))

			continue
		}

		collected = append(collected, pullRequest)
	}

	return collected, errors.Join(errList...)
}

const sortDirectionDesc = "desc"
