package release

import (
	"context"
	"fmt"

	"github.com/monkescience/yeet/internal/provider"
)

// labelLifecycle owns which phase a release PR is in. Callers name what happened
// to the pull request and never which labels that implies, so the label protocol
// lives entirely on the forge side of the provider seam.
type labelLifecycle struct {
	setter releasePRLabelSetter
	labels provider.ReleasePRLabels
}

func newLabelLifecycle(core *releaseCore, setter releasePRLabelSetter) labelLifecycle {
	return labelLifecycle{setter: setter, labels: core.releasePRLabels()}
}

// opened marks a release PR that is now open and waiting to be tagged, whether
// this run created it or adopted one an interrupted run left unlabelled.
func (l labelLifecycle) opened(ctx context.Context, number int) error {
	if err := l.setter.SetReleasePRLabels(ctx, number, l.labels, provider.ReleasePRPhasePending); err != nil {
		return fmt.Errorf("mark release PR pending: %w", err)
	}

	return nil
}

// published marks a release PR whose releases have been published.
func (l labelLifecycle) published(ctx context.Context, number int) error {
	if err := l.setter.SetReleasePRLabels(ctx, number, l.labels, provider.ReleasePRPhaseTagged); err != nil {
		return fmt.Errorf("mark release PR tagged: %w", err)
	}

	return nil
}
