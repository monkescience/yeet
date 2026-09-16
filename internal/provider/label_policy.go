package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/monkescience/yeet/internal/forge"
)

type labelChange struct {
	anchor string
	add    []string
	remove []string
}

func managedLabelChange(labels forge.ReleasePRLabels, phase forge.ReleasePRPhase) labelChange {
	if phase == forge.ReleasePRPhaseTagged {
		return labelChange{
			anchor: labels.Tagged,
			remove: []string{labels.Pending},
		}
	}

	add := slices.Clone(labels.Extra)
	if labels.Yeet {
		add = append(add, ReleaseLabelYeet)
	}

	return labelChange{
		anchor: labels.Pending,
		add:    add,
		remove: []string{labels.Tagged},
	}
}

func labelsAnchoredFirst(anchor string, add []string) []string {
	return append([]string{anchor}, add...)
}

type releasePRLabelState int

const (
	releasePRLabelsPending releasePRLabelState = iota
	releasePRLabelsAdoptable
	releasePRLabelsMismatched
)

type labelMatch func(found, configured string) bool

func foldedLabelMatch(found, configured string) bool {
	return strings.EqualFold(found, configured)
}

func exactLabelMatch(found, configured string) bool {
	return found == configured
}

func classifyReleasePRLabels(found []string, pendingLabel string, match labelMatch) releasePRLabelState {
	for _, label := range found {
		if match(label, pendingLabel) {
			return releasePRLabelsPending
		}
	}

	if len(found) > 0 {
		return releasePRLabelsMismatched
	}

	return releasePRLabelsAdoptable
}

func needsPendingLabel(
	found []string,
	pendingLabel string,
	match labelMatch,
	reference, branch string,
) (bool, error) {
	state := classifyReleasePRLabels(found, pendingLabel, match)
	if state == releasePRLabelsMismatched {
		return false, releasePRLabelMismatch(reference, branch, pendingLabel)
	}

	return state == releasePRLabelsAdoptable, nil
}

func releasePRLabelMismatch(reference, branch, pendingLabel string) error {
	return &LabelError{
		Label:     pendingLabel,
		Role:      "pending",
		Reference: reference,
		Branch:    branch,
		Err: fmt.Errorf(
			"%w: trusted %s on branch %q is missing configured pending label %q",
			forge.ErrReleasePRLabelMismatch,
			reference,
			branch,
			pendingLabel,
		),
	}
}

type labelDefinitions struct {
	get        func(ctx context.Context, name string) error
	create     func(ctx context.Context, name, color, description string) error
	isNotFound func(err error) bool
	cache      *labelDefinitionCache
	normalize  func(name string) string
}

type labelDefinitionCache struct {
	mu    sync.Mutex
	names map[string]struct{}
}

func (c *labelDefinitionCache) contains(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, exists := c.names[name]

	return exists
}

func (c *labelDefinitionCache) add(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.names == nil {
		c.names = make(map[string]struct{})
	}

	c.names[name] = struct{}{}
}

func (d labelDefinitions) cacheKey(name string) string {
	if d.normalize == nil {
		return name
	}

	return d.normalize(name)
}

func (d labelDefinitions) prepare(
	ctx context.Context,
	labels forge.ReleasePRLabels,
	phase forge.ReleasePRPhase,
) error {
	if phase == forge.ReleasePRPhaseTagged {
		return d.ensure(ctx, labels.Tagged, releaseLabelTaggedColor, releaseLabelTaggedDescription)
	}

	err := d.validateExtras(ctx, labels.Extra)
	if err != nil {
		return err
	}

	if labels.Yeet {
		err = d.ensure(ctx, ReleaseLabelYeet, releaseLabelYeetColor, releaseLabelYeetDescription)
		if err != nil {
			return err
		}
	}

	err = d.ensure(ctx, labels.Pending, releaseLabelPendingColor, releaseLabelPendingDescription)
	if err != nil {
		return err
	}

	return d.ensure(ctx, labels.Tagged, releaseLabelTaggedColor, releaseLabelTaggedDescription)
}

func (d labelDefinitions) validateExtras(ctx context.Context, names []string) error {
	for _, name := range names {
		err := d.validateExisting(ctx, name, "extra")
		if err != nil {
			return err
		}
	}

	return nil
}

func (d labelDefinitions) validateExisting(ctx context.Context, name, role string) error {
	cacheKey := d.cacheKey(name)
	if d.cache != nil && d.cache.contains(cacheKey) {
		return nil
	}

	err := d.get(ctx, name)
	if err == nil {
		if d.cache != nil {
			d.cache.add(cacheKey)
		}

		return nil
	}

	if !d.isNotFound(err) {
		return err
	}

	return &LabelError{
		Label: name,
		Role:  role,
		Err:   fmt.Errorf("%w: %s label %q", forge.ErrReleasePRLabelMissing, role, name),
	}
}

func (d labelDefinitions) ensure(ctx context.Context, name, color, description string) error {
	cacheKey := d.cacheKey(name)
	if d.cache != nil && d.cache.contains(cacheKey) {
		return nil
	}

	err := d.get(ctx, name)
	if err == nil {
		if d.cache != nil {
			d.cache.add(cacheKey)
		}

		return nil
	}

	if !d.isNotFound(err) {
		return err
	}

	err = d.create(ctx, name, color, description)
	if err != nil {
		return err
	}

	if d.cache != nil {
		d.cache.add(cacheKey)
	}

	return nil
}
