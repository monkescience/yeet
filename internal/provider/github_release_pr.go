package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/go-github/v89/github"
)

func (g *GitHub) CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error) {
	if err := g.validateReviewers(ctx, opts.Reviewers); err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "github: creating pull request",
		slog.String("head", opts.ReleaseBranch),
		slog.String("base", opts.BaseBranch),
	)

	pr, _, err := g.client.PullRequests.Create(ctx, g.repo.Owner, g.repo.Name, &github.NewPullRequest{
		Title: new(opts.Title),
		Body:  new(opts.Body),
		Head:  new(opts.ReleaseBranch),
		Base:  new(opts.BaseBranch),
	})
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}

	slog.DebugContext(ctx, "github: created pull request",
		slog.Int("pr_number", pr.GetNumber()),
		slog.String("url", pr.GetHTMLURL()),
	)

	if len(opts.Reviewers) > 0 {
		_, _, err = g.client.PullRequests.RequestReviewers(ctx, g.repo.Owner, g.repo.Name, pr.GetNumber(),
			github.ReviewersRequest{Reviewers: opts.Reviewers})
		if err != nil {
			return nil, fmt.Errorf("request reviewers %v for pull request #%d: %w",
				opts.Reviewers, pr.GetNumber(), err)
		}

		slog.DebugContext(ctx, "github: requested reviewers",
			slog.Int("pr_number", pr.GetNumber()),
			slog.Int("reviewer_count", len(opts.Reviewers)),
		)
	}

	return &PullRequest{
		Number: pr.GetNumber(),
		Title:  pr.GetTitle(),
		Body:   pr.GetBody(),
		URL:    pr.GetHTMLURL(),
		Branch: opts.ReleaseBranch,
	}, nil
}

// validateReviewers runs before the pull request is created: a reviewer
// failure after creation would leave an unlabeled PR that
// FindOpenPendingReleasePRs cannot pick up, wedging every subsequent run.
// GitHub only accepts collaborators as reviewers.
func (g *GitHub) validateReviewers(ctx context.Context, reviewers []string) error {
	if len(reviewers) == 0 {
		return nil
	}

	slog.DebugContext(ctx, "github: validating reviewers", slog.Int("reviewer_count", len(reviewers)))

	for _, reviewer := range reviewers {
		isCollaborator, _, err := g.client.Repositories.IsCollaborator(ctx, g.repo.Owner, g.repo.Name, reviewer)
		if err != nil {
			return fmt.Errorf("check reviewer %q: %w", reviewer, err)
		}

		if !isCollaborator {
			return fmt.Errorf("%w: %q is not a repository collaborator", ErrReviewerNotFound, reviewer)
		}
	}

	return nil
}

// MaxPRBodyLength reports no enforced limit: GitHub accepts pull request bodies
// far larger than the release notes yeet generates.
func (g *GitHub) MaxPRBodyLength() int {
	return 0
}

func (g *GitHub) UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error {
	slog.DebugContext(ctx, "github: updating pull request", slog.Int("pr_number", number))

	_, _, err := g.client.PullRequests.Edit(ctx, g.repo.Owner, g.repo.Name, number, &github.PullRequest{
		Title: new(opts.Title),
		Body:  new(opts.Body),
	})
	if err != nil {
		return fmt.Errorf("update pull request #%d: %w", number, err)
	}

	slog.DebugContext(ctx, "github: updated pull request", slog.Int("pr_number", number))

	return nil
}

