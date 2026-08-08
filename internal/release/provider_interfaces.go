package release

import (
	"context"

	"github.com/monkescience/yeet/internal/history"
	"github.com/monkescience/yeet/internal/provider"
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
		knownTags []provider.TagRef,
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
	PrepareReleasePRLabels(ctx context.Context, labels provider.ReleasePRLabels) error
	FindOpenPendingReleasePRs(
		ctx context.Context,
		baseBranch, pendingLabel string,
	) ([]*provider.PullRequest, error)
	CreateReleasePR(ctx context.Context, opts provider.ReleasePROptions) (*provider.PullRequest, error)
	UpdateReleasePR(ctx context.Context, number int, opts provider.ReleasePROptions) error
	MergeReleasePR(ctx context.Context, number int, opts provider.MergeReleasePROptions) (string, error)
	MarkReleasePRPending(ctx context.Context, number int, labels provider.ReleasePRLabels) error
	MaxPRBodyLength() int
}

type releaseFileProvider interface {
	GetFile(ctx context.Context, branch, path string) (string, error)
	UpdateFiles(ctx context.Context, branch, base string, files map[string]provider.FileUpdate, message string) error
}

type releasePublishingProvider interface {
	PrepareReleasePRLabels(ctx context.Context, labels provider.ReleasePRLabels) error
	FindMergedReleasePR(ctx context.Context, baseBranch, pendingLabel string) (*provider.PullRequest, error)
	GetReleaseByTag(ctx context.Context, tag string) (*provider.Release, error)
	CreateRelease(ctx context.Context, opts provider.ReleaseOptions) (*provider.Release, error)
	MarkReleasePRTagged(ctx context.Context, number int, labels provider.ReleasePRLabels) error
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
