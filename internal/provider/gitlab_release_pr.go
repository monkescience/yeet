package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const gitlabMergeRequestOpenedState = "opened"

const gitlabMergeRequestMergedState = "merged"

var errGitLabReleasePRLabelsInvalid = errors.New("invalid GitLab release PR labels")

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
		if markErr := g.MarkReleasePRPending(ctx, int(mr.IID), opts.Labels); markErr != nil {
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

	slog.DebugContext(ctx, "gitlab: resolving reviewers", slog.Int("reviewer_count", len(usernames)))

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

//nolint:funlen // Pagination closures keep trust and lifecycle checks beside candidate mapping.
func (g *GitLab) FindOpenPendingReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel string,
) ([]*PullRequest, error) {
	if err := validateGitLabLifecycleLabel(pendingLabel); err != nil {
		return nil, err
	}

	state := gitlabMergeRequestOpenedState
	orderBy := "updated_at"
	sortDirection := sortDirectionDesc
	labels := gitlab.LabelOptions{pendingLabel}

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		OrderBy:      new(orderBy),
		Sort:         new(sortDirection),
		Labels:       &labels,
		ListOptions:  gitlab.ListOptions{PerPage: gitLabPageSize},
	}

	slog.DebugContext(ctx, "gitlab: listing open pending release MRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
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
			if !isTrustedGitLabReleasePR(
				mr.SourceBranch,
				baseBranch,
				mr.SourceProjectID,
				mr.TargetProjectID,
			) {
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

	if err := g.validateOpenReleasePRLabelMismatches(ctx, baseBranch, pendingLabel, pendingMRs); err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "gitlab: listed open pending release MRs", slog.Int("count", len(pendingMRs)))

	return pendingMRs, nil
}

func (g *GitLab) validateOpenReleasePRLabelMismatches(
	ctx context.Context,
	baseBranch, pendingLabel string,
	pendingMRs []*PullRequest,
) error {
	knownPending := make(map[int]struct{}, len(pendingMRs))
	for _, pending := range pendingMRs {
		knownPending[pending.Number] = struct{}{}
	}

	state := gitlabMergeRequestOpenedState
	sourceBranch := releaseBranchName(baseBranch)
	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		SourceBranch: new(sourceBranch),
		ListOptions:  gitlab.ListOptions{PerPage: gitLabPageSize},
	}

	return paginate(ctx, "checking open release MR labels",
		func(page int) ([]*gitlab.BasicMergeRequest, int, error) {
			options.Page = int64(page)

			mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(g.projectID, options, gitlab.WithContext(ctx))
			if err != nil {
				return nil, 0, fmt.Errorf("list merge requests without label filter: %w", err)
			}

			return mrs, gitLabNextPage(resp), nil
		},
		func(mr *gitlab.BasicMergeRequest) (bool, error) {
			if _, ok := knownPending[int(mr.IID)]; ok {
				return false, nil
			}

			if !isTrustedGitLabReleasePR(
				mr.SourceBranch,
				baseBranch,
				mr.SourceProjectID,
				mr.TargetProjectID,
			) || slices.Contains(mr.Labels, pendingLabel) {
				return false, nil
			}

			return false, fmt.Errorf(
				"%w: trusted merge request !%d on branch %q is missing configured pending label %q",
				ErrReleasePRLabelMismatch,
				mr.IID,
				mr.SourceBranch,
				pendingLabel,
			)
		},
	)
}

