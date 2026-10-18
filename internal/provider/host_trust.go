package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

var (
	ErrInvalidHost   = errors.New("invalid provider host")
	ErrUntrustedHost = errors.New("provider host is not trusted")
)

var publicSSHHostProviders = map[string]string{
	"ssh.github.com":    providerNameGitHub,
	"altssh.gitlab.com": providerNameGitLab,
}

type untrustedHostError struct {
	host   string
	remote string
	cause  error
}

func (e *untrustedHostError) Error() string {
	return fmt.Sprintf(
		"%s: %q could not be verified against git remote %q: %s",
		ErrUntrustedHost, e.host, e.remote, e.cause,
	)
}

func (e *untrustedHostError) Unwrap() []error {
	return []error{ErrUntrustedHost, e.cause}
}

func validateProviderHostTrust(
	ctx context.Context,
	repository *repositoryDescriptor,
	getRemoteURL gitRemoteURLGetter,
) error {
	host := strings.TrimSpace(repository.Host)

	err := validateHostFormat(host)
	if err != nil {
		return &SetupError{
			Host:     host,
			Provider: repository.Provider,
			Remote:   repository.Remote,
			Problem:  "provider host is invalid",
			Err:      err,
		}
	}

	if spec, known := forgeSpecs[repository.Provider]; known && spec.endpointOverride() == "" {
		apiHost, apiErr := validateConfiguredAPIHost(repository.APIURL, host)
		if apiErr != nil {
			setup := hostTrustError(repository, apiErr)

			setup.APIHost = apiHost
			setup.Problem = "configured provider api host does not match repository host"
			setup.Hint = "use an api_url on the repository host"

			return setup
		}
	}

	if strings.EqualFold(providerURLEnvHost(repository.Provider), host) {
		return nil
	}

	_, err = detectType(host)
	if err == nil {
		return nil
	}

	return validateRemoteHostTrust(ctx, repository, getRemoteURL)
}

func validateRemoteHostTrust(
	ctx context.Context,
	repository *repositoryDescriptor,
	getRemoteURL gitRemoteURLGetter,
) error {
	host := strings.TrimSpace(repository.Host)

	remoteURL, err := getRemoteURL(ctx, repository.Remote)
	if err != nil {
		setup := repositoryLookupSetupError(repository, host, err)
		if setup != nil {
			return setup
		}

		diagnosis := setupProblem(err)
		setup = hostTrustError(
			repository,
			&untrustedHostError{host: host, remote: repository.Remote, cause: err},
		)
		setup.Problem = "git remote could not be read"
		setup.Hint = "check that the local repository has the configured git remote"

		if diagnosis.hint != "" {
			setup.Problem, setup.Hint = diagnosis.problem, diagnosis.hint
		}

		return setup
	}

	detected, err := parseRemote(remoteURL)
	if err != nil {
		setup := hostTrustError(
			repository,
			&untrustedHostError{host: host, remote: repository.Remote, cause: err},
		)
		setup.Problem = "git remote url is invalid"
		setup.Hint = "check the configured git remote url"

		return setup
	}

	if !strings.EqualFold(strings.TrimSpace(detected.Host), host) {
		setup := hostTrustError(
			repository,
			fmt.Errorf("%w: %q does not match git remote host %q", ErrUntrustedHost, host, detected.Host),
		)
		setup.RemoteHost = detected.Host
		setup.Problem = "provider host does not match git remote host"

		return setup
	}

	return nil
}

func validateDefaultedHostRemote(
	ctx context.Context,
	repository *repositoryDescriptor,
	getRemoteURL gitRemoteURLGetter,
) error {
	if spec, known := forgeSpecs[repository.Provider]; !known || spec.endpointOverride() != "" {
		return nil
	}

	remoteURL, err := getRemoteURL(ctx, repository.Remote)
	if err != nil {
		return nil //nolint:nilerr // without a readable remote nothing contradicts the default host
	}

	detected, err := parseRemote(remoteURL)
	if err != nil {
		return nil //nolint:nilerr // an unparseable remote names no host to contradict the default
	}

	if publicRemoteHostProvider(detected.Host) == repository.Provider {
		return nil
	}

	setup := hostTrustError(
		repository,
		fmt.Errorf(
			"%w: default host %q does not match git remote host %q",
			ErrUntrustedHost, repository.Host, detected.Host,
		),
	)
	setup.RemoteHost = detected.Host
	setup.Problem = "provider host does not match git remote host"
	setup.Hint = "set the provider host explicitly to the forge serving this repository"

	return setup
}

func publicRemoteHostProvider(remoteHost string) string {
	hostname := strings.ToLower(hostnameOf(remoteHost))
	if provider, known := publicSSHHostProviders[hostname]; known {
		return provider
	}

	provider, err := detectType(hostname)
	if err != nil {
		return ""
	}

	return provider
}

func hostnameOf(host string) string {
	parsed, err := url.Parse("https://" + host)
	if err != nil {
		return host
	}

	return parsed.Hostname()
}

func repositoryLookupSetupError(repository *repositoryDescriptor, host string, err error) *SetupError {
	diagnosis := setupProblem(err)
	if !diagnosis.repositoryFailure {
		return nil
	}

	return &SetupError{
		Host:     host,
		Remote:   repository.Remote,
		Provider: repository.Provider,
		Problem:  diagnosis.problem,
		Hint:     diagnosis.hint,
		Err:      err,
	}
}

func hostTrustError(repository *repositoryDescriptor, err error) *SetupError {
	return &SetupError{
		Host:     strings.TrimSpace(repository.Host),
		Remote:   repository.Remote,
		Provider: repository.Provider,
		Err:      err,
	}
}

func validateConfiguredAPIHost(apiURL, repositoryHost string) (string, error) {
	if apiURL == "" {
		return "", nil
	}

	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf("%w: configured api_url is invalid", ErrUntrustedHost)
	}

	repositoryHostname := hostnameOf(repositoryHost)

	if !strings.EqualFold(parsed.Hostname(), repositoryHostname) {
		return parsed.Hostname(), fmt.Errorf(
			"%w: configured api_url host %q does not match repository host %q",
			ErrUntrustedHost,
			parsed.Hostname(),
			repositoryHostname,
		)
	}

	return parsed.Hostname(), nil
}

func validateHostFormat(host string) error {
	if host == "" {
		return fmt.Errorf("%w: host must not be empty", ErrInvalidHost)
	}

	for _, r := range host {
		if r == '/' || r == '@' || unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("%w: %q must be a bare hostname without scheme, credentials, or path", ErrInvalidHost, host)
		}
	}

	return nil
}

func providerURLEnvHost(providerType string) string {
	spec, known := forgeSpecs[providerType]
	if !known {
		return ""
	}

	parsed, err := url.Parse(spec.endpointOverride())
	if err != nil {
		return ""
	}

	return parsed.Hostname()
}
