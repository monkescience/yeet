package provider

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/monkescience/yeet/internal/logattr"
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
	APIHost         string
	Host            string
	Remote          string
	RemoteHost      string
	Provider        string
	Project         string
	ExpectedProject string
	Problem         string
	Hint            string
	Err             error
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
	problem           string
	hint              string
	remoteHost        string
	project           string
	expectedProject   string
	repositoryFailure bool
}

func setupProblem(err error) setupDiagnosis {
	if unsupported, ok := errors.AsType[*UnsupportedHostError](err); ok {
		return setupDiagnosis{
			problem:    "remote host is not supported by provider auto-detection",
			hint:       "set provider or repository coordinates, or pass --provider for a custom domain",
			remoteHost: unsupported.Host,
		}
	}

	coordinateDiagnosis, ok := repositoryCoordinateProblem(err)
	if ok {
		return coordinateDiagnosis
	}

	switch {
	case errors.Is(err, git.ErrRepositoryNotExists):
		return setupDiagnosis{
			problem:           "local git repository was not found",
			hint:              "run yeet from a git checkout",
			repositoryFailure: true,
		}
	case errors.Is(err, ErrGitRemoteNotFound), errors.Is(err, ErrGitRemoteHasNoURL), errors.Is(err, ErrGitRemoteURLBlank):
		return setupDiagnosis{
			problem:           "configured git remote is unusable",
			hint:              "check that the local repository has the configured git remote",
			repositoryFailure: true,
		}
	case errors.Is(err, ErrUnknownRemote):
		return setupDiagnosis{problem: "git remote url could not be parsed", hint: "check the configured git remote url"}
	default:
		return setupDiagnosis{problem: "repository setup failed"}
	}
}

func repositoryCoordinateProblem(err error) (setupDiagnosis, bool) {
	if conflict, ok := errors.AsType[*repositoryConflictError](err); ok {
		return setupDiagnosis{
			problem:         "repository project does not match owner and repo",
			hint:            "align repository project with owner and repo",
			project:         conflict.project,
			expectedProject: conflict.expectedProject,
		}, true
	}

	switch {
	case errors.Is(err, ErrGitHubRepoRequired):
		return setupDiagnosis{
			problem: "github repository owner and repo are required",
			hint:    "set repository.github.owner and repository.github.repo",
		}, true
	case errors.Is(err, ErrGitHubOwnerInvalid):
		return setupDiagnosis{
			problem: "github repository owner must not contain '/'",
			hint:    "set repository.github.owner to one path segment",
		}, true
	case errors.Is(err, ErrGitLabProjectNeeded):
		return setupDiagnosis{
			problem: "gitlab repository project is required",
			hint:    "set repository.gitlab.project",
		}, true
	case errors.Is(err, ErrAzureDevOpsCoordsNeeded):
		return setupDiagnosis{
			problem: "azuredevops repository coordinates are incomplete",
			hint:    "set repository.azuredevops.organization, project, and repo",
		}, true
	case errors.Is(err, ErrRepositoryConflict):
		return setupDiagnosis{
			problem: "repository project does not match owner and repo",
			hint:    "align repository project with owner and repo",
		}, true
	case errors.Is(err, ErrUnsupportedProvider):
		return setupDiagnosis{
			problem: "provider is not supported",
			hint:    "use github, gitlab, or azuredevops",
		}, true
	default:
		return setupDiagnosis{}, false
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
	Problem   string
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

func providerLogger(logger *slog.Logger, name string) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}

	return logger.With(logattr.Provider(name))
}
