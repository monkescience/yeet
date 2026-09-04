//nolint:testpackage // This test validates unexported release text behavior.
package release

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/monkescience/testastic"
	changelogpkg "github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
)

func TestReleasePRBody(t *testing.T) {
	t.Parallel()

	t.Run("defaults include generated header and footer", func(t *testing.T) {
		t.Parallel()

		// given: releaser with default config
		r := newTestReleaser(t, config.Default(), newProviderStub())
		changelogBody := readTestFile(
			t,
			"testdata/release_p_r_body/defaults_include_generated_header_and_footer/"+
				"changelog.input.md",
		)

		// when: building PR body
		body, truncated, err := r.lifecycle.text.releasePRBody(changelogBody, "<!-- yeet-release-tag: v1.2.4 -->", 0)

		// then: changelog is wrapped by default header, manifest, and footer notes
		testastic.NoError(t, err)
		testastic.False(t, truncated)
		testastic.AssertFile(
			t,
			"testdata/release_p_r_body/defaults_include_generated_header_and_footer/body.expected.md",
			body,
		)
	})

	t.Run("custom header and footer surround changelog", func(t *testing.T) {
		t.Parallel()

		// given: releaser with custom PR body wrapper text
		cfg := config.Default()
		cfg.Release.PRBodyHeader = "Header"
		cfg.Release.PRBodyFooter = "Footer"

		r := newTestReleaser(t, cfg, newProviderStub())

		// when: building PR body
		body, truncated, err := r.lifecycle.text.releasePRBody("## v1.2.4", "<!-- yeet-release-tag: v1.2.4 -->", 0)

		// then: body contains header, changelog, manifest, and footer in order
		testastic.NoError(t, err)
		testastic.False(t, truncated)
		testastic.AssertFile(
			t,
			"testdata/release_p_r_body/custom_header_and_footer_surround_changelog/body.expected.md",
			body,
		)
	})

	t.Run("empty wrapper fields collapse to changelog and manifest only", func(t *testing.T) {
		t.Parallel()

		// given: releaser with both wrapper fields disabled
		cfg := config.Default()
		cfg.Release.PRBodyHeader = ""
		cfg.Release.PRBodyFooter = ""

		r := newTestReleaser(t, cfg, newProviderStub())

		// when: building PR body
		body, truncated, err := r.lifecycle.text.releasePRBody("## v1.2.4\n", "<!-- yeet-release-tag: v1.2.4 -->", 0)

		// then: body keeps only changelog and manifest, no wrapper text
		testastic.NoError(t, err)
		testastic.False(t, truncated)
		testastic.AssertFile(
			t,
			"testdata/release_p_r_body/empty_wrapper_fields_collapse_to_changelog_and_manifest_only/body.expected.md",
			body,
		)
	})

	t.Run("body within limit is not truncated", func(t *testing.T) {
		t.Parallel()

		// given: a releaser with default config and a short changelog
		r := newTestReleaser(t, config.Default(), newProviderStub())
		marker := testManifestBody(t, "v1.2.4", "CHANGELOG.md")

		// when: building the body with a generous limit
		body, truncated, err := r.lifecycle.text.releasePRBody(readTestFile(
			t,
			"testdata/release_p_r_body/body_within_limit_is_not_truncated/"+
				"changelog.input.md",
		), marker, 4000)

		// then: nothing is truncated and the marker survives
		testastic.NoError(t, err)
		testastic.False(t, truncated)
		testastic.AssertFile(
			t,
			"testdata/release_p_r_body/body_within_limit_is_not_truncated/body.expected.md",
			body,
		)
		assertSingleManifestTag(t, body, "v1.2.4")
	})

	t.Run("oversized body drops the whole changelog but preserves marker, header, and footer", func(t *testing.T) {
		t.Parallel()

		// given: a releaser with default config and a changelog far larger than the limit
		r := newTestReleaser(t, config.Default(), newProviderStub())
		marker := testManifestBody(t, "v1.2.4", "CHANGELOG.md")

		var changelog strings.Builder
		changelog.WriteString(readTestFile(
			t,
			"testdata/release_p_r_body/"+
				"oversized_body_drops_the_whole_changelog_but_preserves_marker__header__and_footer/"+
				"changelog_header.input.md",
		))

		for line := range 500 {
			fmt.Fprintf(&changelog, "- feature number %d that adds yet more text to the notes\n", line)
		}

		// when: building the body with the Azure DevOps limit
		body, omitted, err := r.lifecycle.text.releasePRBody(changelog.String(), marker, 4000)

		// then: the body fits, no changelog lines survive, and the wrapper text plus notice remain
		testastic.NoError(t, err)
		testastic.True(t, omitted)
		testastic.True(t, utf8.RuneCountInString(body) <= 4000)
		testastic.AssertFile(
			t,
			"testdata/release_p_r_body/"+
				"oversized_body_drops_the_whole_changelog_but_preserves_marker__header__and_footer/"+
				"body.expected.md",
			body,
		)
		assertSingleManifestTag(t, body, "v1.2.4")
	})

	t.Run("body one character over the limit drops the changelog entirely", func(t *testing.T) {
		t.Parallel()

		// given: a config with no header or footer and a changelog longer than the omission notice
		cfg := config.Default()
		cfg.Release.PRBodyHeader = ""
		cfg.Release.PRBodyFooter = ""

		r := newTestReleaser(t, cfg, newProviderStub())
		marker := testManifestBody(t, "v1.2.4", "CHANGELOG.md")
		changelog := readTestFile(
			t,
			"testdata/release_p_r_body/"+
				"body_one_byte_over_the_limit_drops_the_changelog_entirely/"+
				"changelog.input.md",
		) + strings.Repeat("x", utf8.RuneCountInString(prBodyOmittedNotice))

		full := changelog + "\n\n" + marker
		limit := utf8.RuneCountInString(full) - 1

		// when: building the body with a limit one character under the full body
		body, omitted, err := r.lifecycle.text.releasePRBody(changelog, marker, limit)

		// then: the changelog is dropped wholesale, the notice and marker remain
		testastic.NoError(t, err)
		testastic.True(t, omitted)
		testastic.AssertFile(
			t,
			"testdata/release_p_r_body/body_one_byte_over_the_limit_drops_the_changelog_entirely/body.expected.md",
			body,
		)
		assertSingleManifestTag(t, body, "v1.2.4")
	})

	t.Run("body limit counts Unicode characters", func(t *testing.T) {
		t.Parallel()

		// given: a body whose byte length exceeds its character length
		cfg := config.Default()
		cfg.Release.PRBodyHeader = ""
		cfg.Release.PRBodyFooter = ""

		r := newTestReleaser(t, cfg, newProviderStub())
		marker := testManifestBody(t, "v1.2.4", "CHANGELOG.md")
		changelog := strings.Repeat("é", 2000)
		fullBody := joinPRBodyParts(changelog, marker)
		limit := utf8.RuneCountInString(fullBody)

		// when: building the body at its exact character limit
		body, omitted, err := r.lifecycle.text.releasePRBody(changelog, marker, limit)

		// then: the changelog is preserved because the character limit is not exceeded
		testastic.NoError(t, err)
		testastic.False(t, omitted)
		testastic.Equal(t, fullBody, body)
	})

	t.Run("fallback cannot exceed the body limit", func(t *testing.T) {
		t.Parallel()

		// given: configured wrapper text that cannot fit beside the required manifest
		cfg := config.Default()
		cfg.Release.PRBodyHeader = strings.Repeat("h", 4001)
		cfg.Release.PRBodyFooter = ""

		r := newTestReleaser(t, cfg, newProviderStub())
		marker := testManifestBody(t, "v1.2.4", "CHANGELOG.md")

		// when: building the body at the provider limit
		body, omitted, err := r.lifecycle.text.releasePRBody("release notes", marker, 4000)

		// then: validation rejects the impossible fallback without returning an oversized body
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.False(t, omitted)
		testastic.Equal(t, "", body)
	})
}

