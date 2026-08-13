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

const releaseBranchTemplateName = "release.branch_template"

type releaseBranchTemplateData struct {
	Branch  string
	Channel string
}

func validateReleaseBranchTemplates(cfg *config.Config) error {
	tmpl, err := newReleaseBranchTemplate(cfg.Release.BranchTemplate)
	if err != nil {
		return err
	}

	seen := make(map[string]string, len(cfg.Release.Channels)+1)

	if strings.TrimSpace(cfg.Branch) != "" {
		branch, renderErr := renderReleaseBranch(tmpl, cfg.Branch, "")
		if renderErr != nil {
			return renderErr
		}

		seen[branch] = "branch"
	}

	for _, name := range slices.Sorted(maps.Keys(cfg.Release.Channels)) {
		channel := cfg.Release.Channels[name]

		branch, renderErr := renderReleaseBranch(tmpl, channel.Branch, name)
		if renderErr != nil {
			return renderErr
		}

		if existing, exists := seen[branch]; exists {
			return fmt.Errorf(
				"%w: rendered %s for release channel %q duplicates %s",
				config.ErrInvalidConfig,
				releaseBranchTemplateName,
				name,
				existing,
			)
		}

		seen[branch] = fmt.Sprintf("release channel %q", name)
	}

	return nil
}

func releaseBranchForConfig(cfg *config.Config) (string, error) {
	tmpl, err := newReleaseBranchTemplate(cfg.Release.BranchTemplate)
	if err != nil {
		return "", err
	}

	return renderReleaseBranch(tmpl, cfg.Branch, cfg.ActiveChannel)
}

func newReleaseBranchTemplate(source string) (*template.Template, error) {
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("%w: %s must not be blank", config.ErrInvalidConfig, releaseBranchTemplateName)
	}

	fields := map[string]struct{}{"Branch": {}, "Channel": {}}

	return parseReleaseTextTemplate(releaseBranchTemplateName, source, fields)
}

func renderReleaseBranch(tmpl *template.Template, baseBranch, channel string) (string, error) {
	branch, err := executeReleaseTextTemplate(tmpl, releaseBranchTemplateData{
		Branch:  strings.TrimSpace(baseBranch),
		Channel: strings.TrimSpace(channel),
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
