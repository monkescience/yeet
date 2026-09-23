package versionfile

import (
	"errors"
	"strings"
)

const defaultHint = "check the file and its version_files configuration"

var problems = []struct {
	cause   error
	problem string
	hint    string
}{
	{cause: ErrNoMarkersFound, problem: "file has no yeet version markers"},
	{cause: ErrUnclosedBlockMarker, problem: "version marker block has no end marker"},
	{cause: ErrNestedBlockMarker, problem: "version marker blocks are nested"},
	{cause: ErrMarkerNoMatch, problem: "version marker has no matching version"},
	{cause: ErrMarkerSchemeMismatch, problem: "version marker does not match the versioning scheme"},
	{
		cause:   ErrInvalidNextVersion,
		problem: "next version is invalid for the configured versioning scheme",
		hint:    "check target versioning and the version_files configuration",
	},
	{
		cause:   ErrInvalidScheme,
		problem: "configured versioning scheme is invalid",
		hint:    "check the target versioning configuration",
	},
	{cause: ErrInvalidJSON, problem: "file contains invalid json"},
	{
		cause:   ErrInvalidJSONPointer,
		problem: "json pointer is invalid",
		hint:    "use a valid JSON pointer in the version_files configuration",
	},
	{cause: ErrJSONPointerNotFound, problem: "json pointer was not found"},
	{cause: ErrJSONPointerNonString, problem: "json pointer must select a string"},
}

func Describe(err error) (string, string, bool) {
	for _, problem := range problems {
		if !errors.Is(err, problem.cause) {
			continue
		}

		hint := defaultHint
		if problem.hint != "" {
			hint = problem.hint
		}

		marker, ok := errors.AsType[*MarkerError](err)
		if ok && len(marker.Suggestions) > 0 {
			hint = "use " + strings.Join(marker.Suggestions, " or ") + " for the configured versioning scheme"
		}

		return problem.problem, hint, true
	}

	return "", "", false
}
