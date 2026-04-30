package changelog_test

import (
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
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
		entry := gen.Generate("v1.2.0", "", commits)

		// then: sections are present with correct commits
		testastic.Equal(t, "v1.2.0", entry.Version)
		testastic.Contains(t, entry.Body, "### Features")
		testastic.Contains(t, entry.Body, "### Bug Fixes")
		testastic.Contains(t, entry.Body, "**auth:** add OAuth2 support")
		testastic.Contains(t, entry.Body, "resolve null pointer")
		testastic.NotContains(t, entry.Body, "update deps")
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
		entry := gen.Generate("v2.0.0", "", commits)

		// then: breaking changes section uses release-please style header
		testastic.Contains(t, entry.Body, "### ⚠ BREAKING CHANGES")
		testastic.Contains(t, entry.Body, "old endpoints removed")
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
		entry := gen.Generate("v1.0.0", "", commits)

		// then: hash is truncated to 7 chars
		testastic.Contains(t, entry.Body, "abc1234")
		testastic.NotContains(t, entry.Body, "abc1234567890def)")
	})

	t.Run("includes revert section", func(t *testing.T) {
		t.Parallel()

		// given: commits with a revert
		gen := &changelog.Generator{
			Sections: map[string]string{
				"feat":   "Features",
				"revert": "Reverts",
			},
			Include: []string{"feat", "revert"},
		}

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "add new endpoint"},
			{Hash: "def5678901", Type: "revert", Description: "revert add new endpoint"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.3.0", "", commits)

		// then: both sections are present
		testastic.Contains(t, entry.Body, "### Features")
		testastic.Contains(t, entry.Body, "### Reverts")
		testastic.Contains(t, entry.Body, "add new endpoint")
		testastic.Contains(t, entry.Body, "revert add new endpoint")
	})

	t.Run("uses capitalizeFirst fallback for unmapped commit type", func(t *testing.T) {
		t.Parallel()

		// given: a generator where "perf" is included but has no Sections mapping
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat", "perf"},
		}

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "perf", Description: "speed up query"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: section header uses capitalized type name
		testastic.Contains(t, entry.Body, "### Perf")
		testastic.Contains(t, entry.Body, "speed up query")
	})

	t.Run("empty commits", func(t *testing.T) {
		t.Parallel()

		// given: no commits
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", nil)

		// then: body is empty
		testastic.Equal(t, "", entry.Body)
	})

	t.Run("linked commit hashes with repo URL", func(t *testing.T) {
		t.Parallel()

		// given: a generator with repo URL configured
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			RepoURL:  "https://github.com/owner/repo",
		}

		commits := []commit.Commit{
			{Hash: "abc1234567890def", Type: "feat", Scope: "auth", Description: "add login"},
			{Hash: "def5678901234abc", Type: "feat", Description: "add signup"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: hashes are linked to commit URLs
		testastic.Contains(t, entry.Body, "[abc1234](https://github.com/owner/repo/commit/abc1234567890def)")
		testastic.Contains(t, entry.Body, "[def5678](https://github.com/owner/repo/commit/def5678901234abc)")
		testastic.Contains(t, entry.Body, "**auth:** add login")
	})

	t.Run("linked commit hashes with gitlab path prefix", func(t *testing.T) {
		t.Parallel()

		// given: a generator with GitLab repo URL
		gen := &changelog.Generator{
			Sections:   map[string]string{"fix": "Bug Fixes"},
			Include:    []string{"fix"},
			RepoURL:    "https://gitlab.com/owner/repo",
			PathPrefix: "/-",
		}

		commits := []commit.Commit{
			{Hash: "abc1234567890def", Type: "fix", Description: "fix crash"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.1", "", commits)

		// then: hashes use GitLab URL format
		testastic.Contains(t, entry.Body, "[abc1234](https://gitlab.com/owner/repo/-/commit/abc1234567890def)")
	})

	t.Run("compare URL with previous tag", func(t *testing.T) {
		t.Parallel()

		// given: a generator with repo URL and a previous tag
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			RepoURL:  "https://github.com/owner/repo",
		}

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "new feature"},
		}

		// when: generating changelog with previous tag
		entry := gen.Generate("v1.1.0", "v1.0.0", commits)

		// then: compare URL is set
		testastic.Equal(t, "https://github.com/owner/repo/compare/v1.0.0...v1.1.0", entry.CompareURL)
	})

	t.Run("no compare URL without previous tag", func(t *testing.T) {
		t.Parallel()

		// given: a generator with repo URL but no previous tag
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			RepoURL:  "https://github.com/owner/repo",
		}

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "initial feature"},
		}

		// when: generating changelog without previous tag
		entry := gen.Generate("v1.0.0", "", commits)

		// then: compare URL is empty
		testastic.Equal(t, "", entry.CompareURL)
	})

	t.Run("no compare URL without repo URL", func(t *testing.T) {
		t.Parallel()

		// given: a generator without repo URL
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
		}

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "feature"},
		}

		// when: generating changelog with previous tag but no repo URL
		entry := gen.Generate("v1.1.0", "v1.0.0", commits)

		// then: compare URL is empty
		testastic.Equal(t, "", entry.CompareURL)
	})

	t.Run("unlinked hashes without repo URL", func(t *testing.T) {
		t.Parallel()

		// given: a generator without repo URL
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
		}

		commits := []commit.Commit{
			{Hash: "abc1234567890def", Type: "feat", Description: "something"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: hash is plain text, not linked
		testastic.Contains(t, entry.Body, "(abc1234)")
		testastic.NotContains(t, entry.Body, "[abc1234]")
	})

	t.Run("inline pattern replaces reference with link", func(t *testing.T) {
		t.Parallel()

		// given: a generator with an inline reference pattern
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			References: config.ReferencesConfig{
				Patterns: []config.ReferencePattern{
					{Pattern: `JIRA-\d+`, URL: "https://jira.example.com/browse/{value}"},
				},
			},
		}

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "add OAuth2 support JIRA-123"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: reference is linked inline
		testastic.Contains(t, entry.Body, "[JIRA-123](https://jira.example.com/browse/JIRA-123)")
		testastic.NotContains(t, entry.Body, "support JIRA-123 (")
	})

	t.Run("inline pattern with empty URL leaves text as-is", func(t *testing.T) {
		t.Parallel()

		// given: a generator with a plain-text reference pattern
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			References: config.ReferencesConfig{
				Patterns: []config.ReferencePattern{
					{Pattern: `#\d+`, URL: ""},
				},
			},
		}

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "add feature #456"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: reference is left as plain text
		testastic.Contains(t, entry.Body, "add feature #456")
		testastic.NotContains(t, entry.Body, "[#456]")
	})

	t.Run("footer reference appended after hash", func(t *testing.T) {
		t.Parallel()

		// given: a generator with footer reference config
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			References: config.ReferencesConfig{
				Footers: map[string]string{
					"Refs": "https://jira.example.com/browse/{value}",
				},
			},
		}

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "add OAuth2 support",
				Footers: []commit.Footer{{Key: "Refs", Value: "JIRA-123"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: footer reference is appended after hash
		testastic.Contains(t, entry.Body, "(abc1234) ([JIRA-123](https://jira.example.com/browse/JIRA-123))")
	})

	t.Run("footer reference with empty URL renders plain text", func(t *testing.T) {
		t.Parallel()

		// given: a generator with plain-text footer reference
		gen := &changelog.Generator{
			Sections: map[string]string{"fix": "Bug Fixes"},
			Include:  []string{"fix"},
			References: config.ReferencesConfig{
				Footers: map[string]string{
					"Closes": "",
				},
			},
		}

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "fix", Description: "fix crash",
				Footers: []commit.Footer{{Key: "Closes", Value: "#789"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: footer reference is plain text
		testastic.Contains(t, entry.Body, "(abc1234) (#789)")
		testastic.NotContains(t, entry.Body, "[#789]")
	})

	t.Run("multiple footers on one commit", func(t *testing.T) {
		t.Parallel()

		// given: a commit with multiple matching footers
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			References: config.ReferencesConfig{
				Footers: map[string]string{
					"Refs": "https://jira.example.com/browse/{value}",
				},
			},
		}

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "big feature",
				Footers: []commit.Footer{
					{Key: "Refs", Value: "JIRA-100"},
					{Key: "Refs", Value: "JIRA-200"},
				},
			},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: both references appear
		testastic.Contains(t, entry.Body, "[JIRA-100]")
		testastic.Contains(t, entry.Body, "[JIRA-200]")
	})

	t.Run("no references configured leaves output unchanged", func(t *testing.T) {
		t.Parallel()

		// given: a generator with no references config
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
		}

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "add feature JIRA-123",
				Footers: []commit.Footer{{Key: "Refs", Value: "JIRA-123"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: no linking or reference extraction
		testastic.Contains(t, entry.Body, "add feature JIRA-123 (abc1234)\n")
		testastic.NotContains(t, entry.Body, "[JIRA-123]")
	})

	t.Run("non-matching footer key is ignored", func(t *testing.T) {
		t.Parallel()

		// given: a generator with footer config that doesn't match the commit's footer
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			References: config.ReferencesConfig{
				Footers: map[string]string{
					"Refs": "https://jira.example.com/browse/{value}",
				},
			},
		}

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "add feature",
				Footers: []commit.Footer{{Key: "Reviewed-by", Value: "Alice"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: no reference text appended
		testastic.Contains(t, entry.Body, "add feature (abc1234)\n")
		testastic.NotContains(t, entry.Body, "Alice")
	})

	t.Run("references in breaking changes section", func(t *testing.T) {
		t.Parallel()

		// given: a breaking commit with a footer reference
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			References: config.ReferencesConfig{
				Footers: map[string]string{
					"Refs": "https://jira.example.com/browse/{value}",
				},
			},
		}

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "new API", Breaking: true,
				Footers: []commit.Footer{
					{Key: "BREAKING CHANGE", Value: "old endpoints removed"},
					{Key: "Refs", Value: "JIRA-456"},
				},
			},
		}

		// when: generating changelog
		entry := gen.Generate("v2.0.0", "", commits)

		// then: reference appears in breaking changes section
		testastic.Contains(t, entry.Body, "### ⚠ BREAKING CHANGES")
		testastic.Contains(t, entry.Body, "[JIRA-456]")
	})

	t.Run("invalid regex pattern is skipped", func(t *testing.T) {
		t.Parallel()

		// given: a generator with an invalid regex pattern
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			References: config.ReferencesConfig{
				Patterns: []config.ReferencePattern{
					{Pattern: `[invalid`, URL: "https://example.com/{value}"},
				},
			},
		}

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "add feature"},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: no crash, description unchanged
		testastic.Contains(t, entry.Body, "add feature (abc1234)")
	})

	t.Run("both inline patterns and footer references", func(t *testing.T) {
		t.Parallel()

		// given: a generator with both patterns and footers
		gen := &changelog.Generator{
			Sections: map[string]string{"feat": "Features"},
			Include:  []string{"feat"},
			RepoURL:  "https://github.com/owner/repo",
			References: config.ReferencesConfig{
				Patterns: []config.ReferencePattern{
					{Pattern: `JIRA-\d+`, URL: "https://jira.example.com/browse/{value}"},
				},
				Footers: map[string]string{
					"Closes": "",
				},
			},
		}

		commits := []commit.Commit{
			{
				Hash: "abc1234567890def", Type: "feat", Description: "add OAuth2 JIRA-123",
				Footers: []commit.Footer{{Key: "Closes", Value: "#456"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate("v1.0.0", "", commits)

		// then: inline pattern is linked in description
		testastic.Contains(t, entry.Body, "[JIRA-123](https://jira.example.com/browse/JIRA-123)")
		// and: footer reference is appended after hash
		testastic.Contains(t, entry.Body, "(#456)")
		// and: commit hash is still linked
		testastic.Contains(t, entry.Body, "[abc1234](https://github.com/owner/repo/commit/abc1234567890def)")
	})
}

func TestRender(t *testing.T) {
	t.Parallel()

	t.Run("renders entry as markdown", func(t *testing.T) {
		t.Parallel()

		// given: a changelog entry without compare URL
		entry := changelog.Entry{
			Version: "v1.2.0",
			Body:    "### Features\n\n- something new (abc1234)\n",
		}

		// when: rendering
		output := changelog.Render(entry)

		// then: output has plain version header and body
		testastic.HasPrefix(t, output, "## v1.2.0")
		testastic.Contains(t, output, "### Features")
	})

	t.Run("renders linked version header with compare URL", func(t *testing.T) {
		t.Parallel()

		// given: a changelog entry with compare URL
		entry := changelog.Entry{
			Version:    "v1.2.0",
			Body:       "### Features\n\n- something new (abc1234)\n",
			CompareURL: "https://github.com/owner/repo/compare/v1.1.0...v1.2.0",
		}

		// when: rendering
		output := changelog.Render(entry)

		// then: version header is linked
		testastic.Contains(t, output, "## [v1.2.0](https://github.com/owner/repo/compare/v1.1.0...v1.2.0)")
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
		testastic.HasPrefix(t, result, "# Changelog")
		testastic.Contains(t, result, newEntry)
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

		testastic.Less(t, newIdx, oldIdx)
	})

	t.Run("separates entries when new entry has no trailing newline", func(t *testing.T) {
		t.Parallel()

		// given: a generated entry whose manual sections removed the trailing newline
		existing := "# Changelog\n\n## v1.0.0 (2026-01-01)\n\n- old stuff\n"
		newEntry := "## v1.1.0 (2026-02-28)\n\n### Features\n\n- new stuff"

		// when: prepending the entry
		result := changelog.Prepend(existing, newEntry)

		// then: adjacent release entries remain separated by a blank line
		testastic.Contains(t, result, "- new stuff\n\n## v1.0.0")
	})
}
