package config

import (
	"regexp"
	"strings"

	"github.com/monkescience/yeet/internal/build"
)

const (
	schemaDirectivePrefix = "# yaml-language-server: $schema="
	schemaURLPrefix       = "https://raw.githubusercontent.com/monkescience/yeet/"
	schemaFileName        = "/yeet.schema.json"

	// schemaDevelopmentRef is used whenever the binary cannot name the tag it was
	// built from, because an unreleased ref is the only one guaranteed to serve
	// rules at least as new as the ones this binary enforces.
	schemaDevelopmentRef = "main"

	dirtyVersionSuffix = "-dirty"
)

// releaseVersion matches a version injected from an exact release tag. A
// `git describe` fallback appends `-<commits>-g<sha>`, which the single-hyphen
// prerelease group rejects, and neither that nor a dirty marker names a ref
// GitHub can serve.
var releaseVersion = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?$`)

// SchemaDirective returns the editor modeline yeet init writes, pinned to the
// release that produced this binary so a generated config is checked against
// the rules its own yeet enforces.
func SchemaDirective() string {
	return schemaDirectivePrefix + schemaURLPrefix + schemaRef(build.Version()) + schemaFileName
}

func schemaRef(version string) string {
	if strings.HasSuffix(version, dirtyVersionSuffix) || !releaseVersion.MatchString(version) {
		return schemaDevelopmentRef
	}

	return "v" + strings.TrimPrefix(version, "v")
}
