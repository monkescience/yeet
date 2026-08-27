package release

import (
	"context"
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/history"
)

func TestReleaseTimezoneUsesOneCapturedCalendarDate(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Timezone = "America/Los_Angeles"
	cfg.Versioning = config.VersioningCalVer
	cfg.Targets = map[string]config.Target{
		"app": {
			Type:      config.TargetTypePath,
			Path:      ".",
			TagPrefix: "v",
		},
	}

	stub := newProviderStub()
	stub.commits = []history.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "fix: close year boundary bug",
	}}

	now := time.Date(2026, time.January, 1, 0, 30, 0, 0, time.UTC)
	run, err := resolveRun(cfg, cfg.Branch, Options{})
	testastic.NoError(t, err)

	core, err := newReleaseCoreAt(t.Context(), cfg, stub, run, now)
	testastic.NoError(t, err)

	r, err := newReleaser(core, sourceFromTestDeps(cfg.Branch, stub), stub, stub, stub)
	testastic.NoError(t, err)

	result, err := r.Release(context.Background(), true)

	testastic.NoError(t, err)
	testastic.Len(t, result.Plans, 1)
	testastic.Equal(t, "2025.12.1", result.Plans[0].NextVersion)
	testastic.Equal(t, "2025-12-31", result.Plans[0].Entry.Date.Format("2006-01-02"))
	testastic.Equal(t, result.Plans[0].Entry.Date, result.Plans[0].PREntry.Date)
}
