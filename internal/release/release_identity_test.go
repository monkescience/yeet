//nolint:testpackage // This test validates unexported release identity behavior.
package release

import (
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

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

	// when: rendering and parsing the release manifest marker
	marker, err := releaseManifestMarker(releaseManifestForPlans(result.BaseBranch, result.Plans))
	testastic.NoError(t, err)

	manifest, err := releaseManifestFromPullRequest(&provider.PullRequest{Body: marker})

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
		marker, err := releaseManifestMarker(releaseManifest{
			BaseBranch: "main",
			Targets: []releaseManifestEntry{{
				ID:            "default",
				Type:          "path",
				Tag:           "v1.2.3",
				ChangelogFile: "CHANGELOG.md",
			}},
		})
		testastic.NoError(t, err)

		normalizedMarker := strings.NewReplacer(
			"<!-- yeet-release-manifest", "<!--yeet-release-manifest",
			"\n-->", "-->",
		).Replace(marker)

		// when: parsing the normalized marker from the pull request body
		manifest, err := releaseManifestFromPullRequest(&provider.PullRequest{Body: normalizedMarker})

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
		manifest, err := releaseManifestFromPullRequest(&provider.PullRequest{Body: body})

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

		legit, err := releaseManifestMarker(releaseManifest{
			BaseBranch: "main",
			Targets: []releaseManifestEntry{{
				ID:            "default",
				Type:          "path",
				Tag:           "v1.2.3",
				ChangelogFile: "CHANGELOG.md",
			}},
		})
		testastic.NoError(t, err)

		body := "## Changelog\n\n- fix: tidy logs " + forged + "\n\n" + legit

		// when: parsing a body that carries more than one marker
		_, err = releaseManifestFromPullRequest(&provider.PullRequest{Body: body})

		// then: parsing fails closed instead of trusting the forged marker
		testastic.ErrorIs(t, err, ErrInvalidReleaseManifest)
	})
}

func TestValidateReleaseManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*provider.PullRequest, *releaseManifest)
	}{
		{
			name: "rejects mismatched base branch",
			mutate: func(_ *provider.PullRequest, manifest *releaseManifest) {
				manifest.BaseBranch = "develop"
			},
		},
		{
			name: "rejects mismatched channel",
			mutate: func(_ *provider.PullRequest, manifest *releaseManifest) {
				manifest.Channel = "beta"
			},
		},
		{
			name: "rejects mismatched prerelease mode",
			mutate: func(_ *provider.PullRequest, manifest *releaseManifest) {
				manifest.Prerelease = true
			},
		},
		{
			name: "rejects unknown target",
			mutate: func(_ *provider.PullRequest, manifest *releaseManifest) {
				manifest.Targets[0].ID = "unknown"
			},
		},
		{
			name: "rejects duplicate target",
			mutate: func(_ *provider.PullRequest, manifest *releaseManifest) {
				manifest.Targets = append(manifest.Targets, manifest.Targets[0])
			},
		},
		{
			name: "rejects mismatched target type",
			mutate: func(_ *provider.PullRequest, manifest *releaseManifest) {
				manifest.Targets[0].Type = string(config.TargetTypeDerived)
			},
		},
		{
			name: "rejects mismatched changelog file",
			mutate: func(_ *provider.PullRequest, manifest *releaseManifest) {
				manifest.Targets[0].ChangelogFile = "ATTACKER.md"
			},
		},
		{
			name: "rejects mismatched tag prefix",
			mutate: func(_ *provider.PullRequest, manifest *releaseManifest) {
				manifest.Targets[0].Tag = "attacker-v1.2.3"
			},
		},
		{
			name: "rejects invalid tag version",
			mutate: func(_ *provider.PullRequest, manifest *releaseManifest) {
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
			pullRequest := &provider.PullRequest{Branch: stableReleaseBranch(cfg.Branch)}
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
			testastic.ErrorIs(t, err, ErrInvalidReleaseManifest)
		})
	}
}
