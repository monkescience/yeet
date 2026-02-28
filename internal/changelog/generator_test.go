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
		entry := gen.Generate("v1.2.0", "", commits)

		// then: sections are present with correct commits
		testastic.Equal(t, "v1.2.0", entry.Version)
		testastic.True(t, strings.Contains(entry.Body, "### Features"))
		testastic.True(t, strings.Contains(entry.Body, "### Bug Fixes"))
		testastic.True(t, strings.Contains(entry.Body, "**auth:** add OAuth2 support"))
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
		entry := gen.Generate("v2.0.0", "", commits)

		// then: breaking changes section uses release-please style header
		testastic.True(t, strings.Contains(entry.Body, "### ⚠ BREAKING CHANGES"))
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
		entry := gen.Generate("v1.0.0", "", commits)

		// then: hash is truncated to 7 chars
		testastic.True(t, strings.Contains(entry.Body, "abc1234"))
		testastic.False(t, strings.Contains(entry.Body, "abc1234567890def)"))
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
		testastic.True(t, strings.Contains(entry.Body, "### Features"))
		testastic.True(t, strings.Contains(entry.Body, "### Reverts"))
		testastic.True(t, strings.Contains(entry.Body, "add new endpoint"))
		testastic.True(t, strings.Contains(entry.Body, "revert add new endpoint"))
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
		testastic.True(t, strings.Contains(entry.Body, "[abc1234](https://github.com/owner/repo/commit/abc1234567890def)"))
		testastic.True(t, strings.Contains(entry.Body, "[def5678](https://github.com/owner/repo/commit/def5678901234abc)"))
		testastic.True(t, strings.Contains(entry.Body, "**auth:** add login"))
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
		testastic.True(t, strings.Contains(entry.Body, "[abc1234](https://gitlab.com/owner/repo/-/commit/abc1234567890def)"))
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
		testastic.True(t, strings.Contains(entry.Body, "(abc1234)"))
		testastic.False(t, strings.Contains(entry.Body, "[abc1234]"))
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
		testastic.True(t, strings.HasPrefix(output, "## v1.2.0"))
		testastic.True(t, strings.Contains(output, "### Features"))
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
		testastic.True(t, strings.Contains(output, "## [v1.2.0](https://github.com/owner/repo/compare/v1.1.0...v1.2.0)"))
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
