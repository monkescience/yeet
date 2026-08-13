package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v90/github"
	"github.com/monkescience/yeet/internal/forge"
)

func (g *GitHub) CreateReleasePR(ctx context.Context, opts forge.ReleasePROptions) (*forge.PullRequest, error) {
	if err := g.labelDefinitions().validateExtras(ctx, opts.Labels.Extra); err != nil {
		return nil, wrapReleasePRLabelsError(err)
	}

	if err := g.validateReviewers(ctx, opts.Reviewers); err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "github: creating pull request",
		slog.String("head", opts.ReleaseBranch),
		slog.String("base", opts.BaseBranch),
	)

	pr, _, err := g.client.PullRequests.Create(ctx, g.repo.Owner, g.repo.Name, github.CreatePullRequest{
		Title: new(opts.Title),
		Body:  new(opts.Body),
		Head:  opts.ReleaseBranch,
		Base:  opts.BaseBranch,
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

	return &forge.PullRequest{
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
			return fmt.Errorf("%w: %q is not a repository collaborator", forge.ErrReviewerNotFound, reviewer)
		}
	}

	return nil
}

// MaxPRBodyLength reports no enforced limit: GitHub accepts pull request bodies
// far larger than the release notes yeet generates.
func (g *GitHub) MaxPRBodyLength() int {
	return 0
}

