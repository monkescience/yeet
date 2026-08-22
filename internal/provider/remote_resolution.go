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
	overrides RepositoryOverrides,
) (*repositoryDescriptor, error) {
	repository, err := repositoryDescriptorFromSources(ctx, cfg, getRemoteURL, overrides)
	if err != nil {
		return nil, err
	}

	err = resolveRepositoryProvider(repository)
	if err != nil {
		return nil, err
	}

	applyRepositoryProviderDefaults(repository)
	normalizeRepositoryDescriptor(repository)

	err = validateRepositoryDescriptor(repository)
	if err != nil {
		return nil, err
	}

	err = validateProviderHostTrust(ctx, repository, getRemoteURL)
	if err != nil {
		return nil, err
	}

	return repository, nil
}

func repositoryDescriptorFromSources(
	ctx context.Context,
	cfg *config.Config,
	getRemoteURL gitRemoteURLGetter,
	overrides RepositoryOverrides,
) (*repositoryDescriptor, error) {
	coordinateErr := validateRepositoryOverrides(cfg, overrides)
	if coordinateErr != nil {
		return nil, coordinateErr
	}

	repository := repositoryFromConfig(cfg)
	applyRepositoryOverrides(repository, cfg.Provider, overrides)

	overrideErr := validateOverriddenRepositoryFields(cfg, repository, overrides)
	if overrideErr != nil {
		return nil, overrideErr
	}

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

func validateOverriddenRepositoryFields(
	cfg *config.Config,
	repository *repositoryDescriptor,
	overrides RepositoryOverrides,
) error {
	hasFieldOverride := overrides.Host != nil || overrides.Owner != nil ||
		overrides.Repo != nil || overrides.Project != nil

	hasAzureProviderOverride := overrides.Provider != nil &&
		strings.EqualFold(strings.TrimSpace(*overrides.Provider), providerNameAzureDevOps)
	if !hasFieldOverride && !hasAzureProviderOverride {
		return nil
	}

	fieldError := func(path, field, message string) error {
		return fmt.Errorf("%w: %s.%s %s", config.ErrInvalidConfig, path, field, message)
	}

	if overrides.Host != nil && *overrides.Host != "" && strings.TrimSpace(*overrides.Host) == "" {
		path := "repository." + repository.Provider

		return fieldError(path, "host", "must not be blank")
	}

	switch repository.Provider {
	case providerNameGitHub:
		return validateGitHubOverrideFields(repository, overrides, fieldError)

	case providerNameGitLab:
		return validateGitLabOverrideFields(repository, overrides, fieldError)

	case providerNameAzureDevOps:
		return validateAzureOverrideFields(cfg, repository, overrides, fieldError)
	}

	return nil
}

func validateGitHubOverrideFields(
	repository *repositoryDescriptor,
	overrides RepositoryOverrides,
	fieldError func(string, string, string) error,
) error {
	owner := strings.TrimSpace(repository.Owner)
	repo := strings.TrimSpace(repository.Repo)
	project := normalizeRepositoryProjectPath(repository.Project)

	if overrides.Owner != nil && *overrides.Owner != "" && owner == "" {
		return fieldError("repository.github", "owner", "must not be blank")
	}

	if overrides.Repo != nil && *overrides.Repo != "" && repo == "" {
		return fieldError("repository.github", "repo", "must not be blank")
	}

	if overrides.Project != nil && *overrides.Project != "" && project == "" {
		return fieldError("repository.github", "project", "must not be blank")
	}

	if (owner == "") != (repo == "") {
		return fmt.Errorf(
			"%w: repository.github.owner and repository.github.repo must be set together",
			config.ErrInvalidConfig,
		)
	}

	if project != "" && owner != "" && repo != "" && project != owner+"/"+repo {
		return fmt.Errorf(
			"%w: repository.github.project must match repository.github.owner/repo",
			config.ErrInvalidConfig,
		)
	}

	if strings.Contains(owner, "/") {
		return fieldError("repository.github", "owner", "must not contain '/'")
	}

	if project == "" {
		return nil
	}

	projectOwner, _, ok := splitGitHubProjectPath(project)
	if !ok || strings.Contains(projectOwner, "/") {
		return fieldError("repository.github", "project", "must be in owner/repo form")
	}

	return nil
}

func validateGitLabOverrideFields(
	repository *repositoryDescriptor,
	overrides RepositoryOverrides,
	fieldError func(string, string, string) error,
) error {
	if overrides.Project == nil || *overrides.Project == "" || normalizeRepositoryProjectPath(repository.Project) != "" {
		return nil
	}

	return fieldError("repository.gitlab", "project", "must not be blank")
}

func validateAzureOverrideFields(
	cfg *config.Config,
	repository *repositoryDescriptor,
	overrides RepositoryOverrides,
	fieldError func(string, string, string) error,
) error {
	organization := strings.TrimSpace(repository.Organization)
	project := normalizeRepositoryProjectPath(repository.Project)
	repo := strings.TrimSpace(repository.Repo)

	if overrides.Project != nil && *overrides.Project != "" && project == "" {
		return fieldError("repository.azuredevops", "project", "must not be blank")
	}

	if overrides.Repo != nil && *overrides.Repo != "" && repo == "" {
		return fieldError("repository.azuredevops", "repo", "must not be blank")
	}

	requiresExplicitCoordinates := overrides.Provider != nil || cfg.Provider == config.ProviderAzureDevOps
	if !requiresExplicitCoordinates {
		return nil
	}

	if organization == "" {
		return fmt.Errorf("%w: repository.azuredevops.organization is required", config.ErrInvalidConfig)
	}

	if project == "" {
		return fmt.Errorf("%w: repository.azuredevops.project is required", config.ErrInvalidConfig)
	}

	if repo == "" {
		return fmt.Errorf("%w: repository.azuredevops.repo is required", config.ErrInvalidConfig)
	}

	return nil
}

func normalizeRepositoryProjectPath(project string) string {
	return strings.Trim(strings.TrimSpace(project), "/")
}

const githubProjectSegments = 2

func splitGitHubProjectPath(project string) (string, string, bool) {
	parts := strings.Split(project, "/")
	if len(parts) != githubProjectSegments {
		return "", "", false
	}

	owner := strings.TrimSpace(parts[0])

	repo := strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return "", "", false
	}

	return owner, repo, true
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

func validateRepositoryOverrides(cfg *config.Config, overrides RepositoryOverrides) error {
	provider := cfg.Provider
	if provider == config.ProviderAuto && overrides.Provider == nil {
		var section string

		switch {
		case cfg.Repository.GitHub != nil:
			section = providerNameGitHub
		case cfg.Repository.GitLab != nil:
			section = providerNameGitLab
		case cfg.Repository.AzureDevOps != nil:
			section = providerNameAzureDevOps
		}

		if section != "" {
			return fmt.Errorf(
				"%w: repository.%s set but provider is auto. Set an explicit provider",
				config.ErrInvalidConfig,
				section,
			)
		}
	}

	if overrides.Provider != nil {
		provider = config.ProviderType(*overrides.Provider)

		providerErr := validateOverrideProvider(provider)
		if providerErr != nil {
			return providerErr
		}
	}

	if overrides.Remote != nil && strings.TrimSpace(*overrides.Remote) == "" {
		return fmt.Errorf("%w: repository.remote must not be empty", config.ErrInvalidConfig)
	}

	hasCoordinates := overrides.Host != nil || overrides.Owner != nil ||
		overrides.Repo != nil || overrides.Project != nil
	if provider == config.ProviderAuto && hasCoordinates {
		return fmt.Errorf(
			"%w: repository field flags require an explicit --provider (auto cannot route them)",
			config.ErrInvalidConfig,
		)
	}

	return validateRepositoryOverrideRouting(provider, overrides)
}

func validateRepositoryOverrideRouting(
	provider config.ProviderType,
	overrides RepositoryOverrides,
) error {
	switch provider {
	case config.ProviderGitLab:
		if overrides.Owner != nil || overrides.Repo != nil {
			return fmt.Errorf(
				"%w: --owner/--repo are not valid for provider gitlab. Use --project",
				config.ErrInvalidConfig,
			)
		}
	case config.ProviderAzureDevOps:
		if overrides.Owner != nil {
			return fmt.Errorf(
				"%w: --owner is not valid for provider azuredevops",
				config.ErrInvalidConfig,
			)
		}
	case config.ProviderAuto, config.ProviderGitHub:
	}

	return nil
}

func validateOverrideProvider(provider config.ProviderType) error {
	switch provider {
	case config.ProviderAuto, config.ProviderGitHub, config.ProviderGitLab, config.ProviderAzureDevOps:
		return nil
	default:
		return fmt.Errorf(
			"%w: provider must be \"auto\", \"github\", \"gitlab\", or \"azuredevops\", got %q",
			config.ErrInvalidConfig,
			provider,
		)
	}
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
		descriptor.APIURL = strings.TrimSpace(cfg.Repository.GitHub.APIURL)
		descriptor.WebURL = strings.TrimSpace(cfg.Repository.GitHub.WebURL)
		descriptor.Owner = strings.TrimSpace(cfg.Repository.GitHub.Owner)
		descriptor.Repo = strings.TrimSpace(cfg.Repository.GitHub.Repo)
		descriptor.Project = strings.TrimSpace(cfg.Repository.GitHub.Project)
	case config.ProviderGitLab:
		if cfg.Repository.GitLab == nil {
			break
		}

		descriptor.Host = strings.TrimSpace(cfg.Repository.GitLab.Host)
		descriptor.APIURL = strings.TrimSpace(cfg.Repository.GitLab.APIURL)
		descriptor.WebURL = strings.TrimSpace(cfg.Repository.GitLab.WebURL)
		descriptor.Project = strings.TrimSpace(cfg.Repository.GitLab.Project)
	case config.ProviderAzureDevOps:
		if cfg.Repository.AzureDevOps == nil {
			break
		}

		descriptor.Host = strings.TrimSpace(cfg.Repository.AzureDevOps.Host)
		descriptor.APIURL = strings.TrimSpace(cfg.Repository.AzureDevOps.APIURL)
		descriptor.WebURL = strings.TrimSpace(cfg.Repository.AzureDevOps.WebURL)
		descriptor.Organization = strings.TrimSpace(cfg.Repository.AzureDevOps.Organization)
		descriptor.Project = strings.TrimSpace(cfg.Repository.AzureDevOps.Project)
		descriptor.Repo = strings.TrimSpace(cfg.Repository.AzureDevOps.Repo)
		descriptor.Collection = strings.TrimSpace(cfg.Repository.AzureDevOps.Collection)
	case config.ProviderAuto:
	}

	return descriptor
}

