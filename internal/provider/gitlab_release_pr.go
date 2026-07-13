package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const gitlabMergeRequestOpenedState = "opened"

const gitlabMergeRequestMergedState = "merged"

func (g *GitLab) CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error) {
	slog.DebugContext(ctx, "gitlab: creating merge request",
		slog.String("source_branch", opts.ReleaseBranch),
		slog.String("target_branch", opts.BaseBranch),
	)

	reviewerIDs, err := g.resolveReviewerIDs(ctx, opts.Reviewers)
	if err != nil {
		return nil, err
	}

	createOptions := &gitlab.CreateMergeRequestOptions{
		Title:        new(opts.Title),
		Description:  new(opts.Body),
		SourceBranch: new(opts.ReleaseBranch),
		TargetBranch: new(opts.BaseBranch),
	}
	if len(reviewerIDs) > 0 {
		createOptions.ReviewerIDs = new(reviewerIDs)
	}

	mr, _, err := g.client.MergeRequests.CreateMergeRequest(g.projectID, createOptions, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create merge request: %w", err)
	}

	if err := verifyGitLabReviewers(opts.Reviewers, reviewerIDs, mr.Reviewers); err != nil {
		if markErr := g.MarkReleasePRPending(ctx, int(mr.IID)); markErr != nil {
			return nil, errors.Join(err, markErr)
		}

		return nil, err
	}

	slog.DebugContext(ctx, "gitlab: created merge request",
		slog.Int("iid", int(mr.IID)),
		slog.String("url", mr.WebURL),
	)

	return &PullRequest{
		Number: int(mr.IID),
		Title:  mr.Title,
		Body:   mr.Description,
		URL:    mr.WebURL,
		Branch: opts.ReleaseBranch,
	}, nil
}

// resolveReviewerIDs resolves usernames against project members (including
// inherited group members) instead of the instance-wide users API: on
// gitlab.com an instance-wide lookup resolves almost any typo to some
// unrelated account, and GitLab silently drops reviewer IDs that cannot read
// the merge request instead of failing.
func (g *GitLab) resolveReviewerIDs(ctx context.Context, usernames []string) ([]int64, error) {
	if len(usernames) == 0 {
		return nil, nil
	}

	slog.DebugContext(ctx, "gitlab: resolving reviewers", slog.Any("reviewers", usernames))

	ids := make([]int64, 0, len(usernames))

	for _, username := range usernames {
		members, _, err := g.client.ProjectMembers.ListAllProjectMembers(g.projectID, &gitlab.ListProjectMembersOptions{
			Query: new(username),
		}, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("look up reviewer %q: %w", username, err)
		}

		id, found := matchGitLabMember(members, username)
		if !found {
			return nil, fmt.Errorf("%w: %q is not a project member", ErrReviewerNotFound, username)
		}

		ids = append(ids, id)
	}

	return ids, nil
}

// matchGitLabMember picks the exact username match: the members query
// parameter is a fuzzy search over name and username.
func matchGitLabMember(members []*gitlab.ProjectMember, username string) (int64, bool) {
	for _, member := range members {
		if strings.EqualFold(member.Username, username) {
			return member.ID, true
		}
	}

	return 0, false
}

// verifyGitLabReviewers guards against GitLab silently applying fewer
// reviewers than requested: the create API drops IDs without read access, and
// the Free tier truncates the list to a single reviewer, both without error.
func verifyGitLabReviewers(usernames []string, requestedIDs []int64, applied []*gitlab.BasicUser) error {
	appliedIDs := make(map[int64]struct{}, len(applied))
	for _, user := range applied {
		appliedIDs[user.ID] = struct{}{}
	}

	missing := make([]string, 0, len(requestedIDs))

	for i, id := range requestedIDs {
		if _, exists := appliedIDs[id]; !exists {
			missing = append(missing, usernames[i])
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"%w: %v (multiple merge request reviewers require GitLab Premium or Ultimate)",
			ErrReviewerNotApplied,
			missing,
		)
	}

	return nil
}

// MaxPRBodyLength reports no enforced limit: GitLab accepts merge request
// descriptions far larger than the release notes yeet generates.
func (g *GitLab) MaxPRBodyLength() int {
	return 0
}

