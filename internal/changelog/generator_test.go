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

		date := time.Date(2025, time.December, 31, 16, 30, 0, 0, time.FixedZone("PST", -8*60*60))
		gen := changelog.New(changelog.WithDate(date))

		entry := gen.Generate(t.Context(), "v1.2.3", "", nil)

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
			changelog.RenderSections(entry.Sections),
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

		// then: breaking changes section uses release-please style header
		testastic.AssertFile(
			t,
			"testdata/generate/includes_breaking_changes_section/body.expected.md",
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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

		// then: reference appears in breaking changes section
		testastic.AssertFile(
			t,
			"testdata/generate/references_in_breaking_changes_section/body.expected.md",
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
		)
	})
}

func TestRender(t *testing.T) {
	t.Parallel()

	t.Run("normalizes only freeform block edges", func(t *testing.T) {
		t.Parallel()

		entry := changelog.Entry{
			Intro: []string{"", "First intro paragraph.", "", "", "Second intro paragraph.", ""},
			Sections: []changelog.Section{{
				Heading: "Bug Fixes",
				Lines:   []string{"- patch issue (abc1234)"},
			}},
			Outro: []string{"", "First outro paragraph.", "", "Second outro paragraph.", ""},
		}

		output := changelog.RenderBody(entry)

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

func TestPrepend(t *testing.T) {
	t.Parallel()

	t.Run("prepend to empty changelog", func(t *testing.T) {
		t.Parallel()

		// given: no existing changelog
		newEntry := readTestFile(t, "testdata/prepend/prepend_to_empty_changelog/new_entry.input.md")

		// when: prepending
		result := changelog.Prepend("", newEntry)

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
		result := changelog.Prepend(existing, newEntry)

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
		result := changelog.Prepend(existing, newEntry)

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
		result := changelog.Prepend(existing, newEntry)

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
		result := changelog.Prepend(existing, newEntry)

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
		result := changelog.Prepend(existing, newEntry)

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
		result := changelog.Prepend(existing, newEntry)

		// then: the complete preamble remains above the new release
		testastic.Equal(
			t,
			"# Changelog\n\nExample format:\n\n```markdown\n## v0.0.0 (example)\nExample notes.\n```\n\n"+
				"## v1.1.0 (2026-08-10)\n\nNew release.\n\n## v1.0.0 (2026-08-09)\n\nPrevious release.\n",
			result,
		)
	})
}

func TestGenerateSanitizesCommitText(t *testing.T) {
	t.Parallel()

	t.Run("preserves word boundaries in multiline footer values", func(t *testing.T) {
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

		// then: the removed newline leaves a space between the adjacent words
		testastic.AssertFile(
			t,
			"testdata/generate_sanitizes_commit_text/preserves_word_boundaries_in_multiline_footer_values/body.expected.md",
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
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
			changelog.RenderSections(entry.Sections),
		)
	})
}
