package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/go-github/v84/github"
)

type GitHub struct {
	client  *github.Client
	repo    RepoInfo
	baseURL string
}

const releaseBranchPrefix = "yeet/release-"

const releasePendingLabelColor = "FBCA04"

const releaseTaggedLabelColor = "0E8A16"

func NewGitHub(client *github.Client, owner, repo string) *GitHub {
	baseURL := strings.TrimSuffix(client.BaseURL.String(), "/")

	// Default github.com API uses api.github.com; enterprise uses <host>/api/v3.
	if baseURL == "https://api.github.com" {
		baseURL = "https://github.com"
	} else {
		baseURL = strings.TrimSuffix(baseURL, "/api/v3")
	}

	return &GitHub{
		client:  client,
		repo:    RepoInfo{Owner: owner, Name: repo},
		baseURL: baseURL,
	}
}

func (g *GitHub) RepoURL() string {
	return fmt.Sprintf("%s/%s/%s", g.baseURL, g.repo.Owner, g.repo.Name)
}

func (g *GitHub) PathPrefix() string {
	return ""
}

func (g *GitHub) GetLatestVersionRef(ctx context.Context) (string, error) {
	release, err := g.latestRelease(ctx)
	if err == nil {
		return release.GetTagName(), nil
	}

	if !errors.Is(err, ErrNoRelease) {
		return "", err
	}

	tags, err := g.ListTags(ctx)
	if err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "", ErrNoVersionRef
	}

	return tags[0], nil
}

func (g *GitHub) ListTags(ctx context.Context) ([]string, error) {
	options := &github.ListOptions{PerPage: 100} //nolint:mnd // reasonable API page size
	tags := make([]string, 0)

	for {
		pageTags, resp, err := g.client.Repositories.ListTags(ctx, g.repo.Owner, g.repo.Name, options)
		if err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}

		for _, tag := range pageTags {
			name := strings.TrimSpace(tag.GetName())
			if name == "" {
				continue
			}

			tags = append(tags, name)
		}

		if resp.NextPage == 0 {
			break
		}

		options.Page = resp.NextPage
	}

	return tags, nil
}

func (g *GitHub) GetReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	release, resp, err := g.client.Repositories.GetReleaseByTag(ctx, g.repo.Owner, g.repo.Name, tag)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get release by tag %q: %w", tag, err)
	}

	return gitHubRelease(release), nil
}

func (g *GitHub) TagExists(ctx context.Context, tag string) (bool, error) {
	_, resp, err := g.client.Git.GetRef(ctx, g.repo.Owner, g.repo.Name, "tags/"+tag)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return false, nil
		}

		return false, fmt.Errorf("get tag ref %q: %w", tag, err)
	}

	return true, nil
}

func (g *GitHub) GetCommitsSince(ctx context.Context, ref, branch string) ([]CommitEntry, error) {
	boundaryRef := strings.TrimSpace(ref)
	resolvedBoundarySHA := boundaryRef
	branch = strings.TrimSpace(branch)

	if resolvedBoundarySHA != "" {
		resolvedSHA, err := g.resolveCommitSHA(ctx, resolvedBoundarySHA)
		if err != nil {
			return nil, fmt.Errorf("resolve ref %q: %w", boundaryRef, err)
		}

		resolvedBoundarySHA = resolvedSHA
	}

	opts := &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: 100}, //nolint:mnd // reasonable page size
	}

	if branch != "" {
		opts.SHA = branch
	} else if resolvedBoundarySHA != "" {
		opts.SHA = "HEAD"
	}

	var entries []CommitEntry

	for {
		commits, resp, err := g.client.Repositories.ListCommits(ctx, g.repo.Owner, g.repo.Name, opts)
		if err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}

		for _, c := range commits {
			sha := c.GetSHA()

			if resolvedBoundarySHA != "" && sha == resolvedBoundarySHA {
				return entries, nil
			}

			entries = append(entries, CommitEntry{
				Hash:    sha,
				Message: c.GetCommit().GetMessage(),
			})
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	if resolvedBoundarySHA != "" {
		return nil, &CommitBoundaryNotFoundError{Ref: boundaryRef, Branch: branch}
	}

	return entries, nil
}

func (g *GitHub) CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error) {
	pr, _, err := g.client.PullRequests.Create(ctx, g.repo.Owner, g.repo.Name, &github.NewPullRequest{
		Title: github.Ptr(opts.Title),
		Body:  github.Ptr(opts.Body),
		Head:  github.Ptr(opts.ReleaseBranch),
		Base:  github.Ptr(opts.BaseBranch),
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
		Title: github.Ptr(opts.Title),
		Body:  github.Ptr(opts.Body),
	})
	if err != nil {
		return fmt.Errorf("update pull request #%d: %w", number, err)
	}

	return nil
}

