package provider

import (
	"errors"

	"github.com/monkescience/yeet/internal/forge"
)

const (
	ReleaseLabelYeet               = "yeet"
	releaseLabelPendingColor       = "FFD866"
	releaseLabelTaggedColor        = "A9DC76"
	releaseLabelYeetColor          = "FF6188"
	releaseLabelPendingDescription = "release PR is pending tagging"
	releaseLabelTaggedDescription  = "release PR already tagged"
	releaseLabelYeetDescription    = "release PR managed by yeet"
)

type repoInfo struct {
	Owner string
	Name  string
}

const DefaultGitHubHost = "github.com"

const DefaultGitLabHost = "gitlab.com"

const DefaultAzureDevOpsHost = "dev.azure.com"

const (
	providerNameAuto        = "auto"
	providerNameGitHub      = "github"
	providerNameGitLab      = "gitlab"
	providerNameAzureDevOps = "azuredevops"
)

var (
	ErrUnknownRemote    = errors.New("unable to parse remote URL")
	ErrUnsupportedHost  = errors.New("unsupported remote host")
	errMergeTimeMissing = errors.New("merged release PR completion time is unavailable")
)

type releasePRLabelsError struct {
	cause error
}

func (e *releasePRLabelsError) Error() string {
	return e.cause.Error()
}

func (e *releasePRLabelsError) Unwrap() error {
	return e.cause
}

func (e *releasePRLabelsError) Is(target error) bool {
	return target == forge.ErrReleasePRLabelsRejected
}

func wrapReleasePRLabelsError(err error) error {
	if err == nil || errors.Is(err, forge.ErrReleasePRLabelsRejected) {
		return err
	}

	return &releasePRLabelsError{cause: err}
}

const maxPaginationPages = 100

func isFullCommitSHA(ref string) bool {
	const commitSHALength = 40

	if len(ref) != commitSHALength {
		return false
	}

	for _, r := range ref {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
			continue
		default:
			return false
		}
	}

	return true
}
