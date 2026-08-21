package changelog_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
)

func sectionHeadings(sections []changelog.Section) []string {
	headings := make([]string, 0, len(sections))
	for _, section := range sections {
		headings = append(headings, section.Heading)
	}

	return headings
}

func generatedEntry(sections ...changelog.Section) changelog.Entry {
	return changelog.Entry{
		Version:       "v1.2.4",
		Sections:      sections,
		OwnedHeadings: []string{"⚠ BREAKING CHANGES", "Features", "Bug Fixes", "Documentation"},
	}
}

func TestMerge(t *testing.T) {
	t.Parallel()

	t.Run("skips a regenerated section", func(t *testing.T) {
		t.Parallel()

		// given: an existing entry whose only section was regenerated with different lines
		generated := generatedEntry(changelog.Section{
			Heading: "Bug Fixes",
			Lines:   []string{"- patch issue (abc1234)"},
		})
		foreign := changelog.Entry{Sections: []changelog.Section{{
			Heading: "Bug Fixes",
			Lines:   []string{"- custom rewrite of the generated note (abc1234)"},
		}}}

		// when: merging
		merged := changelog.Merge(generated, foreign)

		// then: the regenerated section stays authoritative
		testastic.SliceEqual(t, []string{"Bug Fixes"}, sectionHeadings(merged.Sections))
		testastic.SliceEqual(t, []string{"- patch issue (abc1234)"}, merged.Sections[0].Lines)
	})

	t.Run("skips a regenerated section with optional closing hashes", func(t *testing.T) {
		t.Parallel()

		// given: an existing generated section written with a valid closing hash sequence
		generated := generatedEntry(changelog.Section{
			Heading: "Bug Fixes",
			Lines:   []string{"- patch issue (abc1234)"},
		})
		foreign := changelog.ParseEntry("## v1.2.4 (2026-03-01)\n\n" +
			"### Bug Fixes ###\n\n- stale edited note (abc1234)\n")

		// when: merging
		merged := changelog.Merge(generated, foreign)

		// then: the equivalent owned heading is regenerated without a stale duplicate
		testastic.SliceEqual(t, []string{"Bug Fixes"}, sectionHeadings(merged.Sections))
		testastic.SliceEqual(t, []string{"- patch issue (abc1234)"}, merged.Sections[0].Lines)
	})

	t.Run("skips an owned section absent from this entry", func(t *testing.T) {
		t.Parallel()

		// given: an existing entry carrying a section the generator owns but did not emit
		generated := generatedEntry(changelog.Section{
			Heading: "Bug Fixes",
			Lines:   []string{"- patch issue (abc1234)"},
		})
		foreign := changelog.Entry{Sections: []changelog.Section{
			{Heading: "Features", Lines: []string{"- add tokens (def5678)"}},
			{Heading: "Bug Fixes", Lines: []string{"- patch issue (abc1234)"}},
		}}

		// when: merging
		merged := changelog.Merge(generated, foreign)

		// then: the dropped section is not inherited back
		testastic.SliceEqual(t, []string{"Bug Fixes"}, sectionHeadings(merged.Sections))
	})

	t.Run("skips a section whose lines are already present", func(t *testing.T) {
		t.Parallel()

		// given: an existing section under an unknown heading repeating generated lines
		generated := generatedEntry(changelog.Section{
			Heading: "Bug Fixes",
			Lines:   []string{"- patch issue (abc1234)"},
		})
		foreign := changelog.Entry{Sections: []changelog.Section{{
			Heading: "Notes",
			Lines:   []string{"- patch issue (abc1234)"},
		}}}

		// when: merging
		merged := changelog.Merge(generated, foreign)

		// then: nothing is duplicated
		testastic.SliceEqual(t, []string{"Bug Fixes"}, sectionHeadings(merged.Sections))
	})

	t.Run("preserves an unknown section at the end", func(t *testing.T) {
		t.Parallel()

		// given: a hand-written section following every generated one
		generated := generatedEntry(changelog.Section{
			Heading: "Bug Fixes",
			Lines:   []string{"- patch issue (abc1234)"},
		})
		foreign := changelog.Entry{Sections: []changelog.Section{
			{Heading: "Bug Fixes", Lines: []string{"- patch issue (abc1234)"}},
			{Heading: "Migration Notes", Lines: []string{"Run database migrations."}},
		}}

		// when: merging
		merged := changelog.Merge(generated, foreign)

		// then: the human addition survives after the generated sections
		testastic.SliceEqual(t, []string{"Bug Fixes", "Migration Notes"}, sectionHeadings(merged.Sections))
		testastic.SliceEqual(t, []string{"Run database migrations."}, merged.Sections[1].Lines)
	})

	t.Run("preserves human sections before, between and after generated sections", func(t *testing.T) {
		t.Parallel()

		// given: manual sections interleaved with the sections this run regenerated
		generated := generatedEntry(
			changelog.Section{Heading: "Features", Lines: []string{"- add token refresh (def5678)"}},
			changelog.Section{Heading: "Bug Fixes", Lines: []string{"- patch expiry handling (fed4321)"}},
		)
		foreign := changelog.Entry{Sections: []changelog.Section{
			{Heading: "Migration Notes", Lines: []string{"Rotate existing sessions."}},
			{Heading: "Features", Lines: []string{"- add token refresh (def5678)"}},
			{Heading: "Rollback Notes", Lines: []string{"Restore the previous session key."}},
			{Heading: "Bug Fixes", Lines: []string{"- patch expiry handling (fed4321)"}},
			{Heading: "Follow-up", Lines: []string{"Schedule the cleanup job."}},
		}}

		// when: merging
		merged := changelog.Merge(generated, foreign)

		// then: each manual section keeps its position relative to the next generated one
		testastic.SliceEqual(
			t,
			[]string{"Migration Notes", "Features", "Rollback Notes", "Bug Fixes", "Follow-up"},
			sectionHeadings(merged.Sections),
		)
	})

	t.Run("preserves intro and moves a blank-line-delimited outro to the end", func(t *testing.T) {
		t.Parallel()

		// given: generated and existing entries with freeform edges
		generated := generatedEntry(
			changelog.Section{Heading: "Features", Lines: []string{"- add token refresh (def5678)"}},
			changelog.Section{Heading: "Bug Fixes", Lines: []string{"- patch expiry handling (fed4321)"}},
		)
		foreign := changelog.ParseEntry("## v1.2.4 (2026-03-01)\n\n" +
			"Validate configuration before upgrading.\n\n" +
			"### Features\n\n- add token refresh (def5678)\n\n" +
			"Existing configurations that violate the schema stop before release planning.\n")

		// when: merging them
		merged := changelog.Merge(generated, foreign)

		// then: intro stays first and trailing prose moves last
		testastic.Equal(
			t,
			"Validate configuration before upgrading.\n\n"+
				"### Features\n\n- add token refresh (def5678)\n\n"+
				"### Bug Fixes\n\n- patch expiry handling (fed4321)\n\n"+
				"Existing configurations that violate the schema stop before release planning.\n",
			changelog.RenderBody(merged),
		)
	})

	t.Run("preserves an outro after a stale owned section", func(t *testing.T) {
		t.Parallel()

		// given: a stale owned section followed by freeform prose
		generated := generatedEntry(changelog.Section{
			Heading: "Bug Fixes",
			Lines:   []string{"- patch issue (abc1234)"},
		})
		foreign := changelog.ParseEntry("## v1.2.4 (2026-03-01)\n\n" +
			"### Features\n\n- stale feature (def5678)\n\n" +
			"Review the new configuration before upgrading.\n")

		// when: merging with current sections
		merged := changelog.Merge(generated, foreign)

		// then: the stale section drops while the prose remains
		testastic.SliceEqual(t, []string{"Bug Fixes"}, sectionHeadings(merged.Sections))
		testastic.SliceEqual(t, []string{"Review the new configuration before upgrading."}, merged.Outro)
	})

	t.Run("keeps multiple paragraphs in a final manual section", func(t *testing.T) {
		t.Parallel()

		// given: a final manual section with two paragraphs
		generated := generatedEntry(changelog.Section{
			Heading: "Bug Fixes",
			Lines:   []string{"- patch issue (abc1234)"},
		})
		foreign := changelog.ParseEntry("## v1.2.4 (2026-03-01)\n\n" +
			"### Bug Fixes\n\n- patch issue (abc1234)\n\n" +
			"### Migration Notes\n\nRun database migrations.\n\nThen restart workers.\n")

		// when: merging entries
		merged := changelog.Merge(generated, foreign)

		// then: both paragraphs remain in the manual section
		testastic.SliceEqual(t, []string{"Bug Fixes", "Migration Notes"}, sectionHeadings(merged.Sections))
		testastic.SliceEqual(
			t,
			[]string{"Run database migrations.", "", "Then restart workers."},
			merged.Sections[1].Lines,
		)
		testastic.Equal(t, 0, len(merged.Outro))
	})

	t.Run("drops trailing prose without a blank-line boundary", func(t *testing.T) {
		t.Parallel()

		// given: prose directly attached to an owned section
		generated := generatedEntry(changelog.Section{
			Heading: "Bug Fixes",
			Lines:   []string{"- patch issue (abc1234)"},
		})
		foreign := changelog.ParseEntry("## v1.2.4 (2026-03-01)\n\n" +
			"### Bug Fixes\n\n- patch issue (abc1234)\nThis has no separating blank line.\n")

		// when: merging with regenerated content
		merged := changelog.Merge(generated, foreign)

		// then: attached stale prose drops with that section
		testastic.NotContains(t, changelog.Render(merged), "This has no separating blank line")
	})

	t.Run("returns the generated entry untouched when nothing is preserved", func(t *testing.T) {
		t.Parallel()

		// given: a derived entry whose nesting must survive a merge with nothing to carry
		generated := generatedEntry(changelog.Section{
			Heading:  "api",
			Sections: []changelog.Section{{Heading: "Features", Lines: []string{"- add tokens (abc1234)"}}},
		})
		generated.OwnedHeadings = append(generated.OwnedHeadings, "api")

		foreign := changelog.ParseEntry(changelog.Render(generated))

		// when: merging
		merged := changelog.Merge(generated, foreign)

		// then: the nested shape is returned as-is
		testastic.Equal(t, changelog.Render(generated), changelog.Render(merged))
	})

	t.Run("flattens child targets when a human section is preserved", func(t *testing.T) {
		t.Parallel()

		// given: a derived entry and a hand-written section on the release branch
		generated := generatedEntry(changelog.Section{
			Heading:  "api",
			Sections: []changelog.Section{{Heading: "Features", Lines: []string{"- add tokens (abc1234)"}}},
		})
		generated.OwnedHeadings = append(generated.OwnedHeadings, "api")

		foreign := changelog.Entry{Sections: []changelog.Section{
			{Heading: "api"},
			{Heading: "Features", Lines: []string{"- add tokens (abc1234)"}},
			{Heading: "Migration Notes", Lines: []string{"Run database migrations."}},
		}}

		// when: merging
		merged := changelog.Merge(generated, foreign)

		// then: the child target and its section sit side by side with the human addition
		testastic.SliceEqual(t, []string{"api", "Features", "Migration Notes"}, sectionHeadings(merged.Sections))
	})
}
