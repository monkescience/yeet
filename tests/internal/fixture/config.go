// Package fixture writes per-test on-disk artifacts used by blackbox tests,
// such as a minimal .yeet.yaml that drives a single provider.
package fixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
)

// ConfigOptions describes the values to render into a .yeet.yaml. Empty
// fields are omitted so callers can mix and match defaults.
type ConfigOptions struct {
	Provider     string
	Branch       string
	Owner        string
	Repo         string
	Project      string
	Host         string
	Organization string
	Versioning   string
	CalVerFormat string
	VersionFiles []VersionFileOptions
	Channels     map[string]ChannelOptions
	Targets      []TargetOptions
}

// TargetOptions describes one entry in the targets map. When empty, the
// fixture writes a single "default" target.
type TargetOptions struct {
	Name      string
	Path      string
	TagPrefix string
}

// VersionFileOptions describes a version_files entry. Format and JSONPointer
// are optional.
type VersionFileOptions struct {
	Path        string
	Format      string
	JSONPointer string
}

// ChannelOptions describes a prerelease channel entry.
type ChannelOptions struct {
	Branch     string
	Prerelease string
}

// WriteConfig renders a .yeet.yaml under a fresh t.TempDir() and returns the
// absolute path. The caller passes this path to the binary via --config.
func WriteConfig(t *testing.T, opts ConfigOptions) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, ".yeet.yaml")

	const filePerm = 0o600

	err := os.WriteFile(path, []byte(renderConfig(opts)), filePerm)
	testastic.NoError(t, err)

	return path
}

func renderConfig(opts ConfigOptions) string {
	var b strings.Builder

	writeTop(&b, opts)
	writeRepository(&b, opts)
	writeVersionFiles(&b, opts.VersionFiles)
	writeChannels(&b, opts.Channels)
	writeTargets(&b, opts.Targets)

	return b.String()
}

func writeTop(b *strings.Builder, opts ConfigOptions) {
	writeScalar(b, "provider: ", opts.Provider)
	writeScalar(b, "branch: ", opts.Branch)
	writeScalar(b, "versioning: ", opts.Versioning)

	if opts.CalVerFormat != "" {
		b.WriteString("calver:\n  format: ")
		b.WriteString(opts.CalVerFormat)
		b.WriteString("\n")
	}
}

func writeRepository(b *strings.Builder, opts ConfigOptions) {
	b.WriteString("repository:\n")
	writeScalar(b, "  host: ", opts.Host)
	writeScalar(b, "  owner: ", opts.Owner)
	writeScalar(b, "  repo: ", opts.Repo)
	writeScalar(b, "  project: ", opts.Project)
	writeScalar(b, "  organization: ", opts.Organization)
}

func writeVersionFiles(b *strings.Builder, files []VersionFileOptions) {
	if len(files) == 0 {
		return
	}

	b.WriteString("version_files:\n")

	for _, vf := range files {
		b.WriteString("  - path: ")
		b.WriteString(vf.Path)
		b.WriteString("\n")
		writeScalar(b, "    format: ", vf.Format)
		writeScalar(b, "    json_pointer: ", vf.JSONPointer)
	}
}

func writeChannels(b *strings.Builder, channels map[string]ChannelOptions) {
	if len(channels) == 0 {
		return
	}

	b.WriteString("release:\n  channels:\n")

	for name, channel := range channels {
		b.WriteString("    ")
		b.WriteString(name)
		b.WriteString(":\n      branch: ")
		b.WriteString(channel.Branch)
		b.WriteString("\n")
		writeScalar(b, "      prerelease: ", channel.Prerelease)
	}
}

func writeTargets(b *strings.Builder, targets []TargetOptions) {
	b.WriteString("targets:\n")

	if len(targets) == 0 {
		b.WriteString("  default:\n    type: path\n    path: .\n    tag_prefix: v\n")

		return
	}

	for _, target := range targets {
		b.WriteString("  ")
		b.WriteString(target.Name)
		b.WriteString(":\n    type: path\n    path: ")
		b.WriteString(target.Path)
		b.WriteString("\n    tag_prefix: ")
		b.WriteString(target.TagPrefix)
		b.WriteString("\n")
	}
}

func writeScalar(b *strings.Builder, prefix, value string) {
	if value == "" {
		return
	}

	b.WriteString(prefix)
	b.WriteString(value)
	b.WriteString("\n")
}
