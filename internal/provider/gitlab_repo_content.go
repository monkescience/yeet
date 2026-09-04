package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/monkescience/yeet/internal/forge"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func (g *GitLab) GetFile(ctx context.Context, branch, path string) (string, error) {
	ref := branch

	slog.DebugContext(ctx, "gitlab: reading file",
		slog.String("path", path),
		slog.String("ref", ref),
	)

	raw, _, err := g.client.RepositoryFiles.GetRawFile(
		g.projectID,
		path,
		&gitlab.GetRawFileOptions{Ref: &ref},
		gitlab.WithContext(ctx),
	)
	if err != nil {
		if errors.Is(err, gitlab.ErrNotFound) {
			slog.DebugContext(ctx, "gitlab: file not found",
				slog.String("path", path),
				slog.String("ref", ref),
			)

			return "", forge.ErrFileNotFound
		}

		return "", fmt.Errorf("get file %s on branch %s: %w", path, branch, err)
	}

	return string(raw), nil
}

func (g *GitLab) UpdateFiles(
	ctx context.Context,
	branch, base string,
	files map[string]forge.FileUpdate,
	message string,
) error {
	paths := make([]string, 0, len(files))

	for path := range files {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	actions := make([]*gitlab.CommitActionOptions, 0, len(paths))

	for _, path := range paths {
		update := files[path]

		action := gitlab.FileUpdate
		if !update.Exists {
			action = gitlab.FileCreate
		}

		actions = append(actions, &gitlab.CommitActionOptions{
			Action:   new(action),
			FilePath: new(path),
			Content:  new(update.Content),
		})
	}

	slog.DebugContext(ctx, "gitlab: updating files",
		slog.String("branch", branch),
		slog.String("base", base),
		slog.Int("files", len(actions)),
	)

	_, _, err := g.client.Commits.CreateCommit(g.projectID, &gitlab.CreateCommitOptions{
		Branch:        new(branch),
		CommitMessage: new(message),
		StartBranch:   new(base),
		Actions:       actions,
		Force:         new(true),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("force update branch %s: %w", branch, err)
	}

	slog.DebugContext(ctx, "gitlab: updated files", slog.String("branch", branch))

	return nil
}
