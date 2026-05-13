package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

const azureDevOpsPRPageSize = 100

var errAzureDevOpsLabelIDMissing = errors.New("azure devops label id missing")

func (a *AzureDevOps) CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	pr := git.GitPullRequest{
		SourceRefName: new("refs/heads/" + opts.ReleaseBranch),
		TargetRefName: new("refs/heads/" + opts.BaseBranch),
		Title:         new(opts.Title),
		Description:   new(opts.Body),
	}

	created, err := gitClient.CreatePullRequest(ctx, git.CreatePullRequestArgs{
		GitPullRequestToCreate: &pr,
		RepositoryId:           &a.repo,
		Project:                &a.project,
	})
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}

	return &PullRequest{
		Number: derefInt(created.PullRequestId),
		Title:  derefString(created.Title),
		Body:   derefString(created.Description),
		URL:    a.pullRequestWebURL(derefInt(created.PullRequestId)),
		Branch: opts.ReleaseBranch,
	}, nil
}

func (a *AzureDevOps) UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error {
	gitClient, err := a.client(ctx)
	if err != nil {
		return err
	}

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

	return nil
}

func (a *AzureDevOps) FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error) {
	prs, err := a.listPullRequests(ctx, git.PullRequestStatusValues.Active, baseBranch)
	if err != nil {
		return nil, err
	}

	pending := make([]*PullRequest, 0)

	for _, pr := range prs {
		branch := azureDevOpsRefToBranch(derefString(pr.SourceRefName))
		if !strings.HasPrefix(branch, releaseBranchPrefix) {
			continue
		}

		if !azureDevOpsHasLabel(pr.Labels, ReleaseLabelPending) {
			continue
		}

		pending = append(pending, &PullRequest{
			Number: derefInt(pr.PullRequestId),
			Title:  derefString(pr.Title),
			Body:   derefString(pr.Description),
			URL:    a.pullRequestWebURL(derefInt(pr.PullRequestId)),
			Branch: branch,
		})
	}

	return pending, nil
}

func (a *AzureDevOps) FindMergedReleasePR(ctx context.Context, baseBranch string) (*PullRequest, error) {
	prs, err := a.listPullRequests(ctx, git.PullRequestStatusValues.Completed, baseBranch)
	if err != nil {
		return nil, err
	}

	for _, pr := range prs {
		branch := azureDevOpsRefToBranch(derefString(pr.SourceRefName))
		if !strings.HasPrefix(branch, releaseBranchPrefix) {
			continue
		}

		if !azureDevOpsHasLabel(pr.Labels, ReleaseLabelPending) {
			continue
		}

		number := derefInt(pr.PullRequestId)

		full, err := a.getPullRequest(ctx, number)
		if err != nil {
			return nil, err
		}

		return &PullRequest{
			Number:         number,
			Title:          derefString(full.Title),
			Body:           derefString(full.Description),
			URL:            a.pullRequestWebURL(number),
			Branch:         branch,
			MergeCommitSHA: azureDevOpsMergeCommit(full),
		}, nil
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
	skip := 0
	top := azureDevOpsPRPageSize
	targetRef := "refs/heads/" + baseBranch

	for range maxPaginationPages {
		err := ctx.Err()
		if err != nil {
			return nil, fmt.Errorf("paginate pull requests: %w", err)
		}

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
			return all, nil
		}

		all = append(all, *page...)

		if len(*page) < azureDevOpsPRPageSize {
			return all, nil
		}

		skip += len(*page)
	}

	return nil, fmt.Errorf("%w: exceeded %d pages listing pull requests", ErrPaginationLimitExceeded, maxPaginationPages)
}

