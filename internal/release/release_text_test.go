//nolint:testpackage // This test validates unexported release text behavior.
package release

import (
	"fmt"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestReleasePRBody(t *testing.T) {
	t.Parallel()

	t.Run("defaults include generated header and footer", func(t *testing.T) {
		t.Parallel()

		// given: releaser with default config
		r := newTestReleaser(t, config.Default(), newProviderStub())
		changelogBody := "## v1.2.4 (2026-03-01)\n\n### Bug Fixes\n\n- patch issue (abc1234)\n"

		// when: building PR body
		body, truncated := r.core.releasePRBody(changelogBody, "<!-- yeet-release-tag: v1.2.4 -->", 0)

		// then: changelog is wrapped by default header, manifest, and footer notes
		testastic.False(t, truncated)
		testastic.Equal(
			t,
			"## ٩(^ᴗ^)۶ release created\n\n"+
				strings.TrimSpace(changelogBody)+
				"\n\n<!-- yeet-release-tag: v1.2.4 -->"+
				"\n\n_Auto-generated preview — edit `CHANGELOG.md` to customize release notes._\n\n"+
				"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
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
		body, truncated := r.core.releasePRBody("## v1.2.4", "<!-- yeet-release-tag: v1.2.4 -->", 0)

		// then: body contains header, changelog, manifest, and footer in order
		testastic.False(t, truncated)
		testastic.Equal(
			t,
			"Header\n\n"+
				"## v1.2.4\n\n<!-- yeet-release-tag: v1.2.4 -->\n\nFooter",
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
		body, truncated := r.core.releasePRBody("## v1.2.4\n", "<!-- yeet-release-tag: v1.2.4 -->", 0)

		// then: body keeps only changelog and manifest, no wrapper text
		testastic.False(t, truncated)
		testastic.Equal(
			t,
			"## v1.2.4\n\n<!-- yeet-release-tag: v1.2.4 -->",
			body,
		)
	})

	t.Run("body within limit is not truncated", func(t *testing.T) {
		t.Parallel()

		// given: a releaser with default config and a short changelog
		r := newTestReleaser(t, config.Default(), newProviderStub())
		marker := testManifestBody(t, "v1.2.4", "CHANGELOG.md")

		// when: building the body with a generous limit
		body, truncated := r.core.releasePRBody("## v1.2.4\n\n- one note", marker, 4000)

		// then: nothing is truncated and the marker survives
		testastic.False(t, truncated)
		testastic.Contains(t, body, "- one note")
		assertManifestSurvives(t, body, "v1.2.4")
	})

	t.Run("oversized body drops the whole changelog but preserves marker, header, and footer", func(t *testing.T) {
		t.Parallel()

		// given: a releaser with default config and a changelog far larger than the limit
		r := newTestReleaser(t, config.Default(), newProviderStub())
		marker := testManifestBody(t, "v1.2.4", "CHANGELOG.md")

		var changelog strings.Builder
		changelog.WriteString("## v1.2.4\n\n### Features\n\n")

		for line := range 500 {
			fmt.Fprintf(&changelog, "- feature number %d that adds yet more text to the notes\n", line)
		}

		// when: building the body with the Azure DevOps limit
		body, omitted := r.core.releasePRBody(changelog.String(), marker, 4000)

		// then: the body fits, no changelog lines survive, and the wrapper text plus notice remain
		testastic.True(t, omitted)
		testastic.True(t, len(body) <= 4000)
		testastic.Contains(t, body, "## ٩(^ᴗ^)۶ release created")
		testastic.Contains(t, body, "_Made with [yeet]")
		testastic.Contains(t, body, "Release notes omitted")
		testastic.NotContains(t, body, "- feature number 0")
		testastic.NotContains(t, body, "### Features")
		assertManifestSurvives(t, body, "v1.2.4")
	})

	t.Run("body one byte over the limit drops the changelog entirely", func(t *testing.T) {
		t.Parallel()

		// given: a config with no header or footer and a short changelog
		cfg := config.Default()
		cfg.Release.PRBodyHeader = ""
		cfg.Release.PRBodyFooter = ""

		r := newTestReleaser(t, cfg, newProviderStub())
		marker := testManifestBody(t, "v1.2.4", "CHANGELOG.md")
		changelog := "## v1.2.4\n\n- a note that will not fit"

		full := changelog + "\n\n" + marker
		limit := len(full) - 1

		// when: building the body with a limit one byte under the full body
		body, omitted := r.core.releasePRBody(changelog, marker, limit)

		// then: the changelog is dropped wholesale, the notice and marker remain
		testastic.True(t, omitted)
		testastic.NotContains(t, body, "a note that will not fit")
		testastic.Contains(t, body, "Release notes omitted")
		assertManifestSurvives(t, body, "v1.2.4")
	})
}

func assertManifestSurvives(t *testing.T, body, wantTag string) {
	t.Helper()

	manifest, ok, err := releaseManifestFromBody(body)
	testastic.NoError(t, err)
	testastic.True(t, ok)
	testastic.Equal(t, 1, len(manifest.Targets))
	testastic.Equal(t, wantTag, manifest.Targets[0].Tag)
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
		{name: "provider only", configured: 0, providerLimit: 4000, want: 4000},
		{name: "config only", configured: 2000, providerLimit: 0, want: 2000},
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
			limit := r.core.effectivePRBodyLimit(test.providerLimit)

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
		prChangelog := strings.TrimSpace(`## [v1.2.4](https://example.com/compare/v1.2.3...abc1234) (2026-03-21)

### Bug Fixes

- patch issue (abc1234)
`) + "\n"

		result := &Result{
			BaseBranch: "main",
			Plans: []TargetPlan{{
				ID:          "default",
				Type:        "path",
				PRChangelog: prChangelog,
			}},
		}

		// when: rendering the combined PR changelog
		body := r.core.combinedPRChangelog(result)

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
					PRChangelog: strings.TrimSpace(`## [api-v1.3.0](https://example.com/compare/api-v1.2.0...abc1234) (2026-03-21)

### Features

- add token rotation (abc1234)
`),
				},
				{
					ID:             "web",
					Type:           "path",
					CurrentVersion: "2.1.3",
					NextVersion:    "2.1.4",
					NextTag:        "web-v2.1.4",
					BumpType:       "patch",
					PRChangelog: strings.TrimSpace(`## [web-v2.1.4](https://example.com/compare/web-v2.1.3...def5678) (2026-03-21)

### Bug Fixes

- fix dashboard filters (def5678)
`),
				},
				{
					ID:              "root",
					Type:            "derived",
					CurrentVersion:  "2.9.0",
					NextVersion:     "3.0.0",
					NextTag:         "v3.0.0",
					BumpType:        "major",
					IncludedTargets: []string{"api", "web"},
					PRChangelog: strings.TrimSpace(`## [v3.0.0](https://example.com/compare/v2.9.0...9876abc) (2026-03-21)

### Documentation

- update README install steps (9876abc)

### api

### Features

- add token rotation (abc1234)

### web

### Bug Fixes

- fix dashboard filters (def5678)
`),
				},
			},
		}

		// when: rendering the combined PR changelog
		body := r.core.combinedPRChangelog(result)

		// then: the output matches the expected multi-target release wave markdown
		testastic.AssertFile(t, "testdata/combined_pr_changelog_multi_target.expected.md", body)
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
					PRChangelog: strings.TrimSpace(`## [api-v1.3.0](https://example.com/compare/api-v1.2.0...abc1234) (2026-03-21)

### Features

- add token rotation (abc1234)
`),
				},
				{
					ID:              "root",
					Type:            "derived",
					CurrentVersion:  "2.9.0",
					NextVersion:     "3.0.0",
					NextTag:         "v3.0.0",
					BumpType:        "major",
					IncludedTargets: []string{"api", "web"},
					PRChangelog: strings.TrimSpace(`## [v3.0.0](https://example.com/compare/v2.9.0...9876abc) (2026-03-21)

### Documentation

- update README install steps (9876abc)

### api

### Features

- add token rotation (abc1234)

### web

### Bug Fixes

- fix dashboard filters (def5678)
`),
				},
			},
		}

		// when: rendering the combined PR changelog for the mixed release wave
		body := r.core.combinedPRChangelog(result)

		// then: the output matches the expected derived-target markdown with embedded child sections
		testastic.AssertFile(t, "testdata/combined_pr_changelog_embedded_children.expected.md", body)
	})
}

