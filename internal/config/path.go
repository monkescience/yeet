package config

import (
	"path"
	"path/filepath"
	"strings"
)

// RepoPathContains reports whether candidatePath is inside basePath using
// repo-relative forward-slash semantics. A basePath of "." contains everything.
func RepoPathContains(basePath, candidatePath string) bool {
	if basePath == "." {
		return true
	}

	if candidatePath == basePath {
		return true
	}

	return strings.HasPrefix(candidatePath, basePath+"/")
}

func normalizeRepoPath(rawPath string) (string, error) {
	trimmedPath := strings.TrimSpace(rawPath)
	if trimmedPath == "" {
		return "", errEmptyRepoPath
	}

	if isRepoPathAbsolute(trimmedPath) {
		return "", errPathMustBeRepoRelative
	}

	normalizedPath := filepath.ToSlash(trimmedPath)
	if path.IsAbs(normalizedPath) {
		return "", errPathMustBeRepoRelative
	}

	normalizedPath = path.Clean(normalizedPath)
	if normalizedPath == "." {
		return ".", nil
	}

	if normalizedPath == ".." || strings.HasPrefix(normalizedPath, "../") {
		return "", errPathMustBeRepoRelative
	}

	return normalizedPath, nil
}

func isRepoPathAbsolute(rawPath string) bool {
	const windowsDrivePrefixLength = 3

	if filepath.IsAbs(rawPath) {
		return true
	}

	normalizedPath := filepath.ToSlash(rawPath)
	if len(normalizedPath) < windowsDrivePrefixLength {
		return false
	}

	if normalizedPath[1] != ':' || normalizedPath[2] != '/' {
		return false
	}

	return (normalizedPath[0] >= 'A' && normalizedPath[0] <= 'Z') ||
		(normalizedPath[0] >= 'a' && normalizedPath[0] <= 'z')
}