func TestReleaseTextRender(t *testing.T) {
	t.Parallel()

	t.Run("validate and render agree at the same body limit", func(t *testing.T) {
		t.Parallel()

		// given: configured templates and one complete release plan
		cfg := config.Default()
		cfg.Release.PRTitle = "PR {{ .Tag }}"
		cfg.Release.CommitSubject = "commit {{ .Version }}"
		cfg.Release.NameTemplate = "release {{ .Target }} {{ .Version }}"
		r := newTestReleaser(t, cfg, newProviderStub())
		plans := []TargetPlan{{
			ID: "default", Type: config.TargetTypePath, NextVersion: "1.2.4", NextTag: "v1.2.4",
			ChangelogFile: "CHANGELOG.md",
			PREntry: changelogpkg.ParseEntry(
				"## v1.2.4 (2026-03-01)\n\n### Features\n\n- add rendering\n",
			),
		}}

		// when: validating and rendering with the same provider limit
		validateErr := r.lifecycle.text.validate(plans, 4000)
		rendered, renderErr := r.lifecycle.text.render(plans, "existing-release-branch", 4000, combinedReleaseUnitID)

		// then: both calls succeed and render returns the complete publication values
		testastic.NoError(t, validateErr)
		testastic.NoError(t, renderErr)
		testastic.Equal(t, "PR v1.2.4", rendered.PROptions.Title)
		testastic.Equal(t, "existing-release-branch", rendered.PROptions.ReleaseBranch)
		testastic.Equal(t, "commit 1.2.4", rendered.CommitSubject)
		testastic.Equal(t, "release default 1.2.4", rendered.ReleaseNames["default"])
	})

	t.Run("a render failure returns no partial result", func(t *testing.T) {
		t.Parallel()

		// given: a template that renders empty for the stable release channel
		cfg := config.Default()
		cfg.Release.PRTitle = "{{ if .Channel }}release {{ .Tag }}{{ end }}"
		r := newTestReleaser(t, cfg, newProviderStub())
		plans := []TargetPlan{{ID: "default", NextVersion: "1.2.4", NextTag: "v1.2.4"}}

		// when: validating and rendering the same release data
		validateErr := r.lifecycle.text.validate(plans, 4000)
		rendered, renderErr := r.lifecycle.text.render(plans, r.core.run.releaseBranch, 4000, combinedReleaseUnitID)

		// then: both report the same failure and no partial value escapes
		testastic.ErrorIs(t, validateErr, config.ErrInvalidConfig)
		testastic.ErrorIs(t, renderErr, config.ErrInvalidConfig)
		testastic.Equal(t, validateErr.Error(), renderErr.Error())
		testastic.Equal(t, (*RenderedRelease)(nil), rendered)
	})
}