// FindReleasePR returns ErrNoPR if no open release PR is found.
func (g *GitHub) FindReleasePR(ctx context.Context, branch string) (*PullRequest, error) {
	prs, _, err := g.client.PullRequests.List(ctx, g.repo.Owner, g.repo.Name, &github.PullRequestListOptions{
		State: "open",
		Head:  g.repo.Owner + ":" + branch,
	})
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}

	if len(prs) == 0 {
		return nil, ErrNoPR
	}

	pr := prs[0]

	return &PullRequest{
		Number: pr.GetNumber(),
		Title:  pr.GetTitle(),
		Body:   pr.GetBody(),
		URL:    pr.GetHTMLURL(),
		Branch: branch,
	}, nil
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

	for {
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
			break
		}

		options.Page = resp.NextPage
	}

	return pendingPRs, nil
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

	for {
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
			break
		}

		options.Page = resp.NextPage
	}

	return nil, ErrNoPR
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

func (g *GitHub) CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error) {
	targetCommitish := strings.TrimSpace(opts.Ref)
	releaseRequest := &github.RepositoryRelease{
		TagName: github.Ptr(opts.TagName),
		Name:    github.Ptr(opts.Name),
		Body:    github.Ptr(opts.Body),
	}

	if targetCommitish != "" {
		releaseRequest.TargetCommitish = github.Ptr(targetCommitish)
	}

	rel, _, err := g.client.Repositories.CreateRelease(
		ctx, g.repo.Owner, g.repo.Name, releaseRequest,
	)
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	return gitHubRelease(rel), nil
}

func gitHubRelease(release *github.RepositoryRelease) *Release {
	return &Release{
		TagName: release.GetTagName(),
		Name:    release.GetName(),
		Body:    release.GetBody(),
		URL:     release.GetHTMLURL(),
	}
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

	mergeOptions := &github.PullRequestOptions{MergeMethod: mergeMethod}

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

func (g *GitHub) CreateBranch(ctx context.Context, name, base string) error {
	baseRef, _, err := g.client.Git.GetRef(ctx, g.repo.Owner, g.repo.Name, "refs/heads/"+base)
	if err != nil {
		return fmt.Errorf("get base ref %s: %w", base, err)
	}

	_, _, err = g.client.Git.CreateRef(ctx, g.repo.Owner, g.repo.Name, github.CreateRef{
		Ref: "refs/heads/" + name,
		SHA: baseRef.GetObject().GetSHA(),
	})
	if err != nil {
		// Branch might already exist, try to update it.
		if strings.Contains(err.Error(), "Reference already exists") {
			return nil
		}

		return fmt.Errorf("create branch %s: %w", name, err)
	}

	return nil
}

func (g *GitHub) GetFile(ctx context.Context, branch, path string) (string, error) {
	content, _, resp, err := g.client.Repositories.GetContents(
		ctx,
		g.repo.Owner,
		g.repo.Name,
		path,
		&github.RepositoryContentGetOptions{Ref: branch},
	)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", ErrFileNotFound
		}

		return "", fmt.Errorf("get file %s on branch %s: %w", path, branch, err)
	}

	if content == nil {
		return "", fmt.Errorf("%w: %s", ErrFileNotFound, path)
	}

	decoded, err := content.GetContent()
	if err != nil {
		return "", fmt.Errorf("decode file %s on branch %s: %w", path, branch, err)
	}

	return decoded, nil
}

func (g *GitHub) UpdateFile(ctx context.Context, branch, path, content, message string) error {
	// Try to get existing file to get its SHA.
	getOpts := &github.RepositoryContentGetOptions{Ref: branch}

	existing, _, _, err := g.client.Repositories.GetContents(
		ctx, g.repo.Owner, g.repo.Name, path, getOpts,
	)

	fileOpts := &github.RepositoryContentFileOptions{
		Message: github.Ptr(message),
		Content: []byte(content),
		Branch:  github.Ptr(branch),
	}

	if err == nil && existing != nil {
		fileOpts.SHA = github.Ptr(existing.GetSHA())
	}

	_, _, err = g.client.Repositories.CreateFile(ctx, g.repo.Owner, g.repo.Name, path, fileOpts)
	if err != nil {
		return fmt.Errorf("update file %s on branch %s: %w", path, branch, err)
	}

	return nil
}

