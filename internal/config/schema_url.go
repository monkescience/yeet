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

	schemaDevelopmentRef = "main"

	dirtyVersionSuffix = "-dirty"
)

var releaseVersion = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?$`)

func SchemaDirective() string {
	return schemaDirectivePrefix + schemaURLPrefix + schemaRef(build.Version()) + schemaFileName
}

func schemaRef(version string) string {
	if strings.HasSuffix(version, dirtyVersionSuffix) || !releaseVersion.MatchString(version) {
		return schemaDevelopmentRef
	}

	return "v" + strings.TrimPrefix(version, "v")
}