func TestReleaseTextRenderOmittingNotesDoesNotLog(t *testing.T) {
	// given: release notes larger than the provider body limit and a warning logger
	cfg := config.Default()
	cfg.Release.PRBodyHeader = ""
	cfg.Release.PRBodyFooter = ""
	r := newTestReleaser(t, cfg, newProviderStub())
	plans := []TargetPlan{{
		ID: "default", Type: config.TargetTypePath, NextVersion: "1.2.4", NextTag: "v1.2.4",
		ChangelogFile: "CHANGELOG.md",
		PREntry: changelogpkg.ParseEntry(
			"## v1.2.4 (2026-03-01)\n\n" + strings.Repeat("release note ", 500),
		),
	}}
	warnings := captureWarnings(t)

	// when: rendering at the provider limit
	rendered, err := r.lifecycle.text.render(plans, r.core.run.releaseBranch, 4000, combinedReleaseUnitID)

	// then: the result reports omission and the pure renderer emits no warning
	testastic.NoError(t, err)
	testastic.True(t, rendered.NotesOmitted)
	testastic.Equal(t, "", warnings.String())
}

func assertSingleManifestTag(t *testing.T, body, wantTag string) {
	t.Helper()

	const markerPrefix = "<!-- yeet-release-manifest"

	marker := markerPrefix + "\n" +
		`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"` + wantTag +
		`","changelog_file":"CHANGELOG.md"}]}` +
		"\n-->"

	testastic.Equal(t, 1, strings.Count(body, markerPrefix))
	testastic.Equal(t, 1, strings.Count(body, marker))
}

func TestEffectivePRBodyLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configured    int
		providerLimit int
		want          int
	}{
		{name: "both unbounded", configured: 0, providerLimit: 0, want: 0},
		{name: "uses provider limit when config is unbounded", configured: 0, providerLimit: 4000, want: 4000},
		{name: "uses config limit when provider is unbounded", configured: 2000, providerLimit: 0, want: 2000},
		{name: "config tighter than provider", configured: 1000, providerLimit: 4000, want: 1000},
		{name: "provider tighter than config", configured: 8000, providerLimit: 4000, want: 4000},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a releaser configured with the test's pr_body_max_length cap
			cfg := config.Default()
			cfg.Release.PRBodyMaxLength = test.configured
			r := newTestReleaser(t, cfg, newProviderStub())

			// when: resolving the effective limit against the provider hard limit
			limit := r.lifecycle.text.effectivePRBodyLimit(test.providerLimit)

			// then: the smaller bounding limit wins
			testastic.Equal(t, test.want, limit)
		})
	}
}

