package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/forge"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const gitlabMergeRequestOpenedState = "opened"

const gitlabMergeRequestMergedState = "merged"

const gitlabMergeRequestClosedState = "closed"

const gitLabLabelColorPrefix = "#"

var errGitLabReleasePRLabelsInvalid = errors.New("invalid GitLab release PR labels")

func (g *GitLab) CreateReleasePR(ctx context.Context, opts forge.ReleasePROptions) (*forge.PullRequest, error) {
	if err := g.validateReleasePRLabels(ctx, opts.Labels); err != nil {
		return nil, wrapReleasePRLabelsError(err)
	}

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
		if markErr := g.SetReleasePRLabels(ctx, int(mr.IID), opts.Labels, forge.ReleasePRPhasePending); markErr != nil {
			return nil, errors.Join(err, markErr)
		}

		return nil, err
	}

	slog.DebugContext(ctx, "gitlab: created merge request",
		slog.Int("iid", int(mr.IID)),
		slog.String("url", mr.WebURL),
	)

	return &forge.PullRequest{
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
		id, err := g.findProjectMemberID(ctx, username)
		if err != nil {
			return nil, err
		}

		ids = append(ids, id)
	}

	return ids, nil
}

// findProjectMemberID picks the exact username match: the members query
// parameter is a fuzzy search over name and username, so the wanted member can
// sit behind any number of looser matches.
func (g *GitLab) findProjectMemberID(ctx context.Context, username string) (int64, error) {
	options := &gitlab.ListProjectMembersOptions{
		ListOptions: gitlab.ListOptions{PerPage: gitLabPageSize},
		Query:       new(username),
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
		return 0, fmt.Errorf("%w: %q is not a project member", forge.ErrReviewerNotFound, username)
	}

	return id, nil
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
			forge.ErrReviewerNotApplied,
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

func (g *GitLab) UpdateReleasePR(ctx context.Context, number int, opts forge.ReleasePROptions) error {
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
) ([]*forge.PullRequest, error) {
	if err := validateGitLabLifecycleLabel(pendingLabel); err != nil {
		return nil, err
	}

	state := gitlabMergeRequestOpenedState
	sourceBranch := releaseBranchName(baseBranch)
	orderBy := "updated_at"
	sortDirection := sortDirectionDesc

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        new(state),
		TargetBranch: new(baseBranch),
		SourceBranch: new(sourceBranch),
		OrderBy:      new(orderBy),
		Sort:         new(sortDirection),
		ListOptions:  gitlab.ListOptions{PerPage: gitLabPageSize},
	}

	slog.DebugContext(ctx, "gitlab: listing open pending release MRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
	)

	pendingMRs := make([]*forge.PullRequest, 0)

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

			needsLabel, err := needsPendingLabel(
				mr.Labels,
				pendingLabel,
				exactLabelMatch,
				gitLabMergeRequestReference(int(mr.IID)),
				mr.SourceBranch,
			)
			if err != nil {
				return false, err
			}

			pendingMRs = append(pendingMRs, &forge.PullRequest{
				Number:            int(mr.IID),
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

	slog.DebugContext(ctx, "gitlab: listed open pending release MRs", slog.Int("count", len(pendingMRs)))

	return pendingMRs, nil
}

//nolint:funlen // Pagination closure layout inflates line count without adding complexity.
func (g *GitLab) FindMergedReleasePR(
	ctx context.Context,
	baseBranch, pendingLabel string,
) (*forge.PullRequest, error) {
	if err := validateGitLabLifecycleLabel(pendingLabel); err != nil {
		return nil, err
	}

	state := gitlabMergeRequestMergedState
	sourceBranch := releaseBranchName(baseBranch)
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
		ListOptions:  gitlab.ListOptions{PerPage: gitLabPageSize},
	}

	slog.DebugContext(ctx, "gitlab: searching merged release MRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
	)

	candidates := make([]*gitlab.BasicMergeRequest, 0)

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

			return &full.BasicMergeRequest, true, nil
		},
		reference: func(mergeRequest *gitlab.BasicMergeRequest) string {
			return fmt.Sprintf("merge request !%d", mergeRequest.IID)
		},
	})
	if err != nil {
		return nil, err
	}

	found := &forge.PullRequest{
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

func (g *GitLab) SetReleasePRLabels(
	ctx context.Context,
	number int,
	labels forge.ReleasePRLabels,
	phase forge.ReleasePRPhase,
) error {
	if phase == forge.ReleasePRPhaseTagged {
		if err := validateGitLabLifecycleLabel(labels.Tagged); err != nil {
			return wrapReleasePRLabelsError(err)
		}
	} else {
		if err := validateGitLabReleasePRLabels(labels); err != nil {
			return wrapReleasePRLabelsError(err)
		}
	}

	if err := g.labelDefinitions().prepare(ctx, labels, phase); err != nil {
		return wrapReleasePRLabelsError(err)
	}

	change := managedLabelChange(labels, phase)

	return wrapReleasePRLabelsError(g.applyLabels(ctx, number, change.anchor, change.add, change.remove))
}

func (g *GitLab) PreflightReleasePRTagging(ctx context.Context, taggedLabel string) error {
	if err := validateGitLabLifecycleLabel(taggedLabel); err != nil {
		return wrapReleasePRLabelsError(err)
	}

	return wrapReleasePRLabelsError(g.labelDefinitions().validateExisting(ctx, taggedLabel, "tagged"))
}

// applyLabels sends additions and removals in one atomic update, so the anchor
// can never land without the rest or be dropped on its own.
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

func gitLabMergeCommitSHA(ctx context.Context, mergeRequest *gitlab.BasicMergeRequest) string {
	commitSHA := gitLabMergeRequestCommitSHA(mergeRequest)
	if commitSHA != "" {
		return commitSHA
	}

	slog.WarnContext(ctx, "gitlab: merged MR has no merge, squash, or source commit SHA",
		slog.Int64("iid", mergeRequest.IID))

	return ""
}

func (g *GitLab) MergeReleasePR(ctx context.Context, number int, opts forge.MergeReleasePROptions) (string, error) {
	slog.DebugContext(ctx, "gitlab: merging merge request", slog.Int("iid", number))

	driver := mergeDriver{forge: &gitLabMerge{provider: g, number: number}, polling: g.polling}

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

func (m *gitLabMerge) resolveMethod(ctx context.Context, requested forge.MergeMethod) (any, error) {
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

func (m *gitLabMerge) execute(ctx context.Context, current mergeState, method any) (string, bool, error) {
	acceptOptions, ok := method.(*gitlab.AcceptMergeRequestOptions)
	if !ok {
		return "", false, unsupportedResolvedMethod(method)
	}

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
		// GitLab answers a conflicting or already closed merge request with 405
		// rather than a body explaining itself.
		if response != nil && response.StatusCode == http.StatusMethodNotAllowed {
			return "", false, gitLabAcceptRefused(current.Reference, err.Error())
		}

		return "", false, fmt.Errorf("accept merge request !%d: %w", m.number, err)
	}

	if merged == nil {
		return "", true, nil
	}

	// MergeError is free-form English with no SDK constants, so it is reported
	// verbatim rather than matched on.
	if mergeError := strings.TrimSpace(merged.MergeError); mergeError != "" {
		return "", false, gitLabAcceptRefused(current.Reference, mergeError)
	}

	slog.DebugContext(ctx, "gitlab: merged merge request",
		slog.Int("iid", m.number),
		slog.String("sha", current.HeadSHA),
	)

	if merged.State == gitlabMergeRequestMergedState {
		if mergeSHA := gitLabMergeRequestCommitSHA(&merged.BasicMergeRequest); mergeSHA != "" {
			return mergeSHA, false, nil
		}
	}

	// Fast-forward projects populate no merge or squash commit, so the source tip
	// only becomes the release ref once GitLab reports the MR merged.
	return "", true, nil
}

func gitLabAcceptRefused(reference, detail string) error {
	return blockedMerge(reference, forge.MergeBlockedReasonUnknown, "was refused: "+detail)
}

func gitLabMergeState(reference string, mergeRequest *gitlab.MergeRequest) mergeState {
	mergeStatus := strings.TrimSpace(mergeRequest.DetailedMergeStatus)

	return mergeState{
		Reference:        reference,
		RawReadiness:     "detailed_merge_status=" + mergeStatus,
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

func isTrustedGitLabReleasePR(sourceBranch, baseBranch string, sourceProjectID, targetProjectID int64) bool {
	return isExpectedReleaseBranch(sourceBranch, baseBranch) &&
		isGitLabSameProject(sourceProjectID, targetProjectID)
}

// validateReleasePRLabels runs before the merge request exists, so a label
// configuration GitLab or yeet rejects cannot leave an unlabelled merge request
// behind.
func (g *GitLab) validateReleasePRLabels(ctx context.Context, labels forge.ReleasePRLabels) error {
	if err := validateGitLabReleasePRLabels(labels); err != nil {
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
		return nil, fmt.Errorf("%w: unknown merge method %q", forge.ErrMergeMethodUnsupported, requested)
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
