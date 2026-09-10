package release

import "github.com/monkescience/yeet/internal/config"

type AutoMergeMode string

const (
	AutoMergeModeOff      AutoMergeMode = "off"
	AutoMergeModeProvider AutoMergeMode = "provider"
	AutoMergeModeDirect   AutoMergeMode = "direct"
)

func (o Options) ResolveAutoMerge(cfg config.ReleaseConfig) AutoMergeMode {
	enabled := cfg.AutoMerge

	if o.AutoMerge != nil {
		enabled = *o.AutoMerge
	}

	if !enabled {
		return AutoMergeModeOff
	}

	return AutoMergeMode(o.resolveAutoMergeMode(cfg))
}

func (o Options) resolveAutoMergeMode(cfg config.ReleaseConfig) config.AutoMergeMode {
	mode := cfg.AutoMergeMode

	if o.AutoMergeMode != nil {
		mode = config.AutoMergeMode(*o.AutoMergeMode)
	}

	return mode
}
