package config_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestReleaseLayout(t *testing.T) {
	t.Parallel()

	t.Run("snapshot survives config and returned value mutations", func(t *testing.T) {
		t.Parallel()

		// given: an independent layout with grouped and ungrouped targets
		cfg := config.Default()
		cfg.Release.PullRequestMode = config.PullRequestModeIndependent
		cfg.Targets = map[string]config.Target{"api": {}, "worker": {}, "web": {}}
		cfg.Release.Groups = map[string]config.ReleaseGroupConfig{
			" backend ": {Targets: []string{" worker ", "api"}},
		}
		layout := cfg.ReleaseLayout()

		// when: the source configuration and returned values are changed
		units := layout.Units()
		units[0].ID = "changed"
		units[0].TargetIDs[0] = "changed"
		selection := layout.ExpandSelection([]string{"api"})
		selection[0] = "changed"
		cfg.Release.Groups[" backend "].Targets[0] = "changed"
		delete(cfg.Targets, "worker")

		// then: the snapshot retains its membership and selection behavior
		testastic.AssertJSON(t, "testdata/release_layout/independent.expected.json", layout.Units())
		testastic.SliceEqual(t, []string{"worker", "api", "web", "unknown"},
			layout.ExpandSelection([]string{" api ", "web", "worker", "unknown"}))
	})

	t.Run("combined selection retains caller order and duplicates", func(t *testing.T) {
		t.Parallel()

		// given: a combined layout and a selection containing whitespace and duplicates
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{"web": {}, "api": {}}
		layout := cfg.ReleaseLayout()
		requested := []string{"web", " api ", "web"}

		// when: a returned selection is changed
		selected := layout.ExpandSelection(requested)
		selected[0] = "changed"

		// then: the combined membership and original selection remain intact
		testastic.AssertJSON(t, "testdata/release_layout/combined.expected.json", layout.Units())
		testastic.SliceEqual(t, []string{"web", " api ", "web"}, requested)
		testastic.SliceEqual(t, requested, layout.ExpandSelection(requested))
	})
}