func TestChangelogEntryByTag(t *testing.T) {
	t.Parallel()

	t.Run("extracts linked heading entry", func(t *testing.T) {
		t.Parallel()

		// given: a changelog containing linked version headings
		changelog := strings.TrimSpace(`# Changelog

## [v1.2.3](https://example.com/compare/v1.2.2...v1.2.3) (2026-03-01)

### Features

- add feature

## [v1.2.2](https://example.com/compare/v1.2.1...v1.2.2) (2026-02-20)

### Bug Fixes

- patch
`)

		// when: extracting entry for v1.2.3
		entry, err := changelogEntryByTag(changelog, "v1.2.3")

		// then: only matching section is returned
		testastic.NoError(t, err)
		testastic.HasPrefix(t, entry, "## [v1.2.3]")
		testastic.NotContains(t, entry, "## [v1.2.2]")
	})

	t.Run("extracts plain heading entry", func(t *testing.T) {
		t.Parallel()

		// given: a changelog with plain version heading
		changelog := "# Changelog\n\n## v1.2.3 (2026-03-01)\n\n### Features\n\n- add feature\n"

		// when: extracting entry for v1.2.3
		entry, err := changelogEntryByTag(changelog, "v1.2.3")

		// then: plain heading entry is returned
		testastic.NoError(t, err)
		testastic.HasPrefix(t, entry, "## v1.2.3")
	})

	t.Run("extracts indented heading entry", func(t *testing.T) {
		t.Parallel()

		// given: a changelog whose version heading carries CommonMark leading indentation
		changelog := "# Changelog\n\n  ## v1.2.3 (2026-03-01)\n\n### Features\n\n" +
			"- add feature\n\n## v1.2.2 (2026-02-20)\n\n### Bug Fixes\n\n- patch\n"

		// when: extracting entry for the indented heading
		entry, err := changelogEntryByTag(changelog, "v1.2.3")

		// then: the indented entry is found and bounded at the next heading
		testastic.NoError(t, err)
		testastic.HasPrefix(t, entry, "## v1.2.3")
		testastic.NotContains(t, entry, "v1.2.2")
	})

	t.Run("returns error for missing tag", func(t *testing.T) {
		t.Parallel()

		// given: a changelog without requested tag
		changelog := "# Changelog\n\n## v1.2.2 (2026-02-20)\n"

		// when: extracting entry for missing tag
		_, err := changelogEntryByTag(changelog, "v1.2.3")

		// then: not found error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrChangelogEntryNotFound)
	})
}

