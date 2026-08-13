package provider

func ParseRemoteForTest(remoteURL string) (*repositoryDescriptor, error) {
	return parseRemote(remoteURL)
}

func DetectTypeForTest(host string) (string, error) {
	return detectType(host)
}
