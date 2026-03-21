package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/google/go-github/v84/github"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

var (
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrMissingToken        = errors.New("missing auth token")
	ErrGitHubRepoRequired  = errors.New("resolve github repository: owner and repo are required")
	ErrGitHubOwnerInvalid  = errors.New("resolve github repository: owner must not contain '/'")
	ErrGitLabProjectNeeded = errors.New("resolve gitlab repository: project or owner/repo are required")
	ErrRepositoryConflict  = errors.New("resolve repository: project does not match owner/repo")
	ErrGitRemoteNotFound   = errors.New("git remote not found")
	ErrGitRemoteHasNoURL   = errors.New("git remote has no url")
	ErrGitRemoteURLBlank   = errors.New("git remote url is blank")
)

const minimumProjectSegments = 2

type gitRemoteURLGetter func(context.Context, string) (string, error)

func loadConfig(path string) (*config.Config, string, error) {
	resolvedPath, err := resolveConfigPath(path)
	if err != nil {
		return nil, resolvedPath, err
	}

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		return nil, resolvedPath, fmt.Errorf("load config: %w", err)
	}

	return cfg, resolvedPath, nil
}

func resolveConfigPath(path string) (string, error) {
	explicitPath, hasExplicitPath := explicitConfigPath(path)
	if hasExplicitPath {
		return explicitPath, nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	searchRoot, err := configSearchRoot(workingDir)
	if err != nil {
		return "", err
	}

	configDir, found, err := findAncestorContaining(workingDir, config.DefaultFile, searchRoot)
	if err != nil {
		return "", fmt.Errorf("discover config path: %w", err)
	}

	if !found {
		return config.DefaultFile, missingPathError(config.DefaultFile)
	}

	return filepath.Join(configDir, config.DefaultFile), nil
}

func resolveInitConfigPath(path string) (string, error) {
	explicitPath, hasExplicitPath := explicitConfigPath(path)
	if hasExplicitPath {
		return explicitPath, nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	searchRoot, err := configSearchRoot(workingDir)
	if err != nil {
		return "", err
	}

	configDir, found, err := findAncestorContaining(workingDir, config.DefaultFile, searchRoot)
	if err != nil {
		return "", fmt.Errorf("discover config path: %w", err)
	}

	if found {
		return filepath.Join(configDir, config.DefaultFile), nil
	}

	if searchRoot == "" {
		return config.DefaultFile, nil
	}

	return filepath.Join(searchRoot, config.DefaultFile), nil
}

func explicitConfigPath(path string) (string, bool) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return "", false
	}

	return trimmedPath, true
}

func configSearchRoot(startDir string) (string, error) {
	repositoryRoot, found, err := findAncestorContaining(startDir, ".git", "")
	if err != nil {
		return "", fmt.Errorf("discover git repository: %w", err)
	}

	if !found {
		return "", nil
	}

	return repositoryRoot, nil
}

func findAncestorContaining(startDir string, targetName string, stopDir string) (string, bool, error) {
	currentDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false, fmt.Errorf("resolve absolute path for %s: %w", startDir, err)
	}

	resolvedStopDir := ""
	if stopDir != "" {
		resolvedStopDir, err = filepath.Abs(stopDir)
		if err != nil {
			return "", false, fmt.Errorf("resolve absolute stop path for %s: %w", stopDir, err)
		}
	}

	for {
		candidatePath := filepath.Join(currentDir, targetName)

		_, err := os.Stat(candidatePath)
		switch {
		case err == nil:
			return currentDir, true, nil
		case errors.Is(err, os.ErrNotExist):
		default:
			return "", false, fmt.Errorf("stat %s: %w", candidatePath, err)
		}

		if resolvedStopDir != "" && currentDir == resolvedStopDir {
			return "", false, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return "", false, nil
		}

		currentDir = parentDir
	}
}

func missingPathError(path string) error {
	return &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
}

func createProvider(repository *provider.RepositoryDescriptor) (provider.Provider, error) {
	switch repository.Provider {
	case config.ProviderGitHub:
		return createGitHubProvider(repository)
	case config.ProviderGitLab:
		return createGitLabProvider(repository)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, repository.Provider)
	}
}

