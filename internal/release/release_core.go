package release

import (
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

// releaseCore bundles the resolved configuration and repository metadata shared
// by every release sub-component, along with the config-derived text helpers
// used to build PR subjects and bodies. It owns no I/O collaborators, so a
// component holding a *releaseCore can read config and render text but cannot
// reach the provider that talks to the forge.
type releaseCore struct {
	cfg      *config.Config
	targets  map[string]config.ResolvedTarget
	metadata repoMetadataProvider
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

func (c *releaseCore) releasePROptions(result *Result, releaseBranch string) (provider.ReleasePROptions, error) {
	manifest := releaseManifestForPlans(result.BaseBranch, result.Plans)
	manifest.Channel = strings.TrimSpace(c.cfg.ActiveChannel)
	manifest.Prerelease = c.isPrerelease()

	manifestMarker, err := releaseManifestMarker(manifest)
	if err != nil {
		return provider.ReleasePROptions{}, err
	}

	changelogBody := c.combinedPRChangelog(result)

	return provider.ReleasePROptions{
		Title:         c.releaseSubject(result),
		Body:          c.releasePRBody(changelogBody, manifestMarker),
		BaseBranch:    c.cfg.Branch,
		ReleaseBranch: releaseBranch,
		Files:         map[string]string{},
	}, nil
}

func (c *releaseCore) releaseSubject(result *Result) string {
	plans := result.Plans
	if len(plans) == 1 {
		version := plans[0].NextVersion

		if c.cfg.Release.SubjectIncludeBranch {
			return fmt.Sprintf("chore(%s): release %s", c.cfg.Branch, version)
		}

		return "chore: release " + version
	}

	if c.cfg.Release.SubjectIncludeBranch {
		return fmt.Sprintf("chore(%s): release wave", c.cfg.Branch)
	}

	return "chore: release wave"
}

func (c *releaseCore) combinedPRChangelog(result *Result) string {
	plans := result.Plans
	if len(plans) == 0 {
		return ""
	}

	if len(plans) == 1 {
		if plans[0].PRChangelog != "" {
			return plans[0].PRChangelog
		}

		return plans[0].Changelog
	}

	sections := buildPRSections(plans)

	var body strings.Builder
	body.WriteString("## Release wave\n\n")
	fmt.Fprintf(&body, "Base branch: `%s`\n", result.BaseBranch)
	fmt.Fprintf(&body, "Targets: %s", formatSectionTargetList(sections))

	for _, section := range sections {
		body.WriteString("\n\n")
		body.WriteString(renderFlatPRSection(section))
	}

	return body.String()
}

func (c *releaseCore) releasePRBody(changelogBody, manifestMarker string) string {
	parts := make([]string, 0)

	if header := strings.TrimSpace(c.cfg.Release.PRBodyHeader); header != "" {
		parts = append(parts, header)
	}

	if body := strings.TrimSpace(changelogBody); body != "" {
		parts = append(parts, body)
	}

	if marker := strings.TrimSpace(manifestMarker); marker != "" {
		parts = append(parts, marker)
	}

	if footer := strings.TrimSpace(c.cfg.Release.PRBodyFooter); footer != "" {
		parts = append(parts, footer)
	}

	return strings.Join(parts, "\n\n")
}
