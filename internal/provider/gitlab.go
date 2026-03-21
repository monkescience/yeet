package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLab struct {
	client  *gitlab.Client
	pid     string
	baseURL string
}

const gitlabReleasePendingLabelColor = "#FBCA04"

const gitlabReleaseTaggedLabelColor = "#0E8A16"

const gitlabMergeRequestOpenedState = "opened"

const gitlabMergeRequestMergedState = "merged"

// NewGitLab creates a provider. pid is the project ID or full path (e.g., "owner/repo").
func NewGitLab(client *gitlab.Client, project string) *GitLab {
	baseURL := strings.TrimSuffix(client.BaseURL().String(), "/api/v4/")

	return &GitLab{
		client:  client,
		pid:     project,
		baseURL: baseURL + "/" + project,
	}
}

func (g *GitLab) RepoURL() string {
	return g.baseURL
}

func (g *GitLab) PathPrefix() string {
	return "/-"
}

func (g *GitLab) GetLatestVersionRef(ctx context.Context) (string, error) {
	release, err := g.latestRelease(ctx)
	if err == nil {
		return release.TagName, nil
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

func (g *GitLab) ListTags(ctx context.Context) ([]string, error) {
	options := &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}
	tags := make([]string, 0)

	for {
		pageTags, resp, err := g.client.Tags.ListTags(g.pid, options, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}

		for _, tag := range pageTags {
			name := strings.TrimSpace(tag.Name)
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

func (g *GitLab) GetReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	release, _, err := g.client.Releases.GetRelease(g.pid, tag, gitlab.WithContext(ctx))
	if err != nil {
		if errors.Is(err, gitlab.ErrNotFound) {
			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get release by tag %q: %w", tag, err)
	}

	return gitLabRelease(release), nil
}

func (g *GitLab) TagExists(ctx context.Context, tag string) (bool, error) {
	_, _, err := g.client.Tags.GetTag(g.pid, tag, gitlab.WithContext(ctx))
	if err != nil {
		if errors.Is(err, gitlab.ErrNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("get tag %q: %w", tag, err)
	}

	return true, nil
}

func (g *GitLab) GetCommitsSince(ctx context.Context, ref, branch string) ([]CommitEntry, error) {
	boundaryRef := strings.TrimSpace(ref)
	resolvedBoundaryID := boundaryRef
	branch = strings.TrimSpace(branch)

	if resolvedBoundaryID != "" {
		resolvedID, err := g.resolveCommitID(ctx, resolvedBoundaryID)
		if err != nil {
			return nil, fmt.Errorf("resolve ref %q: %w", boundaryRef, err)
		}

		resolvedBoundaryID = resolvedID
	}

	opts := &gitlab.ListCommitsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable page size
	}

	if branch != "" {
		opts.RefName = gitlab.Ptr(branch)
	}

	var entries []CommitEntry

	for {
		commits, resp, err := g.client.Commits.ListCommits(g.pid, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}

		for _, c := range commits {
			if resolvedBoundaryID != "" && c.ID == resolvedBoundaryID {
				return entries, nil
			}

			entries = append(entries, CommitEntry{
				Hash:    c.ID,
				Message: c.Message,
			})
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	if resolvedBoundaryID != "" {
		return nil, &CommitBoundaryNotFoundError{Ref: boundaryRef, Branch: branch}
	}

	return entries, nil
}

func (g *GitLab) CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error) {
	mr, _, err := g.client.MergeRequests.CreateMergeRequest(g.pid, &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.Ptr(opts.Title),
		Description:  gitlab.Ptr(opts.Body),
		SourceBranch: gitlab.Ptr(opts.ReleaseBranch),
		TargetBranch: gitlab.Ptr(opts.BaseBranch),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create merge request: %w", err)
	}

	return &PullRequest{
		Number: int(mr.IID),
		Title:  mr.Title,
		Body:   mr.Description,
		URL:    mr.WebURL,
		Branch: opts.ReleaseBranch,
	}, nil
}

func (g *GitLab) UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error {
	_, _, err := g.client.MergeRequests.UpdateMergeRequest(g.pid, int64(number), &gitlab.UpdateMergeRequestOptions{
		Title:       gitlab.Ptr(opts.Title),
		Description: gitlab.Ptr(opts.Body),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("update merge request !%d: %w", number, err)
	}

	return nil
}

func (g *GitLab) FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error) {
	state := gitlabMergeRequestOpenedState
	orderBy := "updated_at"
	sortDirection := "desc"
	labels := gitlab.LabelOptions{ReleaseLabelPending}

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        gitlab.Ptr(state),
		TargetBranch: gitlab.Ptr(baseBranch),
		OrderBy:      gitlab.Ptr(orderBy),
		Sort:         gitlab.Ptr(sortDirection),
		Labels:       &labels,
		ListOptions:  gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}

	pendingMRs := make([]*PullRequest, 0)

	for {
		mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(g.pid, options, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list merge requests: %w", err)
		}

		for _, mr := range mrs {
			if !strings.HasPrefix(mr.SourceBranch, releaseBranchPrefix) {
				continue
			}

			pendingMRs = append(pendingMRs, &PullRequest{
				Number: int(mr.IID),
				Title:  mr.Title,
				Body:   mr.Description,
				URL:    mr.WebURL,
				Branch: mr.SourceBranch,
			})
		}

		if resp.NextPage == 0 {
			break
		}

		options.Page = resp.NextPage
	}

	return pendingMRs, nil
}

func (g *GitLab) FindMergedReleasePR(ctx context.Context, baseBranch string) (*PullRequest, error) {
	state := gitlabMergeRequestMergedState
	orderBy := "updated_at"
	sortDirection := "desc"
	labels := gitlab.LabelOptions{ReleaseLabelPending}

	options := &gitlab.ListProjectMergeRequestsOptions{
		State:        gitlab.Ptr(state),
		TargetBranch: gitlab.Ptr(baseBranch),
		OrderBy:      gitlab.Ptr(orderBy),
		Sort:         gitlab.Ptr(sortDirection),
		Labels:       &labels,
		ListOptions:  gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable API page size
	}

	for {
		mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(g.pid, options, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list merge requests: %w", err)
		}

		for _, mr := range mrs {
			if !strings.HasPrefix(mr.SourceBranch, releaseBranchPrefix) {
				continue
			}

			return &PullRequest{
				Number:         int(mr.IID),
				Title:          mr.Title,
				Body:           mr.Description,
				URL:            mr.WebURL,
				Branch:         mr.SourceBranch,
				MergeCommitSHA: gitLabMergeCommitSHA(mr),
			}, nil
		}

		if resp.NextPage == 0 {
			break
		}

		options.Page = resp.NextPage
	}

	return nil, ErrNoPR
}

func (g *GitLab) MarkReleasePRPending(ctx context.Context, number int) error {
	err := g.ensureReleaseLabels(ctx)
	if err != nil {
		return err
	}

	addLabels := gitlab.LabelOptions{ReleaseLabelPending}
	removeLabels := gitlab.LabelOptions{ReleaseLabelTagged}

	_, _, err = g.client.MergeRequests.UpdateMergeRequest(g.pid, int64(number), &gitlab.UpdateMergeRequestOptions{
		AddLabels:    &addLabels,
		RemoveLabels: &removeLabels,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("mark merge request !%d pending: %w", number, err)
	}

	return nil
}

func (g *GitLab) MarkReleasePRTagged(ctx context.Context, number int) error {
	err := g.ensureReleaseLabels(ctx)
	if err != nil {
		return err
	}

	addLabels := gitlab.LabelOptions{ReleaseLabelTagged}
	removeLabels := gitlab.LabelOptions{ReleaseLabelPending}

	_, _, err = g.client.MergeRequests.UpdateMergeRequest(g.pid, int64(number), &gitlab.UpdateMergeRequestOptions{
		AddLabels:    &addLabels,
		RemoveLabels: &removeLabels,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("mark merge request !%d tagged: %w", number, err)
	}

	return nil
}

func (g *GitLab) CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error) {
	ref := strings.TrimSpace(opts.Ref)
	releaseOptions := &gitlab.CreateReleaseOptions{
		TagName:     gitlab.Ptr(opts.TagName),
		Name:        gitlab.Ptr(opts.Name),
		Description: gitlab.Ptr(opts.Body),
	}

	if ref != "" {
		releaseOptions.Ref = gitlab.Ptr(ref)
	}

	release, _, err := g.client.Releases.CreateRelease(g.pid, releaseOptions, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	return gitLabRelease(release), nil
}

func gitLabRelease(release *gitlab.Release) *Release {
	return &Release{
		TagName: release.TagName,
		Name:    release.Name,
		Body:    release.Description,
		URL:     release.Links.Self,
	}
}

func gitLabMergeCommitSHA(mergeRequest *gitlab.BasicMergeRequest) string {
	mergeCommitSHA := strings.TrimSpace(mergeRequest.MergeCommitSHA)
	if mergeCommitSHA != "" {
		return mergeCommitSHA
	}

	return strings.TrimSpace(mergeRequest.SquashCommitSHA)
}

func (g *GitLab) MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) error {
	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.pid, int64(number), nil, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("get merge request !%d: %w", number, err)
	}

	if mr.State == gitlabMergeRequestMergedState {
		return nil
	}

	if mr.State != gitlabMergeRequestOpenedState {
		return fmt.Errorf("%w: merge request !%d is %s", ErrMergeBlocked, number, mr.State)
	}

	if mr.Draft {
		return fmt.Errorf("%w: merge request !%d is draft", ErrMergeBlocked, number)
	}

	if mr.HasConflicts {
		return fmt.Errorf("%w: merge request !%d has conflicts", ErrMergeBlocked, number)
	}

	if !opts.Force {
		mergeStatus := strings.TrimSpace(mr.DetailedMergeStatus)
		if !isGitLabMergeStatusMergeable(mergeStatus) {
			return fmt.Errorf("%w: merge request !%d detailed_merge_status=%s", ErrMergeBlocked, number, mergeStatus)
		}
	}

	project, err := g.projectMergeSettings(ctx)
	if err != nil {
		return err
	}

	acceptOptions, err := gitLabAcceptMergeOptions(project, opts.Method)
	if err != nil {
		return err
	}

	sha := strings.TrimSpace(mr.SHA)
	if sha != "" {
		acceptOptions.SHA = gitlab.Ptr(sha)
	}

	_, _, err = g.client.MergeRequests.AcceptMergeRequest(g.pid, int64(number), acceptOptions, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("accept merge request !%d: %w", number, err)
	}

	return nil
}

func (g *GitLab) CreateBranch(ctx context.Context, name, base string) error {
	_, _, err := g.client.Branches.CreateBranch(g.pid, &gitlab.CreateBranchOptions{
		Branch: gitlab.Ptr(name),
		Ref:    gitlab.Ptr(base),
	}, gitlab.WithContext(ctx))
	if err != nil {
		// Branch might already exist.
		if strings.Contains(err.Error(), "Branch already exists") {
			return nil
		}

		return fmt.Errorf("create branch %s: %w", name, err)
	}

	return nil
}

func (g *GitLab) GetFile(ctx context.Context, branch, path string) (string, error) {
	ref := branch

	raw, _, err := g.client.RepositoryFiles.GetRawFile(
		g.pid,
		path,
		&gitlab.GetRawFileOptions{Ref: &ref},
		gitlab.WithContext(ctx),
	)
	if err != nil {
		if errors.Is(err, gitlab.ErrNotFound) {
			return "", ErrFileNotFound
		}

		return "", fmt.Errorf("get file %s on branch %s: %w", path, branch, err)
	}

	return string(raw), nil
}

func (g *GitLab) UpdateFiles(ctx context.Context, branch, base string, files map[string]string, message string) error {
	paths := make([]string, 0, len(files))

	for path := range files {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	actions := make([]*gitlab.CommitActionOptions, 0, len(paths))

	for _, path := range paths {
		action := gitlab.FileUpdate

		_, err := g.GetFile(ctx, base, path)
		if err != nil {
			if errors.Is(err, ErrFileNotFound) {
				action = gitlab.FileCreate
			} else {
				return fmt.Errorf("get file %s on branch %s: %w", path, base, err)
			}
		}

		pathValue := path
		contentValue := files[path]
		actionValue := action

		actions = append(actions, &gitlab.CommitActionOptions{
			Action:   &actionValue,
			FilePath: &pathValue,
			Content:  &contentValue,
		})
	}

	force := true

	_, _, err := g.client.Commits.CreateCommit(g.pid, &gitlab.CreateCommitOptions{
		Branch:        gitlab.Ptr(branch),
		CommitMessage: gitlab.Ptr(message),
		StartBranch:   gitlab.Ptr(base),
		Actions:       actions,
		Force:         &force,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("force update branch %s: %w", branch, err)
	}

	return nil
}

func (g *GitLab) latestRelease(ctx context.Context) (*gitlab.Release, error) {
	releases, _, err := g.client.Releases.ListReleases(g.pid, &gitlab.ListReleasesOptions{
		ListOptions: gitlab.ListOptions{PerPage: 1},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	if len(releases) == 0 {
		return nil, ErrNoRelease
	}

	return releases[0], nil
}

func (g *GitLab) resolveCommitID(ctx context.Context, ref string) (string, error) {
	commit, _, err := g.client.Commits.GetCommit(g.pid, ref, nil, gitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("get commit for ref %q: %w", ref, err)
	}

	if commit.ID == "" {
		return "", fmt.Errorf("%w: ref %q", ErrEmptyCommitID, ref)
	}

	return commit.ID, nil
}

func (g *GitLab) ensureReleaseLabels(ctx context.Context) error {
	err := g.ensureLabel(ctx, ReleaseLabelPending, gitlabReleasePendingLabelColor, "release PR is pending tagging")
	if err != nil {
		return err
	}

	err = g.ensureLabel(ctx, ReleaseLabelTagged, gitlabReleaseTaggedLabelColor, "release PR already tagged")
	if err != nil {
		return err
	}

	return nil
}

func (g *GitLab) ensureLabel(ctx context.Context, name, color, description string) error {
	_, _, err := g.client.Labels.GetLabel(g.pid, name, gitlab.WithContext(ctx))
	if err == nil {
		return nil
	}

	if !errors.Is(err, gitlab.ErrNotFound) {
		return fmt.Errorf("get label %q: %w", name, err)
	}

	_, _, err = g.client.Labels.CreateLabel(g.pid, &gitlab.CreateLabelOptions{
		Name:        gitlab.Ptr(name),
		Color:       gitlab.Ptr(color),
		Description: gitlab.Ptr(description),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create label %q: %w", name, err)
	}

	return nil
}

func isGitLabMergeStatusMergeable(status string) bool {
	switch status {
	case "", "mergeable", "can_be_merged":
		return true
	default:
		return false
	}
}

func (g *GitLab) projectMergeSettings(ctx context.Context) (*gitlab.Project, error) {
	project, _, err := g.client.Projects.GetProject(g.pid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get project merge settings: %w", err)
	}

	if project == nil {
		return nil, fmt.Errorf("%w: missing project merge settings", ErrMergeBlocked)
	}

	return project, nil
}

func gitLabAcceptMergeOptions(
	project *gitlab.Project,
	requested MergeMethod,
) (*gitlab.AcceptMergeRequestOptions, error) {
	if requested == "" {
		requested = MergeMethodAuto
	}

	options := &gitlab.AcceptMergeRequestOptions{}

	switch requested {
	case MergeMethodAuto:
		return options, nil
	case MergeMethodSquash:
		if project.SquashOption == gitlab.SquashOptionNever {
			return nil, fmt.Errorf(
				"%w: merge method %q disabled by project squash_option=%s",
				ErrMergeBlocked,
				requested,
				project.SquashOption,
			)
		}

		options.Squash = gitlab.Ptr(true)

		return options, nil
	case MergeMethodRebase:
		if project.MergeMethod != gitlab.RebaseMerge {
			return nil, fmt.Errorf(
				"%w: merge method %q incompatible with project merge_method=%s",
				ErrMergeBlocked,
				requested,
				project.MergeMethod,
			)
		}

		return options, nil
	case MergeMethodMerge:
		if project.MergeMethod != gitlab.NoFastForwardMerge {
			return nil, fmt.Errorf(
				"%w: merge method %q incompatible with project merge_method=%s",
				ErrMergeBlocked,
				requested,
				project.MergeMethod,
			)
		}

		return options, nil
	default:
		return nil, fmt.Errorf("%w: unknown merge method %q", ErrMergeMethodUnsupported, requested)
	}
}
