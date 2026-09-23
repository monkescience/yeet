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
	"github.com/monkescience/yeet/internal/logattr"
)

const azureDevOpsPRPageSize = 100

const azureDevOpsMaxPRBodyLength = 4000

var _ forgeMerge[git.GitPullRequestMergeStrategy] = (*azureDevOpsMerge)(nil)

var errAzureDevOpsLabelIDMissing = errors.New("azure devops label id missing")

func (a *AzureDevOps) CreateReleasePR(ctx context.Context, opts forge.ReleasePROptions) (*forge.PullRequest, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	a.logger.DebugContext(ctx, "creating pull request",
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

	a.logger.DebugContext(ctx, "created pull request",
		logattr.PullRequestNumber(int64(prNumber)),
		slog.String("url", a.pullRequestWebURL(prNumber)),
	)

	return &forge.PullRequest{
		Number:    prNumber,
		Reference: azureDevOpsPullRequestReference(prNumber),
		Title:     derefString(created.Title),
		Body:      derefString(created.Description),
		URL:       a.pullRequestWebURL(prNumber),
		Branch:    opts.ReleaseBranch,
	}, nil
}

func (a *AzureDevOps) resolveReviewers(ctx context.Context, names []string) ([]git.IdentityRefWithVote, error) {
	if len(names) == 0 {
		return nil, nil
	}

	a.logger.DebugContext(ctx, "resolving reviewers", slog.Int("reviewer_count", len(names)))

	identityClient, err := identity.NewClient(ctx, a.conn)
	if err != nil {
		return nil, fmt.Errorf("create identity client: %w", err)
	}

	reviewers := make([]git.IdentityRefWithVote, 0, len(names))

	for _, name := range names {
		identities, readErr := identityClient.ReadIdentities(ctx, identity.ReadIdentitiesArgs{
			SearchFilter: new("General"),
			FilterValue:  new(name),
		})
		if readErr != nil {
			return nil, fmt.Errorf("look up reviewer %q: %w", name, readErr)
		}

		if identities == nil || len(*identities) == 0 {
			return nil, &ReviewerError{
				Reviewer: name,
				Problem:  "reviewer identity was not found",
				Err:      fmt.Errorf("%w: %q", forge.ErrReviewerNotFound, name),
			}
		}

		if len(*identities) > 1 {
			return nil, &ReviewerError{
				Reviewer: name,
				Problem:  fmt.Sprintf("reviewer matches %d identities", len(*identities)),
				Err:      fmt.Errorf("%w: %q matches %d identities", forge.ErrReviewerAmbiguous, name, len(*identities)),
			}
		}

		resolved := (*identities)[0]
		if resolved.Id == nil {
			return nil, &ReviewerError{
				Reviewer: name,
				Problem:  "reviewer identity resolved without an id",
				Err:      fmt.Errorf("%w: %q resolved without an id", forge.ErrReviewerNotFound, name),
			}
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

	a.logger.DebugContext(ctx, "updating pull request", logattr.PullRequestNumber(int64(number)))

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

	a.logger.DebugContext(ctx, "updated pull request", logattr.PullRequestNumber(int64(number)))

	return nil
}

func (a *AzureDevOps) FindOpenPendingReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel string,
	expectedBranches ...string,
) ([]*forge.PullRequest, error) {
	if len(expectedBranches) > 1 {
		return collectExpectedBranchPRs(expectedBranches, func(branch string) ([]*forge.PullRequest, error) {
			return a.FindOpenPendingReleasePRs(ctx, baseBranch, pendingLabel, branch)
		})
	}

	expectedBranch := expectedReleaseBranch(a.releaseBranch, baseBranch, expectedBranches)

	return a.findOpenPendingReleasePRs(ctx, baseBranch, pendingLabel, expectedBranch, false)
}

func (a *AzureDevOps) FindOpenPendingReleasePRsForBase(
	ctx context.Context,
	baseBranch, pendingLabel string,
) ([]*forge.PullRequest, error) {
	return a.findOpenPendingReleasePRs(ctx, baseBranch, pendingLabel, "", true)
}

func (a *AzureDevOps) findOpenPendingReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel, expectedBranch string,
	anyBranch bool,
) ([]*forge.PullRequest, error) {
	prs, err := a.listPullRequests(
		ctx,
		git.PullRequestStatusValues.Active,
		baseBranch,
		expectedBranch,
	)
	if err != nil {
		return nil, err
	}

	return a.azureDevOpsPendingReleasePRs(ctx, prs, baseBranch, pendingLabel, expectedBranch, anyBranch)
}

func (a *AzureDevOps) azureDevOpsPendingReleasePRs(
	ctx context.Context,
	prs []git.GitPullRequest,
	baseBranch, pendingLabel, expectedBranch string,
	anyBranch bool,
) ([]*forge.PullRequest, error) {
	pending := make([]*forge.PullRequest, 0)

	for _, pr := range prs {
		branch := azureDevOpsRefToBranch(derefString(pr.SourceRefName))

		trusted := a.isTrustedOpenPRForBase(&pr, baseBranch)
		if !anyBranch {
			trusted = a.isTrustedReleasePR(&pr, baseBranch, expectedBranch)
		}

		if !trusted {
			continue
		}

		number := derefInt(pr.PullRequestId)

		labels := azureDevOpsLabelNames(pr.Labels)
		if anyBranch && classifyReleasePRLabels(labels, pendingLabel, foldedLabelMatch) != releasePRLabelsPending {
			continue
		}

		needsLabel, err := needsPendingLabel(
			labels,
			pendingLabel,
			foldedLabelMatch,
			azureDevOpsPullRequestReference(number),
			branch,
		)
		if err != nil {
			return nil, err
		}

		pending = append(pending, &forge.PullRequest{
			Number:            number,
			Reference:         azureDevOpsPullRequestReference(number),
			Title:             derefString(pr.Title),
			Body:              derefString(pr.Description),
			URL:               a.pullRequestWebURL(number),
			Branch:            branch,
			NeedsPendingLabel: needsLabel,
		})
	}

	a.logger.DebugContext(ctx, "listed open pending release pull requests",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
		slog.Int("count", len(pending)),
	)

	return pending, nil
}

func (a *AzureDevOps) FindMergedReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel string,
	expectedBranches ...string,
) ([]*forge.PullRequest, error) {
	branches := expectedBranches
	if len(branches) == 0 {
		branches = []string{expectedReleaseBranch(a.releaseBranch, baseBranch, nil)}
	}

	return collectExpectedBranchMergedPRs(branches, func(branch string) (*forge.PullRequest, error) {
		return a.FindMergedReleasePR(ctx, baseBranch, pendingLabel, branch)
	})
}

