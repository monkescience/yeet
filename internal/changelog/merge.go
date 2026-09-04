package changelog

import "strings"

// Merge carries hand-written sections from a foreign entry into a freshly
// generated one. A foreign section survives when its heading is neither
// regenerated nor owned by the generator and its lines are not already in the
// generated entry. Survivors keep their position relative to the next
// generated section.
func Merge(generated, foreign Entry) Entry {
	foreign = extractOutro(generated, foreign)

	merged := generated

	merged.Intro = append([]string(nil), foreign.Intro...)
	merged.Outro = append([]string(nil), foreign.Outro...)

	sections := flattenSections(generated.Sections)

	manual := manualSectionsByAnchor(sections, generated.OwnedHeadings, foreign.Sections)
	if len(manual) == 0 {
		return merged
	}

	merged.Sections = make([]Section, 0, len(sections)+len(foreign.Sections))

	for idx, section := range sections {
		merged.Sections = append(merged.Sections, manual[idx]...)
		merged.Sections = append(merged.Sections, section)
	}

	merged.Sections = append(merged.Sections, manual[len(sections)]...)

	return merged
}

func extractOutro(generated, foreign Entry) Entry {
	if len(foreign.Outro) > 0 || len(foreign.Sections) == 0 {
		return foreign
	}

	lastIdx := len(foreign.Sections) - 1
	last := foreign.Sections[lastIdx]
	generatedHeadings := headingSet(flattenSections(generated.Sections))
	ownedHeadings := stringSet(generated.OwnedHeadings)

	_, isGenerated := generatedHeadings[last.Heading]

	_, isOwned := ownedHeadings[last.Heading]
	if !isGenerated && !isOwned {
		return foreign
	}

	sectionLines, outro, found := splitOutro(last.Lines)
	if !found {
		return foreign
	}

	sections := append([]Section(nil), foreign.Sections...)
	last.Lines = sectionLines
	sections[lastIdx] = last
	foreign.Sections = sections
	foreign.Outro = outro

	return foreign
}

func splitOutro(lines []string) ([]string, []string, bool) {
	for idx, line := range lines {
		if strings.TrimSpace(line) != "" {
			continue
		}

		sectionLines := trimBlankEdges(lines[:idx])

		outro := trimBlankEdges(lines[idx+1:])
		if len(sectionLines) == 0 || len(outro) == 0 {
			continue
		}

		return sectionLines, outro, true
	}

	return nil, nil, false
}

// flattenSections projects nested child targets onto one level. A child target
// heading and a section heading are both level 3, so the flat form is what the
// two sides of a merge have in common.
func flattenSections(sections []Section) []Section {
	return flattenSectionsFromTarget(sections, false)
}

func flattenSectionsFromTarget(sections []Section, includedTarget bool) []Section {
	flat := make([]Section, 0, len(sections))

	for _, section := range sections {
		nested := section.Sections
		section.Sections = nil
		section.includedTarget = includedTarget || section.includedTarget

		flat = append(flat, section)
		flat = append(flat, flattenSectionsFromTarget(nested, section.includedTarget)...)
	}

	return flat
}

func manualSectionsByAnchor(generated []Section, owned []string, foreign []Section) map[int][]Section {
	generatedHeadings := headingSet(generated)
	ownedHeadings := stringSet(owned)
	generatedLines := lineSet(generated)
	generatedPositions := matchGeneratedPositions(generated, foreign)

	nextAnchors := make([]int, len(foreign))
	anchor := len(generated)

	for idx := len(foreign) - 1; idx >= 0; idx-- {
		nextAnchors[idx] = anchor
		if position, matched := generatedPositions[idx]; matched {
			anchor = position
		}
	}

	manual := make(map[int][]Section)

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

		anchor := nextAnchors[idx]
		manual[anchor] = append(manual[anchor], section)
	}

	return manual
}

func matchGeneratedPositions(generated, foreign []Section) map[int]int {
	positionsByHeading := make(map[string][]int, len(generated))
	for idx, section := range generated {
		positionsByHeading[section.Heading] = append(positionsByHeading[section.Heading], idx)
	}

	seen := make(map[string]int, len(positionsByHeading))
	matches := make(map[int]int, min(len(generated), len(foreign)))

	for idx, section := range foreign {
		positions, exists := positionsByHeading[section.Heading]
		if !exists {
			continue
		}

		occurrence := seen[section.Heading]
		seen[section.Heading]++

		if occurrence < len(positions) {
			matches[idx] = positions[occurrence]
		}
	}

	return matches
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
