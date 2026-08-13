// Package fixture writes on-disk artifacts for blackbox tests.
package fixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
)

type ConfigOptions struct {
	Provider          string
	Branch            string
	Owner             string
	Repo              string
	Project           string
	Host              string
	Organization      string
	Collection        string
	Versioning        string
	CalVerFormat      string
	VersionFiles      []VersionFileOptions
	Channels          map[string]ChannelOptions
	Targets           []TargetOptions
	ReferencePatterns []ReferencePatternOptions
	ReferenceFooters  map[string]string
	Reviewers         []string
	Labels            *LabelsOptions
	PRTitle           string
	PRTitleGroup      string
	CommitSubject     string
}

type LabelsOptions struct {
	Pending string
	Tagged  string
	Extra   []string
}

type ReferencePatternOptions struct {
	Pattern string
	URL     string
}

type TargetOptions struct {
	Name         string
	Path         string
	TagPrefix    string
	Type         string
	ExcludePaths []string
	Includes     []string
}

type VersionFileOptions struct {
	Path        string
	Format      string
	JSONPointer string
}

type ChannelOptions struct {
	Branch     string
	Prerelease string
}

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
	writeRelease(&b, opts)
	writeChangelog(&b, opts)
	writeTargets(&b, opts.Targets)

	return b.String()
}

func writeRelease(b *strings.Builder, opts ConfigOptions) {
	hasReleaseField := len(opts.Reviewers) > 0 ||
		len(opts.Channels) > 0 ||
		opts.Labels != nil ||
		opts.PRTitle != "" ||
		opts.PRTitleGroup != "" ||
		opts.CommitSubject != ""

	if !hasReleaseField {
		return
	}

	b.WriteString("release:\n")

	writeQuotedScalar(b, "  pr_title: ", opts.PRTitle)
	writeQuotedScalar(b, "  pr_title_group: ", opts.PRTitleGroup)
	writeQuotedScalar(b, "  commit_subject: ", opts.CommitSubject)

	if len(opts.Reviewers) > 0 {
		b.WriteString("  reviewers:\n")

		for _, reviewer := range opts.Reviewers {
			b.WriteString("    - ")
			b.WriteString(reviewer)
			b.WriteString("\n")
		}
	}

	writeLabels(b, opts.Labels)
	writeChannels(b, opts.Channels)
}

func writeLabels(b *strings.Builder, labels *LabelsOptions) {
	if labels == nil {
		return
	}

	b.WriteString("  labels:\n")
	writeQuotedScalar(b, "    pending: ", labels.Pending)
	writeQuotedScalar(b, "    tagged: ", labels.Tagged)

	if len(labels.Extra) == 0 {
		return
	}

	b.WriteString("    extra:\n")

	for _, extra := range labels.Extra {
		b.WriteString("      - ")
		b.WriteString(quoteYAML(extra))
		b.WriteString("\n")
	}
}

func writeChangelog(b *strings.Builder, opts ConfigOptions) {
	if len(opts.ReferencePatterns) == 0 && len(opts.ReferenceFooters) == 0 {
		return
	}

	b.WriteString("changelog:\n  references:\n")

	if len(opts.ReferencePatterns) > 0 {
		b.WriteString("    patterns:\n")

		for _, pattern := range opts.ReferencePatterns {
			b.WriteString("      - pattern: ")
			b.WriteString(pattern.Pattern)
			b.WriteString("\n        url: ")
			b.WriteString(pattern.URL)
			b.WriteString("\n")
		}
	}

	if len(opts.ReferenceFooters) == 0 {
		return
	}

	b.WriteString("    footers:\n")

	for key, url := range opts.ReferenceFooters {
		b.WriteString("      ")
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(url)
		b.WriteString("\n")
	}
}

func writeTop(b *strings.Builder, opts ConfigOptions) {
	writeScalar(b, "provider: ", opts.Provider)
	writeScalar(b, "branch: ", opts.Branch)
	writeScalar(b, "versioning: ", opts.Versioning)

	if opts.CalVerFormat == "" {
		return
	}

	b.WriteString("calver:\n  format: ")
	b.WriteString(opts.CalVerFormat)
	b.WriteString("\n")
}

func writeRepository(b *strings.Builder, opts ConfigOptions) {
	hasSubsectionField := opts.Host != "" ||
		opts.Owner != "" ||
		opts.Repo != "" ||
		opts.Project != "" ||
		opts.Organization != "" ||
		opts.Collection != ""

	if !hasSubsectionField {
		return
	}

	b.WriteString("repository:\n")

	provider := opts.Provider
	if provider == "" {
		provider = inferProvider(opts)
	}

	switch provider {
	case "github":
		b.WriteString("  github:\n")
		writeScalar(b, "    host: ", opts.Host)
		writeScalar(b, "    owner: ", opts.Owner)
		writeScalar(b, "    repo: ", opts.Repo)
		writeScalar(b, "    project: ", opts.Project)
	case "gitlab":
		b.WriteString("  gitlab:\n")
		writeScalar(b, "    host: ", opts.Host)
		writeScalar(b, "    project: ", opts.Project)
	case "azuredevops":
		b.WriteString("  azuredevops:\n")
		writeScalar(b, "    host: ", opts.Host)
		writeScalar(b, "    organization: ", opts.Organization)
		writeScalar(b, "    project: ", opts.Project)
		writeScalar(b, "    repo: ", opts.Repo)
		writeScalar(b, "    collection: ", opts.Collection)
	}
}

func inferProvider(opts ConfigOptions) string {
	if opts.Organization != "" {
		return "azuredevops"
	}

	if opts.Owner != "" || opts.Repo != "" {
		return "github"
	}

	if opts.Project != "" {
		return "gitlab"
	}

	return ""
}

func writeVersionFiles(b *strings.Builder, files []VersionFileOptions) {
	if len(files) == 0 {
		return
	}

	b.WriteString("version_files:\n")

	for _, vf := range files {
		if vf.Format == "" && vf.JSONPointer == "" {
			b.WriteString("  - ")
			b.WriteString(vf.Path)
			b.WriteString("\n")

			continue
		}

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

	b.WriteString("  channels:\n")

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
		targetType := target.Type
		if targetType == "" {
			targetType = "path"
		}

		b.WriteString("  ")
		b.WriteString(target.Name)
		b.WriteString(":\n    type: ")
		b.WriteString(targetType)
		b.WriteString("\n    path: ")
		b.WriteString(target.Path)
		b.WriteString("\n    tag_prefix: ")
		b.WriteString(target.TagPrefix)
		b.WriteString("\n")

		writeStringSlice(b, "    exclude_paths:\n", target.ExcludePaths)
		writeStringSlice(b, "    includes:\n", target.Includes)
	}
}

func writeStringSlice(b *strings.Builder, prefix string, values []string) {
	if len(values) == 0 {
		return
	}

	b.WriteString(prefix)

	for _, v := range values {
		b.WriteString("      - ")
		b.WriteString(v)
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

func writeQuotedScalar(b *strings.Builder, prefix, value string) {
	if value == "" {
		return
	}

	b.WriteString(prefix)
	b.WriteString(quoteYAML(value))
	b.WriteString("\n")
}

func quoteYAML(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)

	return `"` + escaped + `"`
}
