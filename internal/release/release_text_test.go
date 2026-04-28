//nolint:testpackage // This test validates unexported release text behavior.
package release

import (
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
		body := r.releasePRBody(changelogBody, "<!-- yeet-release-tag: v1.2.4 -->")

		// then: changelog is wrapped by default header and footer notes
		testastic.Equal(
			t,
			"## ٩(^ᴗ^)۶ release created\n\n"+
				strings.TrimSpace(changelogBody)+
				"\n\n<!-- yeet-release-tag: v1.2.4 -->"+
				"\n\n_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
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
		body := r.releasePRBody("## v1.2.4", "<!-- yeet-release-tag: v1.2.4 -->")

		// then: body contains header, changelog, and footer in order
		testastic.Equal(t, "Header\n\n## v1.2.4\n\n<!-- yeet-release-tag: v1.2.4 -->\n\nFooter", body)
	})

	t.Run("empty wrapper fields keep changelog only", func(t *testing.T) {
		t.Parallel()

		// given: releaser with both wrapper fields disabled
		cfg := config.Default()
		cfg.Release.PRBodyHeader = ""
		cfg.Release.PRBodyFooter = ""

		r := newTestReleaser(t, cfg, newProviderStub())

		// when: building PR body
		body := r.releasePRBody("## v1.2.4\n", "<!-- yeet-release-tag: v1.2.4 -->")

		// then: body is the changelog without extra sections
		testastic.Equal(t, "## v1.2.4\n\n<!-- yeet-release-tag: v1.2.4 -->", body)
	})
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
		body := r.combinedPRChangelog(result)

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
		body := r.combinedPRChangelog(result)

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
		body := r.combinedPRChangelog(result)

		// then: the output matches the expected derived-target markdown with embedded child sections
		testastic.AssertFile(t, "testdata/combined_pr_changelog_embedded_children.expected.md", body)
	})
}

func TestExtractReleaseNotesBlock(t *testing.T) {
	t.Parallel()

	t.Run("extracts notes from GitLab UI normalized markers", func(t *testing.T) {
		t.Parallel()

		// given: release notes markers whose inner whitespace was stripped by GitLab
		body := "## Release\n\n<!--BEGIN_YEET_RELEASE_NOTES-->\n" +
			"### Upgrade notes\n\nRestart workers.\n<!--END_YEET_RELEASE_NOTES-->"

		// when: extracting release notes from the body
		notes, err := extractReleaseNotesBlock(body)

		// then: the custom notes are recovered
		testastic.NoError(t, err)
		testastic.Equal(t, "### Upgrade notes\n\nRestart workers.", notes)
	})

	t.Run("allows an absent release notes block", func(t *testing.T) {
		t.Parallel()

		// given: a body with no release notes markers
		body := "## Release\n\nNo editable notes block."

		// when: extracting release notes from the body
		notes, err := extractReleaseNotesBlock(body)

		// then: no notes are returned and no parse error is raised
		testastic.NoError(t, err)
		testastic.Equal(t, "", notes)
	})

	t.Run("fails when start marker is missing", func(t *testing.T) {
		t.Parallel()

		// given: a body with only an end release notes marker
		body := "## Release\n\n### Upgrade notes\n\nRestart workers.\n<!--END_YEET_RELEASE_NOTES-->"

		// when: extracting release notes from the body
		_, err := extractReleaseNotesBlock(body)

		// then: yeet refuses to silently drop the notes
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseNotesBlock)
	})

	t.Run("fails when end marker is missing", func(t *testing.T) {
		t.Parallel()

		// given: a body with only a start release notes marker
		body := "## Release\n\n<!--BEGIN_YEET_RELEASE_NOTES-->\n### Upgrade notes\n\nRestart workers."

		// when: extracting release notes from the body
		_, err := extractReleaseNotesBlock(body)

		// then: yeet refuses to silently drop the notes
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseNotesBlock)
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