func (g *GitHub) UpdateReleasePR(ctx context.Context, number int, opts forge.ReleasePROptions) error {
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
) ([]*forge.PullRequest, error) {
	options := &github.PullRequestListOptions{
		State:     "open",
		Head:      g.repo.Owner + ":" + releaseBranchName(g.releaseBranch, baseBranch),
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

	pendingPRs := make([]*forge.PullRequest, 0)

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

			needsLabel, err := needsPendingLabel(
				gitHubLabelNames(pr.Labels),
				pendingLabel,
				foldedLabelMatch,
				gitHubPullRequestReference(pr.GetNumber()),
				branch,
			)
			if err != nil {
				return false, err
			}

			pendingPRs = append(pendingPRs, &forge.PullRequest{
				Number:            pr.GetNumber(),
				Title:             pr.GetTitle(),
				Body:              pr.GetBody(),
				URL:               pr.GetHTMLURL(),
				Branch:            branch,
				NeedsPendingLabel: needsLabel,
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

type gitHubMergedCandidate struct {
	listed *github.PullRequest
	full   *github.PullRequest
}

func (g *GitHub) FindMergedReleasePR(
	ctx context.Context,
	baseBranch, pendingLabel string,
) (*forge.PullRequest, error) {
	slog.DebugContext(ctx, "github: listing merged release PRs",
		slog.String("base", baseBranch),
		slog.String("label", pendingLabel),
	)

	candidates, err := g.listGitHubMergedCandidates(ctx, baseBranch, pendingLabel)
	if err != nil {
		return nil, err
	}

	best, err := g.resolveLatestGitHubMergedCandidate(ctx, candidates)
	if err != nil {
		return nil, err
	}

	full := best.full
	if full == nil {
		var getErr error

		full, _, getErr = g.client.PullRequests.Get(ctx, g.repo.Owner, g.repo.Name, best.listed.GetNumber())
		if getErr != nil {
			return nil, fmt.Errorf("get pull request #%d: %w", best.listed.GetNumber(), getErr)
		}
	}

	if !full.GetMerged() || !g.isTrustedReleasePR(full, baseBranch) {
		return nil, forge.ErrNoPR
	}

	found := &forge.PullRequest{
		Number:         full.GetNumber(),
		Title:          full.GetTitle(),
		Body:           full.GetBody(),
		URL:            full.GetHTMLURL(),
		Branch:         full.GetHead().GetRef(),
		MergeCommitSHA: full.GetMergeCommitSHA(),
	}

	slog.DebugContext(ctx, "github: found merged release PR",
		slog.Int("pr_number", found.Number),
		slog.String("url", found.URL),
		slog.String("merge_sha", found.MergeCommitSHA),
	)

	return found, nil
}

func (g *GitHub) listGitHubMergedCandidates(
	ctx context.Context,
	baseBranch, pendingLabel string,
) ([]gitHubMergedCandidate, error) {
	options := &github.PullRequestListOptions{
		State: "closed",
		Head:  g.repo.Owner + ":" + releaseBranchName(g.releaseBranch, baseBranch),
		Base:  baseBranch,
		ListOptions: github.ListOptions{
			PerPage: gitHubPageSize,
		},
	}

	candidates := make([]gitHubMergedCandidate, 0)

	err := paginate(ctx, "listing merged release PRs",
		func(page int) ([]*github.PullRequest, int, error) {
			options.Page = page

			prs, resp, err := g.client.PullRequests.List(ctx, g.repo.Owner, g.repo.Name, options)
			if err != nil {
				return nil, 0, fmt.Errorf("list pull requests: %w", err)
			}

			return prs, gitHubNextPage(resp), nil
		},
		func(pr *github.PullRequest) (bool, error) {
			if !g.isTrustedReleasePR(pr, baseBranch) || !gitHubHasLabel(pr.Labels, pendingLabel) {
				return false, nil
			}

			candidates = append(candidates, gitHubMergedCandidate{listed: pr})

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return candidates, nil
}

func (g *GitHub) resolveLatestGitHubMergedCandidate(
	ctx context.Context,
	candidates []gitHubMergedCandidate,
) (*gitHubMergedCandidate, error) {
	best, err := resolveLatestMerged(ctx, candidates, mergedCandidates[gitHubMergedCandidate]{
		mergedAt: func(candidate gitHubMergedCandidate) (time.Time, bool) {
			if candidate.listed.MergedAt == nil {
				return time.Time{}, false
			}

			return gitHubMergedAt(candidate.listed), true
		},
		hydrate: func(ctx context.Context, candidate gitHubMergedCandidate) (gitHubMergedCandidate, bool, error) {
			number := candidate.listed.GetNumber()

			full, _, err := g.client.PullRequests.Get(ctx, g.repo.Owner, g.repo.Name, number)
			if err != nil {
				return candidate, false, fmt.Errorf("get pull request #%d: %w", number, err)
			}

			if !full.GetMerged() {
				return candidate, false, nil
			}

			if full.MergedAt == nil {
				return candidate, false, mergeTimeMissingError(gitHubPullRequestReference(number))
			}

			return gitHubMergedCandidate{listed: full, full: full}, true, nil
		},
		reference: func(candidate gitHubMergedCandidate) string {
			return gitHubPullRequestReference(candidate.listed.GetNumber())
		},
	})
	if err != nil {
		return nil, err
	}

	return &best, nil
}

func gitHubHasLabel(labels []*github.Label, target string) bool {
	for _, label := range labels {
		if strings.EqualFold(label.GetName(), target) {
			return true
		}
	}

	return false
}

func gitHubMergedAt(pr *github.PullRequest) time.Time {
	if pr.MergedAt == nil {
		return time.Time{}
	}

	return pr.MergedAt.Time
}

func (g *GitHub) SetReleasePRLabels(
	ctx context.Context,
	number int,
	labels forge.ReleasePRLabels,
	phase forge.ReleasePRPhase,
) error {
	if err := g.labelDefinitions().prepare(ctx, labels, phase); err != nil {
		return wrapReleasePRLabelsError(err)
	}

	change := managedLabelChange(labels, phase)

	return wrapReleasePRLabelsError(g.applyLabels(ctx, number, change.anchor, change.add, change.remove))
}

func (g *GitHub) PreflightReleasePRTagging(ctx context.Context, taggedLabel string) error {
	return wrapReleasePRLabelsError(g.labelDefinitions().validateExisting(ctx, taggedLabel, "tagged"))
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

func (g *GitHub) MergeReleasePR(ctx context.Context, number int, opts forge.MergeReleasePROptions) (string, error) {
	slog.DebugContext(ctx, "github: merging pull request", slog.Int("pr_number", number))

	driver := mergeDriver{
		forge: &gitHubMerge{provider: g, number: number}, polling: g.polling, releaseBranch: g.releaseBranch,
	}

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

func (m *gitHubMerge) resolveMethod(ctx context.Context, requested forge.MergeMethod) (any, error) {
	method, err := m.provider.resolveGitHubMergeMethod(ctx, requested)
	if err != nil {
		return nil, err
	}

	return method, nil
}

func (m *gitHubMerge) execute(ctx context.Context, current mergeState, method any) (string, bool, error) {
	mergeMethod, ok := method.(forge.MergeMethod)
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

		return "", false, blockedMerge(current.Reference, forge.MergeBlockedReasonUnknown, detail)
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

func gitHubMergeState(repo repoInfo, number int, pullRequest *github.PullRequest) mergeState {
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

	return isExpectedReleaseBranch(head.GetRef(), baseBranch, g.releaseBranch) &&
		isGitHubSameRepository(g.repo, head)
}

func isGitHubSameRepository(repo repoInfo, head *github.PullRequestBranch) bool {
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
			_, _, err := g.client.Issues.CreateLabel(ctx, g.repo.Owner, g.repo.Name, github.CreateIssueLabelRequest{
				Name:        name,
				Color:       new(color),
				Description: new(description),
			})
			if err != nil {
				return fmt.Errorf("create label %q: %w", name, err)
			}

			return nil
		},
		isNotFound: isGitHubNotFound,
		cache:      &g.labels,
		normalize:  strings.ToLower,
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

func (g *GitHub) resolveGitHubMergeMethod(ctx context.Context, requested forge.MergeMethod) (forge.MergeMethod, error) {
	repo, _, err := g.client.Repositories.Get(ctx, g.repo.Owner, g.repo.Name)
	if err != nil {
		return "", fmt.Errorf("get repository merge settings: %w", err)
	}

	allowSquash := repo.GetAllowSquashMerge()
	allowRebase := repo.GetAllowRebaseMerge()
	allowMerge := repo.GetAllowMergeCommit()

	if requested == "" {
		requested = forge.MergeMethodAuto
	}

	switch requested {
	case forge.MergeMethodAuto:
		if allowSquash {
			return forge.MergeMethodSquash, nil
		}

		if allowRebase {
			return forge.MergeMethodRebase, nil
		}

		if allowMerge {
			return forge.MergeMethodMerge, nil
		}

		return "", gitHubMergeMethodBlocked("no merge methods enabled in repository settings")
	case forge.MergeMethodSquash:
		if !allowSquash {
			return "", gitHubMergeMethodDisabled(requested)
		}
	case forge.MergeMethodRebase:
		if !allowRebase {
			return "", gitHubMergeMethodDisabled(requested)
		}
	case forge.MergeMethodMerge:
		if !allowMerge {
			return "", gitHubMergeMethodDisabled(requested)
		}
	default:
		return "", fmt.Errorf("%w: unknown merge method %q", forge.ErrMergeMethodUnsupported, requested)
	}

	return requested, nil
}

func gitHubMergeMethodDisabled(requested forge.MergeMethod) error {
	return gitHubMergeMethodBlocked(fmt.Sprintf("merge method %q disabled by repository settings", requested))
}

func gitHubMergeMethodBlocked(detail string) error {
	return blockedMerge("", forge.MergeBlockedReasonMethod, detail)
}
