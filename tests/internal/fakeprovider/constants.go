package fakeprovider

// Shared canned values used by all provider fakes so the fixture corpus stays
// consistent across providers.
const (
	fakeNextTag       = "v1.1.0"
	fakeReleaseBranch = "yeet/release-main"
	fakeBaseBranch    = "main"
	fakeMergeSHA      = "merge-sha"
	fakeBaseSHA       = "base-sha"
)

// Common JSON keys repeated across provider payloads.
const (
	azureKeyCount    = "count"
	azureKeyValue    = "value"
	azureKeyObjectID = "objectId"
)

// Shared values for merged-pending-release PR fixtures.
const (
	fakeStateMerged       = "merged"
	fakePendingReleaseTag = "autorelease: pending"
)
