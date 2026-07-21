package release

import (
	"context"
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
)

type derivedChangelogMode uint8

const (
	derivedChangelogRelease derivedChangelogMode = iota
	derivedChangelogPreview
)

type changelogFileKey struct {
	branch string
	path   string
}

type changelogFileRead struct {
	content string
	err     error
}

type changelogFileCache struct {
	reads map[changelogFileKey]changelogFileRead
}

func newChangelogFileCache() *changelogFileCache {
	return &changelogFileCache{
		reads: make(map[changelogFileKey]changelogFileRead),
	}
}

func (c *changelogFileCache) get(
	branch, path string,
	load func() (string, error),
) (string, error) {
	key := changelogFileKey{branch: branch, path: path}
	if read, exists := c.reads[key]; exists {
		return read.content, read.err
	}

	content, err := load()
	c.reads[key] = changelogFileRead{content: content, err: err}

	return content, err
}

func newTargetChangelogEntry(
	ctx context.Context,
	target config.ResolvedTarget,
	nextTag, ref string,
	commits []commit.Commit,
	metadata repoMetadataProvider,
) changelog.Entry {
	gen := changelog.New(
		changelog.WithSections(target.Changelog.Sections),
		changelog.WithInclude(target.Changelog.Include),
		changelog.WithRepoURL(metadata.RepoURL()),
		changelog.WithPathPrefix(metadata.PathPrefix()),
		changelog.WithCompareURL(metadata.CompareURL),
		changelog.WithReferences(target.Changelog.References),
	)

	return gen.Generate(ctx, nextTag, ref, commits)
}

func renderChangelogEntry(entry changelog.Entry, ref, compareTarget string, metadata repoMetadataProvider) string {
	if ref != "" && compareTarget != "" {
		entry.CompareURL = metadata.CompareURL(ref, compareTarget)
	}

	return changelog.Render(entry)
}

func renderTargetChangelog(
	ctx context.Context,
	target config.ResolvedTarget,
	nextTag, ref, compareTarget string,
	commits []commit.Commit,
	metadata repoMetadataProvider,
) string {
	entry := newTargetChangelogEntry(ctx, target, nextTag, ref, commits, metadata)

	return renderChangelogEntry(entry, ref, compareTarget, metadata)
}

func renderDerivedChangelog(
	ctx context.Context,
	target config.ResolvedTarget,
	nextTag string,
	ref string,
	directCommits []commit.Commit,
	childPlans []TargetPlan,
	prCompareRef string,
	mode derivedChangelogMode,
	metadata repoMetadataProvider,
) string {
	var body strings.Builder

	if len(directCommits) > 0 {
		directEntry := renderTargetChangelog(ctx, target, nextTag, ref, nextTag, directCommits, metadata)
		body.WriteString(changelogBodyWithoutHeading(directEntry))
	}

	for _, childPlan := range childPlans {
		if body.Len() > 0 && !strings.HasSuffix(body.String(), "\n\n") {
			body.WriteString("\n\n")
		}

		fmt.Fprintf(&body, "### %s\n\n", childPlan.ID)

		childChangelog := childPlan.Changelog
		if mode == derivedChangelogPreview && childPlan.PRChangelog != "" {
			childChangelog = childPlan.PRChangelog
		}

		body.WriteString(strings.TrimSpace(changelogBodyWithoutHeading(childChangelog)))
		body.WriteString("\n")
	}

	entry := changelog.Entry{
		Version: nextTag,
		Body:    strings.TrimSpace(body.String()) + "\n",
	}

	if ref != "" {
		compareTarget := nextTag
		if mode == derivedChangelogPreview {
			// The future tag does not exist while the release pull request is open.
			compareTarget = prCompareRef
		}

		if compareTarget != "" {
			entry.CompareURL = metadata.CompareURL(ref, compareTarget)
		}
	}

	return changelog.Render(entry)
}

func changelogBodyWithoutHeading(renderedEntry string) string {
	lines := splitLines(renderedEntry)
	for idx, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			return strings.TrimSpace(strings.Join(lines[idx+1:], "\n"))
		}
	}

	return strings.TrimSpace(renderedEntry)
}
