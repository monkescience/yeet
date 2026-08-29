package release

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

const prBodyPartSeparator = "\n\n"

const prBodyOmittedNotice = "_Release notes omitted to fit this provider's pull request body limit. " +
	"See the changelog in this pull request for the full notes._"

type releaseText struct {
	cfg     *config.Config
	run     releaseRun
	targets map[string]config.ResolvedTarget
	titles  *releaseTitleTemplates
}

// RenderedRelease contains every rendered value used to publish a release wave.
type RenderedRelease struct {
	PROptions     forge.ReleasePROptions
	CommitSubject string
	ReleaseNames  map[string]string
	NotesOmitted  bool
	bodyLimit     int
}

func newReleaseText(
	cfg *config.Config,
	run releaseRun,
	targets map[string]config.ResolvedTarget,
) (*releaseText, error) {
	titles, err := newReleaseTitleTemplates(cfg.Release)
	if err != nil {
		return nil, err
	}

	return &releaseText{cfg: cfg, run: run, targets: targets, titles: titles}, nil
}

func (t *releaseText) validate(plans []TargetPlan, bodyLimit int) error {
	if len(plans) == 0 {
		return nil
	}

	_, err := t.render(plans, t.run.releaseBranch, bodyLimit)

	return err
}

func (t *releaseText) render(
	plans []TargetPlan,
	releaseBranch string,
	providerBodyLimit int,
	unitIDs ...string,
) (*RenderedRelease, error) {
	manifest := releaseManifestForPlans(t.run.baseBranch, plans, unitIDs...)
	if len(unitIDs) > 0 && unitIDs[0] != combinedReleaseUnitID {
		manifest.ConfiguredTargets = t.configuredManifestTargets(unitIDs[0])
	}

	manifest.Channel = t.run.channelName
	manifest.Prerelease = t.run.isPrerelease()

	manifestMarker, err := releaseManifestMarker(manifest)
	if err != nil {
		return nil, err
	}

	limit := t.effectivePRBodyLimit(providerBodyLimit)

	body, omitted, err := t.releasePRBody(t.combinedPRChangelog(plans), manifestMarker, limit)
	if err != nil {
		return nil, err
	}

	title, err := t.releasePRTitle(plans)
	if err != nil {
		return nil, err
	}

	commitSubject, err := t.releaseCommitSubject(plans)
	if err != nil {
		return nil, err
	}

	releaseNames := make(map[string]string, len(plans))
	for _, plan := range plans {
		releaseNames[plan.ID], err = t.releaseNameForPlan(plan)
		if err != nil {
			return nil, err
		}
	}

	return &RenderedRelease{
		PROptions: forge.ReleasePROptions{
			Title:         title,
			Body:          body,
			BaseBranch:    t.run.baseBranch,
			ReleaseBranch: releaseBranch,
			Reviewers:     t.cfg.Release.Reviewers,
			Labels:        releasePRLabels(t.cfg),
		},
		CommitSubject: commitSubject,
		ReleaseNames:  releaseNames,
		NotesOmitted:  omitted,
		bodyLimit:     limit,
	}, nil
}

func (t *releaseText) configuredManifestTargets(unitID string) []releaseManifestTarget {
	targetIDs := make([]string, 0)
	if targetID, found := strings.CutPrefix(unitID, "target:"); found {
		targetIDs = append(targetIDs, targetID)
	} else if groupName, found := strings.CutPrefix(unitID, "group:"); found {
		group, exists := releaseGroupConfig(t.cfg, groupName)
		if exists {
			targetIDs = append(targetIDs, group.Targets...)
		}
	}

	slices.Sort(targetIDs)

	targets := make([]releaseManifestTarget, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		target, exists := t.targets[strings.TrimSpace(targetID)]
		if !exists {
			continue
		}

		targets = append(targets, releaseManifestTarget{ID: target.ID, Type: string(target.Type)})
	}

	return targets
}

func (t *releaseText) nameForManifest(entry releaseManifestEntry) (string, error) {
	target, exists := t.targets[entry.ID]
	if !exists {
		return "", fmt.Errorf("%w: unknown target %q", errInvalidReleaseManifest, entry.ID)
	}

	versionValue, err := versionStrategyForResolvedTarget(target).strategy.Current(entry.Tag)
	if err != nil {
		return "", fmt.Errorf("%w: target %q tag is invalid: %v", errInvalidReleaseManifest, entry.ID, err)
	}

	return t.renderReleaseName(entry.ID, versionValue, entry.Tag)
}

func releasePRLabels(cfg *config.Config) forge.ReleasePRLabels {
	return forge.ReleasePRLabels{
		Pending: cfg.Release.Labels.Pending,
		Tagged:  cfg.Release.Labels.Tagged,
		Yeet:    cfg.Release.Labels.Yeet,
		Extra:   append([]string(nil), cfg.Release.Labels.Extra...),
	}
}

func (t *releaseText) effectivePRBodyLimit(providerLimit int) int {
	configured := t.cfg.Release.PRBodyMaxLength

	switch {
	case providerLimit > 0 && configured > 0:
		return min(providerLimit, configured)
	case providerLimit > 0:
		return providerLimit
	default:
		return configured
	}
}

func (t *releaseText) releaseSubject(plans []TargetPlan) string {
	if len(plans) == 1 {
		return "chore: release " + plans[0].NextVersion
	}

	return "chore: release wave"
}

func (t *releaseText) combinedPRChangelog(plans []TargetPlan) string {
	if len(plans) == 0 {
		return ""
	}

	if len(plans) == 1 {
		return changelog.Render(preferredPREntry(plans[0]))
	}

	sections := buildPRSections(plans)

	var body strings.Builder
	body.WriteString("## Release wave\n\n")
	fmt.Fprintf(&body, "Base branch: `%s`\n", t.run.baseBranch)
	fmt.Fprintf(&body, "Targets: %s", formatSectionTargetList(sections))

	for _, section := range sections {
		body.WriteString("\n\n")
		body.WriteString(renderFlatPRSection(section))
	}

	return body.String()
}

func (t *releaseText) releasePRBody(
	changelogBody, manifestMarker string,
	limit int,
) (string, bool, error) {
	header := strings.TrimSpace(t.cfg.Release.PRBodyHeader)
	notes := strings.TrimSpace(changelogBody)
	marker := strings.TrimSpace(manifestMarker)
	footer := strings.TrimSpace(t.cfg.Release.PRBodyFooter)

	body := joinPRBodyParts(header, notes, marker, footer)
	if limit <= 0 || utf8.RuneCountInString(body) <= limit {
		return body, false, nil
	}

	body = joinPRBodyParts(header, prBodyOmittedNotice, marker, footer)
	if bodyLength := utf8.RuneCountInString(body); bodyLength > limit {
		return "", false, fmt.Errorf(
			"%w: release PR body requires %d characters after omitting release notes, limit is %d",
			config.ErrInvalidConfig,
			bodyLength,
			limit,
		)
	}

	return body, true, nil
}

func joinPRBodyParts(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))

	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}

	return strings.Join(nonEmpty, prBodyPartSeparator)
}
