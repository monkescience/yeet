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

// NewGitLab creates a provider. pid is the project ID or full path (e.g., "owner/repo").
func NewGitLab(client *gitlab.Client, owner, repo string) *GitLab {
	baseURL := strings.TrimSuffix(client.BaseURL().String(), "/api/v4/")

	return &GitLab{
		client:  client,
		pid:     owner + "/" + repo,
		baseURL: baseURL + "/" + owner + "/" + repo,
	}
}

func (g *GitLab) RepoURL() string {
	return g.baseURL
}

func (g *GitLab) PathPrefix() string {
	return "/-"
}

func (g *GitLab) GetLatestRelease(ctx context.Context) (*Release, error) {
	releases, _, err := g.client.Releases.ListReleases(g.pid, &gitlab.ListReleasesOptions{
		ListOptions: gitlab.ListOptions{PerPage: 1},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	if len(releases) == 0 {
		return nil, ErrNoRelease
	}

	r := releases[0]

	return &Release{
		TagName: r.TagName,
		Name:    r.Name,
		Body:    r.Description,
		URL:     r.Links.Self,
	}, nil
}

func (g *GitLab) GetCommitsSince(ctx context.Context, ref string) ([]CommitEntry, error) {
	stopID := strings.TrimSpace(ref)

	if stopID != "" {
		resolvedID, err := g.resolveCommitID(ctx, stopID)
		if err != nil {
			return nil, fmt.Errorf("resolve ref %q: %w", stopID, err)
		}

		stopID = resolvedID
	}

	opts := &gitlab.ListCommitsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100}, //nolint:mnd // reasonable page size
	}

	var entries []CommitEntry

	for {
		commits, resp, err := g.client.Commits.ListCommits(g.pid, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}

		for _, c := range commits {
			if stopID != "" && (c.ID == stopID || strings.HasPrefix(c.ID, stopID)) {
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

func (g *GitLab) FindReleasePR(ctx context.Context, branch string) (*PullRequest, error) {
	state := "opened"

	mrs, _, err := g.client.MergeRequests.ListProjectMergeRequests(g.pid, &gitlab.ListProjectMergeRequestsOptions{
		State:        gitlab.Ptr(state),
		SourceBranch: gitlab.Ptr(branch),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list merge requests: %w", err)
	}

	if len(mrs) == 0 {
		return nil, ErrNoPR
	}

	mr := mrs[0]

	return &PullRequest{
		Number: int(mr.IID),
		Title:  mr.Title,
		Body:   mr.Description,
		URL:    mr.WebURL,
		Branch: branch,
	}, nil
}

func (g *GitLab) FindOpenPendingReleasePRs(ctx context.Context, baseBranch string) ([]*PullRequest, error) {
	state := "opened"
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
	state := "merged"
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
				Number: int(mr.IID),
				Title:  mr.Title,
				Body:   mr.Description,
				URL:    mr.WebURL,
				Branch: mr.SourceBranch,
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
	release, _, err := g.client.Releases.CreateRelease(g.pid, &gitlab.CreateReleaseOptions{
		TagName:     gitlab.Ptr(opts.TagName),
		Name:        gitlab.Ptr(opts.Name),
		Description: gitlab.Ptr(opts.Body),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	return &Release{
		TagName: release.TagName,
		Name:    release.Name,
		Body:    release.Description,
		URL:     release.Links.Self,
	}, nil
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

func (g *GitLab) UpdateFile(ctx context.Context, branch, path, content, message string) error {
	// Try to update first, fall back to create.
	_, _, err := g.client.RepositoryFiles.UpdateFile(g.pid, path, &gitlab.UpdateFileOptions{
		Branch:        gitlab.Ptr(branch),
		Content:       gitlab.Ptr(content),
		CommitMessage: gitlab.Ptr(message),
	}, gitlab.WithContext(ctx))
	if err == nil {
		return nil
	}

	// File doesn't exist, create it.
	_, _, err = g.client.RepositoryFiles.CreateFile(g.pid, path, &gitlab.CreateFileOptions{
		Branch:        gitlab.Ptr(branch),
		Content:       gitlab.Ptr(content),
		CommitMessage: gitlab.Ptr(message),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create file %s on branch %s: %w", path, branch, err)
	}

	return nil
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
