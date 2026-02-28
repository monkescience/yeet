package provider

import (
	"context"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLab struct {
	client  *gitlab.Client
	pid     string
	baseURL string
}

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
			if ref != "" && (c.ID == ref || strings.HasPrefix(c.ID, ref)) {
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
