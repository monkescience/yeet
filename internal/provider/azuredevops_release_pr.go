package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/identity"
)

const azureDevOpsPRPageSize = 100

// azureDevOpsMaxPRBodyLength is Azure DevOps's hard limit on pull request
// descriptions. The REST API rejects a longer body with "a description for a
// pull request must not be longer than 4000 characters".
// Source: https://learn.microsoft.com/en-us/rest/api/azure/devops/git/pull-requests/update
const azureDevOpsMaxPRBodyLength = 4000

var errAzureDevOpsLabelIDMissing = errors.New("azure devops label id missing")

func (a *AzureDevOps) CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error) {
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

	return &PullRequest{
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
			return nil, fmt.Errorf("%w: %q", ErrReviewerNotFound, name)
		}

		if len(*identities) > 1 {
			return nil, fmt.Errorf("%w: %q matches %d identities", ErrReviewerAmbiguous, name, len(*identities))
		}

		resolved := (*identities)[0]
		if resolved.Id == nil {
			return nil, fmt.Errorf("%w: %q resolved without an id", ErrReviewerNotFound, name)
		}

		reviewers = append(reviewers, git.IdentityRefWithVote{Id: new(resolved.Id.String())})
	}

	return reviewers, nil
}

func (a *AzureDevOps) MaxPRBodyLength() int {
	return azureDevOpsMaxPRBodyLength
}

func (a *AzureDevOps) UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error {
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
) ([]*PullRequest, error) {
	slog.DebugContext(ctx, "azure devops: listing open pending release PRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
	)

	prs, err := a.listPullRequests(ctx, git.PullRequestStatusValues.Active, baseBranch)
	if err != nil {
		return nil, err
	}

	pending := make([]*PullRequest, 0)

	for _, pr := range prs {
		branch := azureDevOpsRefToBranch(derefString(pr.SourceRefName))
		if !a.isTrustedReleasePR(&pr, baseBranch) {
			continue
		}

		needsPendingLabel := false

		if !azureDevOpsHasLabel(pr.Labels, pendingLabel) {
			if pr.Labels != nil && len(*pr.Labels) > 0 {
				return nil, fmt.Errorf(
					"%w: trusted pull request !%d on branch %q is missing configured pending label %q",
					ErrReleasePRLabelMismatch,
					derefInt(pr.PullRequestId),
					branch,
					pendingLabel,
				)
			}

			needsPendingLabel = true
		}

		pending = append(pending, &PullRequest{
			Number:            derefInt(pr.PullRequestId),
			Title:             derefString(pr.Title),
			Body:              derefString(pr.Description),
			URL:               a.pullRequestWebURL(derefInt(pr.PullRequestId)),
			Branch:            branch,
			NeedsPendingLabel: needsPendingLabel,
		})
	}

	slog.DebugContext(ctx, "azure devops: listed open pending release PRs", slog.Int("count", len(pending)))

	return pending, nil
}

func (a *AzureDevOps) FindMergedReleasePR(
	ctx context.Context,
	baseBranch, pendingLabel string,
) (*PullRequest, error) {
	slog.DebugContext(ctx, "azure devops: searching merged release PRs",
		slog.String("target_branch", baseBranch),
		slog.String("label", pendingLabel),
	)

	prs, err := a.listPullRequests(ctx, git.PullRequestStatusValues.Completed, baseBranch)
	if err != nil {
		return nil, err
	}

	for _, pr := range prs {
		if !a.isTrustedReleasePR(&pr, baseBranch) {
			continue
		}

		if !azureDevOpsHasLabel(pr.Labels, pendingLabel) {
			continue
		}

		number := derefInt(pr.PullRequestId)

		full, err := a.getPullRequest(ctx, number)
		if err != nil {
			return nil, err
		}

		if !a.isTrustedReleasePR(full, baseBranch) {
			continue
		}

		branch := azureDevOpsRefToBranch(derefString(full.SourceRefName))

		result := &PullRequest{
			Number:         number,
			Title:          derefString(full.Title),
			Body:           derefString(full.Description),
			URL:            a.pullRequestWebURL(number),
			Branch:         branch,
			MergeCommitSHA: azureDevOpsCompletedMergeCommit(full),
		}

		slog.DebugContext(ctx, "azure devops: found merged release PR",
			slog.Int("pr_number", result.Number),
			slog.String("url", result.URL),
			slog.String("merge_sha", result.MergeCommitSHA),
		)

		return result, nil
	}

	return nil, ErrNoPR
}

