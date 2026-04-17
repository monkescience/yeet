package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v84/github"
)

const releasePendingLabelColor = "FBCA04"

const releaseTaggedLabelColor = "0E8A16"

func (g *GitHub) CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error) {
	pr, _, err := g.client.PullRequests.Create(ctx, g.repo.Owner, g.repo.Name, &github.NewPullRequest{
		Title: new(opts.Title),
		Body:  new(opts.Body),
		Head:  new(opts.ReleaseBranch),
		Base:  new(opts.BaseBranch),
	})
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}

	return &PullRequest{
		Number: pr.GetNumber(),
		Title:  pr.GetTitle(),
		Body:   pr.GetBody(),
		URL:    pr.GetHTMLURL(),
		Branch: opts.ReleaseBranch,
	}, nil
}

func (g *GitHub) UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error {
	_, _, err := g.client.PullRequests.Edit(ctx, g.repo.Owner, g.repo.Name, number, &github.PullRequest{
		Title: new(opts.Title),
		Body:  new(opts.Body),
	})
	if err != nil {
		return fmt.Errorf("update pull request #%d: %w", number, err)
	}

	return nil
}

func (g *GitHub) FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error) {
	options := &github.PullRequestListOptions{
		State:     "open",
		Base:      baseBranch,
		Sort:      "updated",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100, //nolint:mnd // reasonable API page size
		},
	}

	pendingPRs := make([]*PullRequest, 0)

	for range maxPaginationPages {
		prs, resp, err := g.client.PullRequests.List(ctx, g.repo.Owner, g.repo.Name, options)
		if err != nil {
			return nil, fmt.Errorf("list pull requests: %w", err)
		}

		for _, pr := range prs {
			branch := pr.GetHead().GetRef()
			if !strings.HasPrefix(branch, releaseBranchPrefix) {
				continue
			}

			if !hasGitHubLabel(pr.Labels, ReleaseLabelPending) {
				continue
			}

			pendingPRs = append(pendingPRs, &PullRequest{
				Number: pr.GetNumber(),
				Title:  pr.GetTitle(),
				Body:   pr.GetBody(),
				URL:    pr.GetHTMLURL(),
				Branch: branch,
			})
		}

		if resp.NextPage == 0 {
			return pendingPRs, nil
		}

		options.Page = resp.NextPage
	}

	return nil, fmt.Errorf(
		"%w: exceeded %d pages listing open pending release PRs",
		ErrPaginationLimitExceeded, maxPaginationPages,
	)
}

func (g *GitHub) FindMergedReleasePR(ctx context.Context, baseBranch string) (*PullRequest, error) {
	options := &github.PullRequestListOptions{
		State:     "closed",
		Base:      baseBranch,
		Sort:      "updated",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100, //nolint:mnd // reasonable API page size
		},
	}

	for range maxPaginationPages {
		prs, resp, err := g.client.PullRequests.List(ctx, g.repo.Owner, g.repo.Name, options)
		if err != nil {
			return nil, fmt.Errorf("list pull requests: %w", err)
		}

		for _, pr := range prs {
			if pr.GetMergedAt().IsZero() {
				continue
			}

			branch := pr.GetHead().GetRef()
			if !strings.HasPrefix(branch, releaseBranchPrefix) {
				continue
			}

			if !hasGitHubLabel(pr.Labels, ReleaseLabelPending) {
				continue
			}

			fullPR, _, err := g.client.PullRequests.Get(ctx, g.repo.Owner, g.repo.Name, pr.GetNumber())
			if err != nil {
				return nil, fmt.Errorf("get pull request #%d: %w", pr.GetNumber(), err)
			}

			return &PullRequest{
				Number:         pr.GetNumber(),
				Title:          pr.GetTitle(),
				Body:           pr.GetBody(),
				URL:            pr.GetHTMLURL(),
				Branch:         branch,
				MergeCommitSHA: fullPR.GetMergeCommitSHA(),
			}, nil
		}

		if resp.NextPage == 0 {
			return nil, ErrNoPR
		}

		options.Page = resp.NextPage
	}

	return nil, fmt.Errorf(
		"%w: exceeded %d pages listing merged release PRs",
		ErrPaginationLimitExceeded, maxPaginationPages,
	)
}

func (g *GitHub) MarkReleasePRPending(ctx context.Context, number int) error {
	err := g.ensureReleaseLabels(ctx)
	if err != nil {
		return err
	}

	err = g.addIssueLabels(ctx, number, []string{ReleaseLabelPending})
	if err != nil {
		return err
	}

	err = g.removeIssueLabel(ctx, number, ReleaseLabelTagged)
	if err != nil {
		return err
	}

	return nil
}

func (g *GitHub) MarkReleasePRTagged(ctx context.Context, number int) error {
	err := g.ensureReleaseLabels(ctx)
	if err != nil {
		return err
	}

	err = g.addIssueLabels(ctx, number, []string{ReleaseLabelTagged})
	if err != nil {
		return err
	}

	err = g.removeIssueLabel(ctx, number, ReleaseLabelPending)
	if err != nil {
		return err
	}

	return nil
}

