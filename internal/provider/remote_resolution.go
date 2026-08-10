package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/monkescience/yeet/internal/config"
)

var (
	ErrGitHubRepoRequired      = errors.New("resolve github repository: owner and repo are required")
	ErrGitHubOwnerInvalid      = errors.New("resolve github repository: owner must not contain '/'")
	ErrGitLabProjectNeeded     = errors.New("resolve gitlab repository: project or owner/repo are required")
	ErrAzureDevOpsCoordsNeeded = errors.New("resolve azuredevops repository: organization, project, and repo are required")
	ErrRepositoryConflict      = errors.New("resolve repository: project does not match owner/repo")
)

const (
	azureDevOpsSSHHost          = "ssh.dev.azure.com"
	azureDevOpsLegacyHostSuffix = ".visualstudio.com"
)

type gitRemoteURLGetter func(context.Context, string) (string, error)

func resolveRepository(
	ctx context.Context,
	cfg *config.Config,
	getRemoteURL gitRemoteURLGetter,
) (*repositoryDescriptor, error) {
	repository, err := repositoryDescriptorFromSources(ctx, cfg, getRemoteURL)
	if err != nil {
		return nil, err
	}

	if err := resolveRepositoryProvider(repository); err != nil {
		return nil, err
	}

	applyRepositoryProviderDefaults(repository)
	normalizeRepositoryDescriptor(repository)

	if err := validateRepositoryDescriptor(repository); err != nil {
		return nil, err
	}

	if err := validateProviderHostTrust(ctx, repository, getRemoteURL); err != nil {
		return nil, err
	}

	return repository, nil
}

func repositoryDescriptorFromSources(
	ctx context.Context,
	cfg *config.Config,
	getRemoteURL gitRemoteURLGetter,
) (*repositoryDescriptor, error) {
	if err := validateAutoProviderCoordinates(cfg); err != nil {
		return nil, err
	}

	repository := repositoryFromConfig(cfg)
	if repository.Remote == "" {
		repository.Remote = "origin"
	}

	if needsRemoteLookup(repository) {
		remoteURL, err := getRemoteURL(ctx, repository.Remote)
		if err != nil {
			return nil, fmt.Errorf("get git remote %q url: %w", repository.Remote, err)
		}

		detected, err := parseRemote(remoteURL)
		if err != nil {
			return nil, fmt.Errorf("parse git remote %q url: %w", repository.Remote, err)
		}

		detected.Remote = repository.Remote
		repository = mergeRepositoryDescriptor(detected, repository)
	}

	normalizeRepositoryDescriptor(repository)

	return repository, nil
}

func resolveRepositoryProvider(repository *repositoryDescriptor) error {
	if repository.Provider == "" {
		providerType, err := detectType(repository.Host)
		if err != nil {
			return unsupportedAutoProviderError(repository.Host, err)
		}

		repository.Provider = providerType
	}

	return nil
}

func applyRepositoryProviderDefaults(repository *repositoryDescriptor) {
	spec, known := forgeSpecs[repository.Provider]
	if !known {
		return
	}

	if repository.Host == "" {
		repository.Host = spec.defaultHost
	}

	if repository.Provider == providerNameAzureDevOps && repository.Collection == "" {
		repository.Collection = repository.Organization
	}
}

func unsupportedAutoProviderError(host string, err error) error {
	return fmt.Errorf(
		"resolve repository provider for host %q: %w. "+
			"Auto-detection only supports github.com, gitlab.com, and dev.azure.com. "+
			"Set provider, [repository], or pass explicit flags for custom domains",
		host,
		err,
	)
}

func validateAutoProviderCoordinates(cfg *config.Config) error {
	if cfg.Provider != config.ProviderAuto {
		return nil
	}

	var section string

	switch {
	case cfg.Repository.GitHub != nil:
		section = providerNameGitHub
	case cfg.Repository.GitLab != nil:
		section = providerNameGitLab
	case cfg.Repository.AzureDevOps != nil:
		section = providerNameAzureDevOps
	default:
		return nil
	}

	return fmt.Errorf(
		"%w: repository.%s set but provider is auto. Set an explicit provider",
		config.ErrInvalidConfig,
		section,
	)
}

func repositoryFromConfig(cfg *config.Config) *repositoryDescriptor {
	descriptor := &repositoryDescriptor{
		Provider: normalizedRepositoryProvider(cfg.Provider),
		Remote:   strings.TrimSpace(cfg.Repository.Remote),
	}

	switch cfg.Provider {
	case config.ProviderGitHub:
		if cfg.Repository.GitHub == nil {
			break
		}

		descriptor.Host = strings.TrimSpace(cfg.Repository.GitHub.Host)
		descriptor.Owner = strings.TrimSpace(cfg.Repository.GitHub.Owner)
		descriptor.Repo = strings.TrimSpace(cfg.Repository.GitHub.Repo)
		descriptor.Project = strings.TrimSpace(cfg.Repository.GitHub.Project)
	case config.ProviderGitLab:
		if cfg.Repository.GitLab != nil {
			descriptor.Host = strings.TrimSpace(cfg.Repository.GitLab.Host)
			descriptor.Project = strings.TrimSpace(cfg.Repository.GitLab.Project)
		}
	case config.ProviderAzureDevOps:
		if cfg.Repository.AzureDevOps == nil {
			break
		}

		descriptor.Host = strings.TrimSpace(cfg.Repository.AzureDevOps.Host)
		descriptor.Organization = strings.TrimSpace(cfg.Repository.AzureDevOps.Organization)
		descriptor.Project = strings.TrimSpace(cfg.Repository.AzureDevOps.Project)
		descriptor.Repo = strings.TrimSpace(cfg.Repository.AzureDevOps.Repo)
		descriptor.Collection = strings.TrimSpace(cfg.Repository.AzureDevOps.Collection)
	case config.ProviderAuto:
	}

	return descriptor
}