func (a *AzureDevOps) FindMergedReleasePR(
	ctx context.Context,
	baseBranch, pendingLabel string,
	expectedBranches ...string,
) (*forge.PullRequest, error) {
	expectedBranch := expectedReleaseBranch(a.releaseBranch, baseBranch, expectedBranches)
	a.logger.DebugContext(ctx, "searching merged release pull requests",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
	)

	prs, err := a.listPullRequests(
		ctx,
		git.PullRequestStatusValues.Completed,
		baseBranch,
		expectedBranch,
	)
	if err != nil {
		return nil, err
	}

	candidates := a.azureDevOpsMergedCandidates(prs, baseBranch, pendingLabel, expectedBranch)

	full, err := a.latestAzureDevOpsMergedPR(ctx, candidates)
	if err != nil {
		return nil, err
	}

	if !a.isTrustedReleasePR(full, baseBranch, expectedBranch) {
		return nil, forge.ErrNoPR
	}

	number := derefInt(full.PullRequestId)
	result := &forge.PullRequest{
		Number:         number,
		Reference:      azureDevOpsPullRequestReference(number),
		Title:          derefString(full.Title),
		Body:           derefString(full.Description),
		URL:            a.pullRequestWebURL(number),
		Branch:         azureDevOpsRefToBranch(derefString(full.SourceRefName)),
		MergeCommitSHA: azureDevOpsCompletedMergeCommit(full),
	}

	a.logger.DebugContext(ctx, "found merged release pull request",
		logattr.PullRequestNumber(int64(result.Number)),
		slog.String("url", result.URL),
		slog.String("merge_sha", result.MergeCommitSHA),
	)

	return result, nil
}

