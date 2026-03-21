package release

import (
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/provider"
)

func (r *Releaser) setResultChangelogs(
	result *Result,
	ref string,
	entries []provider.CommitEntry,
	commits []commit.Commit,
) {
	result.Changelog = r.renderChangelog(result.NextTag, ref, result.NextTag, commits)
	result.prChangelog = result.Changelog

	if ref != "" && len(entries) > 0 {
		result.prChangelog = r.renderChangelog(result.NextTag, ref, entries[0].Hash, commits)
	}
}

func (r *Releaser) renderChangelog(nextTag, ref, compareTarget string, commits []commit.Commit) string {
	gen := &changelog.Generator{
		Sections:   r.cfg.Changelog.Sections,
		Include:    r.cfg.Changelog.Include,
		RepoURL:    r.metadata.RepoURL(),
		PathPrefix: r.metadata.PathPrefix(),
	}

	entry := gen.Generate(nextTag, ref, commits)
	if ref != "" && compareTarget != "" {
		entry.CompareURL = compareURL(r.metadata.RepoURL(), r.metadata.PathPrefix(), ref, compareTarget)
	}

	return changelog.Render(entry)
}

func (r *Releaser) releasePROptions(
	result *Result,
	releaseBranch, releaseTag string,
) provider.ReleasePROptions {
	prChangelog := result.Changelog
	if result.prChangelog != "" {
		prChangelog = result.prChangelog
	}

	return provider.ReleasePROptions{
		Title:         r.releaseSubject(result),
		Body:          r.releasePRBody(prChangelog, releaseTag),
		BaseBranch:    r.cfg.Branch,
		ReleaseBranch: releaseBranch,
		Files: map[string]string{
			r.cfg.Changelog.File: result.Changelog,
		},
	}
}

func compareURL(repoURL, pathPrefix, fromRef, toRef string) string {
	return fmt.Sprintf("%s%s/compare/%s...%s", repoURL, pathPrefix, fromRef, toRef)
}

func (r *Releaser) releaseSubject(result *Result) string {
	version := result.BaseVersion
	if version == "" {
		version = result.NextVersion
	}

	if r.cfg.Release.SubjectIncludeBranch {
		return fmt.Sprintf("chore(%s): release %s", r.cfg.Branch, version)
	}

	return "chore: release " + version
}

func (r *Releaser) releasePRBody(changelogBody, releaseTag string) string {
	parts := make([]string, 0)

	if header := strings.TrimSpace(r.cfg.Release.PRBodyHeader); header != "" {
		parts = append(parts, header)
	}

	if body := strings.TrimSpace(changelogBody); body != "" {
		parts = append(parts, body)
	}

	if marker := releaseTagMarker(releaseTag); marker != "" {
		parts = append(parts, marker)
	}

	if footer := strings.TrimSpace(r.cfg.Release.PRBodyFooter); footer != "" {
		parts = append(parts, footer)
	}

	return strings.Join(parts, "\n\n")
}

func changelogEntryByTag(changelogBody, tag string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(changelogBody, "\r\n", "\n"), "\n")

	start := -1

	for idx, line := range lines {
		if !strings.HasPrefix(line, "## ") {
			continue
		}

		headingTag, ok := headingTag(line)
		if !ok {
			continue
		}

		if headingTag == tag {
			start = idx

			break
		}
	}

	if start == -1 {
		return "", fmt.Errorf("%w: %s", ErrChangelogEntryNotFound, tag)
	}

	end := len(lines)

	for idx := start + 1; idx < len(lines); idx++ {
		if strings.HasPrefix(lines[idx], "## ") {
			end = idx

			break
		}
	}

	entry := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	if entry == "" {
		return "", fmt.Errorf("%w: %s", ErrChangelogEntryNotFound, tag)
	}

	return entry, nil
}

func headingTag(line string) (string, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	if rest == "" {
		return "", false
	}

	if strings.HasPrefix(rest, "[") {
		idx := strings.Index(rest, "]")
		if idx <= 1 {
			return "", false
		}

		return rest[1:idx], true
	}

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}

	return fields[0], true
}
