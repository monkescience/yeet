package release

import (
	"context"
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
)

func renderTargetChangelog(
	ctx context.Context,
	target config.ResolvedTarget,
	nextTag, ref, compareTarget string,
	commits []commit.Commit,
	releaser *Releaser,
) string {
	gen := changelog.New(
		changelog.WithSections(target.Changelog.Sections),
		changelog.WithInclude(target.Changelog.Include),
		changelog.WithRepoURL(releaser.metadata.RepoURL()),
		changelog.WithPathPrefix(releaser.metadata.PathPrefix()),
		changelog.WithCompareURL(releaser.metadata.CompareURL),
		changelog.WithReferences(target.Changelog.References),
	)

	entry := gen.Generate(ctx, nextTag, ref, commits)
	if ref != "" && compareTarget != "" {
		entry.CompareURL = releaser.metadata.CompareURL(ref, compareTarget)
	}

	return changelog.Render(entry)
}

func renderDerivedChangelog(
	ctx context.Context,
	target config.ResolvedTarget,
	nextTag string,
	ref string,
	directCommits []commit.Commit,
	childPlans []TargetPlan,
	prCompareRef string,
	prMode bool,
	releaser *Releaser,
) string {
	var body strings.Builder

	if len(directCommits) > 0 {
		directEntry := renderTargetChangelog(ctx, target, nextTag, ref, nextTag, directCommits, releaser)
		body.WriteString(changelogBodyWithoutHeading(directEntry))
	}

	for _, childPlan := range childPlans {
		if body.Len() > 0 && !strings.HasSuffix(body.String(), "\n\n") {
			body.WriteString("\n\n")
		}

		fmt.Fprintf(&body, "### %s\n\n", childPlan.ID)

		childChangelog := childPlan.Changelog
		if prMode && childPlan.PRChangelog != "" {
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
		if prMode {
			compareTarget = prCompareRef
		}

		if compareTarget != "" {
			entry.CompareURL = releaser.metadata.CompareURL(ref, compareTarget)
		}
	}

	return changelog.Render(entry)
}

func changelogBodyWithoutHeading(renderedEntry string) string {
	lines := strings.Split(strings.ReplaceAll(renderedEntry, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		if strings.HasPrefix(line, "## ") {
			return strings.TrimSpace(strings.Join(lines[idx+1:], "\n"))
		}
	}

	return strings.TrimSpace(renderedEntry)
}
