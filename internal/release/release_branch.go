package release

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/monkescience/yeet/internal/config"
)

const (
	releaseBranchTemplateName          = "release.branch_template"
	defaultReleaseBranchTemplateSource = "yeet/release-{{ .Branch }}"
)

type releaseBranchTemplateData struct {
	Branch  string
	Channel string
	Unit    string
}

func validateReleaseBranchTemplates(cfg *config.Config) error {
	tmpl, err := newReleaseBranchTemplate(effectiveReleaseBranchTemplateSource(cfg))
	if err != nil {
		return err
	}

	configuredUnits := cfg.ReleaseLayout().Units()

	unitValues := make([]string, 0, len(configuredUnits))
	for _, unit := range configuredUnits {
		unitValues = append(unitValues, unit.BranchValue)
	}

	seen := make(map[string]string, (len(cfg.Release.Channels)+1)*len(unitValues))

	if strings.TrimSpace(cfg.Branch) != "" {
		for _, unit := range unitValues {
			branch, renderErr := renderReleaseBranch(tmpl, cfg.Branch, "", unit)
			if renderErr != nil {
				return renderErr
			}

			if existing, exists := seen[branch]; exists {
				return duplicateReleaseBranchError("branch", unit, existing)
			}

			seen[branch] = releaseBranchOwner("branch", unit)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(cfg.Release.Channels)) {
		channel := cfg.Release.Channels[name]

		for _, unit := range unitValues {
			branch, renderErr := renderReleaseBranch(tmpl, channel.Branch, name, unit)
			if renderErr != nil {
				return renderErr
			}

			owner := fmt.Sprintf("release channel %q", name)
			if existing, exists := seen[branch]; exists {
				return duplicateReleaseBranchError(owner, unit, existing)
			}

			seen[branch] = releaseBranchOwner(owner, unit)
		}
	}

	return nil
}

func effectiveReleaseBranchTemplateSource(cfg *config.Config) string {
	if cfg.Release.PullRequestMode == config.PullRequestModeIndependent &&
		cfg.Release.BranchTemplate == defaultReleaseBranchTemplateSource {
		return defaultReleaseBranchTemplateSource + "-{{ .Unit }}"
	}

	return cfg.Release.BranchTemplate
}

func duplicateReleaseBranchError(owner, unit, existing string) error {
	return fmt.Errorf(
		"%w: rendered %s for %s unit %q duplicates %s, use .Unit to disambiguate release units",
		config.ErrInvalidConfig,
		releaseBranchTemplateName,
		owner,
		unit,
		existing,
	)
}

func releaseBranchOwner(owner, unit string) string {
	if unit == "" {
		return owner
	}

	return fmt.Sprintf("%s unit %q", owner, unit)
}

func newReleaseBranchTemplate(source string) (*template.Template, error) {
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("%w: %s must not be blank", config.ErrInvalidConfig, releaseBranchTemplateName)
	}

	fields := map[string]struct{}{
		releaseTemplateFieldBranch:  {},
		releaseTemplateFieldChannel: {},
		releaseTemplateFieldUnit:    {},
	}

	return parseReleaseTextTemplate(releaseBranchTemplateName, source, fields)
}

func renderReleaseBranch(tmpl *template.Template, baseBranch, channel, unit string) (string, error) {
	branch, err := executeReleaseTextTemplate(tmpl, releaseBranchTemplateData{
		Branch:  strings.TrimSpace(baseBranch),
		Channel: strings.TrimSpace(channel),
		Unit:    strings.TrimSpace(unit),
	})
	if err != nil {
		return "", fmt.Errorf("%w: render %s: %v", config.ErrInvalidConfig, releaseBranchTemplateName, err)
	}

	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("%w: rendered %s must not be empty", config.ErrInvalidConfig, releaseBranchTemplateName)
	}

	if strings.ContainsAny(branch, "\r\n") {
		return "", fmt.Errorf("%w: rendered %s must be one line", config.ErrInvalidConfig, releaseBranchTemplateName)
	}

	if branch == strings.TrimSpace(baseBranch) {
		return "", fmt.Errorf(
			"%w: rendered %s must differ from base branch %q",
			config.ErrInvalidConfig,
			releaseBranchTemplateName,
			baseBranch,
		)
	}

	if branch == "HEAD" {
		return "", fmt.Errorf("%w: rendered %s must not be HEAD", config.ErrInvalidConfig, releaseBranchTemplateName)
	}

	err = plumbing.NewBranchReferenceName(branch).Validate()
	if err != nil {
		return "", fmt.Errorf(
			"%w: rendered %s %q is not a valid git branch: %v",
			config.ErrInvalidConfig,
			releaseBranchTemplateName,
			branch,
			err,
		)
	}

	return branch, nil
}
