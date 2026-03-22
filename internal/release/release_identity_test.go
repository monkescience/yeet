//nolint:testpackage // This test validates unexported release identity behavior.
package release

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
)

func TestReleaseManifestRoundTrip(t *testing.T) {
	t.Parallel()

	// given: a wave with multiple planned targets
	result := &Result{
		BaseBranch: "main",
		Plans: []TargetPlan{
			{
				ID:      "api",
				Type:    "path",
				NextTag: "api-v1.2.3",
				Files:   map[string]string{"changelog_file": "services/api/CHANGELOG.md"},
			},
			{ID: "root", Type: "derived", NextTag: "v3.0.0", Files: map[string]string{"changelog_file": "CHANGELOG.md"}},
		},
	}

	// when: rendering and parsing the release manifest marker
	marker, err := releaseManifestMarker(releaseManifestForResult(result))
	testastic.NoError(t, err)

	manifest, err := releaseManifestFromPullRequest(&provider.PullRequest{Body: marker})

	// then: all manifest entries survive the round trip
	testastic.NoError(t, err)
	testastic.Equal(t, "main", manifest.BaseBranch)
	testastic.Equal(t, 2, len(manifest.Targets))
	testastic.Equal(t, "api-v1.2.3", manifest.Targets[0].Tag)
	testastic.Equal(t, "CHANGELOG.md", manifest.Targets[1].ChangelogFile)
}
