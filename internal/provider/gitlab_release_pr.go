package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const gitlabReleasePendingLabelColor = "#FBCA04"

const gitlabReleaseTaggedLabelColor = "#0E8A16"

const gitlabMergeRequestOpenedState = "opened"

const gitlabMergeRequestMergedState = "merged"

func (g *GitLab) CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error) {
	mr, _, err := g.client.MergeRequests.CreateMergeRequest(g.pid, &gitlab.CreateMergeRequestOptions{
		Title:        new(opts.Title),
		Description:  new(opts.Body),
		SourceBranch: new(opts.ReleaseBranch),
		TargetBranch: new(opts.BaseBranch),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create merge request: %w", err)
	}

	return &PullRequest{
		Number: int(mr.IID),
		Title:  mr.Title,
		Body:   mr.Description,
		URL:    mr.WebURL,
		Branch: opts.ReleaseBranch,
	}, nil
}

func (g *GitLab) UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error {
	_, _, err := g.client.MergeRequests.UpdateMergeRequest(g.pid, int64(number), &gitlab.UpdateMergeRequestOptions{
		Title:       new(opts.Title),
		Description: new(opts.Body),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("update merge request !%d: %w", number, err)
	}

	return nil
}

func (g *GitLab) FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error) {
	state := gitlabMergeRequestOpenedState
	orderBy := "updated_at"
	sortDirection := "desc"
	labels := gitlab.LabelOptions{ReleaseLabelPending}

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		OrderBy:      new(orderBy),
		Sort:         new(sortDirection),
		Labels:       &labels,
		ListOptions:  gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}

	pendingMRs := make([]*PullRequest, 0)

	for {
		mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(g.pid, options, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list merge requests: %w", err)
		}

		for _, mr := range mrs {
			if !strings.HasPrefix(mr.SourceBranch, releaseBranchPrefix) {
				continue
			}

			pendingMRs = append(pendingMRs, &PullRequest{
				Number: int(mr.IID),
				Title:  mr.Title,
				Body:   mr.Description,
				URL:    mr.WebURL,
				Branch: mr.SourceBranch,
			})
		}

		if resp.NextPage == 0 {
			break
		}

		options.Page = resp.NextPage
	}

	return pendingMRs, nil
}

func (g *GitLab) FindMergedReleasePR(ctx context.Context, baseBranch string) (*PullRequest, error) {
	state := gitlabMergeRequestMergedState
	orderBy := "updated_at"
	sortDirection := "desc"
	labels := gitlab.LabelOptions{ReleaseLabelPending}

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		OrderBy:      new(orderBy),
		Sort:         new(sortDirection),
		Labels:       &labels,
		ListOptions:  gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}

	for {
		mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(g.pid, options, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list merge requests: %w", err)
		}

		for _, mr := range mrs {
			if !strings.HasPrefix(mr.SourceBranch, releaseBranchPrefix) {
				continue
			}

			return &PullRequest{
				Number:         int(mr.IID),
				Title:          mr.Title,
				Body:           mr.Description,
				URL:            mr.WebURL,
				Branch:         mr.SourceBranch,
				MergeCommitSHA: gitLabMergeCommitSHA(mr),
			}, nil
		}

		if resp.NextPage == 0 {
			break
		}

		options.Page = resp.NextPage
	}

	return nil, ErrNoPR
}