func TestCombinedPRChangelog(t *testing.T) {
	t.Parallel()

	t.Run("single target preserves existing changelog format", func(t *testing.T) {
		t.Parallel()

		// given: a single target release result
		r := newTestReleaser(t, config.Default(), newProviderStub())
		prChangelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/combined_p_r_changelog/"+
				"single_target_preserves_existing_changelog_format/pr_changelog.input.md",
		)) + "\n"

		result := &Result{
			BaseBranch: "main",
			Plans: []TargetPlan{{
				ID:      "default",
				Type:    "path",
				PREntry: changelogpkg.ParseEntry(prChangelog),
			}},
		}

		// when: rendering the combined PR changelog
		body := r.lifecycle.text.combinedPRChangelog(result.Plans)

		// then: the single-target changelog stays unchanged
		testastic.Equal(t, prChangelog, body)
	})

	t.Run("multi target includes wave summary and detailed target sections", func(t *testing.T) {
		t.Parallel()

		// given: a multi-target release wave with a derived root target
		r := newTestReleaser(t, config.Default(), newProviderStub())
		result := &Result{
			BaseBranch: "main",
			Plans: []TargetPlan{
				{
					ID:             "api",
					Type:           "path",
					CurrentVersion: "1.2.0",
					NextVersion:    "1.3.0",
					NextTag:        "api-v1.3.0",
					BumpType:       "minor",
					PREntry: changelogpkg.ParseEntry(readTestFile(
						t,
						"testdata/combined_p_r_changelog/"+
							"multi_target_includes_wave_summary_and_detailed_target_sections/"+
							"api_changelog.input.md",
					)),
				},
				{
					ID:             "web",
					Type:           "path",
					CurrentVersion: "2.1.3",
					NextVersion:    "2.1.4",
					NextTag:        "web-v2.1.4",
					BumpType:       "patch",
					PREntry: changelogpkg.ParseEntry(readTestFile(
						t,
						"testdata/combined_p_r_changelog/"+
							"multi_target_includes_wave_summary_and_detailed_target_sections/"+
							"web_changelog.input.md",
					)),
				},
				{
					ID:              "root",
					Type:            "derived",
					CurrentVersion:  "2.9.0",
					NextVersion:     "3.0.0",
					NextTag:         "v3.0.0",
					BumpType:        "major",
					IncludedTargets: []string{"api", "web"},
					PREntry: readCombinedPRDerivedEntry(
						t,
						"testdata/combined_p_r_changelog/"+
							"multi_target_includes_wave_summary_and_detailed_target_sections/"+
							"root_changelog.input.md",
					),
				},
			},
		}

		// when: rendering the combined PR changelog
		body := r.lifecycle.text.combinedPRChangelog(result.Plans)

		// then: the output matches the expected multi-target release wave markdown
		testastic.AssertFile(t, "testdata/combined_pr_changelog_multi_target.expected.md", body)
	})

	t.Run("multi target keeps freeform blocks with their target", func(t *testing.T) {
		t.Parallel()

		// given: target entries with freeform intro and outro blocks
		plans := []TargetPlan{
			{
				ID:   "api",
				Type: "path",
				PREntry: changelogpkg.Entry{
					Version:  "api-v1.3.0",
					Intro:    []string{"API intro."},
					Sections: []changelogpkg.Section{{Heading: "Features", Lines: []string{"- add tokens"}}},
					Outro:    []string{"API outro."},
				},
			},
			{
				ID:   "web",
				Type: "path",
				PREntry: changelogpkg.Entry{
					Version:  "web-v2.1.4",
					Intro:    []string{"Web intro."},
					Sections: []changelogpkg.Section{{Heading: "Bug Fixes", Lines: []string{"- patch filters"}}},
					Outro:    []string{"Web outro."},
				},
			},
		}

		// when: building pull request sections for the targets
		sections := buildPRSections(plans)

		// then: each target keeps its freeform blocks around its generated sections
		testastic.Equal(t, 2, len(sections))
		testastic.Equal(t, "API intro.\n\n### Features\n\n- add tokens\n\nAPI outro.\n", sections[0].body)
		testastic.Equal(t, "Web intro.\n\n### Bug Fixes\n\n- patch filters\n\nWeb outro.\n", sections[1].body)
	})

	t.Run("derived target keeps a direct section whose heading matches a child ID", func(t *testing.T) {
		t.Parallel()

		// given: a derived target whose child ID is also a generated section heading
		entry := changelogpkg.DerivedEntry(
			changelogpkg.Entry{
				Version:  "v1.2.3",
				Sections: []changelogpkg.Section{{Heading: "Features", Lines: []string{"- parent feature"}}},
			},
			[]string{"Features"},
			[]changelogpkg.Section{{
				Heading:  "Features",
				Sections: []changelogpkg.Section{{Heading: "Bug Fixes", Lines: []string{"- child fix"}}},
			}},
		)
		plans := []TargetPlan{{
			ID:              "root",
			Type:            config.TargetTypeDerived,
			PREntry:         entry,
			IncludedTargets: []string{"Features"},
		}}

		// when: building the parent section of a multi-target pull request body
		sections := buildPRSections(plans)

		// then: heading equality does not discard the parent's own release note
		testastic.Contains(t, sections[0].body, "- parent feature")
		testastic.NotContains(t, sections[0].body, "- child fix")
	})

	t.Run("derived target keeps a manual section after its child sections", func(t *testing.T) {
		t.Parallel()

		// given: a derived entry with a manual section appended on the release branch
		generated := changelogpkg.DerivedEntry(
			changelogpkg.Entry{
				Version:       "v1.2.3",
				Sections:      []changelogpkg.Section{{Heading: "Bug Fixes", Lines: []string{"- parent fix"}}},
				OwnedHeadings: []string{"Bug Fixes"},
			},
			[]string{"api"},
			[]changelogpkg.Section{{
				Heading:  "api",
				Sections: []changelogpkg.Section{{Heading: "Features", Lines: []string{"- child feature"}}},
			}},
		)
		foreign := changelogpkg.ParseEntry(
			changelogpkg.Render(generated) + "\n### Migration Notes\n\nRun the migration.\n",
		)
		entry := changelogpkg.Merge(generated, foreign)
		plans := []TargetPlan{{
			ID:              "root",
			Type:            config.TargetTypeDerived,
			PREntry:         entry,
			IncludedTargets: []string{"api"},
		}}

		// when: building the parent section of a multi-target pull request body
		sections := buildPRSections(plans)

		// then: the manual parent note survives while the embedded child notes do not
		testastic.Contains(t, sections[0].body, "### Migration Notes")
		testastic.Contains(t, sections[0].body, "Run the migration.")
		testastic.NotContains(t, sections[0].body, "- child feature")
	})

	t.Run("derived target keeps one interleaved manual section across a heading collision", func(t *testing.T) {
		t.Parallel()

		// given: parent and child sections with the same heading and a manual note between them
		generated := changelogpkg.DerivedEntry(
			changelogpkg.Entry{
				Version:       "v1.2.3",
				Sections:      []changelogpkg.Section{{Heading: "Features", Lines: []string{"- parent feature"}}},
				OwnedHeadings: []string{"Features", "Bug Fixes"},
			},
			[]string{"Features"},
			[]changelogpkg.Section{childCollisionSection()},
		)
		foreign := changelogpkg.Entry{Sections: []changelogpkg.Section{
			{Heading: "Features", Lines: []string{"- parent feature"}},
			{Heading: "Migration Notes", Lines: []string{"Run the migration."}},
			{Heading: "Features"},
			{Heading: "Bug Fixes", Lines: []string{"- child fix"}},
		}}
		entry := changelogpkg.Merge(generated, foreign)
		plans := []TargetPlan{{
			ID:              "root",
			Type:            config.TargetTypeDerived,
			PREntry:         entry,
			IncludedTargets: []string{"Features"},
		}}

		// when: building the parent section of a multi-target pull request body
		sections := buildPRSections(plans)

		// then: the manual note stays after the parent feature and appears once
		testastic.Equal(
			t,
			"### Features\n\n- parent feature\n\n### Migration Notes\n\nRun the migration.\n",
			sections[0].body,
		)
	})

	t.Run("derived target preserves embedded child sections when some child plans are omitted", func(t *testing.T) {
		t.Parallel()

		// given: a release wave whose derived target embeds an analyzed child that is not emitted as its own plan
		r := newTestReleaser(t, config.Default(), newProviderStub())
		result := &Result{
			BaseBranch: "main",
			Plans: []TargetPlan{
				{
					ID:             "api",
					Type:           "path",
					CurrentVersion: "1.2.0",
					NextVersion:    "1.3.0",
					NextTag:        "api-v1.3.0",
					BumpType:       "minor",
					PREntry: changelogpkg.ParseEntry(readTestFile(
						t,
						"testdata/combined_p_r_changelog/"+
							"derived_target_preserves_embedded_child_sections_when_some_child_plans_are_omitted/"+
							"api_changelog.input.md",
					)),
				},
				{
					ID:              "root",
					Type:            "derived",
					CurrentVersion:  "2.9.0",
					NextVersion:     "3.0.0",
					NextTag:         "v3.0.0",
					BumpType:        "major",
					IncludedTargets: []string{"api", "web"},
					PREntry: readCombinedPRDerivedEntry(
						t,
						"testdata/combined_p_r_changelog/"+
							"derived_target_preserves_embedded_child_sections_when_some_child_plans_are_omitted/"+
							"root_changelog.input.md",
					),
				},
			},
		}

		// when: rendering the combined PR changelog for the mixed release wave
		body := r.lifecycle.text.combinedPRChangelog(result.Plans)

		// then: the output matches the expected derived-target markdown with embedded child sections
		testastic.AssertFile(t, "testdata/combined_pr_changelog_embedded_children.expected.md", body)
	})
}

