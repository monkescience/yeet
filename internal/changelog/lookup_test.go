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
