package changelog_test

import (
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	t.Run("generates changelog with sections", func(t *testing.T) {
		t.Parallel()

		// given: a generator and some commits
		gen := &changelog.Generator{
			Sections: map[string]string{
				"feat": "Features",
				"fix":  "Bug Fixes",
			},
			Include: []string{"feat", "fix"},
		}

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Scope: "auth", Description: "add OAuth2 support"},
			{Hash: "def5678901", Type: "fix", Description: "resolve null pointer"},
			{Hash: "ghi9012345", Type: "chore", Description: "update deps"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.2.0", commits)

		// then: sections are present with correct commits
		testastic.Equal(t, "v1.2.0", entry.Version)
		testastic.True(t, strings.Contains(entry.Body, "### Features"))
		testastic.True(t, strings.Contains(entry.Body, "### Bug Fixes"))
		testastic.True(t, strings.Contains(entry.Body, "**auth**: add OAuth2 support"))
		testastic.True(t, strings.Contains(entry.Body, "resolve null pointer"))
		testastic.False(t, strings.Contains(entry.Body, "update deps"))
	})

	t.Run("includes breaking changes section", func(t *testing.T) {
		t.Parallel()

		// given: commits with a breaking change
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
		}

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "new API", Breaking: true,
				Footers: []commit.Footer{{Key: "BREAKING CHANGE", Value: "old endpoints removed"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate("v2.0.0", commits)

		// then: breaking changes section is present
		testastic.True(t, strings.Contains(entry.Body, "### Breaking Changes"))
		testastic.True(t, strings.Contains(entry.Body, "old endpoints removed"))
	})

	t.Run("short hash in output", func(t *testing.T) {
		t.Parallel()

		// given: a commit with a long hash
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
		}

		commits := []commit.Commit{
			{Hash: "abc1234567890def", Type: "feat", Description: "something new"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", commits)

		// then: hash is truncated to 7 chars
		testastic.True(t, strings.Contains(entry.Body, "abc1234"))
		testastic.False(t, strings.Contains(entry.Body, "abc1234567890def"))
	})

	t.Run("empty commits", func(t *testing.T) {
		t.Parallel()

		// given: no commits
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", nil)

		// then: body is empty
		testastic.Equal(t, "", entry.Body)
	})
}

func TestRender(t *testing.T) {
	t.Parallel()

	t.Run("renders entry as markdown", func(t *testing.T) {
		t.Parallel()

		// given: a changelog entry
		entry := changelog.Entry{
			Version: "v1.2.0",
			Body:    "### Features\n\n- something new (abc1234)\n",
		}

		// when: rendering
		output := changelog.Render(entry)

		// then: output has version header and body
		testastic.True(t, strings.HasPrefix(output, "## v1.2.0"))
		testastic.True(t, strings.Contains(output, "### Features"))
	})
}

func TestPrepend(t *testing.T) {
	t.Parallel()

	t.Run("prepend to empty changelog", func(t *testing.T) {
		t.Parallel()

		// given: no existing changelog
		newEntry := "## v1.0.0 (2026-02-28)\n\n### Features\n\n- initial release\n"

		// when: prepending
		result := changelog.Prepend("", newEntry)

		// then: header is added
		testastic.True(t, strings.HasPrefix(result, "# Changelog"))
		testastic.True(t, strings.Contains(result, newEntry))
	})

	t.Run("prepend to existing changelog", func(t *testing.T) {
		t.Parallel()

		// given: an existing changelog
		existing := "# Changelog\n\n## v1.0.0 (2026-01-01)\n\n- old stuff\n"
		newEntry := "## v1.1.0 (2026-02-28)\n\n### Features\n\n- new stuff\n"

		// when: prepending
		result := changelog.Prepend(existing, newEntry)

		// then: new entry is before old entry
		newIdx := strings.Index(result, "v1.1.0")
		oldIdx := strings.Index(result, "v1.0.0")

		testastic.True(t, newIdx < oldIdx)
	})
}
