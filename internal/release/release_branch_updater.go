package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/versionfile"
)

type releaseBranchUpdater struct {
	releaser *Releaser
}

func newReleaseBranchUpdater(releaser *Releaser) *releaseBranchUpdater {
	return &releaseBranchUpdater{releaser: releaser}
}

func (u *releaseBranchUpdater) updateFiles(ctx context.Context, branch string, result *Result) error {
	r := u.releaser
	files := map[string]string{}

	for _, plan := range result.Plans {
		target, exists := r.targets[plan.ID]
		if !exists {
			return fmt.Errorf("%w: %s", ErrUnknownTarget, plan.ID)
		}

		changelogContent, err := u.releaseChangelogFileContent(ctx, files, target, plan.Changelog)
		if err != nil {
			return err
		}

		files[target.Changelog.File] = changelogContent

		for _, path := range target.VersionFiles {
			content, fileErr := r.files.GetFile(ctx, r.cfg.Branch, path)
			if fileErr != nil {
				return fmt.Errorf("get version file %s: %w", path, fileErr)
			}

			updatedContent, changed, markerErr := versionfile.ApplyGenericMarkers(content, plan.NextVersion)
			if markerErr != nil {
				return fmt.Errorf("update version file %s: %w", path, markerErr)
			}

			if !changed {
				slog.InfoContext(ctx, "version file already at target version", "path", path)

				continue
			}

			err = setBranchFileContent(files, path, updatedContent)
			if err != nil {
				return err
			}
		}
	}

	err := r.files.UpdateFiles(ctx, branch, r.cfg.Branch, files, r.releaseSubject(result))
	if err != nil {
		return fmt.Errorf("update release branch files: %w", err)
	}

	return nil
}

func (u *releaseBranchUpdater) releaseChangelogFileContent(
	ctx context.Context,
	pendingFiles map[string]string,
	target config.ResolvedTarget,
	changelogEntry string,
) (string, error) {
	r := u.releaser

	if existing, exists := pendingFiles[target.Changelog.File]; exists {
		return prependChangelogEntry(existing, changelogEntry), nil
	}

	existing, err := r.files.GetFile(ctx, r.cfg.Branch, target.Changelog.File)
	if err != nil {
		if errors.Is(err, provider.ErrFileNotFound) {
			return changelog.Prepend("", changelogEntry), nil
		}

		return "", fmt.Errorf("get changelog file %s: %w", target.Changelog.File, err)
	}

	return prependChangelogEntry(existing, changelogEntry), nil
}

func setBranchFileContent(files map[string]string, path, content string) error {
	if existingContent, exists := files[path]; exists && existingContent != content {
		return fmt.Errorf("%w: %s", ErrConflictingFileUpdate, path)
	}

	files[path] = content

	return nil
}

func prependChangelogEntry(existing, changelogEntry string) string {
	if strings.TrimSpace(existing) == "" {
		return changelog.Prepend("", changelogEntry)
	}

	if strings.HasPrefix(existing, "# ") {
		return changelog.Prepend(existing, changelogEntry)
	}

	combined := strings.TrimRight(changelogEntry, "\n") + "\n\n" + strings.TrimLeft(existing, "\n")

	return changelog.Prepend("", combined)
}