//nolint:funlen // Pagination closure layout inflates line count without adding complexity.
func (g *GitLab) FindMergedReleasePR(
	ctx context.Context,
	baseBranch, pendingLabel string,
) (*PullRequest, error) {
	if err := validateGitLabLifecycleLabel(pendingLabel); err != nil {
		return nil, err
	}

	state := gitlabMergeRequestMergedState
	orderBy := "updated_at"
	sortDirection := sortDirectionDesc
	labels := gitlab.LabelOptions{pendingLabel}

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		OrderBy:      new(orderBy),
		Sort:         new(sortDirection),
		Labels:       &labels,
		ListOptions:  gitlab.ListOptions{PerPage: gitLabPageSize},
	}

	slog.DebugContext(ctx, "gitlab: searching merged release MRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
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
			if !isTrustedGitLabReleasePR(
				mr.SourceBranch,
				baseBranch,
				mr.SourceProjectID,
				mr.TargetProjectID,
			) {
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

func (g *GitLab) MarkReleasePRPending(ctx context.Context, number int, labels ReleasePRLabels) error {
	addLabels := append([]string{labels.Pending}, labels.Extra...)
	if labels.Yeet {
		addLabels = append(addLabels, ReleaseLabelYeet)
	}

	return g.updateReleasePRLabels(
		ctx,
		number,
		addLabels,
		[]string{labels.Tagged},
		"pending",
	)
}

func (g *GitLab) MarkReleasePRTagged(ctx context.Context, number int, labels ReleasePRLabels) error {
	return g.updateReleasePRLabels(ctx, number, []string{labels.Tagged}, []string{labels.Pending}, "tagged")
}

func (g *GitLab) updateReleasePRLabels(
	ctx context.Context,
	number int,
	addLabelNames, removeLabelNames []string,
	state string,
) error {
	addLabels := gitlab.LabelOptions(addLabelNames)
	removeLabels := gitlab.LabelOptions(removeLabelNames)

	_, _, err := g.client.MergeRequests.UpdateMergeRequest(g.projectID, int64(number), &gitlab.UpdateMergeRequestOptions{
		AddLabels:    &addLabels,
		RemoveLabels: &removeLabels,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("mark merge request !%d %s: %w", number, state, err)
	}

	return nil
}

func gitLabMergedAt(mergeRequest *gitlab.BasicMergeRequest) time.Time {
	if mergeRequest.MergedAt == nil {
		return time.Time{}
	}

	return *mergeRequest.MergedAt
}

func gitLabMergeRequestCommitSHA(mergeRequest *gitlab.BasicMergeRequest) string {
	return gitLabCommitSHA(mergeRequest.MergeCommitSHA, mergeRequest.SquashCommitSHA, mergeRequest.SHA)
}

func gitLabCommitSHA(mergeCommit, squashCommit, sourceCommit string) string {
	for _, candidate := range []string{mergeCommit, squashCommit, sourceCommit} {
		if commitSHA := strings.TrimSpace(candidate); commitSHA != "" {
			return commitSHA
		}
	}

	return ""
}

func gitLabMergeCommitSHA(ctx context.Context, mergeRequest *gitlab.BasicMergeRequest) string {
	commitSHA := gitLabMergeRequestCommitSHA(mergeRequest)
	if commitSHA != "" {
		return commitSHA
	}

	slog.WarnContext(ctx, "gitlab: merged MR has no merge, squash, or source commit SHA",
		slog.Int64("iid", mergeRequest.IID))

	return ""
}

func (g *GitLab) MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) (string, error) {
	slog.DebugContext(ctx, "gitlab: merging merge request", slog.Int("iid", number))

	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.projectID, int64(number), nil, gitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("get merge request !%d: %w", number, err)
	}

	if mr.State == gitlabMergeRequestMergedState {
		return gitLabMergeCommitSHA(ctx, &mr.BasicMergeRequest), nil
	}

	if !isTrustedGitLabReleasePR(mr.SourceBranch, mr.TargetBranch, mr.SourceProjectID, mr.TargetProjectID) {
		return "", fmt.Errorf("%w: merge request !%d", ErrUntrustedReleasePR, number)
	}

	if err := validateGitLabMergeRequestForMerge(number, mr, opts.BypassMergeChecks); err != nil {
		return "", err
	}

	project, err := g.projectMergeSettings(ctx)
	if err != nil {
		return "", err
	}

	acceptOptions, err := gitLabAcceptMergeOptions(project, opts.Method)
	if err != nil {
		return "", err
	}

	sha := strings.TrimSpace(mr.SHA)
	if sha != "" {
		acceptOptions.SHA = new(sha)
	}

	merged, _, err := g.client.MergeRequests.AcceptMergeRequest(
		g.projectID,
		int64(number),
		acceptOptions,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return "", fmt.Errorf("accept merge request !%d: %w", number, err)
	}

	slog.DebugContext(ctx, "gitlab: merged merge request",
		slog.Int("iid", number),
		slog.String("sha", sha),
	)

	if merged == nil || merged.State != gitlabMergeRequestMergedState {
		return "", nil
	}

	return gitLabMergeCommitSHA(ctx, &merged.BasicMergeRequest), nil
}

func isTrustedGitLabReleasePR(sourceBranch, baseBranch string, sourceProjectID, targetProjectID int64) bool {
	return isExpectedReleaseBranch(sourceBranch, baseBranch) &&
		sourceProjectID != 0 &&
		sourceProjectID == targetProjectID
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

func (g *GitLab) PrepareReleasePRLabels(ctx context.Context, labels ReleasePRLabels) error {
	if err := validateGitLabReleasePRLabels(labels); err != nil {
		return err
	}

	for _, label := range labels.Extra {
		if err := g.validateExistingLabel(ctx, label); err != nil {
			return err
		}
	}

	if labels.Yeet {
		err := g.ensureLabel(ctx, ReleaseLabelYeet, "#"+releaseLabelYeetColor, releaseLabelYeetDescription)
		if err != nil {
			return err
		}
	}

	err := g.ensureLabel(ctx, labels.Pending, "#"+releaseLabelPendingColor, releaseLabelPendingDescription)
	if err != nil {
		return err
	}

	err = g.ensureLabel(ctx, labels.Tagged, "#"+releaseLabelTaggedColor, releaseLabelTaggedDescription)
	if err != nil {
		return err
	}

	return nil
}

func validateGitLabReleasePRLabels(labels ReleasePRLabels) error {
	for _, lifecycle := range []string{labels.Pending, labels.Tagged} {
		if err := validateGitLabLifecycleLabel(lifecycle); err != nil {
			return err
		}
	}

	type scopedLabel struct {
		name  string
		scope string
	}

	var scoped []scopedLabel

	for _, name := range []string{labels.Pending, labels.Tagged} {
		scope, ok := gitLabLabelScope(name)
		if !ok {
			continue
		}

		scoped = append(scoped, scopedLabel{name: name, scope: scope})
	}

	for _, name := range labels.Extra {
		scope, ok := gitLabLabelScope(name)
		if !ok {
			continue
		}

		for _, existing := range scoped {
			if strings.EqualFold(existing.scope, scope) {
				return fmt.Errorf(
					"%w: labels %q and %q share GitLab scope %s",
					errGitLabReleasePRLabelsInvalid,
					existing.name,
					name,
					scope,
				)
			}
		}

		scoped = append(scoped, scopedLabel{name: name, scope: scope})
	}

	return nil
}

func validateGitLabLifecycleLabel(name string) error {
	if strings.EqualFold(name, "any") || strings.EqualFold(name, "none") {
		return fmt.Errorf(
			"%w: %q is a reserved GitLab label filter value",
			errGitLabReleasePRLabelsInvalid,
			name,
		)
	}

	return nil
}

func gitLabLabelScope(name string) (string, bool) {
	separator := strings.LastIndex(name, "::")
	if separator <= 0 || separator+2 == len(name) {
		return "", false
	}

	return name[:separator], true
}

func (g *GitLab) validateExistingLabel(ctx context.Context, name string) error {
	_, _, err := g.client.Labels.GetLabel(g.projectID, name, gitlab.WithContext(ctx))
	if err == nil {
		return nil
	}

	if !errors.Is(err, gitlab.ErrNotFound) {
		return fmt.Errorf("get label %q: %w", name, err)
	}

	return fmt.Errorf("%w: extra label %q", ErrReleasePRLabelMissing, name)
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
