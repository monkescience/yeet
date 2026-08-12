package release

import (
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
)

const changelogDateLayout = "2006-01-02"

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

		entry := preferredPREntry(plan)
		entry.Sections = changelog.DirectSections(entry.Sections, plan.IncludedTargets)

		sections = append(sections, prSection{
			id:   plan.ID,
			plan: plan,
			body: changelog.RenderBody(entry),
		})
	}

	for _, plan := range plans {
		if plan.Type == config.TargetTypeDerived {
			continue
		}

		entry := preferredPREntry(plan)
		sections = append(sections, prSection{
			id:   plan.ID,
			plan: plan,
			body: changelog.RenderBody(entry),
		})
	}

	return sections
}

func renderFlatPRSection(section prSection) string {
	var body strings.Builder
	fmt.Fprintf(&body, "## %s\n\n", section.id)

	body.WriteString(renderPlanMetadata(section.plan, preferredPREntry(section.plan)))
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

func preferredPREntry(plan TargetPlan) changelog.Entry {
	if plan.PREntry.Version != "" {
		return plan.PREntry
	}

	return plan.Entry
}

func renderPlanMetadata(plan TargetPlan, entry changelog.Entry) string {
	var body strings.Builder

	previousTag := planPreviousTag(plan)

	nextTag := plan.NextTag

	fmt.Fprintf(&body, "Tag: `%s` -> `%s`\n", previousTag, nextTag)
	fmt.Fprintf(&body, "Bump: `%s`", plan.BumpType)

	if entry.Version != "" {
		fmt.Fprintf(&body, "\nDate: `%s`", entry.Date.Format(changelogDateLayout))
	}

	if entry.CompareURL != "" {
		fmt.Fprintf(
			&body,
			"\nCompare: [%s](%s)",
			compareRange(entry.CompareURL),
			entry.CompareURL,
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

func appendMarkdownBlock(body *strings.Builder, markdown string) {
	trimmedMarkdown := strings.TrimSpace(markdown)
	if trimmedMarkdown == "" {
		return
	}

	body.WriteString("\n\n")
	body.WriteString(trimmedMarkdown)
}