func (g *GitHub) FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error) {
	options := &github.PullRequestListOptions{
		State:     "open",
		Base:      baseBranch,
		Sort:      "updated",
		Direction: sortDirectionDesc,
		ListOptions: github.ListOptions{
			PerPage: gitHubPageSize,
		},
	}

	slog.DebugContext(ctx, "github: listing open pending release PRs",
		slog.String("base", baseBranch),
		slog.String("label", ReleaseLabelPending),
	)

	pendingPRs := make([]*PullRequest, 0)

	err := paginate(ctx, "listing open pending release PRs",
		func(page int) ([]*github.PullRequest, int, error) {
			options.Page = page

			prs, resp, err := g.client.PullRequests.List(ctx, g.repo.Owner, g.repo.Name, options)
			if err != nil {
				return nil, 0, fmt.Errorf("list pull requests: %w", err)
			}

			return prs, gitHubNextPage(resp), nil
		},
		func(pr *github.PullRequest) (bool, error) {
			if !g.isTrustedReleasePR(pr, baseBranch) {
				return false, nil
			}

			branch := pr.GetHead().GetRef()

			if !hasGitHubLabel(pr.Labels, ReleaseLabelPending) {
				return false, nil
			}

			pendingPRs = append(pendingPRs, &PullRequest{
				Number: pr.GetNumber(),
				Title:  pr.GetTitle(),
				Body:   pr.GetBody(),
				URL:    pr.GetHTMLURL(),
				Branch: branch,
			})

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "github: listed open pending release PRs", slog.Int("count", len(pendingPRs)))

	return pendingPRs, nil
}

//nolint:funlen // Pagination closures keep the search and candidate handling together.
func (g *GitHub) FindMergedReleasePR(ctx context.Context, baseBranch string) (*PullRequest, error) {
	query := fmt.Sprintf("repo:%s/%s is:pr is:merged base:%s label:%q",
		g.repo.Owner, g.repo.Name, baseBranch, ReleaseLabelPending)
	options := &github.SearchOptions{
		Sort:  "updated",
		Order: sortDirectionDesc,
		ListOptions: github.ListOptions{
			PerPage: gitHubPageSize,
		},
	}

	slog.DebugContext(ctx, "github: searching merged release PRs",
		slog.String("base", baseBranch),
		slog.String("label", ReleaseLabelPending),
	)

	var found *PullRequest

	err := paginate(ctx, "searching merged release PRs",
		func(page int) ([]*github.Issue, int, error) {
			options.Page = page

			result, resp, err := g.client.Search.Issues(ctx, query, options)
			if err != nil {
				return nil, 0, fmt.Errorf("search pull requests: %w", err)
			}

			return result.Issues, gitHubNextPage(resp), nil
		},
		func(issue *github.Issue) (bool, error) {
			fullPR, _, err := g.client.PullRequests.Get(ctx, g.repo.Owner, g.repo.Name, issue.GetNumber())
			if err != nil {
				return false, fmt.Errorf("get pull request #%d: %w", issue.GetNumber(), err)
			}

			if !g.isTrustedReleasePR(fullPR, baseBranch) {
				return false, nil
			}

			branch := fullPR.GetHead().GetRef()

			found = &PullRequest{
				Number:         fullPR.GetNumber(),
				Title:          fullPR.GetTitle(),
				Body:           fullPR.GetBody(),
				URL:            fullPR.GetHTMLURL(),
				Branch:         branch,
				MergeCommitSHA: fullPR.GetMergeCommitSHA(),
			}

			return true, nil
		},
	)
	if err != nil {
		return nil, err
	}

	if found == nil {
		return nil, ErrNoPR
	}

	slog.DebugContext(ctx, "github: found merged release PR",
		slog.Int("pr_number", found.Number),
		slog.String("url", found.URL),
		slog.String("merge_sha", found.MergeCommitSHA),
	)

	return found, nil
}

func (g *GitHub) MarkReleasePRPending(ctx context.Context, number int) error {
	return g.updateReleasePRLabels(ctx, number, ReleaseLabelPending, ReleaseLabelTagged)
}

func (g *GitHub) MarkReleasePRTagged(ctx context.Context, number int) error {
	return g.updateReleasePRLabels(ctx, number, ReleaseLabelTagged, ReleaseLabelPending)
}

func (g *GitHub) updateReleasePRLabels(ctx context.Context, number int, addLabel, removeLabel string) error {
	if err := g.ensureReleaseLabels(ctx); err != nil {
		return err
	}

	if err := g.addIssueLabels(ctx, number, []string{addLabel}); err != nil {
		return err
	}

	if err := g.removeIssueLabel(ctx, number, removeLabel); err != nil {
		return err
	}

	return nil
}

func (g *GitHub) MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) error {
	slog.DebugContext(ctx, "github: merging pull request", slog.Int("pr_number", number))

	pr, _, err := g.client.PullRequests.Get(ctx, g.repo.Owner, g.repo.Name, number)
	if err != nil {
		return fmt.Errorf("get pull request #%d: %w", number, err)
	}

	if pr.GetMerged() {
		return nil
	}

	if !g.isTrustedReleasePR(pr, pr.GetBase().GetRef()) {
		return fmt.Errorf("%w: pull request #%d", ErrUntrustedReleasePR, number)
	}

	if err := validateGitHubPullRequestForMerge(number, pr, opts.BypassMergeChecks); err != nil {
		return err
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

	slog.DebugContext(ctx, "github: merged pull request",
		slog.Int("pr_number", number),
		slog.String("method", string(mergeMethod)),
		slog.String("merge_sha", mergeResult.GetSHA()),
	)

	return nil
}

func (g *GitHub) isTrustedReleasePR(pullRequest *github.PullRequest, baseBranch string) bool {
	if pullRequest == nil || pullRequest.GetHead() == nil {
		return false
	}

	head := pullRequest.GetHead()
	expectedRepository := g.repo.Owner + "/" + g.repo.Name

	return isExpectedReleaseBranch(head.GetRef(), baseBranch) &&
		strings.EqualFold(strings.TrimSpace(head.GetRepo().GetFullName()), expectedRepository)
}

func validateGitHubPullRequestForMerge(
	number int,
	pullRequest *github.PullRequest,
	bypassMergeChecks bool,
) error {
	if pullRequest.GetState() != "open" {
		return fmt.Errorf("%w: pull request #%d is %s", ErrMergeBlocked, number, pullRequest.GetState())
	}

	mergeableState := strings.TrimSpace(pullRequest.GetMergeableState())
	if pullRequest.GetDraft() || mergeableState == "draft" {
		return fmt.Errorf("%w: pull request #%d is draft", ErrMergeBlocked, number)
	}

	if isGitHubMergeStateConflicted(mergeableState) {
		return fmt.Errorf("%w: pull request #%d has conflicts", ErrMergeBlocked, number)
	}

	if !bypassMergeChecks && isGitHubMergeStateReadinessBlocked(mergeableState) {
		return fmt.Errorf("%w: pull request #%d mergeable_state=%s", ErrMergeBlocked, number, mergeableState)
	}

	return nil
}

func (g *GitHub) ensureReleaseLabels(ctx context.Context) error {
	err := g.ensureLabel(ctx, ReleaseLabelPending, releaseLabelPendingColor, releaseLabelPendingDescription)
	if err != nil {
		return err
	}

	err = g.ensureLabel(ctx, ReleaseLabelTagged, releaseLabelTaggedColor, releaseLabelTaggedDescription)
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
