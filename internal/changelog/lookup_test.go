package changelog_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
)

func TestEntryByTagIgnoresFencedHeadings(t *testing.T) {
	t.Parallel()

	t.Run("keeps a fenced heading in the matching entry", func(t *testing.T) {
		t.Parallel()

		// given: a release entry containing a level-two heading in a code fence
		document := "# Changelog\n\n" +
			"## v1.2.3 (2026-08-10)\n\n" +
			"Before the example.\n\n" +
			"```markdown\n## Example\n```\n\n" +
			"After the example.\n\n" +
			"## v1.2.2 (2026-08-09)\n\nPrevious release.\n"

		// when: extracting the release entry
		entry, err := changelog.EntryByTag(document, "v1.2.3")

		// then: the fenced heading does not truncate the entry
		testastic.NoError(t, err)
		testastic.Equal(
			t,
			"## v1.2.3 (2026-08-10)\n\nBefore the example.\n\n```markdown\n## Example\n```\n\nAfter the example.",
			entry,
		)
	})

	t.Run("skips a matching tag in a preamble code fence", func(t *testing.T) {
		t.Parallel()

		// given: a preamble example containing the requested tag before the real release
		document := "# Changelog\n\n" +
			"```markdown\n## v1.2.3 (example)\nNot release notes.\n```\n\n" +
			"## v1.2.3 (2026-08-10)\n\nReal release notes.\n"

		// when: extracting the release entry
		entry, err := changelog.EntryByTag(document, "v1.2.3")

		// then: the real release is selected
		testastic.NoError(t, err)
		testastic.Equal(t, "## v1.2.3 (2026-08-10)\n\nReal release notes.", entry)
	})
}

func TestEntryByTagIgnoresIndentedCodeHeadings(t *testing.T) {
	t.Parallel()

	// given: a changelog containing matching and boundary-shaped lines in indented code
	document := "# Changelog\n\n" +
		"    ## v1.2.3 (example)\n" +
		"\t## v1.2.3 (tabbed example)\n\n" +
		"## v1.2.3 (2026-08-10)\n\n" +
		"Before the example.\n\n" +
		"    ## Example\n" +
		"\t## Tabbed example\n\n" +
		"After the example.\n\n" +
		"## v1.2.2 (2026-08-09)\n\nPrevious release.\n"

	// when: extracting the release entry
	entry, err := changelog.EntryByTag(document, "v1.2.3")

	// then: indented code neither matches nor truncates the release
	testastic.NoError(t, err)
	testastic.Equal(
		t,
		"## v1.2.3 (2026-08-10)\n\nBefore the example.\n\n    ## Example\n\t## Tabbed example\n\nAfter the example.",
		entry,
	)
}

func TestEntryByTagIgnoresIndentedCodeFenceMarkers(t *testing.T) {
	t.Parallel()

	// given: an indented code line shaped like a fence before the requested release
	document := "# Changelog\n\n" +
		"    ```markdown\n" +
		"## v1.2.3 (2026-08-10)\n\nReal release notes.\n" +
		"    ```\n" +
		"## v1.2.2 (2026-08-09)\n\nPrevious release.\n"

	// when: extracting the requested release entry
	entry, err := changelog.EntryByTag(document, "v1.2.3")

	// then: the indented marker does not hide the top-level release heading
	testastic.NoError(t, err)
	testastic.Equal(
		t,
		"## v1.2.3 (2026-08-10)\n\nReal release notes.\n    ```",
		entry,
	)
}

func TestEntryByTagPreservesATXReleaseSemantics(t *testing.T) {
	t.Parallel()

	t.Run("does not treat a Setext heading as a release boundary", func(t *testing.T) {
		t.Parallel()

		// given: a Setext example before a real ATX release heading
		document := "# Changelog\n\n" +
			"v9.9.9\n-------\n\nNot release notes.\n\n" +
			"## v1.2.3 (2026-08-10)\n\nReal release notes.\n"

		// when: extracting the real release
		entry, err := changelog.EntryByTag(document, "v1.2.3")

		// then: only the ATX heading participates in release lookup
		testastic.NoError(t, err)
		testastic.Equal(t, "## v1.2.3 (2026-08-10)\n\nReal release notes.", entry)
	})

	t.Run("keeps raw inline markup semantics and accepts a linked tag", func(t *testing.T) {
		t.Parallel()

		// given: a bold example followed by the supported linked-tag form
		document := "# Changelog\n\n" +
			"## **v1.2.3** (example)\n\nNot the requested raw tag.\n\n" +
			"## [v1.2.3](https://example.com/compare) (2026-08-10)\n\nReal release notes.\n"

		// when: extracting the unformatted tag
		entry, err := changelog.EntryByTag(document, "v1.2.3")

		// then: arbitrary inline markup is not normalized and the linked tag matches
		testastic.NoError(t, err)
		testastic.Equal(
			t,
			"## [v1.2.3](https://example.com/compare) (2026-08-10)\n\nReal release notes.",
			entry,
		)
	})

	t.Run("normalizes CRLF while preserving raw entry formatting", func(t *testing.T) {
		t.Parallel()

		// given: a CRLF changelog with Unicode and user-authored Markdown
		document := "# Changelog\r\n\r\n" +
			"## [v1.2.3](https://example.com/compare) (2026-08-10)\r\n\r\n" +
			"<!-- keep me -->\r\n\r\n### 運用 Notes ###\r\n\r\n" +
			"Keep  double spaces and `inline code`.\r\n\r\n" +
			"## v1.2.2 (2026-08-09)\r\n\r\nPrevious release.\r\n"

		// when: extracting the requested release
		entry, err := changelog.EntryByTag(document, "v1.2.3")

		// then: only the documented newline and outer-edge normalization occurs
		testastic.NoError(t, err)
		testastic.Equal(
			t,
			"## [v1.2.3](https://example.com/compare) (2026-08-10)\n\n"+
				"<!-- keep me -->\n\n### 運用 Notes ###\n\n"+
				"Keep  double spaces and `inline code`.",
			entry,
		)
	})
}

func TestPrependIgnoresNestedReleaseHeadings(t *testing.T) {
	t.Parallel()

	// given: a preamble with headings nested in a blockquote and list
	existing := "# Changelog\n\n" +
		"> ## v0.0.0 (quoted example)\n\n" +
		"- example\n  ## v0.0.0 (listed example)\n\n" +
		"## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
	newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

	// when: prepending the new release
	result := changelog.Prepend(existing, newEntry)

	// then: the complete preamble stays before the new release
	testastic.Equal(
		t,
		"# Changelog\n\n> ## v0.0.0 (quoted example)\n\n"+
			"- example\n  ## v0.0.0 (listed example)\n\n"+
			"## v1.1.0 (2026-08-10)\n\nNew release.\n\n"+
			"## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
		result,
	)
}
