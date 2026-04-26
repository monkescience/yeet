package release

import (
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/provider"
)

type prSection struct {
	id   string
	plan *TargetPlan
	body string
}

const releaseNotesStartMarker = "<!-- BEGIN_YEET_RELEASE_NOTES -->"

const releaseNotesEndMarker = "<!-- END_YEET_RELEASE_NOTES -->"

func (r *Releaser) releasePROptions(result *Result, releaseBranch string) (provider.ReleasePROptions, error) {
	manifestMarker, err := releaseManifestMarker(releaseManifestForPlans(result.BaseBranch, result.Plans))
	if err != nil {
		return provider.ReleasePROptions{}, err
	}

	changelogBody := insertReleaseNotesBlock(r.combinedPRChangelog(result), result.ReleaseNotes)

	return provider.ReleasePROptions{
		Title:         r.releaseSubject(result),
		Body:          r.releasePRBody(changelogBody, manifestMarker),
		BaseBranch:    r.cfg.Branch,
		ReleaseBranch: releaseBranch,
		Files:         map[string]string{},
	}, nil
}

func compareURL(repoURL, pathPrefix, fromRef, toRef string) string {
	return fmt.Sprintf("%s%s/compare/%s...%s", repoURL, pathPrefix, fromRef, toRef)
}

func (r *Releaser) releaseSubject(result *Result) string {
	plans := result.Plans
	if len(plans) == 1 {
		version := plans[0].NextVersion

		if r.cfg.Release.SubjectIncludeBranch {
			return fmt.Sprintf("chore(%s): release %s", r.cfg.Branch, version)
		}

		return "chore: release " + version
	}

	if r.cfg.Release.SubjectIncludeBranch {
		return fmt.Sprintf("chore(%s): release wave", r.cfg.Branch)
	}

	return "chore: release wave"
}

func (r *Releaser) combinedPRChangelog(result *Result) string {
	plans := result.Plans
	if len(plans) == 0 {
		return ""
	}

	if len(plans) == 1 {
		if plans[0].PRChangelog != "" {
			return plans[0].PRChangelog
		}

		return plans[0].Changelog
	}

	sections := buildPRSections(plans)

	var body strings.Builder
	body.WriteString("## Release wave\n\n")
	fmt.Fprintf(&body, "Base branch: `%s`\n", result.BaseBranch)
	fmt.Fprintf(&body, "Targets: %s", formatSectionTargetList(sections))

	for _, section := range sections {
		body.WriteString("\n\n")
		body.WriteString(renderFlatPRSection(section))
	}

	return body.String()
}

func buildPRSections(plans []TargetPlan) []prSection {
	sections := make([]prSection, 0, len(plans))

	for _, plan := range plans {
		if plan.Type != "derived" {
			continue
		}

		p := plan
		parsedChangelog := parseRenderedChangelog(preferredPRChangelog(plan))
		directBody, _ := splitDerivedChangelogBody(parsedChangelog.Body, plan.IncludedTargets)

		sections = append(sections, prSection{
			id:   plan.ID,
			plan: &p,
			body: directBody,
		})
	}

	for _, plan := range plans {
		if plan.Type == "derived" {
			continue
		}

		p := plan
		parsedChangelog := parseRenderedChangelog(preferredPRChangelog(plan))

		sections = append(sections, prSection{
			id:   plan.ID,
			plan: &p,
			body: parsedChangelog.Body,
		})
	}

	return sections
}

func renderFlatPRSection(section prSection) string {
	var body strings.Builder
	fmt.Fprintf(&body, "## %s\n\n", section.id)

	parsedChangelog := parseRenderedChangelog(preferredPRChangelog(*section.plan))
	body.WriteString(renderPlanMetadata(*section.plan, parsedChangelog))
	appendMarkdownBlock(&body, section.body)

	return body.String()
}

func formatSectionTargetList(sections []prSection) string {
	targetIDs := make([]string, 0, len(sections))
	for _, section := range sections {
		targetIDs = append(targetIDs, fmt.Sprintf("`%s`", section.id))
	}

	return strings.Join(targetIDs, ", ")
}

func preferredPRChangelog(plan TargetPlan) string {
	if plan.PRChangelog != "" {
		return plan.PRChangelog
	}

	return plan.Changelog
}

