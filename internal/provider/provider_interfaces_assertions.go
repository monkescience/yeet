package provider

var (
	_ Provider = (*GitHub)(nil)
	_ Provider = (*GitLab)(nil)

	_ versionHistoryProvider = (*GitHub)(nil)
	_ versionHistoryProvider = (*GitLab)(nil)

	_ releaseLookupProvider = (*GitHub)(nil)
	_ releaseLookupProvider = (*GitLab)(nil)

	_ releasePRProvider = (*GitHub)(nil)
	_ releasePRProvider = (*GitLab)(nil)

	_ repoContentProvider = (*GitHub)(nil)
	_ repoContentProvider = (*GitLab)(nil)

	_ repoMetadataProvider = (*GitHub)(nil)
	_ repoMetadataProvider = (*GitLab)(nil)
)
