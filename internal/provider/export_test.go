package provider

func ParseRemoteForTest(remoteURL string) (*repositoryDescriptor, error) {
	return parseRemote(remoteURL)
}

func DetectTypeForTest(host string) (string, error) {
	return detectType(host)
}

func ReleaseBranchNameForTest(configuredBranch, baseBranch string) string {
	return releaseBranchName(configuredBranch, baseBranch)
}

func IsExpectedReleaseBranchForTest(sourceBranch, baseBranch, configuredBranch string) bool {
	return isExpectedReleaseBranch(sourceBranch, baseBranch, configuredBranch)
}
