package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/logattr"
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"
)

const gitlabMergeRequestOpenedState = "opened"

const gitlabMergeRequestMergedState = "merged"

const gitlabMergeRequestClosedState = "closed"

const gitLabLabelColorPrefix = "#"

var _ forgeMerge[*gitlab.AcceptMergeRequestOptions] = (*gitLabMerge)(nil)

var errGitLabReleasePRLabelsInvalid = errors.New("invalid GitLab release PR labels")

func (g *GitLab) CreateReleasePR(ctx context.Context, opts forge.ReleasePROptions) (*forge.PullRequest, error) {
	err := g.validateReleasePRLabels(ctx, opts.Labels)
	if err != nil {
		return nil, wrapReleasePRLabelsError(err)
	}

	g.logger.DebugContext(ctx, "creating merge request",
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

	err = verifyGitLabReviewers(opts.Reviewers, reviewerIDs, mr.Reviewers)
	if err != nil {
		markErr := g.SetReleasePRLabels(ctx, int(mr.IID), opts.Labels, forge.ReleasePRPhasePending)
		if markErr != nil {
			return nil, errors.Join(err, markErr)
		}

		return nil, err
	}

	g.logger.DebugContext(ctx, "created merge request",
		logattr.PullRequestNumber(mr.IID),
		slog.String("url", mr.WebURL),
	)

	return &forge.PullRequest{
		Number:    int(mr.IID),
		Reference: gitLabMergeRequestReference(int(mr.IID)),
		Title:     mr.Title,
		Body:      mr.Description,
		URL:       mr.WebURL,
		Branch:    opts.ReleaseBranch,
	}, nil
}

func (g *GitLab) resolveReviewerIDs(ctx context.Context, usernames []string) ([]int64, error) {
	if len(usernames) == 0 {
		return nil, nil
	}

	g.logger.DebugContext(ctx, "resolving reviewers", slog.Int("reviewer_count", len(usernames)))

	ids := make([]int64, 0, len(usernames))

	for _, username := range usernames {
		id, err := g.findProjectMemberID(ctx, username)
		if err != nil {
			return nil, err
		}

		ids = append(ids, id)
	}

	return ids, nil
}

func (g *GitLab) findProjectMemberID(ctx context.Context, username string) (int64, error) {
	options := &gitlab.ListProjectMembersOptions{
		PerPage: gitLabPageSize,
		Query:   new(username),
	}

	var (
		id    int64
		found bool
	)

	err := paginate(ctx, "listing project members",
		func(page int) ([]*gitlab.ProjectMember, int, error) {
			options.Page = int64(page)

			members, resp, err := g.client.ProjectMembers.ListAllProjectMembers(
				g.projectID,
				options,
				gitlab.WithContext(ctx),
			)
			if err != nil {
				return nil, 0, fmt.Errorf("look up reviewer %q: %w", username, err)
			}

			return members, gitLabNextPage(resp), nil
		},
		func(member *gitlab.ProjectMember) (bool, error) {
			if !strings.EqualFold(member.Username, username) {
				return false, nil
			}

			id = member.ID
			found = true

			return true, nil
		},
	)
	if err != nil {
		return 0, err
	}

	if !found {
		return 0, &ReviewerError{
			Reviewer: username,
			Problem:  "reviewer is not a project member",
			Err:      fmt.Errorf("%w: %q is not a project member", forge.ErrReviewerNotFound, username),
		}
	}

	return id, nil
}

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
		return &ReviewerError{
			Reviewers: slices.Clone(missing),
			Problem:   "multiple reviewers require GitLab Premium or Ultimate",
			Err: fmt.Errorf(
				"%w: %v (multiple merge request reviewers require GitLab Premium or Ultimate)",
				forge.ErrReviewerNotApplied,
				missing,
			),
		}
	}

	return nil
}

func (g *GitLab) MaxPRBodyLength() int {
	return 0
}