func normalizedRepositoryProvider(providerType config.ProviderType) string {
	provider := strings.TrimSpace(string(providerType))
	if provider == providerNameAuto {
		return ""
	}

	return provider
}

func needsRemoteLookup(repository *repositoryDescriptor) bool {
	if !hasRepositoryCoordinates(repository) {
		return true
	}

	return repository.Provider == "" && repository.Host == ""
}

func hasRepositoryCoordinates(repository *repositoryDescriptor) bool {
	return repository.Project != "" || (repository.Owner != "" && repository.Repo != "")
}

func mergeRepositoryDescriptor(
	base *repositoryDescriptor,
	override *repositoryDescriptor,
) *repositoryDescriptor {
	if override.Provider != "" {
		base.Provider = override.Provider
	}

	if override.Host != "" {
		base.Host = override.Host
	}

	mergeRepositoryCoordinates(base, override)

	if override.Organization != "" {
		base.Organization = override.Organization
	}

	if override.Collection != "" {
		base.Collection = override.Collection
	}

	if override.Remote != "" {
		base.Remote = override.Remote
	}

	return base
}

func mergeRepositoryCoordinates(base *repositoryDescriptor, override *repositoryDescriptor) {
	switch {
	case override.Project != "":
		base.Project = override.Project
		base.Owner = override.Owner
		base.Repo = override.Repo
	case override.Owner != "" && override.Repo != "":
		base.Owner = override.Owner
		base.Repo = override.Repo
		base.Project = ""
	default:
		if override.Owner != "" {
			base.Owner = override.Owner
		}

		if override.Repo != "" {
			base.Repo = override.Repo
		}
	}
}

func normalizeRepositoryDescriptor(repository *repositoryDescriptor) {
	repository.Provider = strings.TrimSpace(repository.Provider)
	repository.Host = strings.TrimSpace(repository.Host)
	repository.Owner = strings.TrimSpace(repository.Owner)
	repository.Repo = strings.TrimSpace(repository.Repo)
	repository.Project = strings.Trim(strings.TrimSpace(repository.Project), "/")
	repository.Organization = strings.TrimSpace(repository.Organization)
	repository.Collection = strings.TrimSpace(repository.Collection)
	repository.Remote = strings.TrimSpace(repository.Remote)

	if repository.Project == "" && repository.Owner != "" && repository.Repo != "" {
		repository.Project = repository.Owner + "/" + repository.Repo
	}

	if repository.Project == "" || (repository.Owner != "" && repository.Repo != "") {
		return
	}

	owner, repo := splitProjectPath(repository.Project)
	if repository.Owner == "" {
		repository.Owner = owner
	}

	if repository.Repo == "" {
		repository.Repo = repo
	}
}

func validateRepositoryDescriptor(repository *repositoryDescriptor) error {
	if err := validateRepositoryCoordinates(repository); err != nil {
		return err
	}

	switch repository.Provider {
	case providerNameGitHub:
		if repository.Owner == "" || repository.Repo == "" {
			return ErrGitHubRepoRequired
		}

		if strings.Contains(repository.Owner, "/") {
			return fmt.Errorf("%w: %q", ErrGitHubOwnerInvalid, repository.Owner)
		}
	case providerNameGitLab:
		if repository.Project == "" {
			return ErrGitLabProjectNeeded
		}
	case providerNameAzureDevOps:
		if repository.Organization == "" || repository.Project == "" || repository.Repo == "" {
			return ErrAzureDevOpsCoordsNeeded
		}
	case providerNameAuto:
		return fmt.Errorf(
			"%w: %s (provider auto must be resolved before validation)",
			ErrUnsupportedProvider, repository.Provider,
		)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProvider, repository.Provider)
	}

	return nil
}

func validateRepositoryCoordinates(repository *repositoryDescriptor) error {
	if repository.Project == "" || repository.Owner == "" || repository.Repo == "" {
		return nil
	}

	expectedProject := repository.Owner + "/" + repository.Repo
	if strings.EqualFold(repository.Project, expectedProject) {
		return nil
	}

	return fmt.Errorf(
		"%w: project %q does not match owner/repo %q",
		ErrRepositoryConflict,
		repository.Project,
		expectedProject,
	)
}

var scpLikeRemotePattern = regexp.MustCompile(`^(?:[^@]+@)?([^:]+):(.+)$`)

const minimumProjectSegments = 2

type repositoryDescriptor struct {
	Provider     string
	Host         string
	Owner        string
	Repo         string
	Project      string
	Organization string
	Collection   string
	Remote       string
}

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

func detectType(host string) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "", fmt.Errorf("%w: empty host", ErrUnsupportedHost)
	}

	if host == DefaultGitHubHost {
		return providerNameGitHub, nil
	}

	if host == DefaultGitLabHost {
		return providerNameGitLab, nil
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
