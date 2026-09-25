package changelog_test

import (
	"strings"
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
)

func TestGenerateNotes(t *testing.T) {
	t.Parallel()

	t.Run("renders the layered entry layout", func(t *testing.T) {
		t.Parallel()

		// given: breaking and non-breaking commits with notes, including a breaking type outside include
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features", "refactor": "Code Refactoring"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithRepoURL("https://github.com/org/repo"),
			changelog.WithCompareURL(func(from, to string) string {
				return "https://github.com/org/repo/compare/" + from + "..." + to
			}),
			changelog.WithDate(time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC)),
		)
		commits := []commit.Commit{
			{
				Hash: "0a1b2c3", Type: "feat", Scope: "release", Description: "add auto-merge",
				Note: "Set `release.auto_merge: true` to merge release PRs automatically.",
			},
			{
				Hash: "abc1234", Type: "feat", Scope: "config", Description: "rename targets to units", Breaking: true,
				Note: "Rename `targets` to `units` in `yeet.yaml`:\n\n```yaml\nunits:\n  - path: .\n```",
			},
			{Hash: "def5678", Type: "feat", Description: "drop Go 1.24 support", Breaking: true},
			{Hash: "1a2b3c4", Type: "refactor", Description: "remove legacy provider flag", Breaking: true},
			{Hash: "9f8e7d6", Type: "refactor", Description: "tidy internals"},
		}

		// when: generating the entry and rendering it with a manual intro
		entry := gen.Generate(t.Context(), "v2.0.0", "v1.4.0", commits)
		entry.Intro = []string{"Units replace targets. Most configs need a one-line rename."}

		// then: intro, breaking changes and type sections render in order with notes quoted under their bullets
		testastic.AssertFile(t, "testdata/generate_notes/renders_the_layered_entry_layout/entry.expected.md",
			changelog.Render(entry))
	})

	t.Run("chooses the note source by priority", func(t *testing.T) {
		t.Parallel()

		// given: commits with a fence and a footer, a footer only, a fence only and neither
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features", "fix": "Bug Fixes"}),
			changelog.WithInclude([]string{"feat", "fix"}),
		)
		commits := []commit.Commit{
			{Hash: "1111111", Type: "fix", Description: "handle nil config", Note: "Nil configs now load defaults."},
			{
				Hash: "2222222", Type: "fix", Description: "drop legacy flag", Breaking: true,
				Footers: []commit.Footer{{Key: "BREAKING CHANGE", Value: "Remove `--legacy`."}},
			},
			{
				Hash: "3333333", Type: "feat", Scope: "api", Description: "rename endpoint", Breaking: true,
				Note:    "Call `/v2/units` instead of `/v1/targets`.",
				Footers: []commit.Footer{{Key: "BREAKING-CHANGE", Value: "footer text is not used"}},
			},
			{Hash: "4444444", Type: "feat", Description: "add flag"},
		}

		// when: generating the sections
		entry := gen.Generate(t.Context(), "v2.0.0", "", commits)

		// then: fences win over footers, commits without either have no note, breaking notes stay in their section
		testastic.AssertFile(t, "testdata/generate_notes/chooses_the_note_source_by_priority/body.expected.md",
			changelog.RenderSections(entry.Sections))
	})

	t.Run("keeps footer line breaks and escapes footer headings", func(t *testing.T) {
		t.Parallel()

		// given: a multi-line breaking footer with heading-like lines
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)
		c := commit.Parse(t.Context(), "abc1234567",
			"feat: new API\n\nBREAKING CHANGE: Migrate:\n# Steps\n  ### Indented\nUpdate the client.\n---\n#hashtag stays")

		// when: generating the sections
		entry := gen.Generate(t.Context(), "v2.0.0", "", []commit.Commit{c})

		// then: the note keeps its lines and renders every heading as text
		testastic.AssertFile(t,
			"testdata/generate_notes/keeps_footer_line_breaks_and_escapes_footer_headings/body.expected.md",
			changelog.RenderSections(entry.Sections))
	})

	t.Run("escapes an unclosed fence in footer notes", func(t *testing.T) {
		t.Parallel()

		// given: breaking footers with an unclosed fence and with a closed fence holding a heading-like line
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)
		commits := []commit.Commit{
			commit.Parse(t.Context(), "1111111", "feat!: open\n\nBREAKING CHANGE: use\n```yaml\nkey: v"),
			commit.Parse(t.Context(), "2222222", "feat!: closed\n\nBREAKING CHANGE: run\n```sh\n# comment\n```"),
		}

		// when: generating the sections
		entry := gen.Generate(t.Context(), "v2.0.0", "", commits)

		// then: the unclosed fence renders as text and the closed block stays verbatim
		testastic.AssertFile(t, "testdata/generate_notes/escapes_an_unclosed_fence_in_footer_notes/body.expected.md",
			changelog.RenderSections(entry.Sections))
	})

	t.Run("escapes headings exposed by earlier heading escapes", func(t *testing.T) {
		t.Parallel()

		// given: a footer whose ATX heading becomes a setext heading after escaping
		gen := changelog.New()
		c := commit.Parse(t.Context(), "abc1234", "feat!: config changed\n\nBREAKING CHANGE: config changed\n# Steps\n---")

		// when: rendering the breaking note
		entry := gen.Generate(t.Context(), "v2.0.0", "", []commit.Commit{c})

		// then: both heading markers are escaped
		testastic.Contains(t, changelog.RenderSections(entry.Sections), "  > \\# Steps\n  > \\---")
	})

	t.Run("preserves comment markers in note code", func(t *testing.T) {
		t.Parallel()

		// given: a release note and a footer containing code with literal comment markers
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)
		note := "Use `<!-- A -->`.\n\n```mermaid\nA --> B\n```\n\n    <!-- indented -->\n\n<!-- prose -->"
		commits := []commit.Commit{
			{Hash: "1111111", Type: "feat", Description: "new note", Note: note},
			{
				Hash: "2222222", Type: "feat", Description: "new footer", Breaking: true,
				Footers: []commit.Footer{{Key: "BREAKING CHANGE", Value: note}},
			},
		}

		// when: rendering both note sources
		entry := gen.Generate(t.Context(), "v2.0.0", "", commits)
		rendered := changelog.RenderSections(entry.Sections)

		// then: code stays literal and prose markers are escaped for both entries
		quoted := "  > Use `<!-- A -->`.\n  >\n  > ```mermaid\n  > A --> B\n  > ```\n  >\n" +
			"  >     <!-- indented -->\n  >\n  > &lt;!-- prose --&gt;"
		testastic.Equal(t, 2, strings.Count(rendered, quoted))
	})

	t.Run("renders nested code blocks from longer and tilde outer fences", func(t *testing.T) {
		t.Parallel()

		// given: commits whose release-note fences nest code blocks
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)
		messages := []string{
			"feat: backtick outer\n\n````release-note\n#### Before\n\n```yaml\ntargets: []\n```\n````",
			"feat: tilde outer\n\n~~~release-note\n```sh\nyeet release\n```\n~~~",
		}
		commits := make([]commit.Commit, 0, len(messages))

		for idx, message := range messages {
			c, err := commit.ParseWithReleaseNote(t.Context(), []string{"aaaaaaa", "bbbbbbb"}[idx], message)
			testastic.NoError(t, err)

			commits = append(commits, c)
		}

		// when: generating the sections
		entry := gen.Generate(t.Context(), "v1.1.0", "", commits)

		// then: the inner code blocks render verbatim
		testastic.AssertFile(t,
			"testdata/generate_notes/renders_nested_code_blocks_from_longer_and_tilde_outer_fences/body.expected.md",
			changelog.RenderSections(entry.Sections))
	})

	t.Run("sanitizes control characters and comment markers in notes", func(t *testing.T) {
		t.Parallel()

		// given: a note with control characters, CRLF line endings and a forged manifest marker
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)
		commits := []commit.Commit{{
			Hash: "abc1234567", Type: "feat", Description: "add flag",
			Note: "First\x00 line\r\n\x1b[31mred\x1b[0m\r\n<!-- yeet-release-manifest -->\n\tindented",
		}}

		// when: generating the sections
		entry := gen.Generate(t.Context(), "v1.1.0", "", commits)

		// then: control characters are stripped, markers escaped, newlines and tabs kept
		testastic.AssertFile(t,
			"testdata/generate_notes/sanitizes_control_characters_and_comment_markers_in_notes/body.expected.md",
			changelog.RenderSections(entry.Sections))
	})

	t.Run("lists breaking commits of types outside include only in the breaking section", func(t *testing.T) {
		t.Parallel()

		// given: breaking commits whose types are neither included nor mapped
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)
		commits := []commit.Commit{
			{Hash: "1111111", Type: "security", Description: "require TLS 1.3", Breaking: true},
			{Hash: "2222222", Type: "feat", Description: "add flag"},
			{Hash: "3333333", Type: "security", Scope: "auth", Description: "drop basic auth", Breaking: true},
			{Hash: "4444444", Type: "security", Description: "rotate keys"},
		}

		// when: generating the sections
		entry := gen.Generate(t.Context(), "v2.0.0", "", commits)

		// then: the breaking commits appear in the breaking section and get no type section
		testastic.Equal(t, "### ⚠ BREAKING CHANGES\n\n- require TLS 1.3 (1111111)\n- **auth:** drop basic auth (3333333)\n\n"+
			"### Features\n\n- add flag (2222222)\n",
			changelog.RenderSections(entry.Sections))
	})

	t.Run("keeps an entry that ends with breaking notes stable across refreshes", func(t *testing.T) {
		t.Parallel()

		// given: a breaking commit outside include with a note, and a manual outro after the entry
		gen := changelog.New(changelog.WithInclude([]string{"feat"}))
		commits := []commit.Commit{{
			Hash: "abc1234", Type: "refactor", Description: "drop legacy", Breaking: true,
			Note: "Remove legacy mode.\n\nSwitch to the new mode.",
		}}
		initial := gen.Generate(t.Context(), "v2.0.0", "", commits)
		initial.Outro = []string{"Thanks for upgrading."}

		// when: refreshing the entry twice
		first := changelog.Merge(gen.Generate(t.Context(), "v2.0.0", "", commits),
			changelog.ParseEntry(changelog.Render(initial)))
		second := changelog.Merge(gen.Generate(t.Context(), "v2.0.0", "", commits),
			changelog.ParseEntry(changelog.Render(first)))

		// then: the note appears once and the outro stays last
		testastic.Equal(t, "### ⚠ BREAKING CHANGES\n\n- drop legacy (abc1234)\n  > Remove legacy mode.\n  >\n"+
			"  > Switch to the new mode.\n\nThanks for upgrading.\n", changelog.RenderBody(second))
	})

	t.Run("escapes unclosed HTML blocks in footer notes", func(t *testing.T) {
		t.Parallel()

		// given: a breaking footer that opens an HTML block it never closes
		gen := changelog.New(changelog.WithInclude([]string{"feat"}))
		c := commit.Parse(t.Context(), "abc1234", "refactor!: drop legacy\n\nBREAKING CHANGE: run\n<pre>\nyeet migrate")

		// when: generating the entry and indexing it inside a changelog
		entry := gen.Generate(t.Context(), "v2.0.0", "", []commit.Commit{c})
		doc := changelog.Prepend("", changelog.Render(entry)+"\n## v1.0.0\n\n### Features\n\n- init (def5678)\n")

		// then: the opening tag renders as text and later entries stay reachable
		testastic.Equal(t, "### ⚠ BREAKING CHANGES\n\n- drop legacy (abc1234)\n  > run\n  > \\<pre>\n  > yeet migrate\n",
			changelog.RenderSections(entry.Sections))

		older, err := changelog.EntryByTag(doc, "v1.0.0")
		testastic.NoError(t, err)
		testastic.Contains(t, older, "- init (def5678)")
	})

	t.Run("escapes nested unclosed HTML blocks in footer notes", func(t *testing.T) {
		t.Parallel()

		// given: a breaking footer with unclosed HTML blocks inside a blockquote and a list
		gen := changelog.New(changelog.WithInclude([]string{"feat"}))
		c := commit.Commit{
			Hash: "abc1234", Type: "refactor", Description: "drop legacy", Breaking: true,
			Footers: []commit.Footer{{
				Key:   commit.FooterBreakingChange,
				Value: "run\n> <pre>\n> yeet migrate\n\n- <script>\n  x",
			}},
		}

		// when: generating the entry
		entry := gen.Generate(t.Context(), "v2.0.0", "", []commit.Commit{c})

		// then: the opening tags render as text and the container markers stay intact
		testastic.Equal(t, "### ⚠ BREAKING CHANGES\n\n- drop legacy (abc1234)\n  > run\n  > > \\<pre>\n"+
			"  > > yeet migrate\n  >\n  > - \\<script>\n  >   x\n", changelog.RenderSections(entry.Sections))
	})

	t.Run("escapes nested headings in footer notes", func(t *testing.T) {
		t.Parallel()

		// given: a breaking footer with headings inside a blockquote, a list and a nested setext heading
		gen := changelog.New(changelog.WithInclude([]string{"feat"}))
		c := commit.Commit{
			Hash: "abc1234", Type: "refactor", Description: "drop legacy", Breaking: true,
			Footers: []commit.Footer{{
				Key:   commit.FooterBreakingChange,
				Value: "steps\n> # Quote\n- ## Item\n- Title\n  ===",
			}},
		}

		// when: generating the entry
		entry := gen.Generate(t.Context(), "v2.0.0", "", []commit.Commit{c})

		// then: every heading renders as text
		testastic.Equal(t, "### ⚠ BREAKING CHANGES\n\n- drop legacy (abc1234)\n  > steps\n"+
			"  > > \\# Quote\n  > - \\## Item\n  > - Title\n  >   \\===\n", changelog.RenderSections(entry.Sections))
	})

	t.Run("lists breaking commits without notes as bullets", func(t *testing.T) {
		t.Parallel()

		// given: commits without notes or breaking footers
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
		)
		commits := []commit.Commit{{Hash: "1111111", Type: "feat", Description: "add flag", Breaking: true}}

		// when: generating the sections
		entry := gen.Generate(t.Context(), "v2.0.0", "", commits)

		// then: the breaking section has a bullet and there is no notes section
		testastic.Equal(t, "### ⚠ BREAKING CHANGES\n\n- add flag (1111111)\n\n### Features\n\n- add flag (1111111)\n",
			changelog.RenderSections(entry.Sections))
	})

	t.Run("regenerates the breaking section of a release branch entry", func(t *testing.T) {
		t.Parallel()

		// given: a release branch entry with an intro, an outdated breaking section and a manual section
		gen := changelog.New(
			changelog.WithSections(map[string]string{"feat": "Features"}),
			changelog.WithInclude([]string{"feat"}),
			changelog.WithDate(time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC)),
		)
		commits := []commit.Commit{{
			Hash: "abc1234", Type: "feat", Description: "new API", Breaking: true,
			Note: "Update clients:\n\n```sh\n### not a section\nyeet migrate\n```",
		}}
		existing := changelog.ParseEntry("## v2.0.0 (2026-09-24)\n\nA short summary.\n\n" +
			"### ⚠ BREAKING CHANGES\n\n- old endpoints removed (abc1234)\n\n" +
			"### Upgrade Guide\n\nRead the docs.\n\n" +
			"### Features\n\n- new API (abc1234)\n")

		// when: merging the regenerated entry twice, as consecutive refreshes do
		merged := changelog.Merge(gen.Generate(t.Context(), "v2.0.0", "", commits), existing)
		refreshed := changelog.Merge(gen.Generate(t.Context(), "v2.0.0", "", commits),
			changelog.ParseEntry(changelog.Render(merged)))

		// then: the breaking section is regenerated while the intro and manual section survive unchanged
		testastic.AssertFile(t,
			"testdata/generate_notes/regenerates_the_breaking_section_of_a_release_branch_entry/entry.expected.md",
			changelog.Render(merged))
		testastic.Equal(t, changelog.Render(merged), changelog.Render(refreshed))
	})
}
