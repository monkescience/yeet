package release

import (
	"time"

	"github.com/monkescience/yeet/internal/config"
)

// releaseCore bundles the resolved configuration and repository metadata shared
// by every release sub-component. It owns no I/O collaborators, so a component
// holding a *releaseCore can read config but cannot reach the provider that
// talks to the forge.
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
