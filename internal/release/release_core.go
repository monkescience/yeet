package release

import (
	"time"

	"github.com/monkescience/yeet/internal/config"
)

type releaseCore struct {
	cfg         *config.Config
	run         releaseRun
	targets     map[string]config.ResolvedTarget
	layout      config.ReleaseLayout
	metadata    repoMetadataProvider
	releaseTime time.Time
}

func (c *releaseCore) timestamp() time.Time {
	if c.releaseTime.IsZero() {
		return time.Now()
	}

	return c.releaseTime
}
