package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/identity"
	"github.com/monkescience/yeet/internal/forge"
)

const azureDevOpsPRPageSize = 100

// azureDevOpsMaxPRBodyLength is Azure DevOps's hard limit on pull request
// descriptions. The REST API rejects a longer body with "a description for a
// pull request must not be longer than 4000 characters".
// Source: https://learn.microsoft.com/en-us/rest/api/azure/devops/git/pull-requests/update
const azureDevOpsMaxPRBodyLength = 4000

var errAzureDevOpsLabelIDMissing = errors.New("azure devops label id missing")

func (a *AzureDevOps) CreateReleasePR(ctx context.Context, opts forge.ReleasePROptions) (*forge.PullRequest, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "azure devops: creating pull request",
		slog.String("source_branch", opts.ReleaseBranch),
		slog.String("target_branch", opts.BaseBranch),
	)

	reviewers, err := a.resolveReviewers(ctx, opts.Reviewers)
	if err != nil {
		return nil, err
	}

	pr := git.GitPullRequest{
		SourceRefName: new("refs/heads/" + opts.ReleaseBranch),
		TargetRefName: new("refs/heads/" + opts.BaseBranch),
		Title:         new(opts.Title),
		Description:   new(opts.Body),
	}
	if len(reviewers) > 0 {
		pr.Reviewers = new(reviewers)
	}

	created, err := gitClient.CreatePullRequest(ctx, git.CreatePullRequestArgs{
		GitPullRequestToCreate: &pr,
		RepositoryId:           &a.repo,
		Project:                &a.project,
	})
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}

	prNumber := derefInt(created.PullRequestId)

	slog.DebugContext(ctx, "azure devops: created pull request",
		slog.Int("pr_number", prNumber),
		slog.String("url", a.pullRequestWebURL(prNumber)),
	)

	return &forge.PullRequest{
		Number: prNumber,
		Title:  derefString(created.Title),
		Body:   derefString(created.Description),
		URL:    a.pullRequestWebURL(prNumber),
		Branch: opts.ReleaseBranch,
	}, nil
}

// resolveReviewers maps reviewer names (email, display name, or account name)
// to identity GUIDs via the identity search API, since the pull request API
// only accepts reviewers by GUID.
func (a *AzureDevOps) resolveReviewers(ctx context.Context, names []string) ([]git.IdentityRefWithVote, error) {
	if len(names) == 0 {
		return nil, nil
	}

	slog.DebugContext(ctx, "azure devops: resolving reviewers", slog.Int("reviewer_count", len(names)))

	identityClient, err := identity.NewClient(ctx, a.conn)
	if err != nil {
		return nil, fmt.Errorf("create identity client: %w", err)
	}

	reviewers := make([]git.IdentityRefWithVote, 0, len(names))

	for _, name := range names {
		identities, err := identityClient.ReadIdentities(ctx, identity.ReadIdentitiesArgs{
			SearchFilter: new("General"),
			FilterValue:  new(name),
		})
		if err != nil {
			return nil, fmt.Errorf("look up reviewer %q: %w", name, err)
		}

		if identities == nil || len(*identities) == 0 {
			return nil, fmt.Errorf("%w: %q", forge.ErrReviewerNotFound, name)
		}

		if len(*identities) > 1 {
			return nil, fmt.Errorf("%w: %q matches %d identities", forge.ErrReviewerAmbiguous, name, len(*identities))
		}

		resolved := (*identities)[0]
		if resolved.Id == nil {
			return nil, fmt.Errorf("%w: %q resolved without an id", forge.ErrReviewerNotFound, name)
		}

		reviewers = append(reviewers, git.IdentityRefWithVote{Id: new(resolved.Id.String())})
	}

	return reviewers, nil
}

func (a *AzureDevOps) MaxPRBodyLength() int {
	return azureDevOpsMaxPRBodyLength
}

