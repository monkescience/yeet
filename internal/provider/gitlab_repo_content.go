package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func (g *GitLab) CreateBranch(ctx context.Context, name, base string) error {
	_, _, err := g.client.Branches.CreateBranch(g.pid, &gitlab.CreateBranchOptions{
		Branch: new(name),
		Ref:    new(base),
	}, gitlab.WithContext(ctx))
	if err != nil {
		var glErr *gitlab.ErrorResponse
		if errors.As(err, &glErr) && glErr.Response != nil && glErr.Response.StatusCode == http.StatusBadRequest {
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
		Branch:        new(branch),
		CommitMessage: new(message),
		StartBranch:   new(base),
		Actions:       actions,
		Force:         &force,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("force update branch %s: %w", branch, err)
	}

	return nil
}
