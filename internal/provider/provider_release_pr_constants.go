package provider

import "strings"

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

const sortDirectionDesc = "desc"
