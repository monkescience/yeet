package changelog_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
)

func TestParseEntry(t *testing.T) {
	t.Parallel()

	t.Run("reads a linked heading", func(t *testing.T) {
		t.Parallel()

		// given: an entry whose version heading links to a compare URL
		text := "## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-21)\n\n" +
			"### Features\n\n- add tokens (abc1234)\n"

		// when: parsing the entry
		entry := changelog.ParseEntry(text)

		// then: version, compare URL and date are recovered
		testastic.Equal(t, "v1.2.3", entry.Version)
		testastic.Equal(t, "https://example.com/compare/v1.2.2...v1.2.3", entry.CompareURL)
		testastic.Equal(t, "2026-03-21", entry.Date.Format("2006-01-02"))
	})

	t.Run("reads a plain heading", func(t *testing.T) {
		t.Parallel()

		// given: an entry whose version heading carries no link
		text := "## v1.2.3 (2026-03-21)\n\n### Bug Fixes\n\n- patch issue (abc1234)\n"

		// when: parsing the entry
		entry := changelog.ParseEntry(text)

		// then: the version is read from the bare heading
		testastic.Equal(t, "v1.2.3", entry.Version)
		testastic.Equal(t, "", entry.CompareURL)
		testastic.Equal(t, "2026-03-21", entry.Date.Format("2006-01-02"))
	})

	t.Run("reads a CommonMark indented heading", func(t *testing.T) {
		t.Parallel()

		// given: an imported changelog whose headings carry leading indentation
		text := "   ## v1.2.3 (2026-03-21)\n\n   ### Features\n\n- add tokens (abc1234)\n"

		// when: parsing the entry
		entry := changelog.ParseEntry(text)

		// then: the indentation does not hide the heading
		testastic.Equal(t, "v1.2.3", entry.Version)
		testastic.Equal(t, 1, len(entry.Sections))
		testastic.Equal(t, "Features", entry.Sections[0].Heading)
	})

	t.Run("round-trips a rendered entry", func(t *testing.T) {
		t.Parallel()

		// given: an entry rendered by this package
		rendered := changelog.Render(changelog.Entry{
			Version:    "v1.2.3",
			CompareURL: "https://example.com/compare/v1.2.2...v1.2.3",
			Sections: []changelog.Section{
				{Heading: "Features", Lines: []string{"- add tokens (abc1234)"}},
				{Heading: "Bug Fixes", Lines: []string{"- patch issue (def5678)"}},
			},
		})

		// when: parsing and rendering it again
		reparsed := changelog.Render(changelog.ParseEntry(rendered))

		// then: the bytes are unchanged
		testastic.Equal(t, rendered, reparsed)
	})

	t.Run("keeps blank lines inside a section", func(t *testing.T) {
		t.Parallel()

		// given: a hand-written section holding two paragraphs
		text := "## v1.2.3 (2026-03-21)\n\n### Migration Notes\n\nRun migrations.\n\nThen restart workers.\n"

		// when: parsing the entry
		entry := changelog.ParseEntry(text)

		// then: the paragraph break survives as a blank line
		testastic.Equal(t, 1, len(entry.Sections))
		testastic.SliceEqual(t, []string{"Run migrations.", "", "Then restart workers."}, entry.Sections[0].Lines)
	})

	t.Run("drops freeform text under the version heading", func(t *testing.T) {
		t.Parallel()

		// given: an entry with a note written directly under the version heading
		text := "## v1.2.3 (2026-03-21)\n\nA heads-up note.\n\n### Bug Fixes\n\n- patch issue (abc1234)\n"

		// when: parsing the entry
		entry := changelog.ParseEntry(text)

		// then: only level-3 sections are recovered
		testastic.Equal(t, 1, len(entry.Sections))
		testastic.Equal(t, "Bug Fixes", entry.Sections[0].Heading)
	})

	t.Run("reads sections from text with no version heading", func(t *testing.T) {
		t.Parallel()

		// given: a body with no version heading at all
		text := "### Features\n\n- add tokens (abc1234)\n"

		// when: parsing the entry
		entry := changelog.ParseEntry(text)

		// then: the sections are still recovered and the heading fields stay empty
		testastic.Equal(t, "", entry.Version)
		testastic.Equal(t, 1, len(entry.Sections))
		testastic.Equal(t, "Features", entry.Sections[0].Heading)
	})
}

func TestOwnedHeadings(t *testing.T) {
	t.Parallel()

	t.Run("includes a heading configured in sections but absent from include", func(t *testing.T) {
		t.Parallel()

		// given: a generator mapping a commit type it does not include
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features", "perf": "Performance"}),
			changelog.WithInclude([]string{"feat"}),
		)

		// when: generating an entry that emits no performance section
		entry := gen.Generate(t.Context(), "v1.0.0", "", nil)

		// then: the unemitted heading is still owned
		testastic.SliceContains(t, entry.OwnedHeadings, "Performance")
		testastic.SliceContains(t, entry.OwnedHeadings, "Features")
		testastic.SliceContains(t, entry.OwnedHeadings, "⚠ BREAKING CHANGES")
	})

	t.Run("falls back to the capitalized commit type", func(t *testing.T) {
		t.Parallel()

		// given: a generator including a commit type with no section mapping
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat", "perf"}),
		)

		// when: generating an entry
		entry := gen.Generate(t.Context(), "v1.0.0", "", nil)

		// then: the fallback heading is owned
		testastic.SliceContains(t, entry.OwnedHeadings, "Perf")
	})
}
