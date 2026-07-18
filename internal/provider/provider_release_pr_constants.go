package provider

import "strings"

const releaseBranchPrefix = "yeet/release-"

func releaseBranchName(baseBranch string) string {
	return releaseBranchPrefix + strings.TrimSpace(baseBranch)
}

func isExpectedReleaseBranch(sourceBranch, baseBranch string) bool {
	return strings.TrimSpace(sourceBranch) == releaseBranchName(baseBranch)
}

// sortDirectionDesc is the descending sort token both GitHub and GitLab APIs accept.
const sortDirectionDesc = "desc"
