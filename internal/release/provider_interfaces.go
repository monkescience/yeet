package release

import (
	"context"

	"github.com/monkescience/yeet/internal/provider"
)

type versionHistoryProvider interface {
	GetLatestVersionRef(ctx context.Context) (string, error)
	ListTags(ctx context.Context) ([]string, error)
	GetCommitsSince(ctx context.Context, ref, branch string) ([]provider.CommitEntry, error)
}

type repoMetadataProvider interface {
	RepoURL() string
	PathPrefix() string
}

type releasePRProvider interface {
	FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*provider.PullRequest, error)
	CreateReleasePR(ctx context.Context, opts provider.ReleasePROptions) (*provider.PullRequest, error)
	UpdateReleasePR(ctx context.Context, number int, opts provider.ReleasePROptions) error
	MergeReleasePR(ctx context.Context, number int, opts provider.MergeReleasePROptions) error
	MarkReleasePRPending(ctx context.Context, number int) error
	CreateBranch(ctx context.Context, name, base string) error
}

type releaseFileProvider interface {
	GetFile(ctx context.Context, branch, path string) (string, error)
	UpdateFiles(ctx context.Context, branch, base string, files map[string]string, message string) error
}

type releasePublishingProvider interface {
	FindMergedReleasePR(ctx context.Context, baseBranch string) (*provider.PullRequest, error)
	GetReleaseByTag(ctx context.Context, tag string) (*provider.Release, error)
	TagExists(ctx context.Context, tag string) (bool, error)
	CreateRelease(ctx context.Context, opts provider.ReleaseOptions) (*provider.Release, error)
	MarkReleasePRTagged(ctx context.Context, number int) error
	GetFile(ctx context.Context, branch, path string) (string, error)
}

type releaserDependencies interface {
	versionHistoryProvider
	repoMetadataProvider
	releasePRProvider
	releaseFileProvider
	releasePublishingProvider
}
