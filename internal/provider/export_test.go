package provider

// ParseRemoteForTest exposes remote parsing to the external provider contract tests.
func ParseRemoteForTest(remoteURL string) (*repositoryDescriptor, error) {
	return parseRemote(remoteURL)
}

// DetectTypeForTest exposes forge detection to the external provider contract tests.
func DetectTypeForTest(host string) (string, error) {
	return detectType(host)
}