func renderPlanMetadata(plan TargetPlan, parsedChangelog renderedChangelog) string {
	var body strings.Builder

	previousTag := planPreviousTag(plan)

	nextTag := plan.NextTag

	fmt.Fprintf(&body, "Tag: `%s` -> `%s`\n", previousTag, nextTag)
	fmt.Fprintf(&body, "Bump: `%s`", plan.BumpType)

	if parsedChangelog.Date != "" {
		fmt.Fprintf(&body, "\nDate: `%s`", parsedChangelog.Date)
	}

	if parsedChangelog.CompareURL != "" {
		fmt.Fprintf(
			&body,
			"\nCompare: [%s](%s)",
			compareRange(parsedChangelog.CompareURL),
			parsedChangelog.CompareURL,
		)
	}

	return body.String()
}

func planPreviousTag(plan TargetPlan) string {
	if strings.TrimSpace(plan.CurrentVersion) == "" {
		return "none"
	}

	prefix := planTagPrefix(plan)
	if prefix == "" {
		return plan.CurrentVersion
	}

	return prefix + plan.CurrentVersion
}

func planTagPrefix(plan TargetPlan) string {
	if plan.NextTag != "" && plan.NextVersion != "" && strings.HasSuffix(plan.NextTag, plan.NextVersion) {
		return strings.TrimSuffix(plan.NextTag, plan.NextVersion)
	}

	return ""
}

func compareRange(compareURL string) string {
	_, comparePath, found := strings.Cut(compareURL, "/compare/")
	if !found {
		return "compare"
	}

	return comparePath
}

type renderedChangelog struct {
	Heading    string
	Tag        string
	CompareURL string
	Date       string
	Body       string
}

func parseRenderedChangelog(changelogBody string) renderedChangelog {
	lines := strings.Split(strings.ReplaceAll(changelogBody, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		parsedChangelog := renderedChangelog{
			Heading: trimmedLine,
			Body:    strings.TrimSpace(strings.Join(lines[idx+1:], "\n")),
		}

		if !strings.HasPrefix(trimmedLine, "## ") {
			parsedChangelog.Body = strings.TrimSpace(strings.Join(lines[idx:], "\n"))

			return parsedChangelog
		}

		parseRenderedChangelogHeading(strings.TrimSpace(strings.TrimPrefix(trimmedLine, "## ")), &parsedChangelog)

		return parsedChangelog
	}

	return renderedChangelog{}
}

func parseRenderedChangelogHeading(heading string, parsedChangelog *renderedChangelog) {
	rest := heading
	if strings.HasPrefix(rest, "[") {
		tag, compareURL, remainingHeading, ok := parseLinkedChangelogHeading(rest)
		if ok {
			parsedChangelog.Tag = tag
			parsedChangelog.CompareURL = compareURL
			rest = remainingHeading
		}
	} else {
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			parsedChangelog.Tag = fields[0]
			rest = strings.TrimSpace(strings.TrimPrefix(rest, parsedChangelog.Tag))
		}
	}

	if strings.HasPrefix(rest, "(") && strings.HasSuffix(rest, ")") {
		parsedChangelog.Date = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(rest, "("), ")"))
	}
}

func parseLinkedChangelogHeading(heading string) (string, string, string, bool) {
	tagEndIdx := strings.Index(heading, "]")
	if tagEndIdx <= 1 {
		return "", "", heading, false
	}

	tag := heading[1:tagEndIdx]
	linkPortion := heading[tagEndIdx+1:]

	afterOpen, found := strings.CutPrefix(linkPortion, "(")
	if !found {
		return "", "", heading, false
	}

	compareURL, remainingHeading, found := strings.Cut(afterOpen, ")")
	if !found {
		return "", "", heading, false
	}

	return tag, compareURL, strings.TrimSpace(remainingHeading), true
}

