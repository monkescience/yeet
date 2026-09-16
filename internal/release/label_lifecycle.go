package release

import (
	"context"
	"fmt"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

type labelLifecycle struct {
	setter releasePRLabelSetter
	labels forge.ReleasePRLabels
}

func newLabelLifecycle(cfg *config.Config, setter releasePRLabelSetter) labelLifecycle {
	return labelLifecycle{setter: setter, labels: releasePRLabels(cfg)}
}

func (l labelLifecycle) opened(ctx context.Context, number int) error {
	err := l.setter.SetReleasePRLabels(ctx, number, l.labels, forge.ReleasePRPhasePending)
	if err != nil {
		return fmt.Errorf("mark release PR pending: %w", err)
	}

	return nil
}

func (l labelLifecycle) published(ctx context.Context, number int) error {
	err := l.setter.SetReleasePRLabels(ctx, number, l.labels, forge.ReleasePRPhaseTagged)
	if err != nil {
		return fmt.Errorf("mark release PR tagged: %w", err)
	}

	return nil
}