func (g *GitLab) UpdateReleasePR(ctx context.Context, number int, opts forge.ReleasePROptions) error {
	g.logger.DebugContext(ctx, "updating merge request", logattr.PullRequestNumber(int64(number)))

	_, _, err := g.client.MergeRequests.UpdateMergeRequest(g.projectID, int64(number), &gitlab.UpdateMergeRequestOptions{
		Title:       new(opts.Title),
		Description: new(opts.Body),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("update merge request !%d: %w", number, err)
	}

	g.logger.DebugContext(ctx, "updated merge request", logattr.PullRequestNumber(int64(number)))

	return nil
}

func (g *GitLab) FindOpenPendingReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel string,
	expectedBranches ...string,
) ([]*forge.PullRequest, error) {
	if len(expectedBranches) > 1 {
		return collectExpectedBranchPRs(expectedBranches, func(branch string) ([]*forge.PullRequest, error) {
			return g.FindOpenPendingReleasePRs(ctx, baseBranch, pendingLabel, branch)
		})
	}

	sourceBranch := expectedReleaseBranch(g.releaseBranch, baseBranch, expectedBranches)

	return g.findOpenPendingReleasePRs(ctx, baseBranch, pendingLabel, sourceBranch, false)
}

func (g *GitLab) FindOpenPendingReleasePRsForBase(
	ctx context.Context,
	baseBranch, pendingLabel string,
) ([]*forge.PullRequest, error) {
	return g.findOpenPendingReleasePRs(ctx, baseBranch, pendingLabel, "", true)
}

//nolint:funlen // Pagination closures keep trust and lifecycle checks beside candidate mapping.
func (g *GitLab) findOpenPendingReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel, sourceBranch string,
	anyBranch bool,
) ([]*forge.PullRequest, error) {
	err := validateGitLabLifecycleLabel(pendingLabel)
	if err != nil {
		return nil, err
	}

	state := gitlabMergeRequestOpenedState
	orderBy := "updated_at"
	sortDirection := sortDirectionDesc

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		OrderBy:      new(orderBy),
		Sort:         new(sortDirection),
		PerPage:      gitLabPageSize,
	}

	if anyBranch {
		labels := gitlab.LabelOptions{pendingLabel}
		options.Labels = &labels
	} else {
		options.SourceBranch = new(sourceBranch)
	}

	g.logger.DebugContext(ctx, "listing open pending release merge requests",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
	)

	pendingMRs := make([]*forge.PullRequest, 0)

	err = paginate(ctx, "listing open pending release MRs",
		func(page int) ([]*gitlab.BasicMergeRequest, int, error) {
			options.Page = int64(page)

			mrs, resp, listErr := g.client.MergeRequests.ListProjectMergeRequests(g.projectID, options, gitlab.WithContext(ctx))
			if listErr != nil {
				return nil, 0, fmt.Errorf("list merge requests: %w", listErr)
			}

			return mrs, gitLabNextPage(resp), nil
		},
		func(mr *gitlab.BasicMergeRequest) (bool, error) {
			trusted := isGitLabSameProject(mr.SourceProjectID, mr.TargetProjectID)

			if !anyBranch {
				trusted = isTrustedGitLabReleasePR(
					mr.SourceBranch,
					baseBranch,
					sourceBranch,
					mr.SourceProjectID,
					mr.TargetProjectID,
				)
			}

			if !trusted {
				return false, nil
			}

			needsLabel := false

			if !anyBranch {
				var labelErr error

				needsLabel, labelErr = needsPendingLabel(
					mr.Labels,
					pendingLabel,
					exactLabelMatch,
					gitLabMergeRequestReference(int(mr.IID)),
					mr.SourceBranch,
				)
				if labelErr != nil {
					return false, labelErr
				}
			}

			pendingMRs = append(pendingMRs, &forge.PullRequest{
				Number:            int(mr.IID),
				Reference:         gitLabMergeRequestReference(int(mr.IID)),
				Title:             mr.Title,
				Body:              mr.Description,
				URL:               mr.WebURL,
				Branch:            mr.SourceBranch,
				NeedsPendingLabel: needsLabel,
			})

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	g.logger.DebugContext(ctx, "listed open pending release merge requests", slog.Int("count", len(pendingMRs)))

	return pendingMRs, nil
}

func (g *GitLab) FindMergedReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel string,
	expectedBranches ...string,
) ([]*forge.PullRequest, error) {
	branches := expectedBranches
	if len(branches) == 0 {
		branches = []string{expectedReleaseBranch(g.releaseBranch, baseBranch, nil)}
	}

	return collectExpectedBranchMergedPRs(branches, func(branch string) (*forge.PullRequest, error) {
		return g.FindMergedReleasePR(ctx, baseBranch, pendingLabel, branch)
	})
}