func createGitHubProvider(repository *provider.RepositoryDescriptor) (*provider.GitHub, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("%w: GITHUB_TOKEN or GH_TOKEN environment variable is required", ErrMissingToken)
	}

	client := github.NewClient(nil).WithAuthToken(token)

	baseURL := strings.TrimSpace(os.Getenv("GITHUB_URL"))

	host := strings.TrimSpace(repository.Host)
	if host != "" {
		if strings.EqualFold(host, provider.DefaultGitHubHost) {
			baseURL = ""
		} else {
			baseURL = fmt.Sprintf("https://%s/api/v3/", host)
		}
	}

	if baseURL != "" {
		var err error

		client, err = client.WithEnterpriseURLs(baseURL, baseURL)
		if err != nil {
			return nil, fmt.Errorf("configure github enterprise URL: %w", err)
		}
	}

	return provider.NewGitHub(client, repository.Owner, repository.Repo), nil
}

func createGitLabProvider(repository *provider.RepositoryDescriptor) (*provider.GitLab, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		token = os.Getenv("GL_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("%w: GITLAB_TOKEN or GL_TOKEN environment variable is required", ErrMissingToken)
	}

	baseURL := strings.TrimSpace(os.Getenv("GITLAB_URL"))

	host := strings.TrimSpace(repository.Host)
	if host != "" {
		if strings.EqualFold(host, provider.DefaultGitLabHost) {
			baseURL = ""
		} else {
			baseURL = fmt.Sprintf("https://%s/api/v4", host)
		}
	}

	var opts []gitlab.ClientOptionFunc

	if baseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(baseURL))
	}

	client, err := gitlab.NewClient(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}

	return provider.NewGitLab(client, repository.Project), nil
}

func resolveRepository(
	ctx context.Context,
	cfg *config.Config,
	getRemoteURL gitRemoteURLGetter,
) (*provider.RepositoryDescriptor, error) {
	repository := repositoryFromConfig(cfg)
	if repository.Remote == "" {
		repository.Remote = "origin"
	}

	if needsRemoteLookup(repository) {
		remoteURL, err := getRemoteURL(ctx, repository.Remote)
		if err != nil {
			return nil, fmt.Errorf("get git remote %q url: %w", repository.Remote, err)
		}

		detected, err := provider.ParseRemote(remoteURL)
		if err != nil {
			return nil, fmt.Errorf("parse git remote %q url: %w", repository.Remote, err)
		}

		detected.Remote = repository.Remote
		repository = mergeRepositoryDescriptor(detected, repository)
	}

	normalizeRepositoryDescriptor(repository)

	if repository.Provider == "" {
		providerType, err := provider.DetectProviderType(repository.Host)
		if err != nil {
			return nil, fmt.Errorf(
				"resolve repository provider for host %q: %w; set provider, [repository], or pass explicit flags",
				repository.Host,
				err,
			)
		}

		repository.Provider = providerType
	}

	switch repository.Provider {
	case config.ProviderGitHub:
		if repository.Host == "" {
			repository.Host = provider.DefaultGitHubHost
		}
	case config.ProviderGitLab:
		if repository.Host == "" {
			repository.Host = provider.DefaultGitLabHost
		}
	}

	normalizeRepositoryDescriptor(repository)

	err := validateRepositoryDescriptor(repository)
	if err != nil {
		return nil, err
	}

	return repository, nil
}

func repositoryFromConfig(cfg *config.Config) *provider.RepositoryDescriptor {
	return &provider.RepositoryDescriptor{
		Provider: normalizedRepositoryProvider(cfg.Provider),
		Host:     strings.TrimSpace(cfg.Repository.Host),
		Owner:    strings.TrimSpace(cfg.Repository.Owner),
		Repo:     strings.TrimSpace(cfg.Repository.Repo),
		Project:  strings.TrimSpace(cfg.Repository.Project),
		Remote:   strings.TrimSpace(cfg.Repository.Remote),
	}
}

func normalizedRepositoryProvider(providerType config.ProviderType) string {
	provider := strings.TrimSpace(providerType)
	if provider == config.ProviderAuto {
		return ""
	}

	return provider
}

func needsRemoteLookup(repository *provider.RepositoryDescriptor) bool {
	if !hasRepositoryCoordinates(repository) {
		return true
	}

	return repository.Provider == "" && repository.Host == ""
}

func hasRepositoryCoordinates(repository *provider.RepositoryDescriptor) bool {
	return repository.Project != "" || (repository.Owner != "" && repository.Repo != "")
}

