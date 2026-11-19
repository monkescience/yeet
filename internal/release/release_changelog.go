package release

import (
	"context"
	"time"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
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
	date time.Time,
	metadata repoMetadataProvider,
) changelog.Entry {
	gen := changelog.New(
		changelog.WithSections(target.Changelog.Sections),
		changelog.WithInclude(target.Changelog.Include),
		changelog.WithRepoURL(metadata.RepoURL()),
		changelog.WithPathPrefix(metadata.PathPrefix()),
		changelog.WithCompareURL(metadata.CompareURL),
		changelog.WithReferences(changelogReferences(target.Changelog.References)),
		changelog.WithDate(date),
	)

	return gen.Generate(ctx, nextTag, ref, commits)
}

func changelogReferences(references config.ReferencesConfig) changelog.References {
	patterns := make([]changelog.ReferencePattern, 0, len(references.Patterns))
	for _, pattern := range references.Patterns {
		patterns = append(patterns, changelog.ReferencePattern{Pattern: pattern.Pattern, URL: pattern.URL})
	}

	return changelog.References{Patterns: patterns, Footers: references.Footers}
}

func changelogEntryWithCompare(
	entry changelog.Entry,
	ref, compareTarget string,
	metadata repoMetadataProvider,
) changelog.Entry {
	if ref != "" && compareTarget != "" {
		entry.CompareURL = metadata.CompareURL(ref, compareTarget)
	}

	return entry
}

func derivedChangelogEntry(
	direct changelog.Entry,
	includedTargets []string,
	children []derivedChild,
	ref, compareTarget string,
	metadata repoMetadataProvider,
) changelog.Entry {
	sections := make([]changelog.Section, 0, len(children))
	for _, child := range children {
		sections = append(sections, changelog.Section{
			Heading:  child.target.ID,
			Sections: child.sections,
		})
	}

	entry := changelog.DerivedEntry(direct, includedTargets, sections)

	return changelogEntryWithCompare(entry, ref, compareTarget, metadata)
}