func (a *AzureDevOps) azureDevOpsMergedCandidates(
	prs []git.GitPullRequest,
	baseBranch, pendingLabel, expectedBranch string,
) []git.GitPullRequest {
	candidates := make([]git.GitPullRequest, 0)

	for _, pr := range prs {
		if !a.isTrustedReleasePR(&pr, baseBranch, expectedBranch) || !azureDevOpsHasLabel(pr.Labels, pendingLabel) {
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
	fullByNumber := make(map[int]*git.GitPullRequest)

	best, err := resolveLatestMerged(ctx, candidates, mergedCandidates[git.GitPullRequest]{
		mergedAt: func(pr git.GitPullRequest) (time.Time, bool) {
			if pr.ClosedDate == nil {
				return time.Time{}, false
			}

			return azureDevOpsClosedAt(&pr), true
		},
		hydrate: func(ctx context.Context, pr git.GitPullRequest) (git.GitPullRequest, bool, error) {
			number := derefInt(pr.PullRequestId)

			full, err := a.getPullRequest(ctx, number)
			if err != nil {
				return pr, false, err
			}

			if full.ClosedDate == nil {
				return pr, false, mergeTimeMissingError(azureDevOpsPullRequestReference(number))
			}

			fullByNumber[number] = full

			return *full, true, nil
		},
		reference: func(pr git.GitPullRequest) string {
			return azureDevOpsPullRequestReference(derefInt(pr.PullRequestId))
		},
	})
	if err != nil {
		return nil, err
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

	err = paginateAzureDevOpsBySkip(ctx, "listing pull requests", top,
		func(skip int) ([]git.GitPullRequest, error) {
			pageStatus := status
			criteria := &git.GitPullRequestSearchCriteria{
				Status:        &pageStatus,
				TargetRefName: &targetRef,
			}

			if strings.TrimSpace(sourceBranch) != "" {
				sourceRef := "refs/heads/" + sourceBranch
				criteria.SourceRefName = &sourceRef
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
	a.logger.DebugContext(ctx, "completing pull request", logattr.PullRequestNumber(int64(number)))

	driver := mergeDriver[git.GitPullRequestMergeStrategy]{
		forge:         &azureDevOpsMerge{provider: a, number: number},
		polling:       a.polling,
		logger:        a.logger,
		baseBranch:    opts.BaseBranch,
		releaseBranch: mergeExpectedReleaseBranch(a.releaseBranch, opts.ReleaseBranch),
	}

	return driver.run(ctx, opts)
}

func (a *AzureDevOps) EnsureAutoMerge(
	_ context.Context,
	number int,
	_ forge.MergeReleasePROptions,
) error {
	return &forge.AutoMergeUnsupportedError{
		Provider:  providerNameAzureDevOps,
		Reference: azureDevOpsPullRequestReference(number),
		Problem:   "provider requires direct auto-merge mode",
	}
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

func (m *azureDevOpsMerge) resolveMethod(
	_ context.Context,
	requested forge.MergeMethod,
) (git.GitPullRequestMergeStrategy, error) {
	return azureDevOpsMergeStrategy(requested)
}

func (m *azureDevOpsMerge) execute(
	ctx context.Context,
	current mergeState,
	strategy git.GitPullRequestMergeStrategy,
) (string, bool, error) {
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

	m.provider.logger.DebugContext(ctx, "completed pull request",
		logattr.PullRequestNumber(int64(m.number)),
		slog.String("strategy", string(strategy)),
	)

	if mergeSHA := azureDevOpsCompletionResponseCommit(merged); mergeSHA != "" {
		return mergeSHA, false, nil
	}

	if refusal := azureDevOpsMergeRefusal(merged); refusal != nil {
		return "", false, refusal.failure(current.Reference)
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

	return &mergeRefusal{
		reason:  reason,
		detail:  "merge_status=" + mergeStatus,
		status:  mergeStatus,
		message: strings.TrimSpace(derefString(pullRequest.MergeFailureMessage)),
	}
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
		MergeStatus:      mergeStatus,
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

func (a *AzureDevOps) isTrustedReleasePR(
	pullRequest *git.GitPullRequest,
	baseBranch string,
	expectedBranches ...string,
) bool {
	if pullRequest == nil || pullRequest.ForkSource != nil {
		return false
	}

	sourceBranch := azureDevOpsRefToBranch(derefString(pullRequest.SourceRefName))
	expectedBranch := expectedReleaseBranch(a.releaseBranch, baseBranch, expectedBranches)

	return strings.TrimSpace(sourceBranch) == strings.TrimSpace(expectedBranch) &&
		a.isConfiguredRepository(pullRequest.Repository)
}

func (a *AzureDevOps) isTrustedOpenPRForBase(
	pullRequest *git.GitPullRequest,
	baseBranch string,
) bool {
	if pullRequest == nil || pullRequest.ForkSource != nil {
		return false
	}

	targetBranch := azureDevOpsRefToBranch(derefString(pullRequest.TargetRefName))

	return strings.TrimSpace(targetBranch) == strings.TrimSpace(baseBranch) &&
		a.isConfiguredRepository(pullRequest.Repository)
}

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

func (a *AzureDevOps) applyLabels(ctx context.Context, number int, anchor string, add, remove []string) error {
	err := a.attachPullRequestLabel(ctx, number, anchor)
	if err != nil {
		return err
	}

	var errs []error

	for _, label := range add {
		err = a.attachPullRequestLabel(ctx, number, label)
		if err != nil {
			errs = append(errs, err)
		}
	}

	for _, label := range remove {
		err = a.detachPullRequestLabel(ctx, number, label)
		if err != nil {
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
		return "", &forge.MergeMethodUnsupportedError{Method: method}
	}
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}

	return *p
}