func (g *GitLab) UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error {
	slog.DebugContext(ctx, "gitlab: updating merge request", slog.Int("iid", number))

	_, _, err := g.client.MergeRequests.UpdateMergeRequest(g.projectID, int64(number), &gitlab.UpdateMergeRequestOptions{
		Title:       new(opts.Title),
		Description: new(opts.Body),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("update merge request !%d: %w", number, err)
	}

	slog.DebugContext(ctx, "gitlab: updated merge request", slog.Int("iid", number))

	return nil
}

func (g *GitLab) FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error) {
	state := gitlabMergeRequestOpenedState
	orderBy := "updated_at"
	sortDirection := sortDirectionDesc
	labels := gitlab.LabelOptions{ReleaseLabelPending}

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		OrderBy:      new(orderBy),
		Sort:         new(sortDirection),
		Labels:       &labels,
		ListOptions:  gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}

	slog.DebugContext(ctx, "gitlab: listing open pending release MRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", ReleaseLabelPending),
	)

	pendingMRs := make([]*PullRequest, 0)

	err := paginate(ctx, "listing open pending release MRs",
		func(page int) ([]*gitlab.BasicMergeRequest, int, error) {
			options.Page = int64(page)

			mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(g.projectID, options, gitlab.WithContext(ctx))
			if err != nil {
				return nil, 0, fmt.Errorf("list merge requests: %w", err)
			}

			return mrs, gitLabNextPage(resp), nil
		},
		func(mr *gitlab.BasicMergeRequest) (bool, error) {
			if !strings.HasPrefix(mr.SourceBranch, releaseBranchPrefix) {
				return false, nil
			}

			pendingMRs = append(pendingMRs, &PullRequest{
				Number: int(mr.IID),
				Title:  mr.Title,
				Body:   mr.Description,
				URL:    mr.WebURL,
				Branch: mr.SourceBranch,
			})

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "gitlab: listed open pending release MRs", slog.Int("count", len(pendingMRs)))

	return pendingMRs, nil
}