func childCollisionSection() changelogpkg.Section {
	return changelogpkg.Section{
		Heading:  "Features",
		Sections: []changelogpkg.Section{{Heading: "Bug Fixes", Lines: []string{"- child fix"}}},
	}
}

func readCombinedPRDerivedEntry(t *testing.T, path string) changelogpkg.Entry {
	t.Helper()

	flat := changelogpkg.ParseEntry(readTestFile(t, path))
	testastic.Equal(t, 5, len(flat.Sections))

	direct := flat
	direct.Sections = flat.Sections[:1]

	entry := changelogpkg.DerivedEntry(direct, []string{"api", "web"}, []changelogpkg.Section{
		{Heading: flat.Sections[1].Heading, Sections: flat.Sections[2:3]},
		{Heading: flat.Sections[3].Heading, Sections: flat.Sections[4:]},
	})
	entry.CompareURL = flat.CompareURL

	return entry
}

func TestChangelogEntryByTag(t *testing.T) {
	t.Parallel()

	t.Run("extracts linked heading entry", func(t *testing.T) {
		t.Parallel()

		// given: a changelog containing linked version headings
		changelog := strings.TrimSpace(readTestFile(
			t,
			"testdata/changelog_entry_by_tag/extracts_linked_heading_entry/"+
				"changelog.input.md",
		))

		// when: extracting entry for v1.2.3
		entry, err := changelogpkg.EntryByTag(changelog, "v1.2.3")

		// then: only matching section is returned
		testastic.NoError(t, err)
		testastic.AssertFile(
			t,
			"testdata/changelog_entry_by_tag/extracts_linked_heading_entry/entry.expected.md",
			entry,
		)
	})

	t.Run("extracts plain heading entry", func(t *testing.T) {
		t.Parallel()

		// given: a changelog with plain version heading
		changelog := readTestFile(t, "testdata/changelog_entry_by_tag/extracts_plain_heading_entry/changelog.input.md")

		// when: extracting entry for v1.2.3
		entry, err := changelogpkg.EntryByTag(changelog, "v1.2.3")

		// then: plain heading entry is returned
		testastic.NoError(t, err)
		testastic.AssertFile(
			t,
			"testdata/changelog_entry_by_tag/extracts_plain_heading_entry/entry.expected.md",
			entry,
		)
	})

	t.Run("extracts indented heading entry", func(t *testing.T) {
		t.Parallel()

		// given: a changelog whose version heading carries CommonMark leading indentation
		changelog := readTestFile(t, "testdata/changelog_entry_by_tag/extracts_indented_heading_entry/changelog.input.md")

		// when: extracting entry for the indented heading
		entry, err := changelogpkg.EntryByTag(changelog, "v1.2.3")

		// then: the indented entry is found and bounded at the next heading
		testastic.NoError(t, err)
		testastic.AssertFile(
			t,
			"testdata/changelog_entry_by_tag/extracts_indented_heading_entry/entry.expected.md",
			entry,
		)
	})

	t.Run("returns error for missing tag", func(t *testing.T) {
		t.Parallel()

		// given: a changelog without requested tag
		changelog := readTestFile(t, "testdata/changelog_entry_by_tag/returns_error_for_missing_tag/changelog.input.md")

		// when: extracting entry for missing tag
		_, err := changelogpkg.EntryByTag(changelog, "v1.2.3")

		// then: not found error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, changelogpkg.ErrEntryNotFound)
	})
}

