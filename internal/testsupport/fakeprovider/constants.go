package fakeprovider

// Shared canned values used by all provider fakes so the fixture corpus stays
// consistent across providers.
const (
	fakeNextTag       = "v1.1.0"
	fakeReleaseBranch = "yeet/release-main"
	fakeBaseBranch    = "main"
	fakeMergeSHA      = "6d65726765736861000000000000000000000000"
	fakeBaseSHA       = "6261736573686100000000000000000000000000"
)

// Common JSON keys repeated across provider payloads.
const (
	azureKeyCount    = "count"
	azureKeyValue    = "value"
	azureKeyObjectID = "objectId"
	azureKeyResults  = "results"
	keyCommits       = "commits"
)

// Shared values for merged-pending-release PR fixtures.
const (
	fakeStateMerged       = "merged"
	fakePendingReleaseTag = "autorelease: pending"
	fakeStateOpen         = "open"
	fakeMergedAtTimestamp = "2026-01-01T00:00:00Z"
	fakeAssociatedPRID    = 99
)
