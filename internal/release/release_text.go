package release

import (
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/config"
)

type prSection struct {
	id   string
	plan TargetPlan
	body string
}

func buildPRSections(plans []TargetPlan) []prSection {
	sections := make([]prSection, 0, len(plans))

	for _, plan := range plans {
		if plan.Type != config.TargetTypeDerived {
			continue
		}

		parsedChangelog := parseRenderedChangelog(preferredPRChangelog(plan))
		directBody, _ := splitDerivedChangelogBody(parsedChangelog.Body, plan.IncludedTargets)

		sections = append(sections, prSection{
			id:   plan.ID,
			plan: plan,
			body: directBody,
		})
	}

	for _, plan := range plans {
		if plan.Type == config.TargetTypeDerived {
			continue
		}

		parsedChangelog := parseRenderedChangelog(preferredPRChangelog(plan))

		sections = append(sections, prSection{
			id:   plan.ID,
			plan: plan,
			body: parsedChangelog.Body,
		})
	}

	return sections
}

func renderFlatPRSection(section prSection) string {
	var body strings.Builder
	fmt.Fprintf(&body, "## %s\n\n", section.id)

	parsedChangelog := parseRenderedChangelog(preferredPRChangelog(section.plan))
	body.WriteString(renderPlanMetadata(section.plan, parsedChangelog))
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
	lines := splitLines(changelogBody)
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
	tag, linkPortion, ok := bracketTag(heading)
	if !ok {
		return "", "", heading, false
	}

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

// splitLines normalizes CRLF to LF and splits the text into lines. Every
// changelog scanner in this package works on the same line shape.
func splitLines(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

// bracketTag extracts the tag from a "[tag]..." prefix, returning the tag and
// the portion following the closing bracket. It reports false when the text
// does not open with a non-empty "[tag]".
func bracketTag(text string) (string, string, bool) {
	if !strings.HasPrefix(text, "[") {
		return "", "", false
	}

	closeIdx := strings.Index(text, "]")
	if closeIdx <= 1 {
		return "", "", false
	}

	return text[1:closeIdx], text[closeIdx+1:], true
}

func splitDerivedChangelogBody(changelogBody string, includedTargets []string) (string, map[string]string) {
	childBodies := make(map[string]string, len(includedTargets))
	if len(includedTargets) == 0 {
		return strings.TrimSpace(changelogBody), childBodies
	}

	headerToTargetID := make(map[string]string, len(includedTargets))
	for _, includedTargetID := range includedTargets {
		headerToTargetID["### "+includedTargetID] = includedTargetID
	}

	lines := splitLines(changelogBody)
	sections := make([]struct {
		TargetID string
		Start    int
	}, 0, len(includedTargets))

	for idx, line := range lines {
		if includedTargetID, ok := headerToTargetID[strings.TrimSpace(line)]; ok {
			sections = append(sections, struct {
				TargetID string
				Start    int
			}{TargetID: includedTargetID, Start: idx})
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

func appendMarkdown(markdown, insertion string) string {
	trimmedInsertion := strings.TrimSpace(insertion)
	if trimmedInsertion == "" {
		return markdown
	}

	normalizedMarkdown := strings.ReplaceAll(markdown, "\r\n", "\n")

	trimmedMarkdown := strings.TrimSpace(normalizedMarkdown)
	if trimmedMarkdown == "" {
		return trimmedInsertion + "\n"
	}

	return trimmedMarkdown + "\n\n" + trimmedInsertion
}

type changelogSection struct {
	heading   string
	body      string
	startLine int
}

// preserveManualChangelogSections carries hand-written notes from existingEntry
// into generatedEntry when a release PR is refreshed. Only level-3 (### )
// sections are preserved: freeform text directly under the ## version heading,
// or headings deeper than ###, are not considered and will be dropped. Manual
// sections retain their position relative to the next generated section. A
// heading in ownedHeadings is one the generator can emit, so an entry that omits
// it this time dropped it on purpose and must not inherit it back.
func preserveManualChangelogSections(generatedEntry, existingEntry string, ownedHeadings map[string]struct{}) string {
	generatedSections := changelogLevel3Sections(generatedEntry)

	generatedHeadings := make(map[string]struct{}, len(generatedSections))
	for _, section := range generatedSections {
		generatedHeadings[section.heading] = struct{}{}
	}

	manualSectionsBefore := manualSectionsByFollowingHeading(
		generatedEntry,
		generatedHeadings,
		ownedHeadings,
		changelogLevel3Sections(existingEntry),
	)

	if len(manualSectionsBefore) == 0 {
		return generatedEntry
	}

	updatedEntry := strings.TrimSpace(generatedEntry)
	if len(generatedSections) > 0 {
		lines := splitLines(generatedEntry)
		updatedEntry = strings.TrimSpace(strings.Join(lines[:generatedSections[0].startLine], "\n"))
	}

	for _, section := range generatedSections {
		for _, manualSection := range manualSectionsBefore[section.heading] {
			updatedEntry = appendMarkdown(updatedEntry, manualSection)
		}

		updatedEntry = appendMarkdown(updatedEntry, section.body)
	}

	for _, manualSection := range manualSectionsBefore[""] {
		updatedEntry = appendMarkdown(updatedEntry, manualSection)
	}

	return updatedEntry
}

func manualSectionsByFollowingHeading(
	generatedEntry string,
	generatedHeadings, ownedHeadings map[string]struct{},
	existingSections []changelogSection,
) map[string][]string {
	manualSectionsBefore := make(map[string][]string)

	for idx, section := range existingSections {
		if _, generated := generatedHeadings[section.heading]; generated {
			continue
		}

		if _, owned := ownedHeadings[section.heading]; owned {
			continue
		}

		if strings.Contains(generatedEntry, section.body) {
			continue
		}

		followingHeading := ""

		for _, followingSection := range existingSections[idx+1:] {
			if _, generated := generatedHeadings[followingSection.heading]; generated {
				followingHeading = followingSection.heading

				break
			}
		}

		manualSectionsBefore[followingHeading] = append(manualSectionsBefore[followingHeading], section.body)
	}

	return manualSectionsBefore
}

func changelogLevel3Sections(markdown string) []changelogSection {
	lines := splitLines(markdown)
	starts := make([]int, 0)

	for idx, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "### ") {
			starts = append(starts, idx)
		}
	}

	sections := make([]changelogSection, 0, len(starts))
	for idx, start := range starts {
		end := len(lines)
		if idx+1 < len(starts) {
			end = starts[idx+1]
		}

		body := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		if body == "" {
			continue
		}

		sections = append(sections, changelogSection{
			heading:   strings.TrimSpace(lines[start]),
			body:      body,
			startLine: start,
		})
	}

	return sections
}

func changelogEntryByTag(changelogBody, tag string) (string, error) {
	lines := splitLines(changelogBody)

	start := -1

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "## ") {
			continue
		}

		entryTag, ok := headingTag(trimmed)
		if !ok {
			continue
		}

		if entryTag == tag {
			start = idx

			break
		}
	}

	if start == -1 {
		return "", fmt.Errorf("%w: %s", ErrChangelogEntryNotFound, tag)
	}

	end := len(lines)

	for idx := start + 1; idx < len(lines); idx++ {
		if strings.HasPrefix(strings.TrimSpace(lines[idx]), "## ") {
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
		tag, _, ok := bracketTag(rest)

		return tag, ok
	}

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}

	return fields[0], true
}