func (a *AzureDevOps) listPullRequests(
	ctx context.Context,
	status git.PullRequestStatus,
	baseBranch string,
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

func (a *AzureDevOps) MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) (string, error) {
	slog.DebugContext(ctx, "azure devops: completing pull request", slog.Int("pr_number", number))

	pr, err := a.getPullRequest(ctx, number)
	if err != nil {
		return "", err
	}

	if derefString((*string)(pr.Status)) == string(git.PullRequestStatusValues.Completed) {
		if mergeSHA := azureDevOpsCompletedMergeCommit(pr); mergeSHA != "" {
			return mergeSHA, nil
		}

		return a.awaitMergeCommit(ctx, number)
	}

	baseBranch := azureDevOpsRefToBranch(derefString(pr.TargetRefName))
	if !a.isTrustedReleasePR(pr, baseBranch) {
		return "", fmt.Errorf("%w: pull request !%d", ErrUntrustedReleasePR, number)
	}

	if err := validateAzureDevOpsPullRequestForMerge(number, pr, opts.BypassMergeChecks); err != nil {
		return "", err
	}

	merged, err := a.completePullRequest(ctx, number, pr, opts.Method)
	if err != nil {
		return "", err
	}

	if mergeSHA := azureDevOpsCompletionResponseCommit(merged); mergeSHA != "" {
		return mergeSHA, nil
	}

	return a.awaitMergeCommit(ctx, number)
}

func (a *AzureDevOps) completePullRequest(
	ctx context.Context,
	number int,
	pr *git.GitPullRequest,
	method MergeMethod,
) (*git.GitPullRequest, error) {
	strategy, err := azureDevOpsMergeStrategy(method)
	if err != nil {
		return nil, err
	}

	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	completed := git.PullRequestStatusValues.Completed

	update := git.GitPullRequest{
		Status: &completed,
		CompletionOptions: &git.GitPullRequestCompletionOptions{
			MergeStrategy: &strategy,
		},
	}

	if pr.LastMergeSourceCommit != nil && pr.LastMergeSourceCommit.CommitId != nil {
		update.LastMergeSourceCommit = &git.GitCommitRef{CommitId: pr.LastMergeSourceCommit.CommitId}
	}

	merged, err := gitClient.UpdatePullRequest(ctx, git.UpdatePullRequestArgs{
		GitPullRequestToUpdate: &update,
		RepositoryId:           &a.repo,
		Project:                &a.project,
		PullRequestId:          &number,
	})
	if err != nil {
		return nil, fmt.Errorf("complete pull request !%d: %w", number, err)
	}

	slog.DebugContext(ctx, "azure devops: completed pull request",
		slog.Int("pr_number", number),
		slog.String("strategy", string(strategy)),
	)

	return merged, nil
}