func applyRepositoryOverrides(
	repository *repositoryDescriptor,
	configuredProvider config.ProviderType,
	overrides RepositoryOverrides,
) {
	if overrides.Provider != nil {
		overrideProvider := strings.TrimSpace(*overrides.Provider)

		remote := repository.Remote
		if overrideProvider == providerNameAuto ||
			(normalizedRepositoryProvider(configuredProvider) != "" &&
				overrideProvider != normalizedRepositoryProvider(configuredProvider)) {
			*repository = repositoryDescriptor{}
			repository.Remote = remote
		}

		repository.Provider = normalizedRepositoryProvider(config.ProviderType(overrideProvider))
	}

	if overrides.Remote != nil {
		repository.Remote = *overrides.Remote
	}

	if overrides.Host != nil {
		repository.Host = *overrides.Host
	}

	if overrides.Owner != nil {
		repository.Owner = *overrides.Owner
	}

	if overrides.Repo != nil {
		repository.Repo = *overrides.Repo
	}

	if overrides.Project != nil {
		repository.Project = *overrides.Project

		if overrides.Owner == nil {
			repository.Owner = ""
		}

		if overrides.Repo == nil {
			repository.Repo = ""
		}
	}

	if overrides.Project == nil &&
		(overrides.Owner != nil || overrides.Repo != nil) &&
		strings.TrimSpace(repository.Owner) != "" &&
		strings.TrimSpace(repository.Repo) != "" {
		repository.Project = ""
	}
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

	if override.APIURL != "" {
		base.APIURL = override.APIURL
	}

	if override.WebURL != "" {
		base.WebURL = override.WebURL
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
	repository.APIURL = strings.TrimSpace(repository.APIURL)
	repository.WebURL = strings.TrimSpace(repository.WebURL)
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
	err := validateRepositoryCoordinates(repository)
	if err != nil {
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
	APIURL       string
	WebURL       string
	Owner        string
	Repo         string
	Project      string
	Organization string
	Collection   string
	Remote       string
}

func resolvedRepositoryFromDescriptor(repository *repositoryDescriptor) (resolvedRepository, error) {
	switch repository.Provider {
	case providerNameGitHub:
		return &resolvedGitHubRepository{
			Host:   repository.Host,
			APIURL: repository.APIURL,
			WebURL: repository.WebURL,
			Owner:  repository.Owner,
			Repo:   repository.Repo,
		}, nil
	case providerNameGitLab:
		return &resolvedGitLabRepository{
			Host:    repository.Host,
			APIURL:  repository.APIURL,
			WebURL:  repository.WebURL,
			Project: repository.Project,
		}, nil
	case providerNameAzureDevOps:
		return &resolvedAzureDevOpsRepository{
			Host:         repository.Host,
			APIURL:       repository.APIURL,
			WebURL:       repository.WebURL,
			Organization: repository.Organization,
			Collection:   repository.Collection,
			Project:      repository.Project,
			Repo:         repository.Repo,
		}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, repository.Provider)
	}
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
