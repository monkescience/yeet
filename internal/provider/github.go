package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v68/github"
)

type GitHub struct {
	client *github.Client
	repo   RepoInfo
}

func NewGitHub(client *github.Client, owner, repo string) *GitHub {
	return &GitHub{
		client: client,
		repo:   RepoInfo{Owner: owner, Name: repo},
	}
}

// GetLatestRelease returns ErrNoRelease if no releases exist.
func (g *GitHub) GetLatestRelease(ctx context.Context) (*Release, error) {
	release, resp, err := g.client.Repositories.GetLatestRelease(ctx, g.repo.Owner, g.repo.Name)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrNoRelease
		}

		return nil, fmt.Errorf("get latest release: %w", err)
	}

	return &Release{
		TagName: release.GetTagName(),
		Name:    release.GetName(),
		Body:    release.GetBody(),
		URL:     release.GetHTMLURL(),
	}, nil
}

func (g *GitHub) GetCommitsSince(ctx context.Context, ref string) ([]CommitEntry, error) {
	opts := &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: 100}, //nolint:mnd // reasonable page size
	}

	if ref != "" {
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

			if ref != "" && (sha == ref || strings.HasPrefix(sha, ref)) {
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

func (g *GitHub) CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error) {
	rel, _, err := g.client.Repositories.CreateRelease(
		ctx, g.repo.Owner, g.repo.Name, &github.RepositoryRelease{
			TagName: github.Ptr(opts.TagName),
			Name:    github.Ptr(opts.Name),
			Body:    github.Ptr(opts.Body),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	return &Release{
		TagName: rel.GetTagName(),
		Name:    rel.GetName(),
		Body:    rel.GetBody(),
		URL:     rel.GetHTMLURL(),
	}, nil
}

func (g *GitHub) CreateBranch(ctx context.Context, name, base string) error {
	baseRef, _, err := g.client.Git.GetRef(ctx, g.repo.Owner, g.repo.Name, "refs/heads/"+base)
	if err != nil {
		return fmt.Errorf("get base ref %s: %w", base, err)
	}

	_, _, err = g.client.Git.CreateRef(ctx, g.repo.Owner, g.repo.Name, &github.Reference{
		Ref:    github.Ptr("refs/heads/" + name),
		Object: baseRef.Object,
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
