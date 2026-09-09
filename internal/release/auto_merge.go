package release

import "github.com/monkescience/yeet/internal/config"

type AutoMergeMode string

const (
	AutoMergeModeOff    AutoMergeMode = "off"
	AutoMergeModeNormal AutoMergeMode = "normal"
	AutoMergeModeForce  AutoMergeMode = "force"
)

func (o Options) ResolveAutoMerge(cfg config.ReleaseConfig) AutoMergeMode {
	enabled := cfg.AutoMerge
	forced := cfg.AutoMergeForce

	if o.AutoMerge != nil {
		enabled = *o.AutoMerge
		if !enabled {
			forced = false
		}
	}

	if o.AutoMergeForce != nil {
		forced = *o.AutoMergeForce
	}

	if forced {
		return AutoMergeModeForce
	}

	if enabled {
		return AutoMergeModeNormal
	}

	return AutoMergeModeOff
}
