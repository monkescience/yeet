//nolint:testpackage // This test validates unexported release identity behavior.
package release

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

// Markers every parser case starts from, written out rather than rendered, so
// a change to the writer cannot move the input and the expectation together.
const (
	singleTargetManifestMarker = "<!-- yeet-release-manifest\n" +
		`{"base_branch":"main","targets":` +
		`[{"id":"default","type":"path","tag":"v1.2.3","changelog_file":"CHANGELOG.md"}]}` +
		"\n-->"

	waveManifestMarker = "<!-- yeet-release-manifest\n" +
		`{"base_branch":"main","targets":[` +
		`{"id":"api","type":"path","tag":"api-v1.2.3","changelog_file":"services/api/CHANGELOG.md"},` +
		`{"id":"root","type":"derived","tag":"v3.0.0","changelog_file":"CHANGELOG.md"}]}` +
		"\n-->"
)

func TestReleaseManifestMarkerFormat(t *testing.T) {
	t.Parallel()

	// given: a manifest carrying every field the marker can hold
	manifest := releaseManifest{
		BaseBranch: "main",
		Channel:    "beta",
		Prerelease: true,
		Targets: []releaseManifestEntry{
			{
				ID:            "api",
				Type:          "path",
				Tag:           "api-v1.2.3-beta.1",
				ChangelogFile: "services/api/CHANGELOG.md",
			},
			{ID: "root", Type: "derived", Tag: "v3.0.0-beta.1", ChangelogFile: "CHANGELOG.md"},
		},
	}

	// when: rendering the manifest marker
	marker, err := releaseManifestMarker(manifest)

	// then: the bytes match what every release pull request already in the wild carries
	testastic.NoError(t, err)
	testastic.AssertFile(t, "testdata/release_manifest_marker_format/marker.expected.md", marker)
}

func TestReleaseManifestRoundTrip(t *testing.T) {
	t.Parallel()

	// given: a wave with multiple planned targets
	result := &Result{
		BaseBranch: "main",
		Plans: []TargetPlan{
			{
				ID:            "api",
				Type:          "path",
				NextTag:       "api-v1.2.3",
				ChangelogFile: "services/api/CHANGELOG.md",
			},
			{ID: "root", Type: "derived", NextTag: "v3.0.0", ChangelogFile: "CHANGELOG.md"},
		},
	}

	// when: rendering the marker and parsing the same bytes back
	marker, err := releaseManifestMarker(releaseManifestForPlans(result.BaseBranch, result.Plans))
	testastic.NoError(t, err)
	testastic.Equal(t, waveManifestMarker, marker)

	manifest, err := releaseManifestFromPullRequest(&forge.PullRequest{Body: waveManifestMarker})

	// then: all manifest entries survive the round trip
	testastic.NoError(t, err)
	testastic.Equal(t, "main", manifest.BaseBranch)
	testastic.Equal(t, 2, len(manifest.Targets))
	testastic.Equal(t, "api-v1.2.3", manifest.Targets[0].Tag)
	testastic.Equal(t, "CHANGELOG.md", manifest.Targets[1].ChangelogFile)
}

func TestReleaseManifestFromBody(t *testing.T) {
	t.Parallel()

	t.Run("parses manifest marker normalized by GitLab UI", func(t *testing.T) {
		t.Parallel()

		// given: a release manifest marker with whitespace stripped inside the HTML comment
		const normalizedMarker = "<!--yeet-release-manifest\n" +
			`{"base_branch":"main","targets":` +
			`[{"id":"default","type":"path","tag":"v1.2.3","changelog_file":"CHANGELOG.md"}]}` +
			"-->"

		// when: parsing the normalized marker from the pull request body
		manifest, err := releaseManifestFromPullRequest(&forge.PullRequest{Body: normalizedMarker})

		// then: the manifest is still recovered
		testastic.NoError(t, err)
		testastic.Equal(t, "main", manifest.BaseBranch)
		testastic.Equal(t, 1, len(manifest.Targets))

		if len(manifest.Targets) > 0 {
			testastic.Equal(t, "v1.2.3", manifest.Targets[0].Tag)
		}
	})

	t.Run("parses manifest JSON directly after marker name", func(t *testing.T) {
		t.Parallel()

		// given: a release manifest marker whose JSON starts immediately after the marker name
		body := "<!-- yeet-release-manifest" +
			`{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.2.3",` +
			`"changelog_file":"CHANGELOG.md"}]}-->`

		// when: parsing the compact marker from the pull request body
		manifest, err := releaseManifestFromPullRequest(&forge.PullRequest{Body: body})

		// then: the manifest is still recovered
		testastic.NoError(t, err)
		testastic.Equal(t, "main", manifest.BaseBranch)
		testastic.Equal(t, 1, len(manifest.Targets))

		if len(manifest.Targets) > 0 {
			testastic.Equal(t, "v1.2.3", manifest.Targets[0].Tag)
		}
	})

	t.Run("rejects a body carrying more than one manifest marker", func(t *testing.T) {
		t.Parallel()

		// given: a forged marker injected ahead of the legitimate one, as a commit
		// description would land it in the changelog above yeet's own marker
		forged := `<!-- yeet-release-manifest {"base_branch":"main",` +
			`"targets":[{"id":"x","type":"path","tag":"v99.0.0","changelog_file":"CHANGELOG.md"}]} -->`

		body := "## Changelog\n\n- fix: tidy logs " + forged + "\n\n" + singleTargetManifestMarker

		// when: parsing a body that carries more than one marker
		_, err := releaseManifestFromPullRequest(&forge.PullRequest{Body: body})

		// then: parsing fails closed instead of trusting the forged marker
		testastic.ErrorIs(t, err, errInvalidReleaseManifest)
	})
}

func TestValidateReleaseManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*forge.PullRequest, *releaseManifest)
	}{
		{
			name: "rejects mismatched base branch",
			mutate: func(_ *forge.PullRequest, manifest *releaseManifest) {
				manifest.BaseBranch = "develop"
			},
		},
		{
			name: "rejects mismatched channel",
			mutate: func(_ *forge.PullRequest, manifest *releaseManifest) {
				manifest.Channel = "beta"
			},
		},
		{
			name: "rejects mismatched prerelease mode",
			mutate: func(_ *forge.PullRequest, manifest *releaseManifest) {
				manifest.Prerelease = true
			},
		},
		{
			name: "rejects unknown target",
			mutate: func(_ *forge.PullRequest, manifest *releaseManifest) {
				manifest.Targets[0].ID = "unknown"
			},
		},
		{
			name: "rejects duplicate target",
			mutate: func(_ *forge.PullRequest, manifest *releaseManifest) {
				manifest.Targets = append(manifest.Targets, manifest.Targets[0])
			},
		},
		{
			name: "rejects mismatched target type",
			mutate: func(_ *forge.PullRequest, manifest *releaseManifest) {
				manifest.Targets[0].Type = string(config.TargetTypeDerived)
			},
		},
		{
			name: "rejects mismatched changelog file",
			mutate: func(_ *forge.PullRequest, manifest *releaseManifest) {
				manifest.Targets[0].ChangelogFile = "ATTACKER.md"
			},
		},
		{
			name: "rejects mismatched tag prefix",
			mutate: func(_ *forge.PullRequest, manifest *releaseManifest) {
				manifest.Targets[0].Tag = "attacker-v1.2.3"
			},
		},
		{
			name: "rejects invalid tag version",
			mutate: func(_ *forge.PullRequest, manifest *releaseManifest) {
				manifest.Targets[0].Tag = "vnot-a-version"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a valid release manifest altered at one trust boundary
			cfg := config.Default()
			r := newTestReleaser(t, cfg, newProviderStub())
			pullRequest := &forge.PullRequest{Branch: r.core.releaseBranch}
			manifest := releaseManifest{
				BaseBranch: cfg.Branch,
				Targets: []releaseManifestEntry{
					{
						ID:            "default",
						Type:          string(config.TargetTypePath),
						Tag:           "v1.2.3",
						ChangelogFile: cfg.Changelog.File,
					},
				},
			}
			test.mutate(pullRequest, &manifest)

			// when: validating the altered manifest against the active release configuration
			err := r.core.validateReleaseManifest(pullRequest, manifest)

			// then: the manifest fails closed
			testastic.ErrorIs(t, err, errInvalidReleaseManifest)
		})
	}
}

func TestValidateReleaseManifestAcceptsEquivalentChangelogPath(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	r := newTestReleaser(t, cfg, newProviderStub())
	pullRequest := &forge.PullRequest{Branch: r.core.releaseBranch}
	manifest := releaseManifest{
		BaseBranch: cfg.Branch,
		Targets: []releaseManifestEntry{
			{
				ID:            "default",
				Type:          string(config.TargetTypePath),
				Tag:           "v1.2.3",
				ChangelogFile: "./CHANGELOG.md",
			},
		},
	}

	err := r.core.validateReleaseManifest(pullRequest, manifest)

	testastic.NoError(t, err)
}