func (g *GitLab) MarkReleasePRPending(ctx context.Context, number int) error {
	err := g.ensureReleaseLabels(ctx)
	if err != nil {
		return err
	}

	addLabels := gitlab.LabelOptions{ReleaseLabelPending}
	removeLabels := gitlab.LabelOptions{ReleaseLabelTagged}

	_, _, err = g.client.MergeRequests.UpdateMergeRequest(g.pid, int64(number), &gitlab.UpdateMergeRequestOptions{
		AddLabels:    &addLabels,
		RemoveLabels: &removeLabels,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("mark merge request !%d pending: %w", number, err)
	}

	return nil
}

func (g *GitLab) MarkReleasePRTagged(ctx context.Context, number int) error {
	err := g.ensureReleaseLabels(ctx)
	if err != nil {
		return err
	}

	addLabels := gitlab.LabelOptions{ReleaseLabelTagged}
	removeLabels := gitlab.LabelOptions{ReleaseLabelPending}

	_, _, err = g.client.MergeRequests.UpdateMergeRequest(g.pid, int64(number), &gitlab.UpdateMergeRequestOptions{
		AddLabels:    &addLabels,
		RemoveLabels: &removeLabels,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("mark merge request !%d tagged: %w", number, err)
	}

	return nil
}

func gitLabMergeCommitSHA(mergeRequest *gitlab.BasicMergeRequest) string {
	mergeCommitSHA := strings.TrimSpace(mergeRequest.MergeCommitSHA)
	if mergeCommitSHA != "" {
		return mergeCommitSHA
	}

	squashCommitSHA := strings.TrimSpace(mergeRequest.SquashCommitSHA)
	if squashCommitSHA != "" {
		return squashCommitSHA
	}

	slog.Warn("merge request has no merge or squash commit SHA, release will be tagged against branch tip",
		"iid", mergeRequest.IID)

	return ""
}

func (g *GitLab) MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) error {
	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.pid, int64(number), nil, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("get merge request !%d: %w", number, err)
	}

	if mr.State == gitlabMergeRequestMergedState {
		return nil
	}

	if mr.State != gitlabMergeRequestOpenedState {
		return fmt.Errorf("%w: merge request !%d is %s", ErrMergeBlocked, number, mr.State)
	}

	if mr.Draft {
		return fmt.Errorf("%w: merge request !%d is draft", ErrMergeBlocked, number)
	}

	if mr.HasConflicts {
		return fmt.Errorf("%w: merge request !%d has conflicts", ErrMergeBlocked, number)
	}

	if !opts.Force {
		mergeStatus := strings.TrimSpace(mr.DetailedMergeStatus)
		if !isGitLabMergeStatusMergeable(mergeStatus) {
			return fmt.Errorf("%w: merge request !%d detailed_merge_status=%s", ErrMergeBlocked, number, mergeStatus)
		}
	}

	project, err := g.projectMergeSettings(ctx)
	if err != nil {
		return err
	}

	acceptOptions, err := gitLabAcceptMergeOptions(project, opts.Method)
	if err != nil {
		return err
	}

	sha := strings.TrimSpace(mr.SHA)
	if sha != "" {
		acceptOptions.SHA = new(sha)
	}

	_, _, err = g.client.MergeRequests.AcceptMergeRequest(g.pid, int64(number), acceptOptions, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("accept merge request !%d: %w", number, err)
	}

	return nil
}

func (g *GitLab) ensureReleaseLabels(ctx context.Context) error {
	err := g.ensureLabel(ctx, ReleaseLabelPending, gitlabReleasePendingLabelColor, "release PR is pending tagging")
	if err != nil {
		return err
	}

	err = g.ensureLabel(ctx, ReleaseLabelTagged, gitlabReleaseTaggedLabelColor, "release PR already tagged")
	if err != nil {
		return err
	}

	return nil
}

func (g *GitLab) ensureLabel(ctx context.Context, name, color, description string) error {
	_, _, err := g.client.Labels.GetLabel(g.pid, name, gitlab.WithContext(ctx))
	if err == nil {
		return nil
	}

	if !errors.Is(err, gitlab.ErrNotFound) {
		return fmt.Errorf("get label %q: %w", name, err)
	}

	_, _, err = g.client.Labels.CreateLabel(g.pid, &gitlab.CreateLabelOptions{
		Name:        new(name),
		Color:       new(color),
		Description: new(description),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create label %q: %w", name, err)
	}

	return nil
}

func isGitLabMergeStatusMergeable(status string) bool {
	switch status {
	case "", "mergeable", "can_be_merged":
		return true
	default:
		return false
	}
}

func (g *GitLab) projectMergeSettings(ctx context.Context) (*gitlab.Project, error) {
	project, _, err := g.client.Projects.GetProject(g.pid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get project merge settings: %w", err)
	}

	if project == nil {
		return nil, fmt.Errorf("%w: missing project merge settings", ErrMergeBlocked)
	}

	return project, nil
}

func gitLabAcceptMergeOptions(
	project *gitlab.Project,
	requested MergeMethod,
) (*gitlab.AcceptMergeRequestOptions, error) {
	if requested == "" {
		requested = MergeMethodAuto
	}

	options := &gitlab.AcceptMergeRequestOptions{}

	switch requested {
	case MergeMethodAuto:
		return options, nil
	case MergeMethodSquash:
		if project.SquashOption == gitlab.SquashOptionNever {
			return nil, fmt.Errorf(
				"%w: merge method %q disabled by project squash_option=%s",
				ErrMergeBlocked,
				requested,
				project.SquashOption,
			)
		}

		options.Squash = new(true)

		return options, nil
	case MergeMethodRebase:
		if project.MergeMethod != gitlab.RebaseMerge {
			return nil, fmt.Errorf(
				"%w: merge method %q incompatible with project merge_method=%s",
				ErrMergeBlocked,
				requested,
				project.MergeMethod,
			)
		}

		return options, nil
	case MergeMethodMerge:
		if project.MergeMethod != gitlab.NoFastForwardMerge {
			return nil, fmt.Errorf(
				"%w: merge method %q incompatible with project merge_method=%s",
				ErrMergeBlocked,
				requested,
				project.MergeMethod,
			)
		}

		return options, nil
	default:
		return nil, fmt.Errorf("%w: unknown merge method %q", ErrMergeMethodUnsupported, requested)
	}
}