// awaitMergeCommit waits for a completion Azure DevOps has queued but not yet
// applied. The completion response can carry a provisional commit while the
// merge status is still queued, so only a succeeded merge status is trusted.
func (a *AzureDevOps) awaitMergeCommit(ctx context.Context, number int) (string, error) {
	reference := fmt.Sprintf("pull request !%d", number)

	return a.polling.awaitMergedCommit(ctx, reference, func(pollCtx context.Context) (string, error) {
		current, err := a.getPullRequest(pollCtx, number)
		if err != nil {
			return "", err
		}

		if derefString((*string)(current.Status)) == string(git.PullRequestStatusValues.Abandoned) {
			return "", fmt.Errorf("%w: %s was abandoned", ErrMergeBlocked, reference)
		}

		mergeStatus := derefString((*string)(current.MergeStatus))
		if azureDevOpsMergeStatusBlocked(mergeStatus) {
			return "", fmt.Errorf("%w: %s merge status is %s", ErrMergeBlocked, reference, mergeStatus)
		}

		return azureDevOpsCompletedMergeCommit(current), nil
	})
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

func validateAzureDevOpsPullRequestForMerge(
	number int,
	pullRequest *git.GitPullRequest,
	bypassMergeChecks bool,
) error {
	status := derefString((*string)(pullRequest.Status))
	if status != string(git.PullRequestStatusValues.Active) {
		return fmt.Errorf("%w: pull request !%d is %s", ErrMergeBlocked, number, status)
	}

	if pullRequest.IsDraft != nil && *pullRequest.IsDraft {
		return fmt.Errorf("%w: pull request !%d is draft", ErrMergeBlocked, number)
	}

	mergeStatus := derefString((*string)(pullRequest.MergeStatus))
	if !bypassMergeChecks && azureDevOpsMergeStatusBlocked(mergeStatus) {
		return fmt.Errorf("%w: pull request !%d merge_status=%s", ErrMergeBlocked, number, mergeStatus)
	}

	return nil
}

func (a *AzureDevOps) PrepareReleasePRLabels(context.Context, ReleasePRLabels) error {
	return nil
}

// MarkReleasePRPending attaches the pending label first so an interrupted run
// still leaves the PR discoverable, then keeps attaching the remaining labels
// even after a failure so a single rejected label cannot strand the rest.
func (a *AzureDevOps) MarkReleasePRPending(ctx context.Context, number int, labels ReleasePRLabels) error {
	if err := a.attachPullRequestLabel(ctx, number, labels.Pending); err != nil {
		return err
	}

	remaining := slices.Clone(labels.Extra)
	if labels.Yeet {
		remaining = append(remaining, ReleaseLabelYeet)
	}

	var errs []error

	for _, label := range remaining {
		if err := a.attachPullRequestLabel(ctx, number, label); err != nil {
			errs = append(errs, err)
		}
	}

	if err := a.detachPullRequestLabel(ctx, number, labels.Tagged); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (a *AzureDevOps) MarkReleasePRTagged(ctx context.Context, number int, labels ReleasePRLabels) error {
	if err := a.attachPullRequestLabel(ctx, number, labels.Tagged); err != nil {
		return err
	}

	return a.detachPullRequestLabel(ctx, number, labels.Pending)
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
	if labels == nil {
		return false
	}

	for _, label := range *labels {
		if label.Name != nil && strings.EqualFold(*label.Name, target) {
			return true
		}
	}

	return false
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

func azureDevOpsMergeStatusBlocked(status string) bool {
	switch status {
	case string(git.PullRequestAsyncStatusValues.Conflicts),
		string(git.PullRequestAsyncStatusValues.RejectedByPolicy),
		string(git.PullRequestAsyncStatusValues.Failure):
		return true
	default:
		return false
	}
}

func azureDevOpsMergeStrategy(method MergeMethod) (git.GitPullRequestMergeStrategy, error) {
	if method == "" {
		method = MergeMethodAuto
	}

	switch method {
	case MergeMethodAuto, MergeMethodSquash:
		return git.GitPullRequestMergeStrategyValues.Squash, nil
	case MergeMethodRebase:
		return git.GitPullRequestMergeStrategyValues.Rebase, nil
	case MergeMethodMerge:
		return git.GitPullRequestMergeStrategyValues.NoFastForward, nil
	default:
		return "", fmt.Errorf("%w: unknown merge method %q", ErrMergeMethodUnsupported, method)
	}
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}

	return *p
}
