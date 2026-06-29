package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
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

// Label colors are stored without a leading "#" so callers can prepend it when
// the provider's API requires the prefix (GitLab) or omit it (GitHub).
const (
	releaseLabelPendingColor       = "FBCA04"
	releaseLabelTaggedColor        = "0E8A16"
	releaseLabelPendingDescription = "release PR is pending tagging"
	releaseLabelTaggedDescription  = "release PR already tagged"
)

type ReleasePROptions struct {
	Title         string
	Body          string
	BaseBranch    string
	ReleaseBranch string
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

type CommitHistory struct {
	EntriesByRef map[string][]CommitEntry
	MissingRefs  []string
}

//nolint:interfacebloat // intentional aggregate. granular interfaces live consumer-side in package release.
type Provider interface {
	GetLatestVersionRef(ctx context.Context) (string, error)
	ListTags(ctx context.Context) ([]string, error)
	GetCommitsSinceRefs(ctx context.Context, refs []string, branch string, includePaths bool) (CommitHistory, error)

	GetReleaseByTag(ctx context.Context, tag string) (*Release, error)
	TagExists(ctx context.Context, tag string) (bool, error)
	CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error)

	CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error)
	UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error
	FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error)
	FindMergedReleasePR(ctx context.Context, baseBranch string) (*PullRequest, error)
	MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) error
	MarkReleasePRPending(ctx context.Context, number int) error
	MarkReleasePRTagged(ctx context.Context, number int) error
	CommitPullRequestBody(ctx context.Context, hash string) (string, bool, error)

	MaxPRBodyLength() int

	CreateBranch(ctx context.Context, name, base string) error
	GetFile(ctx context.Context, branch, path string) (string, error)
	// UpdateFiles force-updates a branch from base with one commit containing all file changes.
	UpdateFiles(ctx context.Context, branch, base string, files map[string]string, message string) error

	RepoURL() string
	// PathPrefix returns the path prefix for commit URLs (empty for GitHub, "/-" for GitLab).
	PathPrefix() string
	CompareURL(fromRef, toRef string) string
}

type RepoInfo struct {
	Owner string
	Name  string
}

const DefaultGitHubHost = "github.com"

const DefaultGitLabHost = "gitlab.com"

const DefaultAzureDevOpsHost = "dev.azure.com"

const azureDevOpsSSHHost = "ssh.dev.azure.com"

const azureDevOpsLegacyHostSuffix = ".visualstudio.com"

const providerNameAzureDevOps = "azuredevops"

var (
	ErrUnknownRemote           = errors.New("unable to parse remote URL")
	ErrUnsupportedHost         = errors.New("unsupported remote host")
	ErrNoRelease               = errors.New("no release found")
	ErrNoVersionRef            = errors.New("no version ref found")
	ErrCommitBoundaryNotFound  = errors.New("commit boundary not found")
	ErrNoPR                    = errors.New("no release PR found")
	ErrFileNotFound            = errors.New("file not found")
	ErrEmptyCommitSHA          = errors.New("empty commit SHA")
	ErrEmptyCommitID           = errors.New("empty commit ID")
	ErrRefNotFound             = errors.New("ref not found")
	ErrMergeBlocked            = errors.New("release PR merge blocked")
	ErrMergeMethodUnsupported  = errors.New("merge method unsupported")
	ErrPaginationLimitExceeded = errors.New("pagination safety limit exceeded")
)

const maxPaginationPages = 100

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
	Provider     string
	Host         string
	Owner        string
	Repo         string
	Project      string
	Organization string
	Collection   string
	Remote       string
}

func ParseRemote(remoteURL string) (*RepositoryDescriptor, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return nil, fmt.Errorf("%w: %s", ErrUnknownRemote, remoteURL)
	}

	parsed, err := parseAzureDevOpsRemote(remoteURL)
	if err == nil {
		return parsed, nil
	}

	parsed, err = parseRemoteURL(remoteURL)
	if err == nil {
		return parsed, nil
	}

	parsed, err = parseSCPRemote(remoteURL)
	if err == nil {
		return parsed, nil
	}

	return nil, fmt.Errorf("%w: %s", ErrUnknownRemote, remoteURL)
}

func DetectType(host string) (string, error) {
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

	if isAzureDevOpsHost(host) {
		return providerNameAzureDevOps, nil
	}

	return "", fmt.Errorf("%w: %s", ErrUnsupportedHost, host)
}

func isAzureDevOpsHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))

	return host == DefaultAzureDevOpsHost ||
		host == azureDevOpsSSHHost ||
		strings.HasSuffix(host, azureDevOpsLegacyHostSuffix)
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

