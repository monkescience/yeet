package release

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/monkescience/yeet/internal/config"
)

var errReleaseTitleFieldUnavailable = errors.New("field is unavailable")

type singleReleaseTitleData struct {
	Branch  string
	Channel string
	Target  string
	Version string
	Tag     string
}

type groupReleaseTitleData struct {
	Branch      string
	Channel     string
	TargetCount int
}

type releaseTitleTemplates struct {
	single        *template.Template
	group         *template.Template
	commitSingle  *template.Template
	commitGrouped *template.Template
}

func newReleaseTitleTemplates(release config.ReleaseConfig) (*releaseTitleTemplates, error) {
	templates := &releaseTitleTemplates{}
	singleFields := map[string]struct{}{"Branch": {}, "Channel": {}, "Target": {}, "Version": {}, "Tag": {}}
	groupFields := map[string]struct{}{"Branch": {}, "Channel": {}, "TargetCount": {}}

	specs := []struct {
		name        string
		source      string
		fields      map[string]struct{}
		destination **template.Template
	}{
		{
			name: "release.pr_title", source: release.PRTitle,
			fields: singleFields, destination: &templates.single,
		},
		{
			name: "release.pr_title_group", source: release.PRTitleGroup,
			fields: groupFields, destination: &templates.group,
		},
		{
			name: "release.commit_subject", source: release.CommitSubject,
			fields: singleFields, destination: &templates.commitSingle,
		},
		{
			name: "release.commit_subject_group", source: release.CommitSubjectGroup,
			fields: groupFields, destination: &templates.commitGrouped,
		},
	}

	for _, spec := range specs {
		if spec.source == "" {
			continue
		}

		parsed, err := parseReleaseTitleTemplate(spec.name, spec.source, spec.fields)
		if err != nil {
			return nil, err
		}

		*spec.destination = parsed
	}

	return templates, nil
}

func parseReleaseTitleTemplate(
	name, source string,
	allowedFields map[string]struct{},
) (*template.Template, error) {
	tmpl, err := template.New(name).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("%w: parse %s: %v", config.ErrInvalidConfig, name, err)
	}

	for _, parsed := range tmpl.Templates() {
		if err := validateReleaseTitleNode(parsed.Root, allowedFields); err != nil {
			return nil, fmt.Errorf("%w: %s: %v", config.ErrInvalidConfig, name, err)
		}
	}

	return tmpl, nil
}

