package release

import (
	"fmt"

	"github.com/monkescience/yeet/internal/forge"
)

type SelectionError struct {
	Target         string
	IncludedBy     string
	Branch         string
	ExpectedBranch string
	Channel        string
	Ref            string
	Problem        string
	cause          error
}

func (e *SelectionError) Error() string { return e.cause.Error() }

func (e *SelectionError) Unwrap() error { return e.cause }

func unknownTargetError(target, includedBy string) error {
	detail := target
	if includedBy != "" {
		detail = fmt.Sprintf("%s (included by %s)", target, includedBy)
	}

	return &SelectionError{
		Target:     target,
		IncludedBy: includedBy,
		cause:      fmt.Errorf("%w: %s", errUnknownTarget, detail),
	}
}

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

type FileConflictKind string

const (
	FileConflictAcrossUnits          FileConflictKind = "across_units"
	FileConflictIncompatibleVersions FileConflictKind = "incompatible_versions"
	FileConflictChangelogVersion     FileConflictKind = "changelog_version"
)

const (
	FileConflictRemedyAcrossUnits          = "configure separate files or place the targets in one atomic group"
	FileConflictRemedyIncompatibleVersions = "configure separately addressable version files"
	FileConflictRemedyChangelogVersion     = "configure different paths for the changelog and version file"
)

type FileConflictError struct {
	Kind   FileConflictKind
	Units  []string
	Target string
	Path   string
	Hint   string
	cause  error
}

func (e *FileConflictError) Error() string { return e.cause.Error() }

func (e *FileConflictError) Unwrap() error { return e.cause }

func fileConflictError(
	kind FileConflictKind,
	path, target string,
	units []string,
	cause error,
) error {
	return &FileConflictError{
		Kind:   kind,
		Units:  units,
		Target: target,
		Path:   path,
		Hint:   fileConflictRemedy(kind),
		cause:  cause,
	}
}

func fileConflictRemedy(kind FileConflictKind) string {
	switch kind {
	case FileConflictAcrossUnits:
		return FileConflictRemedyAcrossUnits
	case FileConflictIncompatibleVersions:
		return FileConflictRemedyIncompatibleVersions
	case FileConflictChangelogVersion:
		return FileConflictRemedyChangelogVersion
	default:
		return ""
	}
}

func pullRequestReference(pullRequest *forge.PullRequest) string {
	if pullRequest.Reference != "" {
		return pullRequest.Reference
	}

	return fmt.Sprintf("pull request #%d", pullRequest.Number)
}

type PendingReleaseError struct {
	References []string
	URLs       []string
	Unit       string
	Branch     string
	Problem    string
	Hint       string
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