func (g *GitHub) UpdateFiles(ctx context.Context, branch, base string, files map[string]string, message string) error {
	baseCommit, err := g.baseBranchCommit(ctx, base)
	if err != nil {
		return err
	}

	tree, err := g.createTreeForFiles(ctx, baseCommit.GetTree().GetSHA(), files)
	if err != nil {
		return fmt.Errorf("create tree for branch %s: %w", branch, err)
	}

	newCommit, err := g.createCommitFromBase(ctx, baseCommit, tree, message)
	if err != nil {
		return fmt.Errorf("create commit for branch %s: %w", branch, err)
	}

	err = g.upsertBranchRef(ctx, branch, newCommit.GetSHA())
	if err != nil {
		return err
	}

	return nil
}

func (g *GitHub) latestRelease(ctx context.Context) (*github.RepositoryRelease, error) {
	release, resp, err := g.client.Repositories.GetLatestRelease(ctx, g.repo.Owner, g.repo.Name)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get latest release: %w", err)
	}

	return release, nil
}

func (g *GitHub) resolveCommitSHA(ctx context.Context, ref string) (string, error) {
	commit, _, err := g.client.Repositories.GetCommit(ctx, g.repo.Owner, g.repo.Name, ref, nil)
	if err != nil {
		return "", fmt.Errorf("get commit for ref %q: %w", ref, err)
	}

	sha := commit.GetSHA()
	if sha == "" {
		return "", fmt.Errorf("%w: ref %q", ErrEmptyCommitSHA, ref)
	}

	return sha, nil
}

func (g *GitHub) baseBranchCommit(ctx context.Context, base string) (*github.Commit, error) {
	baseRef, _, err := g.client.Git.GetRef(ctx, g.repo.Owner, g.repo.Name, "refs/heads/"+base)
	if err != nil {
		return nil, fmt.Errorf("get base ref %s: %w", base, err)
	}

	baseSHA := baseRef.GetObject().GetSHA()

	baseCommit, _, err := g.client.Git.GetCommit(ctx, g.repo.Owner, g.repo.Name, baseSHA)
	if err != nil {
		return nil, fmt.Errorf("get base commit %s: %w", baseSHA, err)
	}

	return baseCommit, nil
}

func (g *GitHub) createTreeForFiles(
	ctx context.Context,
	baseTreeSHA string,
	files map[string]string,
) (*github.Tree, error) {
	paths := make([]string, 0, len(files))

	for path := range files {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	entries := make([]*github.TreeEntry, 0, len(paths))

	for _, path := range paths {
		pathValue := path
		mode := "100644"
		typeValue := "blob"
		contentValue := files[path]

		entries = append(entries, &github.TreeEntry{
			Path:    &pathValue,
			Mode:    &mode,
			Type:    &typeValue,
			Content: &contentValue,
		})
	}

	tree, _, err := g.client.Git.CreateTree(ctx, g.repo.Owner, g.repo.Name, baseTreeSHA, entries)
	if err != nil {
		return nil, fmt.Errorf("create tree: %w", err)
	}

	return tree, nil
}

func (g *GitHub) createCommitFromBase(
	ctx context.Context,
	baseCommit *github.Commit,
	tree *github.Tree,
	message string,
) (*github.Commit, error) {
	commitMessage := message

	newCommit, _, err := g.client.Git.CreateCommit(ctx, g.repo.Owner, g.repo.Name, github.Commit{
		Message: &commitMessage,
		Tree:    &github.Tree{SHA: tree.SHA},
		Parents: []*github.Commit{{SHA: baseCommit.SHA}},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("create commit: %w", err)
	}

	return newCommit, nil
}

func (g *GitHub) upsertBranchRef(ctx context.Context, branch, sha string) error {
	if sha == "" {
		return fmt.Errorf("%w: branch %q", ErrEmptyCommitSHA, branch)
	}

	refName := "refs/heads/" + branch
	createRef := github.CreateRef{
		Ref: refName,
		SHA: sha,
	}

	_, resp, err := g.client.Git.GetRef(ctx, g.repo.Owner, g.repo.Name, refName)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			_, _, err = g.client.Git.CreateRef(ctx, g.repo.Owner, g.repo.Name, createRef)
			if err != nil {
				return fmt.Errorf("create branch %s: %w", branch, err)
			}

			return nil
		}

		return fmt.Errorf("get ref %s: %w", branch, err)
	}

	_, _, err = g.client.Git.UpdateRef(ctx, g.repo.Owner, g.repo.Name, refName, github.UpdateRef{
		SHA:   sha,
		Force: github.Ptr(true),
	})
	if err != nil {
		return fmt.Errorf("force update branch %s: %w", branch, err)
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
		Name:        github.Ptr(name),
		Color:       github.Ptr(color),
		Description: github.Ptr(description),
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

func (g *GitHub) resolveGitHubMergeMethod(ctx context.Context, requested MergeMethod) (string, error) {
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