// parseAzureDevOpsRemote handles ADO URL shapes that the generic parser cannot:
//   - https://dev.azure.com/{org}/{project}/_git/{repo}
//   - https://{org}@dev.azure.com/{org}/{project}/_git/{repo}
//   - https://{org}.visualstudio.com/{project}/_git/{repo}
//   - git@ssh.dev.azure.com:v3/{org}/{project}/{repo}
//
// Returns ErrUnknownRemote when the URL is not an ADO remote so callers can fall
// through to the generic parsers.
func parseAzureDevOpsRemote(remoteURL string) (*RepositoryDescriptor, error) {
	parsed, err := parseAzureDevOpsHTTPRemote(remoteURL)
	if err == nil {
		return parsed, nil
	}

	parsed, err = parseAzureDevOpsSSHRemote(remoteURL)
	if err == nil {
		return parsed, nil
	}

	return nil, ErrUnknownRemote
}

func parseAzureDevOpsHTTPRemote(remoteURL string) (*RepositoryDescriptor, error) {
	parsedURL, err := url.Parse(remoteURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, ErrUnknownRemote
	}

	host := strings.ToLower(parsedURL.Host)
	if !isAzureDevOpsHost(host) {
		return nil, ErrUnknownRemote
	}

	path := normalizeRemotePath(parsedURL.Path)
	if path == "" {
		return nil, ErrUnknownRemote
	}

	segments := strings.Split(path, "/")

	if strings.HasSuffix(host, azureDevOpsLegacyHostSuffix) {
		return azureDevOpsDescriptorFromLegacySegments(host, segments)
	}

	return azureDevOpsDescriptorFromCloudSegments(host, segments)
}

func azureDevOpsDescriptorFromCloudSegments(host string, segments []string) (*RepositoryDescriptor, error) {
	// Expect {org}/{project}/_git/{repo} (4 segments).
	gitIdx := indexOf(segments, "_git")
	if gitIdx < 2 || gitIdx != len(segments)-2 {
		return nil, ErrUnknownRemote
	}

	repo := strings.TrimSpace(segments[gitIdx+1])
	project := strings.TrimSpace(segments[gitIdx-1])
	org := strings.TrimSpace(strings.Join(segments[:gitIdx-1], "/"))

	if org == "" || project == "" || repo == "" {
		return nil, ErrUnknownRemote
	}

	return &RepositoryDescriptor{
		Provider:     providerNameAzureDevOps,
		Host:         host,
		Organization: org,
		Project:      project,
		Repo:         repo,
	}, nil
}

func azureDevOpsDescriptorFromLegacySegments(host string, segments []string) (*RepositoryDescriptor, error) {
	// Legacy subdomain form: org is the host subdomain, path is {project}/_git/{repo}.
	org := strings.TrimSuffix(host, azureDevOpsLegacyHostSuffix)
	if org == "" {
		return nil, ErrUnknownRemote
	}

	gitIdx := indexOf(segments, "_git")
	if gitIdx != 1 || gitIdx != len(segments)-2 {
		return nil, ErrUnknownRemote
	}

	project := strings.TrimSpace(segments[0])
	repo := strings.TrimSpace(segments[gitIdx+1])

	if project == "" || repo == "" {
		return nil, ErrUnknownRemote
	}

	return &RepositoryDescriptor{
		Provider:     providerNameAzureDevOps,
		Host:         host,
		Organization: org,
		Project:      project,
		Repo:         repo,
	}, nil
}

func parseAzureDevOpsSSHRemote(remoteURL string) (*RepositoryDescriptor, error) {
	matches := scpLikeRemotePattern.FindStringSubmatch(remoteURL)
	if matches == nil {
		return nil, ErrUnknownRemote
	}

	host := strings.ToLower(matches[1])
	if host != azureDevOpsSSHHost {
		return nil, ErrUnknownRemote
	}

	path := normalizeRemotePath(matches[2])
	segments := strings.Split(path, "/")

	// Expect v3/{org}/{project}/{repo}.
	const azureDevOpsSSHSegments = 4
	if len(segments) != azureDevOpsSSHSegments || segments[0] != "v3" {
		return nil, ErrUnknownRemote
	}

	org := strings.TrimSpace(segments[1])
	project := strings.TrimSpace(segments[2])
	repo := strings.TrimSpace(segments[3])

	if org == "" || project == "" || repo == "" {
		return nil, ErrUnknownRemote
	}

	return &RepositoryDescriptor{
		Provider:     providerNameAzureDevOps,
		Host:         DefaultAzureDevOpsHost,
		Organization: org,
		Project:      project,
		Repo:         repo,
	}, nil
}

func indexOf(haystack []string, needle string) int {
	for i, s := range haystack {
		if s == needle {
			return i
		}
	}

	return -1
}
