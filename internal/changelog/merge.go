package changelog

import "strings"

// Merge carries hand-written sections from a foreign entry into a freshly
// generated one. A foreign section survives when its heading is neither
// regenerated nor owned by the generator and its lines are not already in the
// generated entry. Survivors keep their position relative to the next
// generated section.
func Merge(generated, foreign Entry) Entry {
	sections := flattenSections(generated.Sections)

	manual := manualSectionsByAnchor(sections, generated.OwnedHeadings, foreign.Sections)
	if len(manual) == 0 {
		return generated
	}

	merged := generated
	merged.Sections = make([]Section, 0, len(sections)+len(foreign.Sections))

	for _, section := range sections {
		merged.Sections = append(merged.Sections, manual[section.Heading]...)
		merged.Sections = append(merged.Sections, section)
	}

	merged.Sections = append(merged.Sections, manual[""]...)

	return merged
}

// flattenSections projects nested child targets onto one level. A child target
// heading and a section heading are both level 3, so the flat form is what the
// two sides of a merge have in common.
func flattenSections(sections []Section) []Section {
	flat := make([]Section, 0, len(sections))

	for _, section := range sections {
		flat = append(flat, Section{Heading: section.Heading, Lines: section.Lines})
		flat = append(flat, flattenSections(section.Sections)...)
	}

	return flat
}

func manualSectionsByAnchor(generated []Section, owned []string, foreign []Section) map[string][]Section {
	generatedHeadings := headingSet(generated)
	ownedHeadings := stringSet(owned)
	generatedLines := lineSet(generated)

	manual := make(map[string][]Section)

	for idx, section := range foreign {
		if _, regenerated := generatedHeadings[section.Heading]; regenerated {
			continue
		}

		if _, isOwned := ownedHeadings[section.Heading]; isOwned {
			continue
		}

		if linesAlreadyPresent(section, generatedLines) {
			continue
		}

		anchor := followingGeneratedHeading(foreign[idx+1:], generatedHeadings)
		manual[anchor] = append(manual[anchor], section)
	}

	return manual
}

func followingGeneratedHeading(remaining []Section, generatedHeadings map[string]struct{}) string {
	for _, section := range remaining {
		if _, regenerated := generatedHeadings[section.Heading]; regenerated {
			return section.Heading
		}
	}

	return ""
}

func linesAlreadyPresent(section Section, generatedLines map[string]struct{}) bool {
	found := false

	for _, line := range section.Lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		if _, exists := generatedLines[line]; !exists {
			return false
		}

		found = true
	}

	return found
}

func headingSet(sections []Section) map[string]struct{} {
	headings := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		headings[section.Heading] = struct{}{}
	}

	return headings
}

func lineSet(sections []Section) map[string]struct{} {
	lines := make(map[string]struct{})

	for _, section := range sections {
		for _, line := range section.Lines {
			lines[line] = struct{}{}
		}
	}

	return lines
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}

	return set
}
