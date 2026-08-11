package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/monkescience/yeet/internal/forge"
)

// labelChange is the managed-set diff for one phase. The anchor is attached
// before anything else and fail-fast, so an interrupted run still leaves a pull
// request the next run can find. The remaining additions are best effort. See
// ADR 0007.
type labelChange struct {
	anchor string
	add    []string
	remove []string
}

// managedLabelChange diffs within the managed set only, which is Pending,
// Tagged, Yeet and Extra. Every other label on the pull request is left where it
// is, so a phase is idempotent for the managed set and says nothing about the
// rest. See ADR 0006.
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

// labelsAnchoredFirst orders the additions with the anchor at the front, so a
// forge attaching the whole set in one request keeps the ordering guarantee a
// forge attaching them one by one gets from the parameter.
func labelsAnchoredFirst(anchor string, add []string) []string {
	return append([]string{anchor}, add...)
}

// releasePRLabelState places a trusted release pull request in the lifecycle by
// its labels alone: carrying the pending label, carrying none at all and so
// adoptable, or carrying something else.
type releasePRLabelState int

const (
	releasePRLabelsPending releasePRLabelState = iota
	releasePRLabelsAdoptable
	releasePRLabelsMismatched
)

// labelMatch reports whether a label found on a pull request is the configured
// one. GitHub and Azure DevOps fold case. GitLab must not: it treats labels
// differing only by case as distinct and filters them server side, so a
// case-insensitive client-side match would disagree with the server and let yeet
// open a second release merge request on the same branch.
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

// releasePRLabelMismatch reports a trusted release pull request whose labels
// name a lifecycle yeet is not configured for, which means renamed configuration
// rather than an interrupted run. reference carries the forge's own wording.
func releasePRLabelMismatch(reference, branch, pendingLabel string) error {
	return fmt.Errorf(
		"%w: trusted %s on branch %q is missing configured pending label %q",
		forge.ErrReleasePRLabelMismatch,
		reference,
		branch,
		pendingLabel,
	)
}

// labelDefinitions is the forge half of label preparation: reading a label
// definition, creating one, and telling a definition that is absent apart from a
// lookup that failed. get and create own their own error wrapping, so the forge
// names the operation and this decides what to do about it. Azure DevOps exposes
// no definition API, so it supplies no labelDefinitions at all rather than a
// stubbed one.
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

// prepare limits definition work to labels the requested phase owns. Tagged
// finalization cannot depend on creation-only labels that it does not mutate.
func (d labelDefinitions) prepare(
	ctx context.Context,
	labels forge.ReleasePRLabels,
	phase forge.ReleasePRPhase,
) error {
	if phase == forge.ReleasePRPhaseTagged {
		return d.ensure(ctx, labels.Tagged, releaseLabelTaggedColor, releaseLabelTaggedDescription)
	}

	if err := d.validateExtras(ctx, labels.Extra); err != nil {
		return err
	}

	if labels.Yeet {
		if err := d.ensure(ctx, ReleaseLabelYeet, releaseLabelYeetColor, releaseLabelYeetDescription); err != nil {
			return err
		}
	}

	if err := d.ensure(ctx, labels.Pending, releaseLabelPendingColor, releaseLabelPendingDescription); err != nil {
		return err
	}

	return d.ensure(ctx, labels.Tagged, releaseLabelTaggedColor, releaseLabelTaggedDescription)
}

// validateExtras runs before the pull request exists as well as before the
// labels are applied, so an unknown extra label fails the run while nothing has
// been mutated yet.
func (d labelDefinitions) validateExtras(ctx context.Context, names []string) error {
	for _, name := range names {
		if err := d.validateExisting(ctx, name, "extra"); err != nil {
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

	return fmt.Errorf("%w: %s label %q", forge.ErrReleasePRLabelMissing, role, name)
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

	if err := d.create(ctx, name, color, description); err != nil {
		return err
	}

	if d.cache != nil {
		d.cache.add(cacheKey)
	}

	return nil
}