//nolint:funlen // Pagination closure layout inflates line count without adding complexity.
func (g *GitLab) FindMergedReleasePR(ctx context.Context, baseBranch string) (*PullRequest, error) {
	state := gitlabMergeRequestMergedState
	orderBy := "updated_at"
	sortDirection := sortDirectionDesc
	labels := gitlab.LabelOptions{ReleaseLabelPending}

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		OrderBy:      new(orderBy),
		Sort:         new(sortDirection),
		Labels:       &labels,
		ListOptions:  gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}

	slog.DebugContext(ctx, "gitlab: searching merged release MRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", ReleaseLabelPending),
	)

	var (
		bestMR       *gitlab.BasicMergeRequest
		bestMergedAt time.Time
	)

	err := paginate(ctx, "listing merged release MRs",
		func(page int) ([]*gitlab.BasicMergeRequest, int, error) {
			options.Page = int64(page)

			mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(g.projectID, options, gitlab.WithContext(ctx))
			if err != nil {
				return nil, 0, fmt.Errorf("list merge requests: %w", err)
			}

			return mrs, gitLabNextPage(resp), nil
		},
		func(mr *gitlab.BasicMergeRequest) (bool, error) {
			if !strings.HasPrefix(mr.SourceBranch, releaseBranchPrefix) {
				return false, nil
			}

			mergedAt := gitLabMergedAt(mr)
			if bestMR != nil && !mergedAt.After(bestMergedAt) {
				return false, nil
			}

			bestMR = mr
			bestMergedAt = mergedAt

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	if bestMR == nil {
		return nil, ErrNoPR
	}

	found := &PullRequest{
		Number:         int(bestMR.IID),
		Title:          bestMR.Title,
		Body:           bestMR.Description,
		URL:            bestMR.WebURL,
		Branch:         bestMR.SourceBranch,
		MergeCommitSHA: gitLabMergeCommitSHA(ctx, bestMR),
	}

	slog.DebugContext(ctx, "gitlab: found merged release MR",
		slog.Int("iid", found.Number),
		slog.String("url", found.URL),
		slog.String("merge_sha", found.MergeCommitSHA),
	)

	return found, nil
}

func (g *GitLab) MarkReleasePRPending(ctx context.Context, number int) error {
	err := g.ensureReleaseLabels(ctx)
	if err != nil {
		return err
	}

	addLabels := gitlab.LabelOptions{ReleaseLabelPending}
	removeLabels := gitlab.LabelOptions{ReleaseLabelTagged}

	_, _, err = g.client.MergeRequests.UpdateMergeRequest(g.projectID, int64(number), &gitlab.UpdateMergeRequestOptions{
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

	_, _, err = g.client.MergeRequests.UpdateMergeRequest(g.projectID, int64(number), &gitlab.UpdateMergeRequestOptions{
		AddLabels:    &addLabels,
		RemoveLabels: &removeLabels,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("mark merge request !%d tagged: %w", number, err)
	}

	return nil
}

func (g *GitLab) CommitPullRequestBody(ctx context.Context, hash string) (string, bool, error) {
	commitHash := strings.TrimSpace(hash)
	if commitHash == "" {
		return "", false, nil
	}

	var (
		body  string
		found bool
	)

	err := paginate(ctx, fmt.Sprintf("listing merge requests for commit %q", commitHash),
		func(page int) ([]*gitlab.BasicMergeRequest, int, error) {
			options := []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)}
			if page > 0 {
				options = append(options, gitlab.WithOffsetPaginationParameters(int64(page)))
			}

			mrs, resp, err := g.client.Commits.ListMergeRequestsByCommit(g.projectID, commitHash, options...)
			if err != nil {
				return nil, 0, fmt.Errorf("list merge requests for commit %q: %w", commitHash, err)
			}

			return mrs, gitLabNextPage(resp), nil
		},
		func(mr *gitlab.BasicMergeRequest) (bool, error) {
			if gitLabMergeRequestCommitSHA(mr) != commitHash {
				return false, nil
			}

			body = mr.Description
			found = true

			return true, nil
		},
	)
	if err != nil {
		return "", false, err
	}

	return body, found, nil
}

func gitLabMergedAt(mergeRequest *gitlab.BasicMergeRequest) time.Time {
	if mergeRequest.MergedAt == nil {
		return time.Time{}
	}

	return *mergeRequest.MergedAt
}

func gitLabMergeRequestCommitSHA(mergeRequest *gitlab.BasicMergeRequest) string {
	mergeCommitSHA := strings.TrimSpace(mergeRequest.MergeCommitSHA)
	if mergeCommitSHA != "" {
		return mergeCommitSHA
	}

	return strings.TrimSpace(mergeRequest.SquashCommitSHA)
}

func gitLabMergeCommitSHA(ctx context.Context, mergeRequest *gitlab.BasicMergeRequest) string {
	commitSHA := gitLabMergeRequestCommitSHA(mergeRequest)
	if commitSHA != "" {
		return commitSHA
	}

	slog.WarnContext(ctx, "merge request has no merge or squash commit SHA, release will be tagged against branch tip",
		slog.Int64("iid", mergeRequest.IID))

	return ""
}

func (g *GitLab) MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) error {
	slog.DebugContext(ctx, "gitlab: merging merge request", slog.Int("iid", number))

	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.projectID, int64(number), nil, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("get merge request !%d: %w", number, err)
	}

	if mr.State == gitlabMergeRequestMergedState {
		return nil
	}

	if err := validateGitLabMergeRequestForMerge(number, mr, opts.BypassMergeChecks); err != nil {
		return err
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

	_, _, err = g.client.MergeRequests.AcceptMergeRequest(
		g.projectID,
		int64(number),
		acceptOptions,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("accept merge request !%d: %w", number, err)
	}

	slog.DebugContext(ctx, "gitlab: merged merge request",
		slog.Int("iid", number),
		slog.String("sha", sha),
	)

	return nil
}

func validateGitLabMergeRequestForMerge(
	number int,
	mergeRequest *gitlab.MergeRequest,
	bypassMergeChecks bool,
) error {
	if mergeRequest.State != gitlabMergeRequestOpenedState {
		return fmt.Errorf("%w: merge request !%d is %s", ErrMergeBlocked, number, mergeRequest.State)
	}

	if mergeRequest.Draft {
		return fmt.Errorf("%w: merge request !%d is draft", ErrMergeBlocked, number)
	}

	if mergeRequest.HasConflicts {
		return fmt.Errorf("%w: merge request !%d has conflicts", ErrMergeBlocked, number)
	}

	mergeStatus := strings.TrimSpace(mergeRequest.DetailedMergeStatus)
	if !bypassMergeChecks && !isGitLabMergeStatusMergeable(mergeStatus) {
		return fmt.Errorf("%w: merge request !%d detailed_merge_status=%s", ErrMergeBlocked, number, mergeStatus)
	}

	return nil
}

func (g *GitLab) ensureReleaseLabels(ctx context.Context) error {
	err := g.ensureLabel(ctx, ReleaseLabelPending, "#"+releaseLabelPendingColor, releaseLabelPendingDescription)
	if err != nil {
		return err
	}

	err = g.ensureLabel(ctx, ReleaseLabelTagged, "#"+releaseLabelTaggedColor, releaseLabelTaggedDescription)
	if err != nil {
		return err
	}

	return nil
}

func (g *GitLab) ensureLabel(ctx context.Context, name, color, description string) error {
	_, _, err := g.client.Labels.GetLabel(g.projectID, name, gitlab.WithContext(ctx))
	if err == nil {
		return nil
	}

	if !errors.Is(err, gitlab.ErrNotFound) {
		return fmt.Errorf("get label %q: %w", name, err)
	}

	_, _, err = g.client.Labels.CreateLabel(g.projectID, &gitlab.CreateLabelOptions{
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
	case "", "mergeable", "can_be_merged", "checking", "unchecked", "preparing":
		return true
	default:
		return false
	}
}

func (g *GitLab) projectMergeSettings(ctx context.Context) (*gitlab.Project, error) {
	project, _, err := g.client.Projects.GetProject(g.projectID, nil, gitlab.WithContext(ctx))
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