func validateReleaseTitleNode(node parse.Node, allowedFields map[string]struct{}) error {
	if node == nil {
		return nil
	}

	switch typed := node.(type) {
	case *parse.ListNode:
		for _, child := range typed.Nodes {
			if err := validateReleaseTitleNode(child, allowedFields); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return validateReleaseTitleNode(typed.Pipe, allowedFields)
	case *parse.PipeNode:
		for _, command := range typed.Cmds {
			if err := validateReleaseTitleNode(command, allowedFields); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		for _, argument := range typed.Args {
			if err := validateReleaseTitleNode(argument, allowedFields); err != nil {
				return err
			}
		}
	case *parse.FieldNode:
		return validateReleaseTitleField(typed.Ident, typed.String(), allowedFields)
	case *parse.VariableNode:
		return validateReleaseTitleVariable(typed, allowedFields)
	case *parse.ChainNode:
		if err := validateReleaseTitleNode(typed.Node, allowedFields); err != nil {
			return err
		}

		if len(typed.Field) > 0 {
			return fmt.Errorf("%w: %s", errReleaseTitleFieldUnavailable, typed.String())
		}
	case *parse.IfNode:
		return validateReleaseTitleBranch(typed.Pipe, typed.List, typed.ElseList, allowedFields)
	case *parse.RangeNode:
		return validateReleaseTitleBranch(typed.Pipe, typed.List, typed.ElseList, allowedFields)
	case *parse.WithNode:
		return validateReleaseTitleBranch(typed.Pipe, typed.List, typed.ElseList, allowedFields)
	case *parse.TemplateNode:
		return validateReleaseTitleNode(typed.Pipe, allowedFields)
	}

	return nil
}

func validateReleaseTitleField(
	ident []string,
	reference string,
	allowedFields map[string]struct{},
) error {
	if len(ident) != 1 {
		return fmt.Errorf("%w: %s", errReleaseTitleFieldUnavailable, reference)
	}

	if _, ok := allowedFields[ident[0]]; !ok {
		return fmt.Errorf("%w: %s", errReleaseTitleFieldUnavailable, reference)
	}

	return nil
}

func validateReleaseTitleVariable(typed *parse.VariableNode, allowedFields map[string]struct{}) error {
	if len(typed.Ident) == 1 {
		return nil
	}

	if len(typed.Ident) != 2 || typed.Ident[0] != "$" {
		return fmt.Errorf("%w: %s", errReleaseTitleFieldUnavailable, typed.String())
	}

	return validateReleaseTitleField(typed.Ident[1:], typed.String(), allowedFields)
}

func validateReleaseTitleBranch(
	pipe *parse.PipeNode,
	list, elseList *parse.ListNode,
	allowedFields map[string]struct{},
) error {
	if err := validateReleaseTitleNode(pipe, allowedFields); err != nil {
		return err
	}

	if list != nil {
		if err := validateReleaseTitleNode(list, allowedFields); err != nil {
			return err
		}
	}

	if elseList != nil {
		if err := validateReleaseTitleNode(elseList, allowedFields); err != nil {
			return err
		}
	}

	return nil
}

func (c *releaseCore) releasePRTitle(plans []TargetPlan) (string, error) {
	if c.titles == nil {
		return c.releaseSubject(plans), nil
	}

	return c.releaseTemplatedSubject(plans, c.titles.single, c.titles.group)
}

func (c *releaseCore) releaseCommitSubject(plans []TargetPlan) (string, error) {
	if c.titles == nil {
		return c.releaseSubject(plans), nil
	}

	return c.releaseTemplatedSubject(plans, c.titles.commitSingle, c.titles.commitGrouped)
}

func (c *releaseCore) releaseTemplatedSubject(
	plans []TargetPlan,
	single, grouped *template.Template,
) (string, error) {
	if len(plans) == 1 && single != nil {
		plan := plans[0]

		return renderReleaseTitle(single, singleReleaseTitleData{
			Branch:  c.cfg.Branch,
			Channel: strings.TrimSpace(c.cfg.ActiveChannel),
			Target:  plan.ID,
			Version: plan.NextVersion,
			Tag:     plan.NextTag,
		})
	}

	if len(plans) > 1 && grouped != nil {
		return renderReleaseTitle(grouped, groupReleaseTitleData{
			Branch:      c.cfg.Branch,
			Channel:     strings.TrimSpace(c.cfg.ActiveChannel),
			TargetCount: len(plans),
		})
	}

	return c.releaseSubject(plans), nil
}

func renderReleaseTitle(tmpl *template.Template, data any) (string, error) {
	title, err := executeReleaseTitleTemplate(tmpl, data)
	if err != nil {
		return "", fmt.Errorf("%w: render %s: %v", config.ErrInvalidConfig, tmpl.Name(), err)
	}

	return validateRenderedReleaseTitle(tmpl.Name(), title)
}

func validateRenderedReleaseTitle(name, title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", fmt.Errorf("%w: rendered %s must not be empty", config.ErrInvalidConfig, name)
	}

	if strings.ContainsAny(title, "\r\n") {
		return "", fmt.Errorf("%w: rendered %s must be one line", config.ErrInvalidConfig, name)
	}

	return title, nil
}

func executeReleaseTitleTemplate(tmpl *template.Template, data any) (string, error) {
	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return output.String(), nil
}
