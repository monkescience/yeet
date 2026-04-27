// Package provider defines the VCS provider interface and implementations
// for interacting with GitHub and GitLab APIs.
package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	Number         int
	Title          string
	Body           string
	URL            string
	Branch         string
	MergeCommitSHA string
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
	TagName    string
	Ref        string
	Name       string
	Body       string
	Prerelease bool
}

type MergeMethod string

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
	Paths   []string
}

type versionHistoryProvider interface {
	// GetLatestVersionRef returns the preferred release/tag baseline candidate.
	GetLatestVersionRef(ctx context.Context) (string, error)
	// ListTags returns repository tags.
	ListTags(ctx context.Context) ([]string, error)
	// GetCommitsSince returns commits on the given branch since the given ref (tag or SHA).
	// When includePaths is true, each entry includes the list of changed file paths.
	GetCommitsSince(ctx context.Context, ref, branch string, includePaths bool) ([]CommitEntry, error)
}

type releaseLookupProvider interface {
	// GetReleaseByTag returns the release for the exact tag.
	GetReleaseByTag(ctx context.Context, tag string) (*Release, error)
	// TagExists reports whether the exact tag already exists.
	TagExists(ctx context.Context, tag string) (bool, error)
	// CreateRelease creates a release with a tag.
	CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error)
}

type releasePRProvider interface {
	// CreateReleasePR creates a release PR/MR.
	CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error)
	// UpdateReleasePR updates an existing release PR/MR.
	UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error
	// FindOpenPendingReleasePRs finds open release PRs/MRs labeled pending for the base branch.
	FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error)
	// FindMergedReleasePR finds the latest merged release PR/MR waiting for tagging.
	FindMergedReleasePR(ctx context.Context, baseBranch string) (*PullRequest, error)
	// MergeReleasePR merges an existing release PR/MR.
	MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) error
	// MarkReleasePRPending marks a release PR/MR as waiting for tagging.
	MarkReleasePRPending(ctx context.Context, number int) error
	// MarkReleasePRTagged marks a release PR/MR as tagged.
	MarkReleasePRTagged(ctx context.Context, number int) error
	// CommitPullRequestBody returns the merged PR/MR body associated with the commit hash.
	CommitPullRequestBody(ctx context.Context, hash string) (string, bool, error)
}

type repoContentProvider interface {
	// CreateBranch creates a new branch from the base branch.
	CreateBranch(ctx context.Context, name, base string) error
	// GetFile reads a file content from a branch.
	GetFile(ctx context.Context, branch, path string) (string, error)
	// UpdateFiles force-updates a branch from base with one commit containing all file changes.
	UpdateFiles(ctx context.Context, branch, base string, files map[string]string, message string) error
}

type repoMetadataProvider interface {
	// RepoURL returns the HTTPS base URL for the repository.
	RepoURL() string
	// PathPrefix returns the path prefix for commit/compare URLs (empty for GitHub, "/-" for GitLab).
	PathPrefix() string
}

type Provider interface {
	versionHistoryProvider
	releaseLookupProvider
	releasePRProvider
	repoContentProvider
	repoMetadataProvider
}

type RepoInfo struct {
	Owner string
	Name  string
}

const DefaultGitHubHost = "github.com"

const DefaultGitLabHost = "gitlab.com"

func ParseCommits(entries []CommitEntry) []commit.Commit {
	commits := make([]commit.Commit, 0, len(entries))

	for _, e := range entries {
		commits = append(commits, commit.Parse(e.Hash, e.Message))
	}

	return commits
}

var ErrUnknownRemote = errors.New("unable to parse remote URL")

var ErrNoRelease = errors.New("no release found")

var ErrNoVersionRef = errors.New("no version ref found")

// ErrCommitBoundaryNotFound reports that the requested base ref is not reachable from the target branch history.
var ErrCommitBoundaryNotFound = errors.New("commit boundary not found")

var ErrNoPR = errors.New("no release PR found")

var ErrFileNotFound = errors.New("file not found")

