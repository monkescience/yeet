package config

import (
	"os"
	"testing"

	"github.com/monkescience/testastic"
)

func TestParseIndependentReleaseGroups(t *testing.T) {
	t.Parallel()

	// given: an independent layout with one central atomic group
	input, err := os.ReadFile("testdata/release_groups/valid/input.yaml")
	testastic.NoError(t, err)

	// when: parsing the checked-in configuration
	cfg, err := parse(input)

	// then: the mode and group members survive schema and YAML decoding
	testastic.NoError(t, err)
	testastic.Equal(t, PullRequestModeIndependent, cfg.Release.PullRequestMode)
	testastic.SliceEqual(t, []string{"api", "worker"}, cfg.Release.Groups["backend"].Targets)
}
