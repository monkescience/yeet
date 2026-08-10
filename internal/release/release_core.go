package release

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

const prBodyPartSeparator = "\n\n"

const prBodyOmittedNotice = "_Release notes omitted to fit this provider's pull request body limit. " +
	"See the changelog in this pull request for the full notes._"

// releaseCore bundles the resolved configuration and repository metadata shared
// by every release sub-component, along with the config-derived text helpers
// used to build PR subjects and bodies. It owns no I/O collaborators, so a
// component holding a *releaseCore can read config and render text but cannot
// reach the provider that talks to the forge.
type releaseCore struct {
	cfg      *config.Config
	targets  map[string]config.ResolvedTarget
	metadata repoMetadataProvider
	titles   *releaseTitleTemplates
}

func (c *releaseCore) activePrereleaseIdentifier() string {
	channelName := strings.TrimSpace(c.cfg.ActiveChannel)
	if channelName == "" {
		return ""
	}

	channel, exists := c.cfg.Release.Channels[channelName]
	if !exists {
		return ""
	}

	return strings.TrimSpace(channel.Prerelease)
}

func (c *releaseCore) isPrerelease() bool {
	return c.activePrereleaseIdentifier() != ""
}

func (c *releaseCore) releasePROptions(
	ctx context.Context,
	plans []TargetPlan,
	releaseBranch string,
	providerBodyLimit int,
) (forge.ReleasePROptions, error) {
	manifest := releaseManifestForPlans(c.cfg.Branch, plans)
	manifest.Channel = strings.TrimSpace(c.cfg.ActiveChannel)
	manifest.Prerelease = c.isPrerelease()

	manifestMarker, err := releaseManifestMarker(manifest)
	if err != nil {
		return forge.ReleasePROptions{}, err
	}

	changelogBody := c.combinedPRChangelog(plans)
	limit := c.effectivePRBodyLimit(providerBodyLimit)

	body, omitted := c.releasePRBody(changelogBody, manifestMarker, limit)
	if omitted {
		slog.WarnContext(ctx, "omitted release notes from PR body to fit provider limit",
			slog.Int("limit", limit),
			slog.Int("body_length", len(body)),
		)
	}

	title, err := c.releasePRTitle(plans)
	if err != nil {
		return forge.ReleasePROptions{}, err
	}

	return forge.ReleasePROptions{
		Title:         title,
		Body:          body,
		BaseBranch:    c.cfg.Branch,
		ReleaseBranch: releaseBranch,
		Reviewers:     c.cfg.Release.Reviewers,
		Labels:        c.releasePRLabels(),
	}, nil
}

func (c *releaseCore) releasePRLabels() forge.ReleasePRLabels {
	return forge.ReleasePRLabels{
		Pending: c.cfg.Release.Labels.Pending,
		Tagged:  c.cfg.Release.Labels.Tagged,
		Yeet:    c.cfg.Release.Labels.Yeet,
		Extra:   append([]string(nil), c.cfg.Release.Labels.Extra...),
	}
}

func (c *releaseCore) effectivePRBodyLimit(providerLimit int) int {
	configured := c.cfg.Release.PRBodyMaxLength

	switch {
	case providerLimit > 0 && configured > 0:
		return min(providerLimit, configured)
	case providerLimit > 0:
		return providerLimit
	default:
		return configured
	}
}

func (c *releaseCore) releaseSubject(plans []TargetPlan) string {
	if len(plans) == 1 {
		return "chore: release " + plans[0].NextVersion
	}

	return "chore: release wave"
}

func (c *releaseCore) combinedPRChangelog(plans []TargetPlan) string {
	if len(plans) == 0 {
		return ""
	}

	if len(plans) == 1 {
		return changelog.Render(preferredPREntry(plans[0]))
	}

	sections := buildPRSections(plans)

	var body strings.Builder
	body.WriteString("## Release wave\n\n")
	fmt.Fprintf(&body, "Base branch: `%s`\n", c.cfg.Branch)
	fmt.Fprintf(&body, "Targets: %s", formatSectionTargetList(sections))

	for _, section := range sections {
		body.WriteString("\n\n")
		body.WriteString(renderFlatPRSection(section))
	}

	return body.String()
}

func (c *releaseCore) releasePRBody(changelogBody, manifestMarker string, limit int) (string, bool) {
	header := strings.TrimSpace(c.cfg.Release.PRBodyHeader)
	notes := strings.TrimSpace(changelogBody)
	marker := strings.TrimSpace(manifestMarker)
	footer := strings.TrimSpace(c.cfg.Release.PRBodyFooter)

	body := joinPRBodyParts(header, notes, marker, footer)
	if limit <= 0 || len(body) <= limit {
		return body, false
	}

	return joinPRBodyParts(header, prBodyOmittedNotice, marker, footer), true
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