var ErrEmptyCommitSHA = errors.New("empty commit SHA")

var ErrEmptyCommitID = errors.New("empty commit ID")

var ErrMergeBlocked = errors.New("release PR merge blocked")

var ErrMergeMethodUnsupported = errors.New("merge method unsupported")

var ErrPaginationLimitExceeded = errors.New("pagination safety limit exceeded")

const maxPaginationPages = 100

// CommitBoundaryNotFoundError includes the missing boundary ref and the branch being analyzed.
type CommitBoundaryNotFoundError struct {
	Ref    string
	Branch string
}

func (e *CommitBoundaryNotFoundError) Error() string {
	ref := strings.TrimSpace(e.Ref)
	branch := strings.TrimSpace(e.Branch)

	switch {
	case ref == "" && branch == "":
		return ErrCommitBoundaryNotFound.Error()
	case branch == "":
		return fmt.Sprintf("%s: ref %q", ErrCommitBoundaryNotFound, ref)
	case ref == "":
		return fmt.Sprintf("%s: branch %q", ErrCommitBoundaryNotFound, branch)
	default:
		return fmt.Sprintf("%s: ref %q is not reachable from branch %q", ErrCommitBoundaryNotFound, ref, branch)
	}
}

func (e *CommitBoundaryNotFoundError) Unwrap() error {
	return ErrCommitBoundaryNotFound
}

var scpLikeRemotePattern = regexp.MustCompile(`^(?:[^@]+@)?([^:]+):(.+)$`)

const minimumProjectSegments = 2

type RepositoryDescriptor struct {
	Provider string
	Host     string
	Owner    string
	Repo     string
	Project  string
	Remote   string
}

var ErrUnsupportedHost = errors.New("unsupported remote host")

func ParseRemote(remoteURL string) (*RepositoryDescriptor, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return nil, fmt.Errorf("%w: %s", ErrUnknownRemote, remoteURL)
	}

	parsed, err := parseRemoteURL(remoteURL)
	if err == nil {
		return parsed, nil
	}

	parsed, err = parseSCPRemote(remoteURL)
	if err == nil {
		return parsed, nil
	}

	return nil, fmt.Errorf("%w: %s", ErrUnknownRemote, remoteURL)
}

func DetectProviderType(host string) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "", fmt.Errorf("%w: empty host", ErrUnsupportedHost)
	}

	if host == DefaultGitHubHost {
		return "github", nil
	}

	if host == DefaultGitLabHost {
		return "gitlab", nil
	}

	return "", fmt.Errorf("%w: %s", ErrUnsupportedHost, host)
}

func parseRemoteURL(remoteURL string) (*RepositoryDescriptor, error) {
	parsedURL, err := url.Parse(remoteURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, ErrUnknownRemote
	}

	return newRepositoryDescriptor(parsedURL.Host, parsedURL.Path)
}

func parseSCPRemote(remoteURL string) (*RepositoryDescriptor, error) {
	matches := scpLikeRemotePattern.FindStringSubmatch(remoteURL)
	if matches == nil {
		return nil, ErrUnknownRemote
	}

	return newRepositoryDescriptor(matches[1], matches[2])
}

func newRepositoryDescriptor(host, rawPath string) (*RepositoryDescriptor, error) {
	host = strings.TrimSpace(host)

	project := normalizeRemotePath(rawPath)
	if host == "" || project == "" {
		return nil, ErrUnknownRemote
	}

	owner, repo := SplitProjectPath(project)
	if owner == "" || repo == "" {
		return nil, ErrUnknownRemote
	}

	return &RepositoryDescriptor{
		Host:    host,
		Owner:   owner,
		Repo:    repo,
		Project: project,
	}, nil
}

func normalizeRemotePath(rawPath string) string {
	path := strings.TrimSpace(rawPath)
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")

	return path
}

func SplitProjectPath(project string) (string, string) {
	parts := strings.Split(project, "/")
	if len(parts) < minimumProjectSegments {
		return "", ""
	}

	return strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1]
}
