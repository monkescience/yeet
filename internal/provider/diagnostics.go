package provider

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"
)

type MissingTokenError struct {
	Provider  string
	Variables []string
}

func (e *MissingTokenError) Error() string {
	return fmt.Sprintf("%s: %s environment variable is required", ErrMissingToken, strings.Join(e.Variables, " or "))
}

func (e *MissingTokenError) Unwrap() error {
	return ErrMissingToken
}

type SetupError struct {
	Host       string
	Remote     string
	RemoteHost string
	Provider   string
	Problem    string
	Hint       string
	Err        error
}

func (e *SetupError) Error() string {
	return e.Err.Error()
}

func (e *SetupError) Unwrap() error {
	return e.Err
}

func TokenEnvVars() []string {
	names := make([]string, 0, len(forgeSpecs))
	for _, spec := range forgeSpecs {
		names = append(names, spec.tokenEnvVars...)
	}

	slices.Sort(names)

	return slices.Compact(names)
}

type UnsupportedHostError struct {
	Host string
}

func (e *UnsupportedHostError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnsupportedHost, e.Host)
}

func (e *UnsupportedHostError) Unwrap() error {
	return ErrUnsupportedHost
}

type setupDiagnosis struct {
	problem    string
	hint       string
	remoteHost string
}

func setupProblem(err error) setupDiagnosis {
	if unsupported, ok := errors.AsType[*UnsupportedHostError](err); ok {
		return setupDiagnosis{
			problem:    "remote host is not supported by provider auto-detection",
			hint:       "set provider or repository coordinates, or pass --provider for a custom domain",
			remoteHost: unsupported.Host,
		}
	}

	switch {
	case errors.Is(err, git.ErrRepositoryNotExists):
		return setupDiagnosis{problem: "local git repository was not found", hint: "run yeet from a git checkout"}
	case errors.Is(err, ErrGitRemoteNotFound), errors.Is(err, ErrGitRemoteHasNoURL), errors.Is(err, ErrGitRemoteURLBlank):
		return setupDiagnosis{
			problem: "configured git remote is unusable",
			hint:    "check that the local repository has the configured git remote",
		}
	case errors.Is(err, ErrUnknownRemote):
		return setupDiagnosis{problem: "git remote url could not be parsed", hint: "check the configured git remote url"}
	default:
		return setupDiagnosis{problem: "repository setup failed"}
	}
}

type ReviewerError struct {
	Reviewer  string
	Reviewers []string
	Problem   string
	Err       error
}

func (e *ReviewerError) Error() string {
	return e.Err.Error()
}

func (e *ReviewerError) Unwrap() error {
	return e.Err
}

type LabelError struct {
	Label     string
	Role      string
	Reference string
	Branch    string
	Scope     string
	Conflict  string
	Err       error
}

func (e *LabelError) Error() string {
	return e.Err.Error()
}

func (e *LabelError) Unwrap() error {
	return e.Err
}

func ErrorStatus(err error) int {
	if _, response := errorResponse(err); response != nil { //nolint:bodyclose // SDK owns the response body
		return response.StatusCode
	}

	if gitLabError, ok := errors.AsType[*gitlab.ErrorResponse](err); ok {
		return gitLabError.StatusCode
	}

	var azureError azuredevops.WrappedError
	if errors.As(err, &azureError) && azureError.StatusCode != nil {
		return *azureError.StatusCode
	}

	var azureErrorPtr *azuredevops.WrappedError
	if errors.As(err, &azureErrorPtr) && azureErrorPtr != nil && azureErrorPtr.StatusCode != nil {
		return *azureErrorPtr.StatusCode
	}

	return 0
}