func mergeRepositoryDescriptor(
	base *provider.RepositoryDescriptor,
	override *provider.RepositoryDescriptor,
) *provider.RepositoryDescriptor {
	if override.Provider != "" {
		base.Provider = override.Provider
	}

	if override.Host != "" {
		base.Host = override.Host
	}

	mergeRepositoryCoordinates(base, override)

	if override.Remote != "" {
		base.Remote = override.Remote
	}

	return base
}

func mergeRepositoryCoordinates(base *provider.RepositoryDescriptor, override *provider.RepositoryDescriptor) {
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

func normalizeRepositoryDescriptor(repository *provider.RepositoryDescriptor) {
	repository.Provider = strings.TrimSpace(repository.Provider)
	repository.Host = strings.TrimSpace(repository.Host)
	repository.Owner = strings.TrimSpace(repository.Owner)
	repository.Repo = strings.TrimSpace(repository.Repo)
	repository.Project = strings.Trim(strings.TrimSpace(repository.Project), "/")
	repository.Remote = strings.TrimSpace(repository.Remote)

	if repository.Project == "" && repository.Owner != "" && repository.Repo != "" {
		repository.Project = repository.Owner + "/" + repository.Repo
	}

	if repository.Project != "" && (repository.Owner == "" || repository.Repo == "") {
		owner, repo := splitProjectPath(repository.Project)
		if repository.Owner == "" {
			repository.Owner = owner
		}

		if repository.Repo == "" {
			repository.Repo = repo
		}
	}
}

func splitProjectPath(project string) (string, string) {
	parts := strings.Split(project, "/")
	if len(parts) < minimumProjectSegments {
		return "", ""
	}

	return strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1]
}

func validateRepositoryDescriptor(repository *provider.RepositoryDescriptor) error {
	err := validateRepositoryCoordinates(repository)
	if err != nil {
		return err
	}

	switch repository.Provider {
	case config.ProviderGitHub:
		if repository.Owner == "" || repository.Repo == "" {
			return ErrGitHubRepoRequired
		}

		if strings.Contains(repository.Owner, "/") {
			return fmt.Errorf("%w: %q", ErrGitHubOwnerInvalid, repository.Owner)
		}
	case config.ProviderGitLab:
		if repository.Project == "" {
			return ErrGitLabProjectNeeded
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProvider, repository.Provider)
	}

	return nil
}

func validateRepositoryCoordinates(repository *provider.RepositoryDescriptor) error {
	if repository.Project == "" || repository.Owner == "" || repository.Repo == "" {
		return nil
	}

	expectedProject := repository.Owner + "/" + repository.Repo
	if repository.Project == expectedProject {
		return nil
	}

	return fmt.Errorf(
		"%w: project %q does not match owner/repo %q",
		ErrRepositoryConflict,
		repository.Project,
		expectedProject,
	)
}

func getGitRemoteURL(ctx context.Context, remote string) (string, error) {
	_ = ctx

	repository, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		return "", fmt.Errorf("open git repository: %w", err)
	}

	repositoryConfig, err := repository.Config()
	if err != nil {
		return "", fmt.Errorf("read git config: %w", err)
	}

	remoteConfig, exists := repositoryConfig.Remotes[remote]
	if !exists {
		return "", fmt.Errorf("%w: %q", ErrGitRemoteNotFound, remote)
	}

	if len(remoteConfig.URLs) == 0 {
		return "", fmt.Errorf("%w: %q", ErrGitRemoteHasNoURL, remote)
	}

	remoteURL := strings.TrimSpace(remoteConfig.URLs[0])
	if remoteURL == "" {
		return "", fmt.Errorf("%w: %q", ErrGitRemoteURLBlank, remote)
	}

	return rewriteGitRemoteURL(remoteURL, repositoryConfig), nil
}

func rewriteGitRemoteURL(remoteURL string, repositoryConfig *gitconfig.Config) string {
	if repositoryConfig == nil {
		return remoteURL
	}

	rewrittenURL := remoteURL
	longestMatchLength := 0

	for _, rule := range repositoryConfig.URLs {
		insteadOf := strings.TrimSpace(rule.InsteadOf)
		if insteadOf == "" || !strings.HasPrefix(remoteURL, insteadOf) {
			continue
		}

		if len(insteadOf) <= longestMatchLength {
			continue
		}

		rewrittenURL = rule.ApplyInsteadOf(remoteURL)
		longestMatchLength = len(insteadOf)
	}

	return rewrittenURL
}
