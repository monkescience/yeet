package release

import (
	"context"

	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
)

type versionHistoryProvider interface {
	ListTags(ctx context.Context) ([]string, error)
	// GetCommitsSinceRefs resolves each ref to a boundary commit. knownTags
	// supplies boundaries for refs the forge cannot be asked about yet, which is
	// how a run scans from a tag it published itself.
	GetCommitsSinceRefs(
		ctx context.Context,
		refs []string,
		branch string,
		includePaths bool,
		knownTags []forge.TagRef,
	) (history.CommitHistory, error)
}

type releaseSource interface {
	versionHistoryProvider
	GetFile(ctx context.Context, branch, path string) (string, error)
}

type repoMetadataProvider interface {
	RepoURL() string
	PathPrefix() string
	CompareURL(fromRef, toRef string) string
}

type releasePRProvider interface {
	FindOpenPendingReleasePRs(
		ctx context.Context,
		baseBranch, pendingLabel string,
	) ([]*forge.PullRequest, error)
	CreateReleasePR(ctx context.Context, opts forge.ReleasePROptions) (*forge.PullRequest, error)
	UpdateReleasePR(ctx context.Context, number int, opts forge.ReleasePROptions) error
	MergeReleasePR(ctx context.Context, number int, opts forge.MergeReleasePROptions) (string, error)
	releasePRLabelSetter
	MaxPRBodyLength() int
}

// releasePRLabelSetter states the phase a release PR is in and lets the forge
// work out the labels.
type releasePRLabelSetter interface {
	SetReleasePRLabels(
		ctx context.Context,
		number int,
		labels forge.ReleasePRLabels,
		phase forge.ReleasePRPhase,
	) error
}

type releasePRTaggingPreflighter interface {
	PreflightReleasePRTagging(ctx context.Context, taggedLabel string) error
}

type releaseFileProvider interface {
	GetFile(ctx context.Context, branch, path string) (string, error)
	UpdateFiles(ctx context.Context, branch, base string, files map[string]forge.FileUpdate, message string) error
}

type releasePublishingProvider interface {
	FindMergedReleasePR(ctx context.Context, baseBranch, pendingLabel string) (*forge.PullRequest, error)
	GetReleaseByTag(ctx context.Context, tag string) (*forge.Release, error)
	CreateRelease(ctx context.Context, opts forge.ReleaseOptions) (*forge.Release, error)
	releasePRLabelSetter
	releasePRTaggingPreflighter
}

// dependencies is the provider-side capability set. Version history is
// intentionally not part of it: commit ranges come from the local checkout
// through a separate releaseSource.
type dependencies struct {
	metadata  repoMetadataProvider
	prs       releasePRProvider
	files     releaseFileProvider
	publisher releasePublishingProvider
}