func preserveManualChangelogSections(generatedEntry, existingEntry string) string {
	cfg := config.Default()

	generated := changelogpkg.ParseEntry(generatedEntry)
	generated.OwnedHeadings = changelogpkg.New(
		changelogpkg.WithSections(cfg.Changelog.Sections),
		changelogpkg.WithInclude(cfg.Changelog.Include),
	).OwnedHeadings()

	merged := changelogpkg.Merge(generated, changelogpkg.ParseEntry(existingEntry))

	return strings.TrimSpace(changelogpkg.Render(merged))
}

func TestPreserveManualChangelogSections(t *testing.T) {
	t.Parallel()

	t.Run("preserves multiple manual sections in order", func(t *testing.T) {
		t.Parallel()

		// given: regenerated release notes and an existing changelog entry with manual sections
		generatedEntry := strings.TrimSpace(readTestFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"preserves_multiple_manual_sections_in_order/generated_entry.input.md",
		))
		existingEntry := strings.TrimSpace(readTestFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"preserves_multiple_manual_sections_in_order/existing_entry.input.md",
		))

		// when: preserving manual sections from the existing changelog entry
		updatedEntry := preserveManualChangelogSections(generatedEntry, existingEntry)

		// then: all manual sections are appended in their original order
		testastic.AssertFile(
			t,
			"testdata/preserve_manual_changelog_sections/preserves_multiple_manual_sections_in_order/updated_entry.expected.md",
			updatedEntry,
		)
	})

	t.Run("preserves manual section positions among generated sections", func(t *testing.T) {
		t.Parallel()

		// given: manual sections before and between regenerated release-note sections
		generatedEntry := strings.TrimSpace(readTestFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"preserves_manual_section_positions_among_generated_sections/"+
				"generated_entry.input.md",
		))
		existingEntry := strings.TrimSpace(readTestFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"preserves_manual_section_positions_among_generated_sections/"+
				"existing_entry.input.md",
		))

		// when: preserving manual sections from the existing changelog entry
		updatedEntry := preserveManualChangelogSections(generatedEntry, existingEntry)

		// then: each manual section remains before its following generated section
		testastic.AssertFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"preserves_manual_section_positions_among_generated_sections/updated_entry.expected.md",
			updatedEntry,
		)
	})

	t.Run("does not preserve edits inside regenerated sections", func(t *testing.T) {
		t.Parallel()

		// given: a user edited the generated Bug Fixes section on the release branch
		generatedEntry := strings.TrimSpace(readTestFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"does_not_preserve_edits_inside_regenerated_sections/"+
				"generated_entry.input.md",
		))
		existingEntry := strings.TrimSpace(readTestFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"does_not_preserve_edits_inside_regenerated_sections/"+
				"existing_entry.input.md",
		))

		// when: preserving manual sections from the existing changelog entry
		updatedEntry := preserveManualChangelogSections(generatedEntry, existingEntry)

		// then: regenerated sections remain authoritative on rerun
		testastic.AssertFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"does_not_preserve_edits_inside_regenerated_sections/updated_entry.expected.md",
			updatedEntry,
		)
	})

	t.Run("preserves manual content outside level-3 sections", func(t *testing.T) {
		t.Parallel()

		// given: regenerated notes and an existing entry with freeform and manual content
		generatedEntry := strings.TrimSpace(readTestFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"drops_manual_content_outside_level_3_sections/generated_entry.input.md",
		))
		existingEntry := strings.TrimSpace(readTestFile(
			t,
			"testdata/preserve_manual_changelog_sections/"+
				"drops_manual_content_outside_level_3_sections/existing_entry.input.md",
		))

		// when: preserving manual content from the existing entry
		updatedEntry := preserveManualChangelogSections(generatedEntry, existingEntry)

		// then: the freeform intro and manual section both remain
		testastic.AssertFile(t, "testdata/preserve_manual_drops_non_level3.expected.md", updatedEntry)
	})
}
