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
	VersionFiles []string
	Channels     map[string]ChannelOptions
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

	writeScalar(&b, "provider: ", opts.Provider)
	writeScalar(&b, "branch: ", opts.Branch)

	b.WriteString("repository:\n")
	writeScalar(&b, "  host: ", opts.Host)
	writeScalar(&b, "  owner: ", opts.Owner)
	writeScalar(&b, "  repo: ", opts.Repo)
	writeScalar(&b, "  project: ", opts.Project)
	writeScalar(&b, "  organization: ", opts.Organization)

	if len(opts.VersionFiles) > 0 {
		b.WriteString("version_files:\n")

		for _, path := range opts.VersionFiles {
			b.WriteString("  - ")
			b.WriteString(path)
			b.WriteString("\n")
		}
	}

	if len(opts.Channels) > 0 {
		b.WriteString("release:\n  channels:\n")

		for name, channel := range opts.Channels {
			b.WriteString("    ")
			b.WriteString(name)
			b.WriteString(":\n      branch: ")
			b.WriteString(channel.Branch)
			b.WriteString("\n")

			if channel.Prerelease != "" {
				b.WriteString("      prerelease: ")
				b.WriteString(channel.Prerelease)
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("targets:\n  default:\n    type: path\n    path: .\n    tag_prefix: v\n")

	return b.String()
}

func writeScalar(b *strings.Builder, prefix, value string) {
	if value == "" {
		return
	}

	b.WriteString(prefix)
	b.WriteString(value)
	b.WriteString("\n")
}