func (a *AzureDevOps) MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) error {
	pr, err := a.getPullRequest(ctx, number)
	if err != nil {
		return err
	}

	status := derefString((*string)(pr.Status))

	if status == string(git.PullRequestStatusValues.Completed) {
		return nil
	}

	if status != string(git.PullRequestStatusValues.Active) {
		return fmt.Errorf("%w: pull request !%d is %s", ErrMergeBlocked, number, status)
	}

	if pr.IsDraft != nil && *pr.IsDraft {
		return fmt.Errorf("%w: pull request !%d is draft", ErrMergeBlocked, number)
	}

	mergeStatus := derefString((*string)(pr.MergeStatus))
	if !opts.Force && azureDevOpsMergeStatusBlocked(mergeStatus) {
		return fmt.Errorf("%w: pull request !%d merge_status=%s", ErrMergeBlocked, number, mergeStatus)
	}

	strategy, err := azureDevOpsMergeStrategy(opts.Method)
	if err != nil {
		return err
	}

	gitClient, err := a.client(ctx)
	if err != nil {
		return err
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

	_, err = gitClient.UpdatePullRequest(ctx, git.UpdatePullRequestArgs{
		GitPullRequestToUpdate: &update,
		RepositoryId:           &a.repo,
		Project:                &a.project,
		PullRequestId:          &number,
	})
	if err != nil {
		return fmt.Errorf("complete pull request !%d: %w", number, err)
	}

	return nil
}

func (a *AzureDevOps) MarkReleasePRPending(ctx context.Context, number int) error {
	err := a.attachPullRequestLabel(ctx, number, ReleaseLabelPending)
	if err != nil {
		return err
	}

	return a.detachPullRequestLabel(ctx, number, ReleaseLabelTagged)
}

func (a *AzureDevOps) MarkReleasePRTagged(ctx context.Context, number int) error {
	err := a.attachPullRequestLabel(ctx, number, ReleaseLabelTagged)
	if err != nil {
		return err
	}

	return a.detachPullRequestLabel(ctx, number, ReleaseLabelPending)
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
	pr, err := a.getPullRequest(ctx, number)
	if err != nil {
		return fmt.Errorf("get labels for pull request !%d: %w", number, err)
	}

	labelID, ok, err := azureDevOpsPullRequestLabelID(pr.Labels, label)
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

func azureDevOpsPullRequestLabelID(labels *[]core.WebApiTagDefinition, target string) (string, bool, error) {
	if labels == nil {
		return "", false, nil
	}

	for _, label := range *labels {
		if label.Name == nil || *label.Name != target {
			continue
		}

		if label.Id == nil {
			return "", false, fmt.Errorf("%w: %q", errAzureDevOpsLabelIDMissing, target)
		}

		return label.Id.String(), true, nil
	}

	return "", false, nil
}

func (a *AzureDevOps) CommitPullRequestBody(ctx context.Context, hash string) (string, bool, error) {
	commitHash := strings.TrimSpace(hash)
	if commitHash == "" {
		return "", false, nil
	}

	prs, err := a.listPullRequests(ctx, git.PullRequestStatusValues.Completed, "")
	if err != nil {
		return "", false, fmt.Errorf("list pull requests for commit %q: %w", commitHash, err)
	}

	for _, pr := range prs {
		if azureDevOpsMergeCommit(&pr) != commitHash {
			continue
		}

		return derefString(pr.Description), true, nil
	}

	return "", false, nil
}

func (a *AzureDevOps) getPullRequest(ctx context.Context, number int) (*git.GitPullRequest, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	pr, err := gitClient.GetPullRequestById(ctx, git.GetPullRequestByIdArgs{
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
		if label.Name != nil && *label.Name == target {
			return true
		}
	}

	return false
}

func (a *AzureDevOps) pullRequestWebURL(id int) string {
	return fmt.Sprintf("%s/pullrequest/%d", a.RepoURL(), id)
}

func azureDevOpsMergeCommit(pr *git.GitPullRequest) string {
	if pr.LastMergeCommit != nil && pr.LastMergeCommit.CommitId != nil && *pr.LastMergeCommit.CommitId != "" {
		return *pr.LastMergeCommit.CommitId
	}

	if pr.LastMergeSourceCommit != nil && pr.LastMergeSourceCommit.CommitId != nil {
		return *pr.LastMergeSourceCommit.CommitId
	}

	return ""
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
