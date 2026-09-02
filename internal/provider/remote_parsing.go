package provider

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	azureDevOpsSSHHost          = "ssh.dev.azure.com"
	azureDevOpsLegacyHostSuffix = ".visualstudio.com"
)

var scpLikeRemotePattern = regexp.MustCompile(`^(?:[^@]+@)?([^:]+):(.+)$`)

const minimumProjectSegments = 2

func parseRemote(remoteURL string) (*repositoryDescriptor, error) {
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

func parseRemoteURL(remoteURL string) (*repositoryDescriptor, error) {
	parsedURL, err := url.Parse(remoteURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, ErrUnknownRemote
	}

	return newRepositoryDescriptor(parsedURL.Host, parsedURL.Path)
}

func parseSCPRemote(remoteURL string) (*repositoryDescriptor, error) {
	matches := scpLikeRemotePattern.FindStringSubmatch(remoteURL)
	if matches == nil {
		return nil, ErrUnknownRemote
	}

	return newRepositoryDescriptor(matches[1], matches[2])
}

func newRepositoryDescriptor(host, rawPath string) (*repositoryDescriptor, error) {
	host = strings.TrimSpace(host)

	project := normalizeRemotePath(rawPath)
	if host == "" || project == "" {
		return nil, ErrUnknownRemote
	}

	owner, repo := splitProjectPath(project)
	if owner == "" || repo == "" {
		return nil, ErrUnknownRemote
	}

	return &repositoryDescriptor{
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

func splitProjectPath(project string) (string, string) {
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
func parseAzureDevOpsRemote(remoteURL string) (*repositoryDescriptor, error) {
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

func parseAzureDevOpsHTTPRemote(remoteURL string) (*repositoryDescriptor, error) {
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

func azureDevOpsDescriptorFromCloudSegments(host string, segments []string) (*repositoryDescriptor, error) {
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

	return &repositoryDescriptor{
		Provider:     providerNameAzureDevOps,
		Host:         host,
		Organization: org,
		Project:      project,
		Repo:         repo,
	}, nil
}

func azureDevOpsDescriptorFromLegacySegments(host string, segments []string) (*repositoryDescriptor, error) {
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

	return &repositoryDescriptor{
		Provider:     providerNameAzureDevOps,
		Host:         azureDevOpsAPIHost(host),
		Organization: org,
		Project:      project,
		Repo:         repo,
	}, nil
}

// azureDevOpsAPIHost resolves the host the Azure DevOps API is served from. The
// legacy {org}.visualstudio.com form carries the organization in the subdomain
// while the API takes it as the first path segment, so keeping the subdomain
// would append the organization a second time
// (https://{org}.visualstudio.com/{org}/_apis) and 404.
func azureDevOpsAPIHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" || strings.HasSuffix(strings.ToLower(host), azureDevOpsLegacyHostSuffix) {
		return DefaultAzureDevOpsHost
	}

	return host
}

func parseAzureDevOpsSSHRemote(remoteURL string) (*repositoryDescriptor, error) {
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

	return &repositoryDescriptor{
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
