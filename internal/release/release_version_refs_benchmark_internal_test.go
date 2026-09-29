package release

import (
	"fmt"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func BenchmarkVersionHistoryRefs(b *testing.B) {
	cfg := config.Default()
	cfg.Targets = make(map[string]config.Target)
	scan := &historyScan{}

	for _, scheme := range []struct {
		id, format, tagFormat string
	}{
		{"month", "YYYY.0M.MICRO", "2026.02.%d"},
		{"year", "YYYY.MICRO", "2026.%d"},
		{"day", "YYYY.0M.0D.MICRO", "2026.02.10.%d"},
		{"week", "YYYY.0W.MICRO", "2026.06.%d"},
	} {
		cfg.Targets[scheme.id] = config.Target{
			Type: config.TargetTypePath, Path: scheme.id, TagPrefix: scheme.id + "/",
			Versioning: config.VersioningCalVer, CalVer: config.CalVerConfig{Format: scheme.format},
		}
		for index := range 1000 {
			scan.tags = append(scan.tags, scheme.id+"/"+fmt.Sprintf(scheme.tagFormat, (index*997)%1000))
		}
	}

	run, err := resolveRun(cfg, cfg.Branch, Options{})
	testastic.NoError(b, err)
	core, err := newReleaseCore(b.Context(), cfg, &repoMetadataStub{}, run)
	testastic.NoError(b, err)

	analyzer := newReleaseAnalyzer(core, nil)

	b.ReportAllocs()

	for b.Loop() {
		for _, target := range core.targets {
			refs := analyzer.versionHistoryRefs(scan, target)
			if len(refs) != 1000 {
				b.Fatalf("expected 1000 refs for %s, got %d", target.ID, len(refs))
			}
		}
	}
}
