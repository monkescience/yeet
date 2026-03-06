package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/go-github/v84/github"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

var (
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrMissingToken        = errors.New("missing auth token")
)

func loadConfig() (*config.Config, error) {
	path := cfgFile
	if path == "" {
		path = config.DefaultFile
	}

	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return cfg, nil
}

func newProvider(ctx context.Context, cfg *config.Config) (provider.Provider, error) {
	providerType := cfg.Provider

	if providerType == "" {
		detected, err := detectProvider(ctx)
		if err != nil {
			return nil, fmt.Errorf("auto-detect provider: %w", err)
		}

		providerType = detected.ProviderType()
		cfg.Provider = providerType

		return createProvider(ctx, providerType, detected.Owner, detected.Repo)
	}

	// Need to detect owner/repo from remote even if provider is explicitly set.
	detected, err := detectProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("detect repo info: %w", err)
	}

	return createProvider(ctx, providerType, detected.Owner, detected.Repo)
}

func createProvider(_ context.Context, providerType, owner, repo string) (provider.Provider, error) {
	switch providerType {
	case config.ProviderGitHub:
		return createGitHubProvider(owner, repo)
	case config.ProviderGitLab:
		return createGitLabProvider(owner, repo)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, providerType)
	}
}

func createGitHubProvider(owner, repo string) (*provider.GitHub, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("%w: GITHUB_TOKEN or GH_TOKEN environment variable is required", ErrMissingToken)
	}

	client := github.NewClient(nil).WithAuthToken(token)

	baseURL := os.Getenv("GITHUB_URL")
	if baseURL != "" {
		var err error

		client, err = client.WithEnterpriseURLs(baseURL, baseURL)
		if err != nil {
			return nil, fmt.Errorf("configure github enterprise URL: %w", err)
		}
	}

	return provider.NewGitHub(client, owner, repo), nil
}

func createGitLabProvider(owner, repo string) (*provider.GitLab, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		token = os.Getenv("GL_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("%w: GITLAB_TOKEN or GL_TOKEN environment variable is required", ErrMissingToken)
	}

	baseURL := os.Getenv("GITLAB_URL")

	var opts []gitlab.ClientOptionFunc

	if baseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(baseURL))
	}

	client, err := gitlab.NewClient(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}

	return provider.NewGitLab(client, owner, repo), nil
}

func detectProvider(ctx context.Context) (*provider.ProviderFromRemote, error) {
	remoteURL, err := getGitRemoteURL(ctx)
	if err != nil {
		return nil, fmt.Errorf("detect provider: %w", err)
	}

	detected, err := provider.DetectFromRemote(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("detect provider from remote: %w", err)
	}

	return detected, nil
}

func getGitRemoteURL(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("get git remote url: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
