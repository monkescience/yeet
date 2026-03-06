// Package provider defines the VCS provider interface and implementations
// for interacting with GitHub and GitLab APIs.
package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
)

type Release struct {
	TagName string
	Name    string
	Body    string
	URL     string
}

type PullRequest struct {
	Number int
	Title  string
	Body   string
	URL    string
	Branch string
}

const ReleaseLabelPending = "autorelease: pending"

const ReleaseLabelTagged = "autorelease: tagged"

type ReleasePROptions struct {
	Title         string
	Body          string
	BaseBranch    string
	ReleaseBranch string
	Files         map[string]string
}

type ReleaseOptions struct {
	TagName string
	Name    string
	Body    string
}

type MergeMethod = string

const (
	MergeMethodAuto   MergeMethod = "auto"
	MergeMethodSquash MergeMethod = "squash"
	MergeMethodRebase MergeMethod = "rebase"
	MergeMethodMerge  MergeMethod = "merge"
)

type MergeReleasePROptions struct {
	Force  bool
	Method MergeMethod
}

type CommitEntry struct {
	Hash    string
	Message string
}

//nolint:interfacebloat // Provider aggregates VCS operations required by the release flow.
type Provider interface {
	// GetLatestRelease returns the latest release/tag.
	GetLatestRelease(ctx context.Context) (*Release, error)
	// GetCommitsSince returns commits since the given ref (tag or SHA).
	GetCommitsSince(ctx context.Context, ref string) ([]CommitEntry, error)
	// CreateReleasePR creates a release PR/MR.
	CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error)
	// UpdateReleasePR updates an existing release PR/MR.
	UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error
	// FindOpenPendingReleasePRs finds open release PRs/MRs labeled pending for the base branch.
	FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error)
	// FindReleasePR finds an existing open release PR/MR.
	FindReleasePR(ctx context.Context, branch string) (*PullRequest, error)
	// FindMergedReleasePR finds the latest merged release PR/MR waiting for tagging.
	FindMergedReleasePR(ctx context.Context, baseBranch string) (*PullRequest, error)
	// CreateRelease creates a release with a tag.
	CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error)
	// MergeReleasePR merges an existing release PR/MR.
	MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) error
	// MarkReleasePRPending marks a release PR/MR as waiting for tagging.
	MarkReleasePRPending(ctx context.Context, number int) error
	// MarkReleasePRTagged marks a release PR/MR as tagged.
	MarkReleasePRTagged(ctx context.Context, number int) error
	// CreateBranch creates a new branch from the base branch.
	CreateBranch(ctx context.Context, name, base string) error
	// GetFile reads a file content from a branch.
	GetFile(ctx context.Context, branch, path string) (string, error)
	// UpdateFile creates or updates a file on a branch.
	UpdateFile(ctx context.Context, branch, path, content, message string) error
	// UpdateFiles force-updates a branch from base with one commit containing all file changes.
	UpdateFiles(ctx context.Context, branch, base string, files map[string]string, message string) error
	// RepoURL returns the HTTPS base URL for the repository.
	RepoURL() string
	// PathPrefix returns the path prefix for commit/compare URLs (empty for GitHub, "/-" for GitLab).
	PathPrefix() string
}

type RepoInfo struct {
	Owner string
	Name  string
}

func ParseCommits(entries []CommitEntry) []commit.Commit {
	commits := make([]commit.Commit, 0, len(entries))

	for _, e := range entries {
		commits = append(commits, commit.Parse(e.Hash, e.Message))
	}

	return commits
}

var ErrUnknownRemote = errors.New("unable to parse remote URL")

var ErrNoRelease = errors.New("no release found")

var ErrNoPR = errors.New("no release PR found")

var ErrFileNotFound = errors.New("file not found")

var ErrEmptyCommitSHA = errors.New("empty commit SHA")

var ErrEmptyCommitID = errors.New("empty commit ID")

var ErrMergeBlocked = errors.New("release PR merge blocked")

var ErrMergeMethodUnsupported = errors.New("merge method unsupported")

var remotePatterns = []*regexp.Regexp{
	// SSH format: git@github.com:owner/repo.git
	regexp.MustCompile(`^git@([^:]+):([^/]+)/([^/.]+?)(?:\.git)?$`),
	// HTTPS format: https://github.com/owner/repo.git
	regexp.MustCompile(`^https?://([^/]+)/([^/]+)/([^/.]+?)(?:\.git)?$`),
}

type ProviderFromRemote struct {
	Host  string
	Owner string
	Repo  string
}

func DetectFromRemote(remoteURL string) (*ProviderFromRemote, error) {
	remoteURL = strings.TrimSpace(remoteURL)

	for _, pattern := range remotePatterns {
		matches := pattern.FindStringSubmatch(remoteURL)
		if matches != nil {
			return &ProviderFromRemote{
				Host:  matches[1],
				Owner: matches[2],
				Repo:  matches[3],
			}, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrUnknownRemote, remoteURL)
}

// ProviderType returns "github" or "gitlab" based on the host.
func (p *ProviderFromRemote) ProviderType() string {
	host := strings.ToLower(p.Host)

	if strings.Contains(host, "github") {
		return "github"
	}

	if strings.Contains(host, "gitlab") {
		return "gitlab"
	}

	// Default to gitlab for self-hosted instances.
	return "gitlab"
}
