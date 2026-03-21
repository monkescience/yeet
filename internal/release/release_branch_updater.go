package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/changelog"
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

	changelogContent, err := u.releaseChangelogFileContent(ctx, result.Changelog)
	if err != nil {
		return err
	}

	files := map[string]string{
		r.cfg.Changelog.File: changelogContent,
	}

	for _, path := range r.cfg.VersionFiles {
		content, fileErr := r.files.GetFile(ctx, r.cfg.Branch, path)
		if fileErr != nil {
			return fmt.Errorf("get version file %s: %w", path, fileErr)
		}

		updatedContent, changed := versionfile.ApplyGenericMarkers(content, result.NextVersion)
		if !changed {
			slog.InfoContext(ctx, "skipping version file without yeet markers", "path", path)

			continue
		}

		files[path] = updatedContent
	}

	err = r.files.UpdateFiles(ctx, branch, r.cfg.Branch, files, r.releaseSubject(result))
	if err != nil {
		return fmt.Errorf("update release branch files: %w", err)
	}

	return nil
}

func (u *releaseBranchUpdater) releaseChangelogFileContent(ctx context.Context, changelogEntry string) (string, error) {
	r := u.releaser

	existing, err := r.files.GetFile(ctx, r.cfg.Branch, r.cfg.Changelog.File)
	if err != nil {
		if errors.Is(err, provider.ErrFileNotFound) {
			return changelog.Prepend("", changelogEntry), nil
		}

		return "", fmt.Errorf("get changelog file %s: %w", r.cfg.Changelog.File, err)
	}

	return prependChangelogEntry(existing, changelogEntry), nil
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
