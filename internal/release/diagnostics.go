package release

import (
	"fmt"

	"github.com/monkescience/yeet/internal/forge"
)

type SelectionError struct {
	Target         string
	Branch         string
	ExpectedBranch string
	Channel        string
	Ref            string
	cause          error
}

func (e *SelectionError) Error() string { return e.cause.Error() }

func (e *SelectionError) Unwrap() error { return e.cause }

type VersionFileError struct {
	Target string
	Path   string
	cause  error
}

func (e *VersionFileError) Error() string { return e.cause.Error() }

func (e *VersionFileError) Unwrap() error { return e.cause }

type CommitOverrideError struct {
	Commit        string
	Problem       string
	MissingMarker string
	cause         error
}

func (e *CommitOverrideError) Error() string { return e.cause.Error() }

func (e *CommitOverrideError) Unwrap() error { return e.cause }

type FileConflictError struct {
	Units []string
	Path  string
	cause error
}

func (e *FileConflictError) Error() string { return e.cause.Error() }

func (e *FileConflictError) Unwrap() error { return e.cause }

func fileConflictError(path string, units []string, cause error) error {
	return &FileConflictError{Units: units, Path: path, cause: cause}
}

func pullRequestReference(pullRequest *forge.PullRequest) string {
	if pullRequest.Reference != "" {
		return pullRequest.Reference
	}

	return fmt.Sprintf("pull request #%d", pullRequest.Number)
}

type PendingReleaseError struct {
	References []string
	cause      error
}

func (e *PendingReleaseError) Error() string { return e.cause.Error() }

func (e *PendingReleaseError) Unwrap() error { return e.cause }

type ManifestError struct {
	Reference string
	cause     error
}

func (e *ManifestError) Error() string { return e.cause.Error() }

func (e *ManifestError) Unwrap() error { return e.cause }