//nolint:funlen // Pagination closure layout inflates line count without adding complexity.
func (g *GitLab) FindMergedReleasePR(
	ctx context.Context,
	baseBranch, pendingLabel string,
	expectedBranches ...string,
) (*forge.PullRequest, error) {
	err := validateGitLabLifecycleLabel(pendingLabel)
	if err != nil {
		return nil, err
	}

	state := gitlabMergeRequestMergedState
	sourceBranch := expectedReleaseBranch(g.releaseBranch, baseBranch, expectedBranches)
	orderBy := "updated_at"
	sortDirection := sortDirectionDesc
	labels := gitlab.LabelOptions{pendingLabel}

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		SourceBranch: new(sourceBranch),
		OrderBy:      new(orderBy),
		Sort:         new(sortDirection),
		Labels:       &labels,
		PerPage:      gitLabPageSize,
	}

	g.logger.DebugContext(ctx, "searching merged release merge requests",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
	)

	candidates := make([]*gitlab.BasicMergeRequest, 0)

	err = paginate(ctx, "listing merged release MRs",
		func(page int) ([]*gitlab.BasicMergeRequest, int, error) {
			options.Page = int64(page)

			mrs, resp, listErr := g.client.MergeRequests.ListProjectMergeRequests(g.projectID, options, gitlab.WithContext(ctx))
			if listErr != nil {
				return nil, 0, fmt.Errorf("list merge requests: %w", listErr)
			}

			return mrs, gitLabNextPage(resp), nil
		},
		func(mr *gitlab.BasicMergeRequest) (bool, error) {
			if !isTrustedGitLabReleasePR(
				mr.SourceBranch,
				baseBranch,
				sourceBranch,
				mr.SourceProjectID,
				mr.TargetProjectID,
			) {
				return false, nil
			}

			candidates = append(candidates, mr)

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	bestMR, err := resolveLatestMerged(ctx, candidates, mergedCandidates[*gitlab.BasicMergeRequest]{
		mergedAt: func(mergeRequest *gitlab.BasicMergeRequest) (time.Time, bool) {
			if mergeRequest.MergedAt == nil {
				return time.Time{}, false
			}

			return gitLabMergedAt(mergeRequest), true
		},
		hydrate: func(ctx context.Context, mergeRequest *gitlab.BasicMergeRequest) (*gitlab.BasicMergeRequest, bool, error) {
			full, _, getErr := g.client.MergeRequests.GetMergeRequest(
				g.projectID,
				mergeRequest.IID,
				nil,
				gitlab.WithContext(ctx),
			)
			if getErr != nil {
				return nil, false, fmt.Errorf("get merge request !%d: %w", mergeRequest.IID, getErr)
			}

			if full.MergedAt == nil {
				return nil, false, mergeTimeMissingError(gitLabMergeRequestReference(int(mergeRequest.IID)))
			}

			return &full.BasicMergeRequest, true, nil
		},
		reference: func(mergeRequest *gitlab.BasicMergeRequest) string {
			return gitLabMergeRequestReference(int(mergeRequest.IID))
		},
	})
	if err != nil {
		return nil, err
	}

	found := &forge.PullRequest{
		Number:         int(bestMR.IID),
		Reference:      gitLabMergeRequestReference(int(bestMR.IID)),
		Title:          bestMR.Title,
		Body:           bestMR.Description,
		URL:            bestMR.WebURL,
		Branch:         bestMR.SourceBranch,
		MergeCommitSHA: g.mergeCommitSHA(ctx, bestMR),
	}

	g.logger.DebugContext(ctx, "found merged release merge request",
		logattr.PullRequestNumber(int64(found.Number)),
		slog.String("url", found.URL),
		slog.String("merge_sha", found.MergeCommitSHA),
	)

	return found, nil
}

func (g *GitLab) SetReleasePRLabels(
	ctx context.Context,
	number int,
	labels forge.ReleasePRLabels,
	phase forge.ReleasePRPhase,
) error {
	if phase == forge.ReleasePRPhaseTagged {
		err := validateGitLabLifecycleLabel(labels.Tagged)
		if err != nil {
			return wrapReleasePRLabelsError(err)
		}
	} else {
		err := validateGitLabReleasePRLabels(labels)
		if err != nil {
			return wrapReleasePRLabelsError(err)
		}
	}

	err := g.labelDefinitions().prepare(ctx, labels, phase)
	if err != nil {
		return wrapReleasePRLabelsError(err)
	}

	change := managedLabelChange(labels, phase)

	return wrapReleasePRLabelsError(g.applyLabels(ctx, number, change.anchor, change.add, change.remove))
}

func (g *GitLab) PreflightReleasePRTagging(ctx context.Context, taggedLabel string) error {
	err := validateGitLabLifecycleLabel(taggedLabel)
	if err != nil {
		return wrapReleasePRLabelsError(err)
	}

	return wrapReleasePRLabelsError(g.labelDefinitions().validateExisting(ctx, taggedLabel, "tagged"))
}

func (g *GitLab) applyLabels(ctx context.Context, number int, anchor string, add, remove []string) error {
	addLabels := gitlab.LabelOptions(labelsAnchoredFirst(anchor, add))
	removeLabels := gitlab.LabelOptions(remove)

	_, _, err := g.client.MergeRequests.UpdateMergeRequest(g.projectID, int64(number), &gitlab.UpdateMergeRequestOptions{
		AddLabels:    &addLabels,
		RemoveLabels: &removeLabels,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("set labels on merge request !%d: %w", number, err)
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

func (g *GitLab) mergeCommitSHA(ctx context.Context, mergeRequest *gitlab.BasicMergeRequest) string {
	commitSHA := gitLabMergeRequestCommitSHA(mergeRequest)
	if commitSHA != "" {
		return commitSHA
	}

	g.logger.WarnContext(ctx, "merged request has no merge, squash, or source commit hash",
		logattr.PullRequestNumber(mergeRequest.IID))

	return ""
}

func (g *GitLab) MergeReleasePR(ctx context.Context, number int, opts forge.MergeReleasePROptions) (string, error) {
	g.logger.DebugContext(ctx, "merging merge request", logattr.PullRequestNumber(int64(number)))

	driver := mergeDriver[*gitlab.AcceptMergeRequestOptions]{
		forge:         &gitLabMerge{provider: g, number: number},
		polling:       g.polling,
		baseBranch:    opts.BaseBranch,
		releaseBranch: mergeExpectedReleaseBranch(g.releaseBranch, opts.ReleaseBranch),
	}

	return driver.run(ctx, opts)
}

type gitLabMerge struct {
	provider *GitLab
	number   int
}

func (m *gitLabMerge) state(ctx context.Context) (mergeState, error) {
	mergeRequest, _, err := m.provider.client.MergeRequests.GetMergeRequest(
		m.provider.projectID,
		int64(m.number),
		nil,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return mergeState{}, fmt.Errorf("get merge request !%d: %w", m.number, err)
	}

	reference := gitLabMergeRequestReference(m.number)
	if mergeRequest == nil {
		return mergeState{Reference: reference}, nil
	}

	return gitLabMergeState(reference, mergeRequest), nil
}

func (m *gitLabMerge) resolveMethod(
	ctx context.Context,
	requested forge.MergeMethod,
) (*gitlab.AcceptMergeRequestOptions, error) {
	project, err := m.provider.projectMergeSettings(ctx)
	if err != nil {
		return nil, err
	}

	options, err := gitLabAcceptMergeOptions(project, requested)
	if err != nil {
		return nil, err
	}

	return options, nil
}

func (m *gitLabMerge) execute(
	ctx context.Context,
	current mergeState,
	acceptOptions *gitlab.AcceptMergeRequestOptions,
) (string, bool, error) {
	if current.HeadSHA != "" {
		acceptOptions.SHA = new(current.HeadSHA)
	}

	merged, response, err := m.provider.client.MergeRequests.AcceptMergeRequest(
		m.provider.projectID,
		int64(m.number),
		acceptOptions,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusMethodNotAllowed {
			return "", false, gitLabAcceptRefused(current.Reference, "merge request returned HTTP 405", "")
		}

		return "", false, fmt.Errorf("accept merge request !%d: %w", m.number, err)
	}

	if merged == nil {
		return "", true, nil
	}

	if mergeError := strings.TrimSpace(merged.MergeError); mergeError != "" {
		return "", false, gitLabAcceptRefused(current.Reference, "provider reported a merge failure", mergeError)
	}

	m.provider.logger.DebugContext(ctx, "merged merge request",
		logattr.PullRequestNumber(int64(m.number)),
		slog.String("sha", current.HeadSHA),
	)

	if merged.State == gitlabMergeRequestMergedState {
		if mergeSHA := gitLabMergeRequestCommitSHA(&merged.BasicMergeRequest); mergeSHA != "" {
			return mergeSHA, false, nil
		}
	}

	return "", true, nil
}

func gitLabAcceptRefused(reference, detail, message string) error {
	return blockedMergeMessage(reference, forge.MergeBlockedReasonUnknown, "was refused: "+detail, message)
}

func gitLabMergeState(reference string, mergeRequest *gitlab.MergeRequest) mergeState {
	mergeStatus := strings.TrimSpace(mergeRequest.DetailedMergeStatus)

	return mergeState{
		Reference:        reference,
		RawReadiness:     "detailed_merge_status=" + mergeStatus,
		MergeStatus:      mergeStatus,
		MergeCommitSHA:   gitLabMergeRequestCommitSHA(&mergeRequest.BasicMergeRequest),
		HeadSHA:          strings.TrimSpace(mergeRequest.SHA),
		SourceBranch:     mergeRequest.SourceBranch,
		BaseBranch:       mergeRequest.TargetBranch,
		IsOpen:           mergeRequest.State == gitlabMergeRequestOpenedState,
		IsMerged:         mergeRequest.State == gitlabMergeRequestMergedState,
		IsClosedUnmerged: mergeRequest.State == gitlabMergeRequestClosedState,
		IsDraft:          mergeRequest.Draft,
		HasConflicts:     mergeRequest.HasConflicts,
		ReadinessBlocked: !isGitLabMergeStatusMergeable(mergeStatus),
		SameRepository:   isGitLabSameProject(mergeRequest.SourceProjectID, mergeRequest.TargetProjectID),
	}
}

func isGitLabSameProject(sourceProjectID, targetProjectID int64) bool {
	return sourceProjectID != 0 && sourceProjectID == targetProjectID
}

func gitLabMergeRequestReference(number int) string {
	return fmt.Sprintf("merge request !%d", number)
}

func isTrustedGitLabReleasePR(
	sourceBranch, baseBranch, releaseBranch string,
	sourceProjectID, targetProjectID int64,
) bool {
	return isExpectedReleaseBranch(sourceBranch, baseBranch, releaseBranch) &&
		isGitLabSameProject(sourceProjectID, targetProjectID)
}

func (g *GitLab) validateReleasePRLabels(ctx context.Context, labels forge.ReleasePRLabels) error {
	err := validateGitLabReleasePRLabels(labels)
	if err != nil {
		return err
	}

	return g.labelDefinitions().validateExtras(ctx, labels.Extra)
}

func (g *GitLab) labelDefinitions() labelDefinitions {
	return labelDefinitions{
		get: func(ctx context.Context, name string) error {
			_, _, err := g.client.Labels.GetLabel(g.projectID, name, gitlab.WithContext(ctx))
			if err != nil {
				return fmt.Errorf("get label %q: %w", name, err)
			}

			return nil
		},
		create: func(ctx context.Context, name, color, description string) error {
			_, _, err := g.client.Labels.CreateLabel(g.projectID, &gitlab.CreateLabelOptions{
				Name:        new(name),
				Color:       new(gitLabLabelColorPrefix + color),
				Description: new(description),
			}, gitlab.WithContext(ctx))
			if err != nil {
				return fmt.Errorf("create label %q: %w", name, err)
			}

			return nil
		},
		isNotFound: func(err error) bool { return errors.Is(err, gitlab.ErrNotFound) },
		cache:      &g.labels,
	}
}

func validateGitLabReleasePRLabels(labels forge.ReleasePRLabels) error {
	for _, lifecycle := range []string{labels.Pending, labels.Tagged} {
		err := validateGitLabLifecycleLabel(lifecycle)
		if err != nil {
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
				return &LabelError{
					Label:    name,
					Role:     "extra",
					Scope:    scope,
					Conflict: existing.name,
					Err: fmt.Errorf(
						"%w: labels %q and %q share GitLab scope %s",
						errGitLabReleasePRLabelsInvalid,
						existing.name,
						name,
						scope,
					),
				}
			}
		}

		scoped = append(scoped, scopedLabel{name: name, scope: scope})
	}

	return nil
}

func validateGitLabLifecycleLabel(name string) error {
	if strings.EqualFold(name, "any") || strings.EqualFold(name, "none") {
		return &LabelError{
			Label:   name,
			Problem: "configured GitLab lifecycle label is reserved",
			Err: fmt.Errorf(
				"%w: %q is a reserved GitLab label filter value",
				errGitLabReleasePRLabelsInvalid,
				name,
			),
		}
	}

	return nil
}

func gitLabLabelScope(name string) (string, bool) {
	scope, suffix, found := strings.CutLast(name, "::")
	if !found || scope == "" || suffix == "" {
		return "", false
	}

	return scope, true
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
		return nil, gitLabMergeMethodBlocked("missing project merge settings")
	}

	return project, nil
}

func gitLabAcceptMergeOptions(
	project *gitlab.Project,
	requested forge.MergeMethod,
) (*gitlab.AcceptMergeRequestOptions, error) {
	if requested == "" {
		requested = forge.MergeMethodAuto
	}

	options := &gitlab.AcceptMergeRequestOptions{}

	switch requested {
	case forge.MergeMethodAuto:
		if project.SquashOption != gitlab.SquashOptionNever {
			options.Squash = new(true)
		}

		return options, nil
	case forge.MergeMethodSquash:
		if project.SquashOption == gitlab.SquashOptionNever {
			return nil, gitLabMergeMethodBlocked(fmt.Sprintf(
				"merge method %q disabled by project squash_option=%s",
				requested,
				project.SquashOption,
			))
		}

		options.Squash = new(true)

		return options, nil
	case forge.MergeMethodRebase:
		if project.MergeMethod != gitlab.RebaseMerge {
			return nil, gitLabMergeMethodIncompatible(requested, project.MergeMethod)
		}

		return options, nil
	case forge.MergeMethodMerge:
		if project.MergeMethod != gitlab.NoFastForwardMerge {
			return nil, gitLabMergeMethodIncompatible(requested, project.MergeMethod)
		}

		return options, nil
	default:
		return nil, &forge.MergeMethodUnsupportedError{Method: requested}
	}
}

func gitLabMergeMethodIncompatible(requested forge.MergeMethod, configured gitlab.MergeMethodValue) error {
	return gitLabMergeMethodBlocked(fmt.Sprintf(
		"merge method %q incompatible with project merge_method=%s",
		requested,
		configured,
	))
}

func gitLabMergeMethodBlocked(detail string) error {
	return blockedMerge("", forge.MergeBlockedReasonMethod, detail)
}
