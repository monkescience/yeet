package release

import (
	"context"

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
		changelog.WithReferences(changelogReferences(target.Changelog.References)),
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
	ctx context.Context,
	target config.ResolvedTarget,
	nextTag string,
	ref string,
	directCommits []commit.Commit,
	childPlans []TargetPlan,
	prCompareRef string,
	mode derivedChangelogMode,
	metadata repoMetadataProvider,
) changelog.Entry {
	direct := newTargetChangelogEntry(ctx, target, nextTag, ref, directCommits, metadata)

	children := make([]changelog.Section, 0, len(childPlans))
	for _, childPlan := range childPlans {
		children = append(children, changelog.Section{
			Heading:  childPlan.ID,
			Sections: childChangelogSections(childPlan, mode),
		})
	}

	entry := changelog.DerivedEntry(nextTag, direct, target.Includes, children)

	if ref == "" {
		return entry
	}

	compareTarget := nextTag
	if mode == derivedChangelogPreview {
		// The future tag does not exist while the release pull request is open.
		compareTarget = prCompareRef
	}

	return changelogEntryWithCompare(entry, ref, compareTarget, metadata)
}

func childChangelogSections(childPlan TargetPlan, mode derivedChangelogMode) []changelog.Section {
	if mode == derivedChangelogPreview && len(childPlan.PREntry.Sections) > 0 {
		return childPlan.PREntry.Sections
	}

	return childPlan.Entry.Sections
}
