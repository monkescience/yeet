package changelog_test

import (
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	t.Run("uses a captured release date", func(t *testing.T) {
		t.Parallel()

		// given: a generator configured with a fixed release instant
		date := time.Date(2025, time.December, 31, 16, 30, 0, 0, time.FixedZone("PST", -8*60*60))
		gen := changelog.New(changelog.WithDate(date))

		// when: generating an entry
		entry := gen.Generate(t.Context(), "v1.2.3", "", nil)

		// then: the captured instant is retained as the release date
		testastic.Equal(t, date, entry.Date)
	})

	t.Run("generates changelog with sections", func(t *testing.T) {
		t.Parallel()

		// given: a generator and some commits
		gen := changelog.New(
			changelog.WithSections(map[string]string{
				"feat": "Features",
				"fix":  "Bug Fixes",
			}),
			changelog.WithInclude([]string{"feat", "fix"}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Scope: "auth", Description: "add OAuth2 support"},
			{Hash: "def5678901", Type: "fix", Description: "resolve null pointer"},
			{Hash: "ghi9012345", Type: "chore", Description: "update deps"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.2.0", "", commits)

		// then: sections are present with correct commits
		testastic.Equal(t, "v1.2.0", entry.Version)
		testastic.AssertFile(
			t,
			"testdata/generate/generates_changelog_with_sections/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("includes breaking changes section", func(t *testing.T) {
		t.Parallel()

		// given: commits with a breaking change
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "new API", Breaking: true,
				Footers: []commit.Footer{{Key: "BREAKING CHANGE", Value: "old endpoints removed"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v2.0.0", "", commits)

		// then: the footer becomes a note in the breaking changes section
		testastic.AssertFile(
			t,
			"testdata/generate/includes_breaking_changes_section/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("uses the configured breaking changes heading", func(t *testing.T) {
		t.Parallel()

		// given: a breaking commit and a custom heading for breaking changes
		gen := changelog.New(
			changelog.WithSections(map[string]string{
				"breaking": "Compatibility Notes",
				"feat":     "Features",
			}),
			changelog.WithInclude([]string{"feat"}),
		)
		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "new API", Breaking: true},
		}

		// when: generating the changelog
		entry := gen.Generate(t.Context(), "v2.0.0", "", commits)

		// then: the breaking section uses the configured heading
		testastic.Equal(t, "Compatibility Notes", entry.Sections[0].Heading)
	})

	t.Run("short hash in output", func(t *testing.T) {
		t.Parallel()

		// given: a commit with a long hash
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567890def", Type: "feat", Description: "something new"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: hash is truncated to 7 chars
		testastic.AssertFile(
			t,
			"testdata/generate/short_hash_in_output/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("includes revert section", func(t *testing.T) {
		t.Parallel()

		// given: commits with a revert
		gen := changelog.New(
			changelog.WithSections(map[string]string{
				"feat":   "Features",
				"revert": "Reverts",
			}),
			changelog.WithInclude([]string{"feat", "revert"}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "add new endpoint"},
			{Hash: "def5678901", Type: "revert", Description: "revert add new endpoint"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.3.0", "", commits)

		// then: both sections are present
		testastic.AssertFile(
			t,
			"testdata/generate/includes_revert_section/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("uses capitalizeFirst fallback for unmapped commit type", func(t *testing.T) {
		t.Parallel()

		// given: a generator where "perf" is included but has no Sections mapping
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat", "perf"}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "perf", Description: "speed up query"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: section header uses capitalized type name
		testastic.AssertFile(
			t,
			"testdata/generate/uses_capitalize_first_fallback_for_unmapped_commit_type/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("empty commits", func(t *testing.T) {
		t.Parallel()

		// given: no commits
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", nil)

		// then: body is empty
		testastic.Empty(t, entry.Sections)
	})

	t.Run("linked commit hashes with repo URL", func(t *testing.T) {
		t.Parallel()

		// given: a generator with repo URL configured
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithRepoURL("https://github.com/owner/repo"),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567890def", Type: "feat", Scope: "auth", Description: "add login"},
			{Hash: "def5678901234abc", Type: "feat", Description: "add signup"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: hashes are linked to commit URLs
		testastic.AssertFile(
			t,
			"testdata/generate/linked_commit_hashes_with_repo_u_r_l/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("linked commit hashes with gitlab path prefix", func(t *testing.T) {
		t.Parallel()

		// given: a generator with GitLab repo URL
		gen := changelog.New(
			changelog.WithSections(map[string]string{"fix": "Bug Fixes"}),
			changelog.WithInclude([]string{"fix"}),
			changelog.WithRepoURL("https://gitlab.com/owner/repo"),
			changelog.WithPathPrefix("/-"),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567890def", Type: "fix", Description: "fix crash"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.1", "", commits)

		// then: hashes use GitLab URL format
		testastic.AssertFile(
			t,
			"testdata/generate/linked_commit_hashes_with_gitlab_path_prefix/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("compare URL with previous tag", func(t *testing.T) {
		t.Parallel()

		// given: a generator with a compare URL builder and a previous tag
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithRepoURL("https://github.com/owner/repo"),
			changelog.WithCompareURL(func(from, to string) string {
				return "https://github.com/owner/repo/compare/" + from + "..." + to
			}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "new feature"},
		}

		// when: generating changelog with previous tag
		entry := gen.Generate(t.Context(), "v1.1.0", "v1.0.0", commits)

		// then: compare URL is set
		testastic.Equal(t, "https://github.com/owner/repo/compare/v1.0.0...v1.1.0", entry.CompareURL)
	})

	t.Run("no compare URL without previous tag", func(t *testing.T) {
		t.Parallel()

		// given: a generator with a compare URL builder but no previous tag
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithRepoURL("https://github.com/owner/repo"),
			changelog.WithCompareURL(func(from, to string) string {
				return "https://github.com/owner/repo/compare/" + from + "..." + to
			}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "initial feature"},
		}

		// when: generating changelog without previous tag
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: compare URL is empty
		testastic.Equal(t, "", entry.CompareURL)
	})

	t.Run("no compare URL without compare builder", func(t *testing.T) {
		t.Parallel()

		// given: a generator without compare URL builder
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithRepoURL("https://github.com/owner/repo"),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "feature"},
		}

		// when: generating changelog with previous tag but no builder
		entry := gen.Generate(t.Context(), "v1.1.0", "v1.0.0", commits)

		// then: compare URL is empty
		testastic.Equal(t, "", entry.CompareURL)
	})

	t.Run("unlinked hashes without repo URL", func(t *testing.T) {
		t.Parallel()

		// given: a generator without repo URL
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567890def", Type: "feat", Description: "something"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: hash is plain text, not linked
		testastic.AssertFile(
			t,
			"testdata/generate/unlinked_hashes_without_repo_u_r_l/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("inline pattern replaces reference with link", func(t *testing.T) {
		t.Parallel()

		// given: a generator with an inline reference pattern
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithReferences(changelog.References{
				Patterns: []changelog.ReferencePattern{
					{Pattern: `JIRA-\d+`, URL: "https://jira.example.com/browse/{value}"},
				},
			}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "add OAuth2 support JIRA-123"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: reference is linked inline
		testastic.AssertFile(
			t,
			"testdata/generate/inline_pattern_replaces_reference_with_link/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("inline pattern with empty URL leaves text as-is", func(t *testing.T) {
		t.Parallel()

		// given: a generator with a plain-text reference pattern
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithReferences(changelog.References{
				Patterns: []changelog.ReferencePattern{
					{Pattern: `#\d+`, URL: ""},
				},
			}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "add feature #456"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: reference is left as plain text
		testastic.AssertFile(
			t,
			"testdata/generate/inline_pattern_with_empty_u_r_l_leaves_text_as_is/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("footer reference appended after hash", func(t *testing.T) {
		t.Parallel()

		// given: a generator with footer reference config
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithReferences(changelog.References{
				Footers: map[string]string{
					"Refs": "https://jira.example.com/browse/{value}",
				},
			}),
		)

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "add OAuth2 support",
				Footers: []commit.Footer{{Key: "Refs", Value: "JIRA-123"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: footer reference is appended after hash
		testastic.AssertFile(
			t,
			"testdata/generate/footer_reference_appended_after_hash/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("footer reference with empty URL renders plain text", func(t *testing.T) {
		t.Parallel()

		// given: a generator with plain-text footer reference
		gen := changelog.New(
			changelog.WithSections(map[string]string{"fix": "Bug Fixes"}),
			changelog.WithInclude([]string{"fix"}),
			changelog.WithReferences(changelog.References{
				Footers: map[string]string{
					"Closes": "",
				},
			}),
		)

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "fix", Description: "fix crash",
				Footers: []commit.Footer{{Key: "Closes", Value: "#789"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: footer reference is plain text
		testastic.AssertFile(
			t,
			"testdata/generate/footer_reference_with_empty_u_r_l_renders_plain_text/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("multiple footers on one commit", func(t *testing.T) {
		t.Parallel()

		// given: a commit with multiple matching footers
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithReferences(changelog.References{
				Footers: map[string]string{
					"Refs": "https://jira.example.com/browse/{value}",
				},
			}),
		)

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
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: both references appear
		testastic.AssertFile(
			t,
			"testdata/generate/multiple_footers_on_one_commit/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("no references configured leaves output unchanged", func(t *testing.T) {
		t.Parallel()

		// given: a generator with no references config
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "add feature JIRA-123",
				Footers: []commit.Footer{{Key: "Refs", Value: "JIRA-123"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: no linking or reference extraction
		testastic.AssertFile(
			t,
			"testdata/generate/no_references_configured_leaves_output_unchanged/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("non-matching footer key is ignored", func(t *testing.T) {
		t.Parallel()

		// given: a generator with footer config that doesn't match the commit's footer
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithReferences(changelog.References{
				Footers: map[string]string{
					"Refs": "https://jira.example.com/browse/{value}",
				},
			}),
		)

		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "add feature",
				Footers: []commit.Footer{{Key: "Reviewed-by", Value: "Alice"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: no reference text appended
		testastic.AssertFile(
			t,
			"testdata/generate/non_matching_footer_key_is_ignored/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("references in breaking changes section", func(t *testing.T) {
		t.Parallel()

		// given: a breaking commit with a footer reference
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithReferences(changelog.References{
				Footers: map[string]string{
					"Refs": "https://jira.example.com/browse/{value}",
				},
			}),
		)

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
		entry := gen.Generate(t.Context(), "v2.0.0", "", commits)

		// then: the reference appears in the breaking note and the type section
		testastic.AssertFile(
			t,
			"testdata/generate/references_in_breaking_changes_section/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("invalid regex pattern is skipped", func(t *testing.T) {
		t.Parallel()

		// given: a generator with an invalid regex pattern
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithReferences(changelog.References{
				Patterns: []changelog.ReferencePattern{
					{Pattern: `[invalid`, URL: "https://example.com/{value}"},
				},
			}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "feat", Description: "add feature"},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: no crash, description unchanged
		testastic.AssertFile(
			t,
			"testdata/generate/invalid_regex_pattern_is_skipped/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("both inline patterns and footer references", func(t *testing.T) {
		t.Parallel()

		// given: a generator with both patterns and footers
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithRepoURL("https://github.com/owner/repo"),
			changelog.WithReferences(changelog.References{
				Patterns: []changelog.ReferencePattern{
					{Pattern: `JIRA-\d+`, URL: "https://jira.example.com/browse/{value}"},
				},
				Footers: map[string]string{
					"Closes": "",
				},
			}),
		)

		commits := []commit.Commit{
			{
				Hash: "abc1234567890def", Type: "feat", Description: "add OAuth2 JIRA-123",
				Footers: []commit.Footer{{Key: "Closes", Value: "#456"}},
			},
		}

		// when: generating changelog
		entry := gen.Generate(t.Context(), "v1.0.0", "", commits)

		// then: inline pattern is linked in description, footer reference appears, and commit hash is linked
		testastic.AssertFile(
			t,
			"testdata/generate/both_inline_patterns_and_footer_references/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})
}

func TestInlineReferencePatterns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		patterns    []changelog.ReferencePattern
		description string
		expected    string
	}{
		{
			name: "earlier pattern wins on exact overlap without nesting links",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
				{Pattern: `\b[A-Z]+-\d+\b`, URL: "https://other.example.com/{value}"},
			},
			description: "fix PROJ-12 and OTHER-34",
			expected: "### Bug Fixes\n\n- fix [PROJ-12](https://jira.example.com/browse/PROJ-12) and " +
				"[OTHER-34](https://other.example.com/OTHER-34) (abcdef0)\n",
		},
		{
			name: "sorts replacements when a later pattern matches earlier in the description",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
				{Pattern: `\b[A-Z]+-\d+\b`, URL: "https://other.example.com/{value}"},
			},
			description: "fix OTHER-34 and PROJ-12",
			expected: "### Bug Fixes\n\n- fix [OTHER-34](https://other.example.com/OTHER-34) and " +
				"[PROJ-12](https://jira.example.com/browse/PROJ-12) (abcdef0)\n",
		},
		{
			name: "later pattern does not match inside earlier pattern URL",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bJIRA-\d+\b`, URL: "https://tracker.example.com/PR-1/{value}"},
				{Pattern: `\bPR-\d+\b`, URL: "https://github.com/org/repo/pull/{value}"},
			},
			description: "fix JIRA-12 and PR-9",
			expected: "### Bug Fixes\n\n- fix [JIRA-12](https://tracker.example.com/PR-1/JIRA-12) and " +
				"[PR-9](https://github.com/org/repo/pull/PR-9) (abcdef0)\n",
		},
		{
			name: "earlier pattern wins on partial overlap while later non-overlapping matches still link",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
				{Pattern: `FOO-PROJ`, URL: "https://foo.example.com/{value}"},
			},
			description: "update FOO-PROJ-12 and FOO-PROJ",
			expected: "### Bug Fixes\n\n- update FOO-[PROJ-12](https://jira.example.com/browse/PROJ-12) and " +
				"[FOO-PROJ](https://foo.example.com/FOO-PROJ) (abcdef0)\n",
		},
		{
			name: "earlier pattern wins when later pattern matches a substring inside it",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
				{Pattern: `\bPROJ\b`, URL: "https://project.example.com/{value}"},
			},
			description: "fix PROJ-12 for PROJ",
			expected: "### Bug Fixes\n\n- fix [PROJ-12](https://jira.example.com/browse/PROJ-12) for " +
				"[PROJ](https://project.example.com/PROJ) (abcdef0)\n",
		},
		{
			name: "empty URL pattern reserves matched text as plain text against later catch-all",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bINTERNAL-\d+\b`, URL: ""},
				{Pattern: `\b[A-Z]+-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
			},
			description: "track INTERNAL-1 and PROJ-2",
			expected: "### Bug Fixes\n\n- track INTERNAL-1 and " +
				"[PROJ-2](https://jira.example.com/browse/PROJ-2) (abcdef0)\n",
		},
		{
			name: "skips existing markdown links and autolinks without breaking bracketed references",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
			},
			description: `see [PROJ-1](https://existing.example.com/PROJ-1), ` +
				`[PROJ-2](<https://existing.example.com/PROJ-2> "PROJ-2"), ` +
				`[PROJ-3](), <https://existing.example.com/PROJ-4>, ` +
				`![PROJ-5](https://img.example.com/PROJ-5.png), [PROJ-6] and PROJ-7`,
			expected: `### Bug Fixes` + "\n\n" +
				`- see [PROJ-1](https://existing.example.com/PROJ-1), ` +
				`[PROJ-2](<https://existing.example.com/PROJ-2> "PROJ-2"), ` +
				`[PROJ-3](), <https://existing.example.com/PROJ-4>, ` +
				`![PROJ-5](https://img.example.com/PROJ-5.png), ` +
				`[[PROJ-6](https://jira.example.com/browse/PROJ-6)] and ` +
				`[PROJ-7](https://jira.example.com/browse/PROJ-7) (abcdef0)` + "\n",
		},
		{
			name: "skips existing links at start of description",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
			},
			description: `[PROJ-1](https://existing.example.com/PROJ-1) and PROJ-2`,
			expected: "### Bug Fixes\n\n- [PROJ-1](https://existing.example.com/PROJ-1) and " +
				"[PROJ-2](https://jira.example.com/browse/PROJ-2) (abcdef0)\n",
		},
		{
			name: "skips existing links in indented descriptions",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
			},
			description: `    [PROJ-1](https://existing.example.com/PROJ-1) and PROJ-2`,
			expected: "### Bug Fixes\n\n-     [PROJ-1](https://existing.example.com/PROJ-1) and " +
				"[PROJ-2](https://jira.example.com/browse/PROJ-2) (abcdef0)\n",
		},
		{
			name: "skips existing links after leading markdown block markers",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
			},
			description: `- [PROJ-1](https://existing.example.com/PROJ-1) and PROJ-2`,
			expected: "### Bug Fixes\n\n- - [PROJ-1](https://existing.example.com/PROJ-1) and " +
				"[PROJ-2](https://jira.example.com/browse/PROJ-2) (abcdef0)\n",
		},
		{
			name: "skips empty-destination links containing nested images, links, code spans, or raw HTML",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
			},
			description: `[![PROJ-1](https://img.example.com/PROJ-1.png)](), ` +
				`![see [PROJ-2](https://existing.example.com/PROJ-2)](), ` +
				"[PROJ-3 `a]`](), " +
				`[PROJ-4 <span title="]">]() and PROJ-5`,
			expected: "### Bug Fixes\n\n- " +
				`[![PROJ-1](https://img.example.com/PROJ-1.png)](), ` +
				`![see [PROJ-2](https://existing.example.com/PROJ-2)](), ` +
				"[PROJ-3 `a]`](), " +
				`[PROJ-4 <span title="]">]() and ` +
				"[PROJ-5](https://jira.example.com/browse/PROJ-5) (abcdef0)\n",
		},
		{
			name: "skips empty destinations, empty titles, escaped brackets, and email autolinks",
			patterns: []changelog.ReferencePattern{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.com/browse/{value}"},
			},
			description: `see [PROJ-1](<>), [PROJ-2](https://existing.example.com/PROJ-2 ""), ` +
				`[PROJ-3](<> ""), [PROJ-4 *a* \]](), <user@PROJ-5.example.com> and PROJ-6`,
			expected: "### Bug Fixes\n\n- see " +
				`[PROJ-1](<>), [PROJ-2](https://existing.example.com/PROJ-2 ""), ` +
				`[PROJ-3](<> ""), [PROJ-4 *a* \]](), <user@PROJ-5.example.com> and ` +
				"[PROJ-6](https://jira.example.com/browse/PROJ-6) (abcdef0)\n",
		},
		{
			name: "skips zero-width regex matches",
			patterns: []changelog.ReferencePattern{
				{Pattern: `(?:PROJ-\d+)?`, URL: "https://jira.example.com/browse/{value}"},
			},
			description: "fix PROJ-12",
			expected:    "### Bug Fixes\n\n- fix [PROJ-12](https://jira.example.com/browse/PROJ-12) (abcdef0)\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: a generator configured with inline reference patterns and a fix commit
			gen := changelog.New(
				changelog.WithSections(map[string]string{"fix": "Bug Fixes"}),
				changelog.WithInclude([]string{"fix"}),
				changelog.WithReferences(changelog.References{Patterns: tc.patterns}),
			)
			commits := []commit.Commit{{Hash: "abcdef0", Type: "fix", Description: tc.description}}

			// when: generating the changelog body
			body := changelog.RenderBody(gen.Generate(t.Context(), "v1.0.1", "", commits))

			// then: references are linked cleanly without nesting links inside links
			testastic.Equal(t, tc.expected, body)
		})
	}
}

func TestRender(t *testing.T) {
	t.Parallel()

	t.Run("normalizes only freeform block edges", func(t *testing.T) {
		t.Parallel()

		// given: an entry with blank outer edges and intentional internal spacing
		entry := changelog.Entry{
			Intro: []string{"", "First intro paragraph.", "", "", "Second intro paragraph.", ""},
			Sections: []changelog.Section{{
				Heading: "Bug Fixes",
				Lines:   []string{"- patch issue (abc1234)"},
			}},
			Outro: []string{"", "First outro paragraph.", "", "Second outro paragraph.", ""},
		}

		// when: rendering its body
		output := changelog.RenderBody(entry)

		// then: outer blanks are normalized and internal spacing is retained
		testastic.Equal(
			t,
			"First intro paragraph.\n\n\nSecond intro paragraph.\n\n"+
				"### Bug Fixes\n\n- patch issue (abc1234)\n\n"+
				"First outro paragraph.\n\nSecond outro paragraph.\n",
			output,
		)
	})

	t.Run("renders entry as markdown", func(t *testing.T) {
		t.Parallel()

		// given: a changelog entry without compare URL
		entry := changelog.ParseEntry(readTestFile(t, "testdata/render/renders_entry_as_markdown/body.input.md"))
		entry.Version = "v1.2.0"

		// when: rendering
		output := changelog.Render(entry)

		// then: output has plain version header and body
		testastic.AssertFile(
			t,
			"testdata/render/renders_entry_as_markdown/output.expected.md",
			output,
		)
	})

	t.Run("renders linked version header with compare URL", func(t *testing.T) {
		t.Parallel()

		// given: a changelog entry with compare URL
		entry := changelog.ParseEntry(
			readTestFile(t, "testdata/render/renders_linked_version_header_with_compare_u_r_l/body.input.md"),
		)
		entry.Version = "v1.2.0"
		entry.CompareURL = "https://github.com/owner/repo/compare/v1.1.0...v1.2.0"

		// when: rendering
		output := changelog.Render(entry)

		// then: version header is linked
		testastic.AssertFile(
			t,
			"testdata/render/renders_linked_version_header_with_compare_u_r_l/output.expected.md",
			output,
		)
	})
}

func TestDerivedEntry(t *testing.T) {
	t.Parallel()

	t.Run("keeps the release date of the direct entry", func(t *testing.T) {
		t.Parallel()

		// given: a direct entry dated on its release day
		direct := changelog.Entry{
			Version: "v1.3.0",
			Date:    time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
			Sections: []changelog.Section{{
				Heading: "Bug Fixes",
				Lines:   []string{"- patch issue (abc1234)"},
			}},
		}

		// when: nesting a child target under it
		derived := changelog.DerivedEntry(
			direct,
			[]string{"api"},
			[]changelog.Section{{Heading: "api"}},
		)

		// then: the release heading carries that date
		testastic.Contains(t, changelog.Render(derived), "## v1.3.0 (2026-03-01)")
	})

	t.Run("nests children after the parent's own sections", func(t *testing.T) {
		t.Parallel()

		// given: a parent entry with its own section and two child targets
		direct := changelog.Entry{
			Version:       "v1.3.0",
			Sections:      []changelog.Section{{Heading: "Features", Lines: []string{"- add tokens (abc1234)"}}},
			OwnedHeadings: []string{"Features", "Bug Fixes"},
		}
		children := []changelog.Section{
			{Heading: "api", Sections: []changelog.Section{{Heading: "Bug Fixes"}}},
			{Heading: "web"},
		}

		// when: nesting the children under it
		derived := changelog.DerivedEntry(direct, []string{"api", "web", "cli"}, children)

		// then: the parent's sections come first and the children keep their nesting
		testastic.SliceEqual(t, []string{"Features", "api", "web"}, sectionHeadings(derived.Sections))
		testastic.SliceEqual(t, []string{"Bug Fixes"}, sectionHeadings(derived.Sections[1].Sections))
		testastic.SliceEqual(
			t,
			[]string{"Features", "Bug Fixes", "api", "web", "cli"},
			derived.OwnedHeadings,
		)
	})

	t.Run("drops the compare URL of the direct entry", func(t *testing.T) {
		t.Parallel()

		// given: a direct entry already linked against its own previous tag
		direct := changelog.Entry{
			Version:    "v1.3.0",
			CompareURL: "https://github.com/owner/repo/compare/v1.2.0...v1.3.0",
		}

		// when: nesting a child target under it
		derived := changelog.DerivedEntry(direct, []string{"api"}, []changelog.Section{{Heading: "api"}})

		// then: the caller is left to decide what the wave compares against
		testastic.Equal(t, "", derived.CompareURL)
	})
}

func TestPrependEntry(t *testing.T) {
	t.Parallel()

	t.Run("prepend to empty changelog", func(t *testing.T) {
		t.Parallel()

		// given: no existing changelog
		newEntry := readTestFile(t, "testdata/prepend/prepend_to_empty_changelog/new_entry.input.md")

		// when: prepending
		result := changelog.PrependEntry("", newEntry)

		// then: header is added
		testastic.AssertFile(
			t,
			"testdata/prepend/prepend_to_empty_changelog/changelog.expected.md",
			result,
		)
	})

	t.Run("prepend to existing changelog", func(t *testing.T) {
		t.Parallel()

		// given: an existing changelog
		existing := readTestFile(t, "testdata/prepend/prepend_to_existing_changelog/existing.input.md")
		newEntry := readTestFile(t, "testdata/prepend/prepend_to_existing_changelog/new_entry.input.md")

		// when: prepending
		result := changelog.PrependEntry(existing, newEntry)

		// then: new entry is before old entry
		testastic.AssertFile(
			t,
			"testdata/prepend/prepend_to_existing_changelog/changelog.expected.md",
			result,
		)
	})

	t.Run("separates entries when new entry has no trailing newline", func(t *testing.T) {
		t.Parallel()

		// given: a generated entry whose manual sections removed the trailing newline
		existing := readTestFile(
			t,
			"testdata/prepend/"+
				"separates_entries_when_new_entry_has_no_trailing_newline/"+
				"existing.input.md",
		)
		newEntry := readTestFile(
			t,
			"testdata/prepend/"+
				"separates_entries_when_new_entry_has_no_trailing_newline/"+
				"new_entry.input.md",
		)

		// when: prepending the entry
		result := changelog.PrependEntry(existing, newEntry)

		// then: adjacent release entries remain separated by a blank line
		testastic.AssertFile(
			t,
			"testdata/prepend/separates_entries_when_new_entry_has_no_trailing_newline/changelog.expected.md",
			result,
		)
	})

	t.Run("preserves header without trailing blank line", func(t *testing.T) {
		t.Parallel()

		// given: a changelog containing only an H1 without a trailing blank line
		existing := readTestFile(t, "testdata/prepend/preserves_header_without_trailing_blank_line/existing.input.md")
		newEntry := readTestFile(t, "testdata/prepend/preserves_header_without_trailing_blank_line/new_entry.input.md")

		// when: prepending the entry
		result := changelog.PrependEntry(existing, newEntry)

		// then: the existing H1 is reused instead of duplicated
		testastic.AssertFile(
			t,
			"testdata/prepend/preserves_header_without_trailing_blank_line/changelog.expected.md",
			result,
		)
	})

	t.Run("inserts before release heading without splitting its body", func(t *testing.T) {
		t.Parallel()

		// given: a changelog whose H1 is not followed by a blank line
		existing := readTestFile(
			t,
			"testdata/prepend/"+
				"inserts_before_release_heading_without_splitting_its_body/"+
				"existing.input.md",
		)
		newEntry := readTestFile(
			t,
			"testdata/prepend/"+
				"inserts_before_release_heading_without_splitting_its_body/"+
				"new_entry.input.md",
		)

		// when: prepending the entry
		result := changelog.PrependEntry(existing, newEntry)

		// then: the old release heading remains attached to its body
		testastic.AssertFile(
			t,
			"testdata/prepend/inserts_before_release_heading_without_splitting_its_body/changelog.expected.md",
			result,
		)
	})

	t.Run("preserves preamble before release entries", func(t *testing.T) {
		t.Parallel()

		// given: a changelog with explanatory text before its first release
		existing := readTestFile(t, "testdata/prepend/preserves_preamble_before_release_entries/existing.input.md")
		newEntry := readTestFile(t, "testdata/prepend/preserves_preamble_before_release_entries/new_entry.input.md")

		// when: prepending the entry
		result := changelog.PrependEntry(existing, newEntry)

		// then: the preamble remains above the new release
		testastic.AssertFile(
			t,
			"testdata/prepend/preserves_preamble_before_release_entries/changelog.expected.md",
			result,
		)
	})

	t.Run("preserves fenced headings in the preamble", func(t *testing.T) {
		t.Parallel()

		// given: a changelog preamble containing a level-two heading in a code fence
		existing := "# Changelog\n\n" +
			"Example format:\n\n" +
			"```markdown\n## v0.0.0 (example)\nExample notes.\n```\n\n" +
			"## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the complete preamble remains above the new release
		testastic.Equal(
			t,
			"# Changelog\n\nExample format:\n\n```markdown\n## v0.0.0 (example)\nExample notes.\n```\n\n"+
				"## v1.1.0 (2026-08-10)\n\nNew release.\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("preserves non-release headings in the preamble", func(t *testing.T) {
		t.Parallel()

		// given: a changelog preamble containing a document-level heading
		existing := "# Changelog\n\n" +
			"## About\n\nRelease notes for this project.\n\n" +
			"## Q1-2026\n\nPlanned work.\n\n" +
			"## api-v1.0.0-beta.1\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the complete preamble remains above the new release
		testastic.Equal(
			t,
			"# Changelog\n\n## About\n\nRelease notes for this project.\n\n## Q1-2026\n\nPlanned work.\n\n"+
				"## v1.1.0 (2026-08-10)\n\nNew release.\n\n## api-v1.0.0-beta.1\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("inserts before Keep a Changelog release headings", func(t *testing.T) {
		t.Parallel()

		// given: a Keep a Changelog file with bracketed releases and link references
		existing := "# Changelog\n\n" +
			"## [Unreleased]\n\n" +
			"## [1.0.0] - 2026-01-01\n\nPrevious release.\n\n" +
			"[1.0.0]: https://example.com/releases/1.0.0\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the new release precedes the first existing release
		testastic.Equal(
			t,
			"# Changelog\n\n## [Unreleased]\n\n"+
				"## v1.1.0 (2026-08-10)\n\nNew release.\n\n"+
				"## [1.0.0] - 2026-01-01\n\nPrevious release.\n\n"+
				"[1.0.0]: https://example.com/releases/1.0.0\n",
			result,
		)
	})

	t.Run("inserts before release headings with a leading word", func(t *testing.T) {
		t.Parallel()

		// given: a changelog whose release headings start with a word before the version
		existing := "# Changelog\n\n## Upgrading from 1.x to 2.0\n\nSee docs.\n\n## Version 1.0.0\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the new release follows the preamble and precedes the existing release
		testastic.Equal(
			t,
			"# Changelog\n\n## Upgrading from 1.x to 2.0\n\nSee docs.\n\n"+
				"## v1.1.0 (2026-08-10)\n\nNew release.\n\n## Version 1.0.0\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("treats a whitespace-only changelog as empty", func(t *testing.T) {
		t.Parallel()

		// given: an existing changelog that contains only blank lines
		existing := "\n  \n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the changelog starts fresh with a header and the new release
		testastic.Equal(t, "# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n", result)
	})

	t.Run("adds a header above an untitled changelog", func(t *testing.T) {
		t.Parallel()

		// given: an existing changelog without an H1 and with surrounding blank lines
		existing := "\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n\n\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: a header is added and the new release precedes the existing content
		testastic.Equal(
			t,
			"# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("preserves YAML front matter above an H1 title", func(t *testing.T) {
		t.Parallel()

		// given: a changelog starting with YAML front matter followed by an H1 title
		existing := "---\ntitle: Changelog\nsidebar_position: 5\n---\n\n" +
			"# Changelog\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: front matter stays at the top and the existing H1 is not duplicated
		testastic.Equal(
			t,
			"---\ntitle: Changelog\nsidebar_position: 5\n---\n\n# Changelog\n\n"+
				"## v1.1.0 (2026-08-10)\n\nNew release.\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("preserves YAML front matter without an H1 title", func(t *testing.T) {
		t.Parallel()

		// given: a static-site changelog whose title lives only in YAML front matter
		existing := "---\ntitle: Changelog\n---\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: front matter remains at the top without injecting an H1 heading
		testastic.Equal(
			t,
			"---\ntitle: Changelog\n---\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n"+
				"## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("preserves YAML front matter when no release entries exist yet", func(t *testing.T) {
		t.Parallel()

		// given: a changelog containing only YAML front matter closed with dots
		existing := "---\ntitle: Changelog\n...\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the first release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the new release is placed below the front matter
		testastic.Equal(
			t,
			"---\ntitle: Changelog\n...\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n",
			result,
		)
	})

	t.Run("ignores heading-shaped YAML comments inside front matter", func(t *testing.T) {
		t.Parallel()

		// given: YAML front matter containing comment lines shaped like release headings
		existing := "---\n# generated changelog\n## v0.1.0\ntitle: Changelog\n---\n\n" +
			"## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: front matter is kept intact and the entry is inserted before the real release
		testastic.Equal(
			t,
			"---\n# generated changelog\n## v0.1.0\ntitle: Changelog\n---\n\n"+
				"## v1.1.0 (2026-08-10)\n\nNew release.\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("preserves empty YAML front matter", func(t *testing.T) {
		t.Parallel()

		// given: a changelog starting with an empty YAML front matter block
		existing := "---\n---\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the empty front matter stays at the top without adding a heading
		testastic.Equal(
			t,
			"---\n---\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("does not treat markdown horizontal rules around releases as front matter", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name     string
			existing string
		}{
			{
				name:     "horizontal rules around release list",
				existing: "---\n\n## v1.0.0 (2026-08-09)\n\n- previous change\n\n---\n",
			},
			{
				name:     "horizontal rules around heading-only content",
				existing: "---\n\n## v1.0.0 (2026-08-09)\n\n---\n",
			},
			{
				name:     "unclosed leading horizontal rule",
				existing: "---\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// given: an untitled changelog beginning with a horizontal rule rather than YAML front matter
				newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

				// when: prepending the new release
				result := changelog.PrependEntry(tc.existing, newEntry)

				// then: the untitled changelog receives a header above the new release
				testastic.Equal(
					t,
					"# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n"+tc.existing,
					result,
				)
			})
		}
	})

	t.Run("detects an H1 title after a leading HTML comment", func(t *testing.T) {
		t.Parallel()

		// given: a changelog preceded by a markdownlint directive comment
		existing := "<!-- markdownlint-disable MD024 -->\n\n# Changelog\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the leading comment and existing H1 stay above the new release without duplicating the title
		testastic.Equal(
			t,
			"<!-- markdownlint-disable MD024 -->\n\n# Changelog\n\n"+
				"## v1.1.0 (2026-08-10)\n\nNew release.\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("detects an H1 title after a leading UTF-8 BOM", func(t *testing.T) {
		t.Parallel()

		// given: a changelog beginning with a UTF-8 byte order mark before the H1 heading
		existing := "\xef\xbb\xbf# Changelog\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the BOM stays at byte 0 and the existing H1 is reused
		testastic.Equal(
			t,
			"\xef\xbb\xbf# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n"+
				"## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("preserves a leading UTF-8 BOM on an untitled changelog", func(t *testing.T) {
		t.Parallel()

		// given: an untitled changelog beginning with a UTF-8 byte order mark
		existing := "\xef\xbb\xbf## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the BOM remains at byte 0 before the added header
		testastic.Equal(
			t,
			"\xef\xbb\xbf# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n"+
				"## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("preserves a leading UTF-8 BOM before YAML front matter", func(t *testing.T) {
		t.Parallel()

		// given: a changelog starting with both a UTF-8 BOM and YAML front matter
		existing := "\xef\xbb\xbf---\ntitle: Changelog\n---\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: both the BOM and front matter stay at the top without adding an H1
		testastic.Equal(
			t,
			"\xef\xbb\xbf---\ntitle: Changelog\n---\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n"+
				"## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("preserves CRLF YAML front matter without splitting releases", func(t *testing.T) {
		t.Parallel()

		// given: a CRLF changelog starting with YAML front matter
		existing := "---\r\ntitle: Changelog\r\n---\r\n\r\n## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: front matter, separators, and prepended entry all preserve CRLF endings
		testastic.Equal(
			t,
			"---\r\ntitle: Changelog\r\n---\r\n\r\n## v1.1.0 (2026-08-10)\r\n\r\nNew release.\r\n\r\n"+
				"## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n",
			result,
		)
	})

	t.Run("preserves CRLF line endings across changelog shapes", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name     string
			existing string
			newEntry string
			expected string
		}{
			{
				name:     "standard header and release",
				existing: "# Changelog\r\n\r\n## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n",
				newEntry: "## v1.1.0 (2026-08-10)\n\n### Features\n\n- add a thing\n",
				expected: "# Changelog\r\n\r\n## v1.1.0 (2026-08-10)\r\n\r\n### Features\r\n\r\n- add a thing\r\n\r\n" +
					"## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n",
			},
			{
				name:     "header without releases",
				existing: "# Changelog\r\n\r\nAll notable changes.\r\n",
				newEntry: "## v1.1.0 (2026-08-10)\n\nNew release.\n",
				expected: "# Changelog\r\n\r\nAll notable changes.\r\n\r\n## v1.1.0 (2026-08-10)\r\n\r\nNew release.\r\n",
			},
			{
				name:     "untitled changelog",
				existing: "## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n",
				newEntry: "## v1.1.0 (2026-08-10)\n\nNew release.\n",
				expected: "# Changelog\r\n\r\n## v1.1.0 (2026-08-10)\r\n\r\nNew release.\r\n\r\n" +
					"## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n",
			},
			{
				name:     "whitespace-only CRLF changelog",
				existing: "\r\n  \r\n",
				newEntry: "## v1.1.0 (2026-08-10)\n\nNew release.\n",
				expected: "# Changelog\r\n\r\n## v1.1.0 (2026-08-10)\r\n\r\nNew release.\r\n",
			},
			{
				name:     "UTF-8 BOM with CRLF changelog",
				existing: "\xef\xbb\xbf# Changelog\r\n\r\n## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n",
				newEntry: "## v1.1.0 (2026-08-10)\n\nNew release.\n",
				expected: "\xef\xbb\xbf# Changelog\r\n\r\n## v1.1.0 (2026-08-10)\r\n\r\nNew release.\r\n\r\n" +
					"## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n",
			},
			{
				name:     "CRLF new entry into CRLF changelog does not double CR",
				existing: "# Changelog\r\n\r\n## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n",
				newEntry: "## v1.1.0 (2026-08-10)\r\n\r\nNew release.\r\n",
				expected: "# Changelog\r\n\r\n## v1.1.0 (2026-08-10)\r\n\r\nNew release.\r\n\r\n" +
					"## v1.0.0 (2026-08-09)\r\n\r\nPrevious release.\r\n",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// given: a CRLF changelog and a release entry to prepend
				// when: prepending the new release
				result := changelog.PrependEntry(tc.existing, tc.newEntry)

				// then: the resulting changelog uses CRLF consistently without mixed endings
				testastic.Equal(t, tc.expected, result)
			})
		}
	})

	t.Run("normalizes CRLF new entry to LF for LF and empty changelogs", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name     string
			existing string
			expected string
		}{
			{
				name:     "empty changelog",
				existing: "",
				expected: "# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n",
			},
			{
				name:     "LF changelog",
				existing: "# Changelog\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
				expected: "# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n" +
					"## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// given: a new entry with CRLF line endings and an LF or empty changelog
				newEntry := "## v1.1.0 (2026-08-10)\r\n\r\nNew release.\r\n"

				// when: prepending the new release
				result := changelog.PrependEntry(tc.existing, newEntry)

				// then: the prepended entry is normalized to LF to match the file
				testastic.Equal(t, tc.expected, result)
			})
		}
	})

	t.Run("uses the dominant line ending for mixed-ending changelogs", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name     string
			existing string
			expected string
		}{
			{
				name: "mostly CRLF changelog",
				existing: "# Changelog\n\n## v1.0.0 (2026-08-09)\r\n\r\n" +
					"### Features\r\n\r\n- item one\r\n- item two\r\n",
				expected: "# Changelog\r\n\r\n## v1.1.0 (2026-08-10)\r\n\r\nNew release.\r\n\r\n" +
					"## v1.0.0 (2026-08-09)\r\n\r\n### Features\r\n\r\n- item one\r\n- item two\r\n",
			},
			{
				name: "mostly LF changelog",
				existing: "# Changelog\r\n\n## v1.0.0 (2026-08-09)\n\n" +
					"### Features\n\n- item one\n- item two\n",
				expected: "# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n" +
					"## v1.0.0 (2026-08-09)\n\n### Features\n\n- item one\n- item two\n",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// given: a changelog with mixed line endings and a new LF entry
				newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

				// when: prepending the new release
				result := changelog.PrependEntry(tc.existing, newEntry)

				// then: the prepended entry and separators match the dominant line ending
				testastic.Equal(t, tc.expected, result)
			})
		}
	})

	t.Run("detects an H1 title after leading blank lines", func(t *testing.T) {
		t.Parallel()

		// given: a changelog starting with a blank line before its H1 title
		existing := "\n# Changelog\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the existing H1 is reused instead of adding a duplicate title
		testastic.Equal(
			t,
			"\n# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("detects a Setext H1 title", func(t *testing.T) {
		t.Parallel()

		// given: a changelog titled with a Setext level-one heading
		existing := "Changelog\n=========\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n"
		newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

		// when: prepending the new release
		result := changelog.PrependEntry(existing, newEntry)

		// then: the Setext H1 is reused without adding a second title
		testastic.Equal(
			t,
			"Changelog\n=========\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n"+
				"## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})

	t.Run("does not treat non-title headings as a changelog title", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name     string
			existing string
		}{
			{
				name:     "H1 inside HTML comment",
				existing: "<!-- # Not a title -->\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			},
			{
				name:     "H1 inside fenced code block",
				existing: "```markdown\n# Example\n```\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			},
			{
				name:     "H1 after first release heading",
				existing: "## v1.0.0 (2026-08-09)\n\nPrevious release.\n\n# Appendix\n",
			},
			{
				name:     "empty H1 heading",
				existing: "# \n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// given: an untitled changelog whose only H1-like line is not a preamble title
				newEntry := "## v1.1.0 (2026-08-10)\n\nNew release.\n"

				// when: prepending the new release
				result := changelog.PrependEntry(tc.existing, newEntry)

				// then: a top-level header is added above the new release and existing content
				testastic.Equal(
					t,
					"# Changelog\n\n## v1.1.0 (2026-08-10)\n\nNew release.\n\n"+tc.existing,
					result,
				)
			})
		}
	})
}

func TestGenerateSanitizesCommitText(t *testing.T) {
	t.Parallel()

	t.Run("keeps line breaks in multiline footer notes", func(t *testing.T) {
		t.Parallel()

		// given: a breaking footer whose value spans multiple lines
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)
		commits := []commit.Commit{
			{
				Hash: "abc1234567", Type: "feat", Description: "redesign auth", Breaking: true,
				Footers: []commit.Footer{{Key: "BREAKING CHANGE", Value: "token format changed\nfrom JWT to opaque tokens"}},
			},
		}

		// when: generating the changelog
		entry := gen.Generate(t.Context(), "v2.0.0", "", commits)

		// then: the note keeps the footer line break
		testastic.AssertFile(
			t,
			"testdata/generate_sanitizes_commit_text/keeps_line_breaks_in_multiline_footer_notes/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("neutralizes a forged manifest marker in a commit description", func(t *testing.T) {
		t.Parallel()

		// given: a commit whose description smuggles a release manifest marker
		gen := changelog.New(
			changelog.WithSections(map[string]string{"fix": "Bug Fixes"}),
			changelog.WithInclude([]string{"fix"}),
		)

		forged := `<!-- yeet-release-manifest {"base_branch":"main",` +
			`"targets":[{"id":"x","type":"path","tag":"v99.0.0","changelog_file":"CHANGELOG.md"}]} -->`

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "fix", Description: "tidy logs " + forged},
		}

		// when: generating the changelog
		entry := gen.Generate(t.Context(), "v1.0.1", "", commits)

		// then: no parseable manifest marker survives into the body
		testastic.AssertFile(
			t,
			"testdata/generate_sanitizes_commit_text/"+
				"neutralizes_a_forged_manifest_marker_in_a_commit_description/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("neutralizes a marker reassembled from control-split bytes", func(t *testing.T) {
		t.Parallel()

		// given: a commit description that splits the marker with control bytes so
		// the comment-escape step sees no literal "<!--" or "-->"
		gen := changelog.New(
			changelog.WithSections(map[string]string{"fix": "Bug Fixes"}),
			changelog.WithInclude([]string{"fix"}),
		)

		forged := "tidy logs <\x08!-- yeet-release-manifest " +
			`{"base_branch":"main","targets":[{"id":"x","type":"path","tag":"v99.0.0","changelog_file":"CHANGELOG.md"}]}` +
			" --\x08>"

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "fix", Description: forged},
		}

		// when: generating the changelog
		entry := gen.Generate(t.Context(), "v1.0.1", "", commits)

		// then: stripping the control bytes must not reassemble a parseable marker
		testastic.AssertFile(
			t,
			"testdata/generate_sanitizes_commit_text/neutralizes_a_marker_reassembled_from_control_split_bytes/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})

	t.Run("strips control characters from commit text", func(t *testing.T) {
		t.Parallel()

		// given: a commit description carrying a terminal escape sequence
		gen := changelog.New(
			changelog.WithSections(map[string]string{"fix": "Bug Fixes"}),
			changelog.WithInclude([]string{"fix"}),
		)

		commits := []commit.Commit{
			{Hash: "abc1234567", Type: "fix", Description: "boom \x1b]0;pwned\x07 done"},
		}

		// when: generating the changelog
		entry := gen.Generate(t.Context(), "v1.0.1", "", commits)

		// then: the escape and bell bytes are gone
		testastic.AssertFile(
			t,
			"testdata/generate_sanitizes_commit_text/strips_control_characters_from_commit_text/body.expected.md",
			changelog.RenderSectionsForTest(entry.Sections),
		)
	})
}

func TestReferenceCaseInsensitive(t *testing.T) {
	t.Parallel()

	// given: one reference rule and footer tokens with different casing
	gen := changelog.New(
		changelog.WithSections(map[string]string{"feat": "Features"}),
		changelog.WithInclude([]string{"feat"}),
		changelog.WithReferences(changelog.References{Footers: map[string]string{
			"Refs": "https://first.example/{value}",
		}}),
	)
	c := commit.Parse(t.Context(), "abcdef0", "feat: setup\n\nREFS: 123\nRefs: 456\nrefs: 789")

	// when: rendering the references
	body := changelog.RenderBody(gen.Generate(t.Context(), "v1.1.0", "", []commit.Commit{c}))

	// then: every spelling uses the same reference rule
	testastic.Equal(t, "### Features\n\n- setup (abcdef0) ([123](https://first.example/123), "+
		"[456](https://first.example/456), [789](https://first.example/789))\n", body)
}

func TestFooterReferences(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		footers  map[string]string
		raw      string
		expected string
	}{
		{
			name:     "ignores continuation lines after a bare squash-body footer",
			footers:  map[string]string{"Closes": ""},
			raw:      "fix: handle edge case\n\nCloses #12\n\n## Testing\nRan unit tests.",
			expected: "### Bug Fixes\n\n- handle edge case (abcdef0) (#12)\n",
		},
		{
			name:     "ignores unindented prose immediately following a footer without blank line",
			footers:  map[string]string{"Closes": ""},
			raw:      "fix: handle edge case\n\nCloses #12\n## Testing\nRan unit tests.",
			expected: "### Bug Fixes\n\n- handle edge case (abcdef0) (#12)\n",
		},
		{
			name:     "keeps indented wrapped comma-separated references for bare footers",
			footers:  map[string]string{"Closes": ""},
			raw:      "fix: close wrapped issues\n\nCloses #12,\n  #34",
			expected: "### Bug Fixes\n\n- close wrapped issues (abcdef0) (#12, #34)\n",
		},
		{
			name:    "keeps indented wrapped comma-separated links before squash body",
			footers: map[string]string{"Refs": "https://jira.example.com/browse/{value}"},
			raw:     "fix: link wrapped issues\n\nRefs: PROJ-1,\n  PROJ-2\n\n## Testing\nRan unit tests.",
			expected: "### Bug Fixes\n\n- link wrapped issues (abcdef0) " +
				"([PROJ-1](https://jira.example.com/browse/PROJ-1), " +
				"[PROJ-2](https://jira.example.com/browse/PROJ-2))\n",
		},
		{
			name:    "keeps unindented line after trailing comma and stops before unindented heading",
			footers: map[string]string{"Refs": "https://jira.example.com/browse/{value}"},
			raw:     "fix: link comma-continued issues\n\nRefs: PROJ-1,\nPROJ-2\n## Testing",
			expected: "### Bug Fixes\n\n- link comma-continued issues (abcdef0) " +
				"([PROJ-1](https://jira.example.com/browse/PROJ-1), " +
				"[PROJ-2](https://jira.example.com/browse/PROJ-2))\n",
		},
		{
			name:    "keeps folded references starting on an indented second line",
			footers: map[string]string{"Refs": "https://jira.example.com/browse/{value}"},
			raw:     "fix: link folded references\n\nRefs: \n  PROJ-1, PROJ-2\n\n## Testing",
			expected: "### Bug Fixes\n\n- link folded references (abcdef0) " +
				"([PROJ-1](https://jira.example.com/browse/PROJ-1), " +
				"[PROJ-2](https://jira.example.com/browse/PROJ-2))\n",
		},
		{
			name:     "ignores CRLF continuation lines after a linked footer",
			footers:  map[string]string{"Refs": "https://jira.example.com/browse/{value}"},
			raw:      "fix: handle edge case\r\n\r\nRefs: PROJ-1\r\n\r\n## Testing\r\nRan unit tests.",
			expected: "### Bug Fixes\n\n- handle edge case (abcdef0) ([PROJ-1](https://jira.example.com/browse/PROJ-1))\n",
		},
		{
			name:     "skips a footer whose first line is blank before continuation text",
			footers:  map[string]string{"Refs": "https://jira.example.com/browse/{value}"},
			raw:      "fix: handle edge case\n\nRefs: \n\n## Testing\nRan unit tests.",
			expected: "### Bug Fixes\n\n- handle edge case (abcdef0)\n",
		},
		{
			name:     "skips a footer whose first line is blank before an unindented heading",
			footers:  map[string]string{"Refs": "https://jira.example.com/browse/{value}"},
			raw:      "fix: handle edge case\n\nRefs: \n## Testing",
			expected: "### Bug Fixes\n\n- handle edge case (abcdef0)\n",
		},
		{
			name:    "splits comma-separated values into separate links",
			footers: map[string]string{"Refs": "https://jira.example.com/browse/{value}"},
			raw:     "fix: link multiple issues\n\nRefs: PROJ-1, PROJ-2\n\n## Testing\nRan unit tests.",
			expected: "### Bug Fixes\n\n- link multiple issues (abcdef0) " +
				"([PROJ-1](https://jira.example.com/browse/PROJ-1), " +
				"[PROJ-2](https://jira.example.com/browse/PROJ-2))\n",
		},
		{
			name:    "ignores empty comma-separated segments",
			footers: map[string]string{"Refs": "https://jira.example.com/browse/{value}"},
			raw:     "fix: trim empty segments\n\nRefs: , PROJ-1 , , PROJ-2 ,",
			expected: "### Bug Fixes\n\n- trim empty segments (abcdef0) " +
				"([PROJ-1](https://jira.example.com/browse/PROJ-1), " +
				"[PROJ-2](https://jira.example.com/browse/PROJ-2))\n",
		},
		{
			name:     "splits comma-separated values for plain-text footers",
			footers:  map[string]string{"Closes": ""},
			raw:      "fix: close two issues\n\nCloses #12, #34\n\n## Testing\nRan unit tests.",
			expected: "### Bug Fixes\n\n- close two issues (abcdef0) (#12, #34)\n",
		},
		{
			name:    "percent-encodes path segment values and markdown parens",
			footers: map[string]string{"Refs": "https://tracker.example.com/browse/{value}"},
			raw:     "fix: encode path ref\n\nRefs: a/b (c) #42",
			expected: "### Bug Fixes\n\n- encode path ref (abcdef0) " +
				"([a/b (c) #42](https://tracker.example.com/browse/a%2Fb%20%28c%29%20%2342))\n",
		},
		{
			name:    "percent-encodes query parameter values",
			footers: map[string]string{"Refs": "https://tracker.example.com/search?q={value}"},
			raw:     "fix: encode query ref\n\nRefs: a&b=c + d",
			expected: "### Bug Fixes\n\n- encode query ref (abcdef0) " +
				"([a&b=c + d](https://tracker.example.com/search?q=a%26b%3Dc%20%2B%20d))\n",
		},
		{
			name:    "percent-encodes fragment values after query parameters",
			footers: map[string]string{"Refs": "https://tracker.example.com/issues?a=1#{value}"},
			raw:     "fix: encode fragment ref\n\nRefs: sec (1)",
			expected: "### Bug Fixes\n\n- encode fragment ref (abcdef0) " +
				"([sec (1)](https://tracker.example.com/issues?a=1#sec%20%281%29))\n",
		},
		{
			name:    "escapes markdown link label brackets and comment markers",
			footers: map[string]string{"Refs": "https://tracker.example.com/browse/{value}"},
			raw:     "fix: escape link label\n\nRefs: PROJ]1, a<!--b-->, a\\b[c]",
			expected: "### Bug Fixes\n\n- escape link label (abcdef0) " +
				"([PROJ\\]1](https://tracker.example.com/browse/PROJ%5D1), " +
				"[a&lt;!--b--&gt;](https://tracker.example.com/browse/a%3C%21--b--%3E), " +
				"[a\\\\b\\[c\\]](https://tracker.example.com/browse/a%5Cb%5Bc%5D))\n",
		},
		{
			name:    "escapes angle brackets and backticks in markdown link labels",
			footers: map[string]string{"Reported-by": "https://tracker.example.com/search?q={value}"},
			raw: "fix: escape autolinks and code spans\n\n" +
				"Reported-by: Alice <alice@example.com>, <https://example.com/123>, `a, b`, `PROJ-1]`",
			expected: "### Bug Fixes\n\n- escape autolinks and code spans (abcdef0) " +
				"([Alice \\<alice@example.com>](https://tracker.example.com/search?q=Alice%20%3Calice%40example.com%3E), " +
				"[\\<https://example.com/123>](https://tracker.example.com/search?q=%3Chttps%3A%2F%2Fexample.com%2F123%3E), " +
				"[\\`a](https://tracker.example.com/search?q=%60a), " +
				"[b\\`](https://tracker.example.com/search?q=b%60), " +
				"[\\`PROJ-1\\]\\`](https://tracker.example.com/search?q=%60PROJ-1%5D%60))\n",
		},
		{
			name:    "encodes multiple placeholders across path, query, and fragment",
			footers: map[string]string{"Refs": "https://tracker.example.com/{value}?q={value}#{value}"},
			raw:     "fix: encode multi-placeholder url\n\nRefs: a/b&c (1)",
			expected: "### Bug Fixes\n\n- encode multi-placeholder url (abcdef0) " +
				"([a/b&c (1)](https://tracker.example.com/a%2Fb&c%20%281%29?q=a%2Fb%26c%20%281%29#a%2Fb&c%20%281%29))\n",
		},
		{
			name:    "strips control characters and neutralizes control-split comment markers",
			footers: map[string]string{"Refs": "https://tracker.example.com/browse/{value}"},
			raw:     "fix: strip control chars\n\nRefs: a\x07, <\x08!--b--\x08>, \x07",
			expected: "### Bug Fixes\n\n- strip control chars (abcdef0) " +
				"([a](https://tracker.example.com/browse/a), " +
				"[&lt;!--b--&gt;](https://tracker.example.com/browse/%3C%21--b--%3E))\n",
		},
		{
			name:     "keeps static URL template without placeholder",
			footers:  map[string]string{"Refs": "https://tracker.example.com/issues"},
			raw:      "fix: static url\n\nRefs: PROJ-1",
			expected: "### Bug Fixes\n\n- static url (abcdef0) ([PROJ-1](https://tracker.example.com/issues))\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: a generator configured with footer reference templates and a parsed commit
			gen := changelog.New(
				changelog.WithSections(map[string]string{"fix": "Bug Fixes"}),
				changelog.WithInclude([]string{"fix"}),
				changelog.WithReferences(changelog.References{Footers: tc.footers}),
			)
			c := commit.Parse(t.Context(), "abcdef0", tc.raw)

			// when: generating the changelog body
			body := changelog.RenderBody(gen.Generate(t.Context(), "v1.0.1", "", []commit.Commit{c}))

			// then: the first line and any indented or comma-continued lines are split into safe Markdown references
			testastic.Equal(t, tc.expected, body)
		})
	}
}
