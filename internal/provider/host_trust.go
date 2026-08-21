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
		return err
	}

	if spec, known := forgeSpecs[repository.Provider]; known && spec.endpointOverride() == "" {
		err = validateConfiguredAPIHost(repository.APIURL, host)
		if err != nil {
			return err
		}
	}

	if strings.EqualFold(providerURLEnvHost(repository.Provider), host) {
		return nil
	}

	_, err = detectType(host)
	if err == nil {
		return nil
	}

	remoteURL, err := getRemoteURL(ctx, repository.Remote)
	if err != nil {
		return &untrustedHostError{host: host, remote: repository.Remote, cause: err}
	}

	detected, err := parseRemote(remoteURL)
	if err != nil {
		return &untrustedHostError{host: host, remote: repository.Remote, cause: err}
	}

	if !strings.EqualFold(strings.TrimSpace(detected.Host), host) {
		return fmt.Errorf("%w: %q does not match git remote host %q", ErrUntrustedHost, host, detected.Host)
	}

	return nil
}

func validateConfiguredAPIHost(apiURL, repositoryHost string) error {
	if apiURL == "" {
		return nil
	}

	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Hostname() == "" {
		return fmt.Errorf("%w: configured api_url is invalid", ErrUntrustedHost)
	}

	repositoryHostname := repositoryHost

	parsedRepositoryHost, parseErr := url.Parse("https://" + repositoryHost)
	if parseErr == nil {
		repositoryHostname = parsedRepositoryHost.Hostname()
	}

	if !strings.EqualFold(parsed.Hostname(), repositoryHostname) {
		return fmt.Errorf(
			"%w: configured api_url host %q does not match repository host %q",
			ErrUntrustedHost,
			parsed.Hostname(),
			repositoryHostname,
		)
	}

	return nil
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
