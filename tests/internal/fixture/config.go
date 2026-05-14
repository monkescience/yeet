// Package fixture writes per-test on-disk artifacts used by blackbox tests,
// such as a minimal .yeet.yaml that drives a single provider.
package fixture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
)

// ConfigOptions describes the values to render into a .yeet.yaml. Empty
// fields are omitted so callers can mix and match defaults.
type ConfigOptions struct {
	Provider string
	Branch   string
	Owner    string
	Repo     string
	Project  string
	Host     string
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
	out := ""

	if opts.Provider != "" {
		out += "provider: " + opts.Provider + "\n"
	}

	if opts.Branch != "" {
		out += "branch: " + opts.Branch + "\n"
	}

	out += "repository:\n"

	if opts.Host != "" {
		out += "  host: " + opts.Host + "\n"
	}

	if opts.Owner != "" {
		out += "  owner: " + opts.Owner + "\n"
	}

	if opts.Repo != "" {
		out += "  repo: " + opts.Repo + "\n"
	}

	if opts.Project != "" {
		out += "  project: " + opts.Project + "\n"
	}

	out += "targets:\n"
	out += "  default:\n"
	out += "    type: path\n"
	out += "    path: .\n"
	out += "    tag_prefix: v\n"

	return out
}
