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
	if err := g.labelDefinitions().validateExtras(ctx, opts.Labels.Extra); err != nil {
		return nil, err
	}

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

//nolint:funlen // Pagination closures keep trust and lifecycle checks beside candidate mapping.
func (g *GitHub) FindOpenPendingReleasePRs(
	ctx context.Context,
	baseBranch, pendingLabel string,
) ([]*PullRequest, error) {
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
		slog.String("label", pendingLabel),
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

			state := classifyReleasePRLabels(gitHubLabelNames(pr.Labels), pendingLabel, foldedLabelMatch)
			if state == releasePRLabelsMismatched {
				return false, releasePRLabelMismatch(
					gitHubPullRequestReference(pr.GetNumber()),
					branch,
					pendingLabel,
				)
			}

			pendingPRs = append(pendingPRs, &PullRequest{
				Number:            pr.GetNumber(),
				Title:             pr.GetTitle(),
				Body:              pr.GetBody(),
				URL:               pr.GetHTMLURL(),
				Branch:            branch,
				NeedsPendingLabel: state == releasePRLabelsAdoptable,
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
func (g *GitHub) FindMergedReleasePR(
	ctx context.Context,
	baseBranch, pendingLabel string,
) (*PullRequest, error) {
	query := fmt.Sprintf("repo:%s/%s is:pr is:merged base:%s label:%q",
		g.repo.Owner, g.repo.Name, baseBranch, pendingLabel)
	options := &github.SearchOptions{
		Sort:  "updated",
		Order: sortDirectionDesc,
		ListOptions: github.ListOptions{
			PerPage: gitHubPageSize,
		},
	}

	slog.DebugContext(ctx, "github: searching merged release PRs",
		slog.String("base", baseBranch),
		slog.String("label", pendingLabel),
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

func (g *GitHub) SetReleasePRLabels(
	ctx context.Context,
	number int,
	labels ReleasePRLabels,
	phase ReleasePRPhase,
) error {
	if err := g.labelDefinitions().prepare(ctx, labels, phase); err != nil {
		return err
	}

	change := managedLabelChange(labels, phase)

	return g.applyLabels(ctx, number, change.anchor, change.add, change.remove)
}

func (g *GitHub) PreflightReleasePRTagging(ctx context.Context, taggedLabel string) error {
	return g.labelDefinitions().validateExisting(ctx, taggedLabel, "tagged")
}

// applyLabels sends every addition in one request, which puts the anchor on the
// pull request before any removal is attempted.
func (g *GitHub) applyLabels(ctx context.Context, number int, anchor string, add, remove []string) error {
	if err := g.addIssueLabels(ctx, number, labelsAnchoredFirst(anchor, add)); err != nil {
		return err
	}

	for _, label := range remove {
		if err := g.removeIssueLabel(ctx, number, label); err != nil {
			return err
		}
	}

	return nil
}

func (g *GitHub) MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) (string, error) {
	slog.DebugContext(ctx, "github: merging pull request", slog.Int("pr_number", number))

	driver := mergeDriver{forge: &gitHubMerge{provider: g, number: number}, polling: g.polling}

	return driver.run(ctx, opts)
}

type gitHubMerge struct {
	provider *GitHub
	number   int
}

func (m *gitHubMerge) state(ctx context.Context) (mergeState, error) {
	pullRequest, _, err := m.provider.client.PullRequests.Get(
		ctx,
		m.provider.repo.Owner,
		m.provider.repo.Name,
		m.number,
	)
	if err != nil {
		return mergeState{}, fmt.Errorf("get pull request #%d: %w", m.number, err)
	}

	return gitHubMergeState(m.provider.repo, m.number, pullRequest), nil
}

func (m *gitHubMerge) resolveMethod(ctx context.Context, requested MergeMethod) (any, error) {
	method, err := m.provider.resolveGitHubMergeMethod(ctx, requested)
	if err != nil {
		return nil, err
	}

	return method, nil
}

func (m *gitHubMerge) execute(ctx context.Context, current mergeState, method any) (string, bool, error) {
	mergeMethod, ok := method.(MergeMethod)
	if !ok {
		return "", false, unsupportedResolvedMethod(method)
	}

	mergeOptions := &github.PullRequestOptions{MergeMethod: string(mergeMethod)}
	if current.HeadSHA != "" {
		mergeOptions.SHA = current.HeadSHA
	}

	result, _, err := m.provider.client.PullRequests.Merge(
		ctx,
		m.provider.repo.Owner,
		m.provider.repo.Name,
		m.number,
		"",
		mergeOptions,
	)
	if err != nil {
		return "", false, fmt.Errorf("merge pull request #%d: %w", m.number, err)
	}

	if !result.GetMerged() {
		detail := strings.TrimSpace(result.GetMessage())
		if detail == "" {
			detail = "merge not completed"
		}

		return "", false, blockedMerge(current.Reference, MergeBlockedReasonUnknown, detail)
	}

	slog.DebugContext(ctx, "github: merged pull request",
		slog.Int("pr_number", m.number),
		slog.String("method", string(mergeMethod)),
		slog.String("merge_sha", result.GetSHA()),
	)

	// GitHub occasionally reports a merge without its commit SHA, most often when
	// reading a pull request that merged moments earlier.
	mergeSHA := strings.TrimSpace(result.GetSHA())

	return mergeSHA, mergeSHA == "", nil
}

func gitHubMergeState(repo RepoInfo, number int, pullRequest *github.PullRequest) mergeState {
	head := pullRequest.GetHead()
	state := pullRequest.GetState()
	mergeableState := strings.TrimSpace(pullRequest.GetMergeableState())

	return mergeState{
		Reference:        gitHubPullRequestReference(number),
		RawReadiness:     "mergeable_state=" + mergeableState,
		MergeCommitSHA:   strings.TrimSpace(pullRequest.GetMergeCommitSHA()),
		HeadSHA:          strings.TrimSpace(head.GetSHA()),
		SourceBranch:     head.GetRef(),
		BaseBranch:       pullRequest.GetBase().GetRef(),
		IsOpen:           state == "open",
		IsMerged:         pullRequest.GetMerged(),
		IsClosedUnmerged: state == "closed" && !pullRequest.GetMerged(),
		IsDraft:          pullRequest.GetDraft() || mergeableState == "draft",
		HasConflicts:     isGitHubMergeStateConflicted(mergeableState),
		ReadinessBlocked: isGitHubMergeStateReadinessBlocked(mergeableState),
		SameRepository:   isGitHubSameRepository(repo, head),
	}
}

func (g *GitHub) isTrustedReleasePR(pullRequest *github.PullRequest, baseBranch string) bool {
	if pullRequest == nil || pullRequest.GetHead() == nil {
		return false
	}

	head := pullRequest.GetHead()

	return isExpectedReleaseBranch(head.GetRef(), baseBranch) && isGitHubSameRepository(g.repo, head)
}

func isGitHubSameRepository(repo RepoInfo, head *github.PullRequestBranch) bool {
	return strings.EqualFold(strings.TrimSpace(head.GetRepo().GetFullName()), repo.Owner+"/"+repo.Name)
}

func (g *GitHub) labelDefinitions() labelDefinitions {
	return labelDefinitions{
		get: func(ctx context.Context, name string) error {
			_, _, err := g.client.Issues.GetLabel(ctx, g.repo.Owner, g.repo.Name, name)
			if err != nil {
				return fmt.Errorf("get label %q: %w", name, err)
			}

			return nil
		},
		create: func(ctx context.Context, name, color, description string) error {
			_, _, err := g.client.Issues.CreateLabel(ctx, g.repo.Owner, g.repo.Name, &github.Label{
				Name:        new(name),
				Color:       new(color),
				Description: new(description),
			})
			if err != nil {
				return fmt.Errorf("create label %q: %w", name, err)
			}

			return nil
		},
		isNotFound: isGitHubNotFound,
	}
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

func gitHubLabelNames(labels []*github.Label) []string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.GetName())
	}

	return names
}

func gitHubPullRequestReference(number int) string {
	return fmt.Sprintf("pull request #%d", number)
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

		return "", gitHubMergeMethodBlocked("no merge methods enabled in repository settings")
	case MergeMethodSquash:
		if !allowSquash {
			return "", gitHubMergeMethodDisabled(requested)
		}
	case MergeMethodRebase:
		if !allowRebase {
			return "", gitHubMergeMethodDisabled(requested)
		}
	case MergeMethodMerge:
		if !allowMerge {
			return "", gitHubMergeMethodDisabled(requested)
		}
	default:
		return "", fmt.Errorf("%w: unknown merge method %q", ErrMergeMethodUnsupported, requested)
	}

	return requested, nil
}

func gitHubMergeMethodDisabled(requested MergeMethod) error {
	return gitHubMergeMethodBlocked(fmt.Sprintf("merge method %q disabled by repository settings", requested))
}

func gitHubMergeMethodBlocked(detail string) error {
	return blockedMerge("", MergeBlockedReasonMethod, detail)
}