func (g *GitHub) MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) error {
	pr, _, err := g.client.PullRequests.Get(ctx, g.repo.Owner, g.repo.Name, number)
	if err != nil {
		return fmt.Errorf("get pull request #%d: %w", number, err)
	}

	if pr.GetMerged() {
		return nil
	}

	if pr.GetState() != "open" {
		return fmt.Errorf("%w: pull request #%d is %s", ErrMergeBlocked, number, pr.GetState())
	}

	mergeableState := strings.TrimSpace(pr.GetMergeableState())

	if pr.GetDraft() || mergeableState == "draft" {
		return fmt.Errorf("%w: pull request #%d is draft", ErrMergeBlocked, number)
	}

	if isGitHubMergeStateConflicted(mergeableState) {
		return fmt.Errorf("%w: pull request #%d has conflicts", ErrMergeBlocked, number)
	}

	if !opts.Force {
		if isGitHubMergeStateReadinessBlocked(mergeableState) {
			return fmt.Errorf("%w: pull request #%d mergeable_state=%s", ErrMergeBlocked, number, mergeableState)
		}
	}

	mergeMethod, err := g.resolveGitHubMergeMethod(ctx, opts.Method)
	if err != nil {
		return err
	}

	mergeOptions := &github.PullRequestOptions{MergeMethod: string(mergeMethod)}

	headSHA := strings.TrimSpace(pr.GetHead().GetSHA())
	if headSHA != "" {
		mergeOptions.SHA = headSHA
	}

	mergeResult, _, err := g.client.PullRequests.Merge(ctx, g.repo.Owner, g.repo.Name, number, "", mergeOptions)
	if err != nil {
		return fmt.Errorf("merge pull request #%d: %w", number, err)
	}

	if !mergeResult.GetMerged() {
		message := strings.TrimSpace(mergeResult.GetMessage())
		if message == "" {
			message = "merge not completed"
		}

		return fmt.Errorf("%w: pull request #%d: %s", ErrMergeBlocked, number, message)
	}

	return nil
}

func (g *GitHub) ensureReleaseLabels(ctx context.Context) error {
	err := g.ensureLabel(ctx, ReleaseLabelPending, releasePendingLabelColor, "release PR is pending tagging")
	if err != nil {
		return err
	}

	err = g.ensureLabel(ctx, ReleaseLabelTagged, releaseTaggedLabelColor, "release PR already tagged")
	if err != nil {
		return err
	}

	return nil
}

func (g *GitHub) ensureLabel(ctx context.Context, name, color, description string) error {
	_, resp, err := g.client.Issues.GetLabel(ctx, g.repo.Owner, g.repo.Name, name)
	if err == nil {
		return nil
	}

	if resp == nil || resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("get label %q: %w", name, err)
	}

	_, _, err = g.client.Issues.CreateLabel(ctx, g.repo.Owner, g.repo.Name, &github.Label{
		Name:        new(name),
		Color:       new(color),
		Description: new(description),
	})
	if err != nil {
		return fmt.Errorf("create label %q: %w", name, err)
	}

	return nil
}

func (g *GitHub) addIssueLabels(ctx context.Context, number int, labels []string) error {
	_, _, err := g.client.Issues.AddLabelsToIssue(ctx, g.repo.Owner, g.repo.Name, number, labels)
	if err != nil {
		return fmt.Errorf("add labels to pull request #%d: %w", number, err)
	}

	return nil
}

func (g *GitHub) removeIssueLabel(ctx context.Context, number int, label string) error {
	resp, err := g.client.Issues.RemoveLabelForIssue(ctx, g.repo.Owner, g.repo.Name, number, label)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil
		}

		return fmt.Errorf("remove label %q from pull request #%d: %w", label, number, err)
	}

	return nil
}

func hasGitHubLabel(labels []*github.Label, target string) bool {
	for _, label := range labels {
		if label.GetName() == target {
			return true
		}
	}

	return false
}

func isGitHubMergeStateReadinessBlocked(state string) bool {
	switch state {
	case "blocked":
		return true
	default:
		return false
	}
}

func isGitHubMergeStateConflicted(state string) bool {
	return state == "dirty"
}

func (g *GitHub) resolveGitHubMergeMethod(ctx context.Context, requested MergeMethod) (MergeMethod, error) {
	repo, _, err := g.client.Repositories.Get(ctx, g.repo.Owner, g.repo.Name)
	if err != nil {
		return "", fmt.Errorf("get repository merge settings: %w", err)
	}

	allowSquash := repo.GetAllowSquashMerge()
	allowRebase := repo.GetAllowRebaseMerge()
	allowMerge := repo.GetAllowMergeCommit()

	if requested == "" {
		requested = MergeMethodAuto
	}

	switch requested {
	case MergeMethodAuto:
		if allowSquash {
			return MergeMethodSquash, nil
		}

		if allowRebase {
			return MergeMethodRebase, nil
		}

		if allowMerge {
			return MergeMethodMerge, nil
		}

		return "", fmt.Errorf("%w: no merge methods enabled in repository settings", ErrMergeBlocked)
	case MergeMethodSquash:
		if !allowSquash {
			return "", fmt.Errorf("%w: merge method %q disabled by repository settings", ErrMergeBlocked, requested)
		}
	case MergeMethodRebase:
		if !allowRebase {
			return "", fmt.Errorf("%w: merge method %q disabled by repository settings", ErrMergeBlocked, requested)
		}
	case MergeMethodMerge:
		if !allowMerge {
			return "", fmt.Errorf("%w: merge method %q disabled by repository settings", ErrMergeBlocked, requested)
		}
	default:
		return "", fmt.Errorf("%w: unknown merge method %q", ErrMergeMethodUnsupported, requested)
	}

	return requested, nil
}