func TestPreserveManualChangelogSections(t *testing.T) {
	t.Parallel()

	t.Run("preserves multiple manual sections in order", func(t *testing.T) {
		t.Parallel()

		// given: regenerated release notes and an existing changelog entry with manual sections
		generatedEntry := strings.TrimSpace(`## v1.2.4 (2026-03-01)

### Bug Fixes

- patch issue (abc1234)
`)
		existingEntry := strings.TrimSpace(`## v1.2.4 (2026-03-01)

### Bug Fixes

- patch issue (abc1234)

### Migration Notes

Run database migrations before deploying workers.

### Rollback Notes

Redeploy the previous worker image if queue latency spikes.
`)

		// when: preserving manual sections from the existing changelog entry
		updatedEntry := preserveManualChangelogSections(generatedEntry, existingEntry)

		// then: all manual sections are appended in their original order
		migrationIndex := strings.Index(updatedEntry, "### Migration Notes")
		rollbackIndex := strings.Index(updatedEntry, "### Rollback Notes")

		testastic.Contains(t, updatedEntry, "### Bug Fixes")
		testastic.True(t, migrationIndex > strings.Index(updatedEntry, "### Bug Fixes"))
		testastic.True(t, rollbackIndex > migrationIndex)
	})

	t.Run("does not preserve edits inside regenerated sections", func(t *testing.T) {
		t.Parallel()

		// given: a user edited the generated Bug Fixes section on the release branch
		generatedEntry := strings.TrimSpace(`## v1.2.4 (2026-03-01)

### Bug Fixes

- patch issue (abc1234)
`)
		existingEntry := strings.TrimSpace(`## v1.2.4 (2026-03-01)

### Bug Fixes

- custom rewrite of the generated note (abc1234)
- extra hand-written fix note
`)

		// when: preserving manual sections from the existing changelog entry
		updatedEntry := preserveManualChangelogSections(generatedEntry, existingEntry)

		// then: regenerated sections remain authoritative on rerun
		testastic.Contains(t, updatedEntry, "- patch issue (abc1234)")
		testastic.NotContains(t, updatedEntry, "custom rewrite")
		testastic.NotContains(t, updatedEntry, "extra hand-written fix note")
	})

	t.Run("drops manual content outside level-3 sections", func(t *testing.T) {
		t.Parallel()

		// given: an existing entry with a manual ### section plus freeform text written
		// directly under the ## version heading, which is not a level-3 section
		generatedEntry := strings.TrimSpace(`## v1.2.4 (2026-03-01)

### Bug Fixes

- patch issue (abc1234)
`)
		existingEntry := strings.TrimSpace(`## v1.2.4 (2026-03-01)

A heads-up note written directly under the version heading.

### Bug Fixes

- patch issue (abc1234)

### Migration Notes

Run database migrations before deploying workers.
`)

		// when: preserving manual sections from the existing changelog entry
		updatedEntry := preserveManualChangelogSections(generatedEntry, existingEntry)

		// then: the level-3 manual section survives but freeform text under ## is dropped
		testastic.AssertFile(t, "testdata/preserve_manual_drops_non_level3.expected.md", updatedEntry)
	})
}
