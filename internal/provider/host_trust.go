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
	if err := validateHostFormat(host); err != nil {
		return err
	}

	if strings.EqualFold(providerURLEnvHost(repository.Provider), host) {
		return nil
	}

	if _, err := detectType(host); err == nil {
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