func (a *AzureDevOps) UpdateReleasePR(ctx context.Context, number int, opts forge.ReleasePROptions) error {
	gitClient, err := a.client(ctx)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "azure devops: updating pull request", slog.Int("pr_number", number))

	update := git.GitPullRequest{
		Title:       new(opts.Title),
		Description: new(opts.Body),
	}

	_, err = gitClient.UpdatePullRequest(ctx, git.UpdatePullRequestArgs{
		GitPullRequestToUpdate: &update,
		RepositoryId:           &a.repo,
		Project:                &a.project,
		PullRequestId:          &number,
	})
	if err != nil {
		return fmt.Errorf("update pull request !%d: %w", number, err)
	}

	slog.DebugContext(ctx, "azure devops: updated pull request", slog.Int("pr_number", number))

	return nil
}

func (a *AzureDevOps) FindOpenPendingReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel string,
) ([]*forge.PullRequest, error) {
	slog.DebugContext(ctx, "azure devops: listing open pending release PRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
	)

	prs, err := a.listPullRequests(
		ctx,
		git.PullRequestStatusValues.Active,
		baseBranch,
		releaseBranchName(baseBranch),
	)
	if err != nil {
		return nil, err
	}

	pending := make([]*forge.PullRequest, 0)

	for _, pr := range prs {
		branch := azureDevOpsRefToBranch(derefString(pr.SourceRefName))
		if !a.isTrustedReleasePR(&pr, baseBranch) {
			continue
		}

		number := derefInt(pr.PullRequestId)

		state := classifyReleasePRLabels(azureDevOpsLabelNames(pr.Labels), pendingLabel, foldedLabelMatch)
		if state == releasePRLabelsMismatched {
			return nil, releasePRLabelMismatch(
				azureDevOpsPullRequestReference(number),
				branch,
				pendingLabel,
			)
		}

		pending = append(pending, &forge.PullRequest{
			Number:            number,
			Title:             derefString(pr.Title),
			Body:              derefString(pr.Description),
			URL:               a.pullRequestWebURL(number),
			Branch:            branch,
			NeedsPendingLabel: state == releasePRLabelsAdoptable,
		})
	}

	slog.DebugContext(ctx, "azure devops: listed open pending release PRs", slog.Int("count", len(pending)))

	return pending, nil
}

func (a *AzureDevOps) FindMergedReleasePR(
	ctx context.Context,
	baseBranch, pendingLabel string,
) (*forge.PullRequest, error) {
	slog.DebugContext(ctx, "azure devops: searching merged release PRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
	)

	prs, err := a.listPullRequests(
		ctx,
		git.PullRequestStatusValues.Completed,
		baseBranch,
		releaseBranchName(baseBranch),
	)
	if err != nil {
		return nil, err
	}

	candidates := a.azureDevOpsMergedCandidates(prs, baseBranch, pendingLabel)

	full, err := a.latestAzureDevOpsMergedPR(ctx, candidates)
	if err != nil {
		return nil, err
	}

	if !a.isTrustedReleasePR(full, baseBranch) {
		return nil, forge.ErrNoPR
	}

	number := derefInt(full.PullRequestId)
	result := &forge.PullRequest{
		Number:         number,
		Title:          derefString(full.Title),
		Body:           derefString(full.Description),
		URL:            a.pullRequestWebURL(number),
		Branch:         azureDevOpsRefToBranch(derefString(full.SourceRefName)),
		MergeCommitSHA: azureDevOpsCompletedMergeCommit(full),
	}

	slog.DebugContext(ctx, "azure devops: found merged release PR",
		slog.Int("pr_number", result.Number),
		slog.String("url", result.URL),
		slog.String("merge_sha", result.MergeCommitSHA),
	)

	return result, nil
}

func (a *AzureDevOps) azureDevOpsMergedCandidates(
	prs []git.GitPullRequest,
	baseBranch, pendingLabel string,
) []git.GitPullRequest {
	candidates := make([]git.GitPullRequest, 0)

	for _, pr := range prs {
		if !a.isTrustedReleasePR(&pr, baseBranch) || !azureDevOpsHasLabel(pr.Labels, pendingLabel) {
			continue
		}

		candidates = append(candidates, pr)
	}

	return candidates
}

