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
	Number            int
	Title             string
	Body              string
	URL               string
	Branch            string
	MergeCommitSHA    string
	NeedsPendingLabel bool
}

const ReleaseLabelYeet = "yeet"

// Label colors are stored without a leading "#" so callers can prepend it when
// the provider's API requires the prefix (GitLab) or omit it (GitHub).
const (
	releaseLabelPendingColor       = "FBCA04"
	releaseLabelTaggedColor        = "0E8A16"
	releaseLabelYeetColor          = "1D76DB"
	releaseLabelPendingDescription = "release PR is pending tagging"
	releaseLabelTaggedDescription  = "release PR already tagged"
	releaseLabelYeetDescription    = "release PR managed by yeet"
)

type ReleasePROptions struct {
	Title         string
	Body          string
	BaseBranch    string
	ReleaseBranch string
	Reviewers     []string
	Labels        ReleasePRLabels
}

type ReleasePRLabels struct {
	Pending string
	Tagged  string
	Yeet    bool
	Extra   []string
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
	BypassMergeChecks bool
	Method            MergeMethod
}

type CommitEntry struct {
	Hash    string
	Message string
	Paths   []string
}

type CommitHistory struct {
	// EntriesByRef contains each reachable commit range in newest-first order.
	EntriesByRef map[string][]CommitEntry
	// MissingRefs contains refs that do not exist or are unreachable from the branch.
	MissingRefs []string
}

// FileUpdate holds new content and whether the file exists on the base branch.
type FileUpdate struct {
	Content string
	Exists  bool
}

// TagRef identifies a remote tag and the commit it resolves to. CommitSHA is
// the peeled commit hash for annotated tags.
type TagRef struct {
	Name      string
	CommitSHA string
}

//nolint:interfacebloat // intentional aggregate. granular interfaces live consumer-side in package release.
type Provider interface {
	ListTagRefs(ctx context.Context) ([]TagRef, error)
	// GetBranchHead returns the commit SHA the branch currently points at,
	// wrapping ErrRefNotFound when the branch does not exist. Release commit
	// ranges are computed from the local checkout (internal/history), which
	// uses this to validate that the checkout matches the remote branch.
	GetBranchHead(ctx context.Context, branch string) (string, error)

	GetReleaseByTag(ctx context.Context, tag string) (*Release, error)
	CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error)

	CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error)
	UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error
	PrepareReleasePRLabels(ctx context.Context, labels ReleasePRLabels) error
	FindOpenPendingReleasePRs(ctx context.Context, baseBranch, pendingLabel string) ([]*PullRequest, error)
	FindMergedReleasePR(ctx context.Context, baseBranch, pendingLabel string) (*PullRequest, error)
	MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) (string, error)
	MarkReleasePRPending(ctx context.Context, number int, labels ReleasePRLabels) error
	MarkReleasePRTagged(ctx context.Context, number int, labels ReleasePRLabels) error
	// MaxPRBodyLength returns zero when the provider has no known limit.
	MaxPRBodyLength() int

	CreateBranch(ctx context.Context, name, base string) error
	GetFile(ctx context.Context, branch, path string) (string, error)
	// UpdateFiles force-updates a branch from base with one commit containing all file changes.
	UpdateFiles(ctx context.Context, branch, base string, files map[string]FileUpdate, message string) error

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
	ErrReleasePRLabelMismatch  = errors.New("release PR lifecycle label mismatch")
	ErrReleasePRLabelMissing   = errors.New("release PR label does not exist")
	ErrCommitBoundaryNotFound  = errors.New("commit boundary not found")
	ErrNoPR                    = errors.New("no release PR found")
	ErrFileNotFound            = errors.New("file not found")
	ErrEmptyCommitSHA          = errors.New("empty commit SHA")
	ErrEmptyCommitID           = errors.New("empty commit ID")
	ErrRefNotFound             = errors.New("ref not found")
	ErrMergeBlocked            = errors.New("release PR merge blocked")
	ErrUntrustedReleasePR      = errors.New("untrusted release PR")
	ErrMergeMethodUnsupported  = errors.New("merge method unsupported")
	ErrReviewerNotFound        = errors.New("reviewer not found")
	ErrReviewerAmbiguous       = errors.New("reviewer is ambiguous")
	ErrReviewerNotApplied      = errors.New("reviewer not applied")
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

	return nil, fmt.Errorf("%w: %s", ErrUnknownRemote, redactRemoteURL(remoteURL))
}

var remoteURLUserinfoPattern = regexp.MustCompile(`://[^/@]+@`)

// redactRemoteURL hides the entire userinfo because tokens appear both as
// password (user:token@) and as username (token@), and must never reach
// error output or CI logs.
func redactRemoteURL(remoteURL string) string {
	return remoteURLUserinfoPattern.ReplaceAllString(remoteURL, "://***@")
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
	// The host is normalized to dev.azure.com (as the SSH parser already does) so
	// the provider builds the API base URL as dev.azure.com/{org}. Keeping the
	// {org}.visualstudio.com host would append the org a second time
	// (https://{org}.visualstudio.com/{org}/_apis) and 404.
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
		Host:         DefaultAzureDevOpsHost,
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
