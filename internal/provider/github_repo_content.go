package provider

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/go-github/v84/github"
)

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
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusUnprocessableEntity {
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
	entries := make([]*github.TreeEntry, 0, len(files))

	for path, content := range files {
		pathValue := path
		mode := "100644"
		typeValue := "blob"
		contentValue := content

		entries = append(entries, &github.TreeEntry{
			Path:    &pathValue,
			Mode:    &mode,
			Type:    &typeValue,
			Content: &contentValue,
		})
	}

	slices.SortFunc(entries, func(a, b *github.TreeEntry) int {
		return cmp.Compare(a.GetPath(), b.GetPath())
	})

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
		Force: new(true),
	})
	if err != nil {
		return fmt.Errorf("force update branch %s: %w", branch, err)
	}

	return nil
}