func (a *AzureDevOps) latestAzureDevOpsMergedPR(
	ctx context.Context,
	candidates []git.GitPullRequest,
) (*git.GitPullRequest, error) {
	if len(candidates) == 0 {
		return nil, forge.ErrNoPR
	}

	fullByNumber := make(map[int]*git.GitPullRequest)

	if len(candidates) > 1 {
		for index := range candidates {
			if candidates[index].ClosedDate != nil {
				continue
			}

			number := derefInt(candidates[index].PullRequestId)

			full, err := a.getPullRequest(ctx, number)
			if err != nil {
				return nil, err
			}

			if full.ClosedDate == nil {
				return nil, fmt.Errorf("%w: pull request !%d", errMergeTimeMissing, number)
			}

			candidates[index] = *full
			fullByNumber[number] = full
		}
	}

	best := &candidates[0]
	for index := 1; index < len(candidates); index++ {
		if azureDevOpsClosedAt(&candidates[index]).After(azureDevOpsClosedAt(best)) {
			best = &candidates[index]
		}
	}

	number := derefInt(best.PullRequestId)
	if full := fullByNumber[number]; full != nil {
		return full, nil
	}

	return a.getPullRequest(ctx, number)
}

func (a *AzureDevOps) listPullRequests(
	ctx context.Context,
	status git.PullRequestStatus,
	baseBranch, sourceBranch string,
) ([]git.GitPullRequest, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	all := make([]git.GitPullRequest, 0)
	top := azureDevOpsPRPageSize
	targetRef := "refs/heads/" + baseBranch
	sourceRef := "refs/heads/" + sourceBranch

	err = paginateAzureDevOpsBySkip(ctx, "listing pull requests", top,
		func(skip int) ([]git.GitPullRequest, error) {
			pageStatus := status
			criteria := &git.GitPullRequestSearchCriteria{
				Status:        &pageStatus,
				SourceRefName: &sourceRef,
				TargetRefName: &targetRef,
			}

			page, err := gitClient.GetPullRequests(ctx, git.GetPullRequestsArgs{
				RepositoryId:   &a.repo,
				Project:        &a.project,
				SearchCriteria: criteria,
				Skip:           &skip,
				Top:            &top,
			})
			if err != nil {
				return nil, fmt.Errorf("list pull requests: %w", err)
			}

			if page == nil {
				return nil, nil
			}

			return *page, nil
		},
		func(pr git.GitPullRequest) (bool, error) {
			all = append(all, pr)

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return all, nil
}

func azureDevOpsClosedAt(pr *git.GitPullRequest) time.Time {
	if pr.ClosedDate == nil {
		return time.Time{}
	}

	return pr.ClosedDate.Time
}

func (a *AzureDevOps) MergeReleasePR(
	ctx context.Context,
	number int,
	opts forge.MergeReleasePROptions,
) (string, error) {
	slog.DebugContext(ctx, "azure devops: completing pull request", slog.Int("pr_number", number))

	driver := mergeDriver{forge: &azureDevOpsMerge{provider: a, number: number}, polling: a.polling}

	return driver.run(ctx, opts)
}

type azureDevOpsMerge struct {
	provider *AzureDevOps
	number   int
}

func (m *azureDevOpsMerge) state(ctx context.Context) (mergeState, error) {
	pullRequest, err := m.provider.getPullRequest(ctx, m.number)
	if err != nil {
		return mergeState{}, err
	}

	return m.provider.azureDevOpsMergeState(m.number, pullRequest), nil
}

func (m *azureDevOpsMerge) resolveMethod(_ context.Context, requested forge.MergeMethod) (any, error) {
	strategy, err := azureDevOpsMergeStrategy(requested)
	if err != nil {
		return nil, err
	}

	return strategy, nil
}

func (m *azureDevOpsMerge) execute(ctx context.Context, current mergeState, method any) (string, bool, error) {
	strategy, ok := method.(git.GitPullRequestMergeStrategy)
	if !ok {
		return "", false, unsupportedResolvedMethod(method)
	}

	gitClient, err := m.provider.client(ctx)
	if err != nil {
		return "", false, err
	}

	completed := git.PullRequestStatusValues.Completed

	update := git.GitPullRequest{
		Status: &completed,
		CompletionOptions: &git.GitPullRequestCompletionOptions{
			MergeStrategy: &strategy,
		},
	}

	if current.HeadSHA != "" {
		update.LastMergeSourceCommit = &git.GitCommitRef{CommitId: &current.HeadSHA}
	}

	merged, err := gitClient.UpdatePullRequest(ctx, git.UpdatePullRequestArgs{
		GitPullRequestToUpdate: &update,
		RepositoryId:           &m.provider.repo,
		Project:                &m.provider.project,
		PullRequestId:          &m.number,
	})
	if err != nil {
		return "", false, fmt.Errorf("complete pull request !%d: %w", m.number, err)
	}

	slog.DebugContext(ctx, "azure devops: completed pull request",
		slog.Int("pr_number", m.number),
		slog.String("strategy", string(strategy)),
	)

	// The completion response can carry a provisional commit while the merge
	// status is still queued, so only a succeeded merge status is trusted.
	if mergeSHA := azureDevOpsCompletionResponseCommit(merged); mergeSHA != "" {
		return mergeSHA, false, nil
	}

	if refusal := azureDevOpsMergeRefusal(merged); refusal != nil {
		return "", false, blockedMerge(current.Reference, refusal.reason, refusal.detail)
	}

	return "", true, nil
}

func azureDevOpsMergeRefusal(pullRequest *git.GitPullRequest) *mergeRefusal {
	if pullRequest == nil {
		return nil
	}

	mergeStatus := derefString((*string)(pullRequest.MergeStatus))

	reason, refused := azureDevOpsMergeRefusalReason(mergeStatus)
	if !refused {
		return nil
	}

	detail := strings.TrimSpace(derefString(pullRequest.MergeFailureMessage))
	if detail == "" {
		detail = "merge_status=" + mergeStatus
	}

	return &mergeRefusal{reason: reason, detail: "was refused: " + detail}
}

func azureDevOpsMergeRefusalReason(mergeStatus string) (forge.MergeBlockedReason, bool) {
	switch mergeStatus {
	case string(git.PullRequestAsyncStatusValues.Conflicts):
		return forge.MergeBlockedReasonConflicts, true
	case string(git.PullRequestAsyncStatusValues.RejectedByPolicy):
		return forge.MergeBlockedReasonPolicy, true
	case string(git.PullRequestAsyncStatusValues.Failure):
		return forge.MergeBlockedReasonFailure, true
	default:
		return forge.MergeBlockedReasonUnknown, false
	}
}

func (a *AzureDevOps) azureDevOpsMergeState(number int, pullRequest *git.GitPullRequest) mergeState {
	status := derefString((*string)(pullRequest.Status))
	mergeStatus := derefString((*string)(pullRequest.MergeStatus))

	return mergeState{
		Reference:        azureDevOpsPullRequestReference(number),
		RawReadiness:     "merge_status=" + mergeStatus,
		MergeCommitSHA:   azureDevOpsCompletedMergeCommit(pullRequest),
		HeadSHA:          azureDevOpsLastMergeSourceCommit(pullRequest),
		SourceBranch:     azureDevOpsRefToBranch(derefString(pullRequest.SourceRefName)),
		BaseBranch:       azureDevOpsRefToBranch(derefString(pullRequest.TargetRefName)),
		IsOpen:           status == string(git.PullRequestStatusValues.Active),
		IsMerged:         status == string(git.PullRequestStatusValues.Completed),
		IsClosedUnmerged: status == string(git.PullRequestStatusValues.Abandoned),
		IsDraft:          pullRequest.IsDraft != nil && *pullRequest.IsDraft,
		HasConflicts:     azureDevOpsMergeStatusConflicted(mergeStatus),
		ReadinessBlocked: azureDevOpsMergeStatusReadinessBlocked(mergeStatus),
		SameRepository:   pullRequest.ForkSource == nil && a.isConfiguredRepository(pullRequest.Repository),
		Refusal:          azureDevOpsMergeRefusal(pullRequest),
	}
}

func azureDevOpsLastMergeSourceCommit(pullRequest *git.GitPullRequest) string {
	if pullRequest.LastMergeSourceCommit == nil {
		return ""
	}

	return derefString(pullRequest.LastMergeSourceCommit.CommitId)
}

func (a *AzureDevOps) isTrustedReleasePR(pullRequest *git.GitPullRequest, baseBranch string) bool {
	if pullRequest == nil || pullRequest.ForkSource != nil {
		return false
	}

	sourceBranch := azureDevOpsRefToBranch(derefString(pullRequest.SourceRefName))

	return isExpectedReleaseBranch(sourceBranch, baseBranch) &&
		a.isConfiguredRepository(pullRequest.Repository)
}

// Azure DevOps addresses a repository by either name or id, so the configured
// value is compared against both.
func (a *AzureDevOps) isConfiguredRepository(repository *git.GitRepository) bool {
	configured := strings.TrimSpace(a.repo)
	if repository == nil || configured == "" {
		return false
	}

	name := strings.TrimSpace(derefString(repository.Name))
	if name != "" && strings.EqualFold(name, configured) {
		return true
	}

	return repository.Id != nil && strings.EqualFold(repository.Id.String(), configured)
}

// SetReleasePRLabels has no definition step to run first: Azure DevOps creates a
// tag definition when a label is attached and exposes no project-scoped or
// repository-scoped pull request label listing to check one against.
func (a *AzureDevOps) SetReleasePRLabels(
	ctx context.Context,
	number int,
	labels forge.ReleasePRLabels,
	phase forge.ReleasePRPhase,
) error {
	change := managedLabelChange(labels, phase)

	return wrapReleasePRLabelsError(a.applyLabels(ctx, number, change.anchor, change.add, change.remove))
}

func (a *AzureDevOps) PreflightReleasePRTagging(context.Context, string) error {
	return nil
}

// applyLabels attaches one label per request, since Azure DevOps has no bulk
// label API. The anchor goes first and fails the whole call, and everything
// after it is best effort so a single rejected label cannot strand the rest.
func (a *AzureDevOps) applyLabels(ctx context.Context, number int, anchor string, add, remove []string) error {
	if err := a.attachPullRequestLabel(ctx, number, anchor); err != nil {
		return err
	}

	var errs []error

	for _, label := range add {
		if err := a.attachPullRequestLabel(ctx, number, label); err != nil {
			errs = append(errs, err)
		}
	}

	for _, label := range remove {
		if err := a.detachPullRequestLabel(ctx, number, label); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (a *AzureDevOps) attachPullRequestLabel(ctx context.Context, number int, label string) error {
	gitClient, err := a.client(ctx)
	if err != nil {
		return err
	}

	request := &core.WebApiCreateTagRequestData{
		Name: new(label),
	}

	_, err = gitClient.CreatePullRequestLabel(ctx, git.CreatePullRequestLabelArgs{
		Label:         request,
		RepositoryId:  &a.repo,
		Project:       &a.project,
		PullRequestId: &number,
	})
	if err != nil {
		return fmt.Errorf("add label %q to pull request !%d: %w", label, number, err)
	}

	return nil
}

func (a *AzureDevOps) detachPullRequestLabel(ctx context.Context, number int, label string) error {
	labels, err := a.pullRequestLabels(ctx, number)
	if err != nil {
		return fmt.Errorf("get labels for pull request !%d: %w", number, err)
	}

	labelID, ok, err := azureDevOpsPullRequestLabelID(labels, label)
	if err != nil {
		return fmt.Errorf("resolve label %q on pull request !%d: %w", label, number, err)
	}

	if !ok {
		return nil
	}

	gitClient, err := a.client(ctx)
	if err != nil {
		return err
	}

	err = gitClient.DeletePullRequestLabels(ctx, git.DeletePullRequestLabelsArgs{
		RepositoryId:  &a.repo,
		Project:       &a.project,
		PullRequestId: &number,
		LabelIdOrName: &labelID,
	})
	if err != nil {
		if isAzureDevOpsNotFound(err) {
			return nil
		}

		return fmt.Errorf("remove label %q from pull request !%d: %w", label, number, err)
	}

	return nil
}

func (a *AzureDevOps) pullRequestLabels(ctx context.Context, number int) ([]core.WebApiTagDefinition, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	labels, err := gitClient.GetPullRequestLabels(ctx, git.GetPullRequestLabelsArgs{
		RepositoryId:  &a.repo,
		Project:       &a.project,
		PullRequestId: &number,
	})
	if err != nil {
		return nil, fmt.Errorf("get pull request labels !%d: %w", number, err)
	}

	if labels == nil {
		return nil, nil
	}

	return *labels, nil
}

func azureDevOpsPullRequestLabelID(labels []core.WebApiTagDefinition, target string) (string, bool, error) {
	for _, label := range labels {
		if label.Name == nil || !strings.EqualFold(*label.Name, target) {
			continue
		}

		if label.Id == nil {
			return "", false, fmt.Errorf("%w: %q", errAzureDevOpsLabelIDMissing, target)
		}

		return label.Id.String(), true, nil
	}

	return "", false, nil
}

func (a *AzureDevOps) getPullRequest(ctx context.Context, number int) (*git.GitPullRequest, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	pr, err := gitClient.GetPullRequest(ctx, git.GetPullRequestArgs{
		RepositoryId:  &a.repo,
		Project:       &a.project,
		PullRequestId: &number,
	})
	if err != nil {
		return nil, fmt.Errorf("get pull request !%d: %w", number, err)
	}

	return pr, nil
}

func azureDevOpsRefToBranch(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

func azureDevOpsHasLabel(labels *[]core.WebApiTagDefinition, target string) bool {
	for _, name := range azureDevOpsLabelNames(labels) {
		if strings.EqualFold(name, target) {
			return true
		}
	}

	return false
}

func azureDevOpsLabelNames(labels *[]core.WebApiTagDefinition) []string {
	if labels == nil {
		return nil
	}

	names := make([]string, 0, len(*labels))

	for _, label := range *labels {
		if label.Name != nil {
			names = append(names, *label.Name)
		}
	}

	return names
}

func azureDevOpsPullRequestReference(number int) string {
	return fmt.Sprintf("pull request !%d", number)
}

func (a *AzureDevOps) pullRequestWebURL(id int) string {
	return fmt.Sprintf("%s/pullrequest/%d", a.RepoURL(), id)
}

func azureDevOpsMergeCommit(pr *git.GitPullRequest) string {
	if pr == nil {
		return ""
	}

	if pr.LastMergeCommit != nil && pr.LastMergeCommit.CommitId != nil && *pr.LastMergeCommit.CommitId != "" {
		return *pr.LastMergeCommit.CommitId
	}

	return ""
}

func azureDevOpsCompletedMergeCommit(pr *git.GitPullRequest) string {
	if pr == nil || derefString((*string)(pr.Status)) != string(git.PullRequestStatusValues.Completed) {
		return ""
	}

	if pr.MergeStatus != nil &&
		derefString((*string)(pr.MergeStatus)) != string(git.PullRequestAsyncStatusValues.Succeeded) {
		return ""
	}

	return azureDevOpsMergeCommit(pr)
}

func azureDevOpsCompletionResponseCommit(pr *git.GitPullRequest) string {
	if pr == nil ||
		derefString((*string)(pr.Status)) != string(git.PullRequestStatusValues.Completed) ||
		derefString((*string)(pr.MergeStatus)) != string(git.PullRequestAsyncStatusValues.Succeeded) {
		return ""
	}

	return azureDevOpsMergeCommit(pr)
}

// Azure DevOps reports conflicts and policy refusals through one enum field, so
// the two are split here to keep --auto-merge-force from bypassing conflicts.
func azureDevOpsMergeStatusConflicted(status string) bool {
	return status == string(git.PullRequestAsyncStatusValues.Conflicts)
}

func azureDevOpsMergeStatusReadinessBlocked(status string) bool {
	switch status {
	case string(git.PullRequestAsyncStatusValues.RejectedByPolicy),
		string(git.PullRequestAsyncStatusValues.Failure):
		return true
	default:
		return false
	}
}

func azureDevOpsMergeStrategy(method forge.MergeMethod) (git.GitPullRequestMergeStrategy, error) {
	if method == "" {
		method = forge.MergeMethodAuto
	}

	switch method {
	case forge.MergeMethodAuto, forge.MergeMethodSquash:
		return git.GitPullRequestMergeStrategyValues.Squash, nil
	case forge.MergeMethodRebase:
		return git.GitPullRequestMergeStrategyValues.Rebase, nil
	case forge.MergeMethodMerge:
		return git.GitPullRequestMergeStrategyValues.NoFastForward, nil
	default:
		return "", fmt.Errorf("%w: unknown merge method %q", forge.ErrMergeMethodUnsupported, method)
	}
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}

	return *p
}