func splitDerivedChangelogBody(changelogBody string, includedTargets []string) (string, map[string]string) {
	childBodies := make(map[string]string, len(includedTargets))
	if len(includedTargets) == 0 {
		return strings.TrimSpace(changelogBody), childBodies
	}

	childHeaders := make(map[string]string, len(includedTargets))
	for _, includedTargetID := range includedTargets {
		childHeaders[includedTargetID] = "### " + includedTargetID
	}

	lines := strings.Split(strings.ReplaceAll(changelogBody, "\r\n", "\n"), "\n")
	sections := make([]struct {
		TargetID string
		Start    int
	}, 0, len(includedTargets))

	for idx, line := range lines {
		for includedTargetID, header := range childHeaders {
			if strings.TrimSpace(line) == header {
				sections = append(sections, struct {
					TargetID string
					Start    int
				}{TargetID: includedTargetID, Start: idx})
			}
		}
	}

	if len(sections) == 0 {
		return strings.TrimSpace(changelogBody), childBodies
	}

	directBody := strings.TrimSpace(strings.Join(lines[:sections[0].Start], "\n"))
	for idx, section := range sections {
		end := len(lines)
		if idx+1 < len(sections) {
			end = sections[idx+1].Start
		}

		childBodies[section.TargetID] = strings.TrimSpace(strings.Join(lines[section.Start+1:end], "\n"))
	}

	return directBody, childBodies
}

func appendMarkdownBlock(body *strings.Builder, markdown string) {
	trimmedMarkdown := strings.TrimSpace(markdown)
	if trimmedMarkdown == "" {
		return
	}

	body.WriteString("\n\n")
	body.WriteString(trimmedMarkdown)
}

func releaseNotesFromPullRequest(pullRequest *provider.PullRequest) string {
	if pullRequest == nil {
		return ""
	}

	return extractReleaseNotesBlock(pullRequest.Body)
}

func extractReleaseNotesBlock(body string) string {
	_, afterStart, found := strings.Cut(body, releaseNotesStartMarker)
	if !found {
		return ""
	}

	notes, _, found := strings.Cut(afterStart, releaseNotesEndMarker)
	if !found {
		return ""
	}

	return strings.TrimSpace(notes)
}

func applyReleaseNotesToResult(result *Result) {
	if result == nil {
		return
	}

	notes := strings.TrimSpace(result.ReleaseNotes)
	if notes == "" {
		return
	}

	for idx := range result.Plans {
		result.Plans[idx].Changelog = insertReleaseNotes(result.Plans[idx].Changelog, notes)
	}
}

func insertReleaseNotes(changelogBody, notes string) string {
	trimmedNotes := strings.TrimSpace(notes)
	if trimmedNotes == "" || strings.Contains(changelogBody, trimmedNotes) {
		return changelogBody
	}

	return insertMarkdownAfterFirstHeading(changelogBody, trimmedNotes)
}

func insertReleaseNotesBlock(changelogBody, notes string) string {
	return insertMarkdownAfterFirstHeading(changelogBody, renderReleaseNotesBlock(notes))
}

func renderReleaseNotesBlock(notes string) string {
	trimmedNotes := strings.TrimSpace(notes)
	if trimmedNotes == "" {
		return releaseNotesStartMarker + "\n\n" + releaseNotesEndMarker
	}

	return releaseNotesStartMarker + "\n" + trimmedNotes + "\n" + releaseNotesEndMarker
}

func insertMarkdownAfterFirstHeading(markdown, insertion string) string {
	trimmedInsertion := strings.TrimSpace(insertion)
	if trimmedInsertion == "" {
		return markdown
	}

	normalizedMarkdown := strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(normalizedMarkdown, "\n")

	for idx, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		if !strings.HasPrefix(strings.TrimSpace(line), "## ") {
			break
		}

		heading := strings.TrimRight(strings.Join(lines[:idx+1], "\n"), "\n")

		rest := strings.TrimLeft(strings.Join(lines[idx+1:], "\n"), "\n")
		if rest == "" {
			return heading + "\n\n" + trimmedInsertion + "\n"
		}

		return heading + "\n\n" + trimmedInsertion + "\n\n" + rest
	}

	trimmedMarkdown := strings.TrimSpace(normalizedMarkdown)
	if trimmedMarkdown == "" {
		return trimmedInsertion + "\n"
	}

	return trimmedInsertion + "\n\n" + trimmedMarkdown
}

func (r *Releaser) releasePRBody(changelogBody, manifestMarker string) string {
	parts := make([]string, 0)

	if header := strings.TrimSpace(r.cfg.Release.PRBodyHeader); header != "" {
		parts = append(parts, header)
	}

	if body := strings.TrimSpace(changelogBody); body != "" {
		parts = append(parts, body)
	}

	if marker := strings.TrimSpace(manifestMarker); marker != "" {
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
