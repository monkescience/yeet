package changelog_test

import (
	"fmt"
	"strings"
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

	t.Run("keeps indented code headings as content", func(t *testing.T) {
		t.Parallel()

		// given: a release entry containing heading-shaped lines in indented code
		text := "## v1.2.3 (2026-03-21)\n\n" +
			"    ### Migration Notes\n" +
			"\t### Deployment Notes\n\n" +
			"### Bug Fixes\n\n- patch issue (abc1234)\n"

		// when: parsing the entry
		entry := changelog.ParseEntry(text)

		// then: only the CommonMark heading starts a section
		testastic.SliceEqual(t, []string{"    ### Migration Notes", "\t### Deployment Notes"}, entry.Intro)
		testastic.Equal(t, 1, len(entry.Sections))
		testastic.Equal(t, "Bug Fixes", entry.Sections[0].Heading)
	})

	t.Run("does not read an indented code line as the entry heading", func(t *testing.T) {
		t.Parallel()

		// given: heading-shaped indented code before a real section
		text := "    ## v1.2.3 (2026-03-21)\n\n### Bug Fixes\n\n- patch issue (abc1234)\n"

		// when: parsing the text
		entry := changelog.ParseEntry(text)

		// then: the code stays in the intro and no release version is inferred
		testastic.Equal(t, "", entry.Version)
		testastic.SliceEqual(t, []string{"    ## v1.2.3 (2026-03-21)"}, entry.Intro)
		testastic.Equal(t, 1, len(entry.Sections))
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

	t.Run("normalizes optional closing hashes in section headings", func(t *testing.T) {
		t.Parallel()

		// given: section headings with closing hashes and literal trailing hashes
		text := "## v1.2.3 (2026-03-21)\n\n" +
			"### Bug Fixes ###   \n\n- patch issue (abc1234)\n\n" +
			"### C# Integration\n\n- support C# clients (def5678)\n\n" +
			"### Release###\n\n- keep attached hashes literal (fed4321)\n"

		// when: parsing the entry
		entry := changelog.ParseEntry(text)

		// then: only the whitespace-delimited closing sequence is removed
		testastic.SliceEqual(t, []string{"Bug Fixes", "C# Integration", "Release###"}, sectionHeadings(entry.Sections))
	})

	t.Run("round-trips a freeform intro containing fenced headings", func(t *testing.T) {
		t.Parallel()

		// given: an intro containing fenced heading-shaped lines
		text := "## v1.2.3 (2026-03-21)\n\nA heads-up note.\n\n" +
			"```markdown\n### This is sample text\n```\n\n" +
			"~~~markdown\n### This is another sample\n~~~~\n\n" +
			"### Bug Fixes\n\n- patch issue (abc1234)\n"

		// when: parsing and rendering the entry
		entry := changelog.ParseEntry(text)
		rendered := changelog.Render(entry)

		// then: fenced headings remain and the bytes round-trip
		testastic.Equal(t, text, rendered)
		testastic.Equal(t, 1, len(entry.Sections))
		testastic.Equal(t, "Bug Fixes", entry.Sections[0].Heading)
	})

	t.Run("round-trips headingless content when no sections exist", func(t *testing.T) {
		t.Parallel()

		// given: a versioned entry with no sections
		text := "## v1.2.3 (2026-03-21)\n\nA short release summary.\n"

		// when: parsing and rendering the entry
		rendered := changelog.Render(changelog.ParseEntry(text))

		// then: the freeform body remains intact
		testastic.Equal(t, text, rendered)
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

	t.Run("ignores headings nested in blockquotes and lists", func(t *testing.T) {
		t.Parallel()

		// given: an entry with heading-shaped content inside Markdown containers
		text := "## v1.2.3 (2026-03-21)\n\n" +
			"> ### Quoted Notes\n> Keep quoted.\n\n" +
			"- item\n  ### Listed Notes\n  Keep listed.\n\n" +
			"### Bug Fixes\n\n- patch issue (abc1234)\n"

		// when: parsing the entry
		entry := changelog.ParseEntry(text)

		// then: only the document-level heading starts a section
		testastic.SliceEqual(
			t,
			[]string{
				"> ### Quoted Notes",
				"> Keep quoted.",
				"",
				"- item",
				"  ### Listed Notes",
				"  Keep listed.",
			},
			entry.Intro,
		)
		testastic.Equal(t, 1, len(entry.Sections))
		testastic.Equal(t, "Bug Fixes", entry.Sections[0].Heading)
	})

	t.Run("recognizes zero through three leading spaces", func(t *testing.T) {
		t.Parallel()

		for indent := range 4 {
			t.Run(fmt.Sprintf("indent %d", indent), func(t *testing.T) {
				t.Parallel()

				// given: an ATX entry and section heading with valid CommonMark indentation
				spaces := strings.Repeat(" ", indent)
				text := spaces + "## v1.2.3 (2026-03-21)\n\n" +
					spaces + "### Café Notes\n\n- preserve Unicode\n"

				// when: parsing the entry
				entry := changelog.ParseEntry(text)

				// then: both headings remain structural and their text stays exact
				testastic.Equal(t, "v1.2.3", entry.Version)
				testastic.Equal(t, 1, len(entry.Sections))
				testastic.Equal(t, "Café Notes", entry.Sections[0].Heading)
			})
		}
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

	t.Run("keeps the legacy breaking heading owned after customization", func(t *testing.T) {
		t.Parallel()

		// given: a generator with a customized breaking changes heading
		gen := changelog.New(
			changelog.WithSections(map[string]string{"breaking": "Compatibility Notes"}),
		)

		// when: collecting every heading owned by the generator
		headings := gen.OwnedHeadings()

		// then: refreshed changelogs replace both the configured and legacy headings
		testastic.SliceContains(t, headings, "Compatibility Notes")
		testastic.SliceContains(t, headings, "⚠ BREAKING CHANGES")
	})
}
