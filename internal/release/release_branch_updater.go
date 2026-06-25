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
	"github.com/monkescience/yeet/internal/version"
	"github.com/monkescience/yeet/internal/versionfile"
)

type releaseBranchUpdater struct {
	core  *releaseCore
	files releaseFileProvider
}

func newReleaseBranchUpdater(core *releaseCore, files releaseFileProvider) *releaseBranchUpdater {
	return &releaseBranchUpdater{core: core, files: files}
}

func (u *releaseBranchUpdater) updateFiles(ctx context.Context, branch string, result *Result) error {
	r := u.core
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

		scheme, schemeErr := markerScheme(target)
		if schemeErr != nil {
			return fmt.Errorf("build marker scheme for target %s: %w", plan.ID, schemeErr)
		}

		for _, versionFile := range target.VersionFiles {
			content, fileErr := u.files.GetFile(ctx, r.cfg.Branch, versionFile.Path)
			if fileErr != nil {
				return fmt.Errorf("get version file %s: %w", versionFile.Path, fileErr)
			}

			updatedContent, changed, markerErr := applyVersionFile(content, plan.NextVersion, scheme, versionFile)
			if markerErr != nil {
				return fmt.Errorf("update version file %s: %w", versionFile.Path, markerErr)
			}

			if !changed {
				slog.InfoContext(ctx, "version file already at target version",
					slog.String("path", versionFile.Path),
				)

				continue
			}

			slog.DebugContext(ctx, "versionfile: rewrote",
				slog.String("path", versionFile.Path),
				slog.String("format", string(versionFile.Format)),
				slog.String("next_version", plan.NextVersion),
			)

			if err := setBranchFileContent(files, versionFile.Path, updatedContent); err != nil {
				return err
			}
		}
	}

	err := u.files.UpdateFiles(ctx, branch, r.cfg.Branch, files, r.releaseSubject(result))
	if err != nil {
		return fmt.Errorf("update release branch files: %w", err)
	}

	return nil
}

func applyVersionFile(
	content string,
	nextVersion string,
	scheme versionfile.Scheme,
	versionFile config.VersionFile,
) (string, bool, error) {
	if versionFile.Format == config.VersionFileFormatJSON {
		updated, changed, err := versionfile.ApplyJSONPointer(content, nextVersion, versionFile.JSONPointer)
		if err != nil {
			return content, false, fmt.Errorf("apply json pointer: %w", err)
		}

		return updated, changed, nil
	}

	updated, changed, err := versionfile.ApplyGenericMarkers(content, nextVersion, scheme)
	if err != nil {
		return content, false, fmt.Errorf("apply markers: %w", err)
	}

	return updated, changed, nil
}

func (u *releaseBranchUpdater) releaseChangelogFileContent(
	ctx context.Context,
	pendingFiles map[string]string,
	target config.ResolvedTarget,
	changelogEntry string,
) (string, error) {
	r := u.core

	if existing, exists := pendingFiles[target.Changelog.File]; exists {
		return prependChangelogEntry(existing, changelogEntry), nil
	}

	existing, err := u.files.GetFile(ctx, r.cfg.Branch, target.Changelog.File)
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

func markerScheme(target config.ResolvedTarget) (versionfile.Scheme, error) {
	if target.Versioning != config.VersioningCalVer {
		return versionfile.SemVerScheme(), nil
	}

	calver, err := version.NewCalVerScheme(target.CalVer.Format)
	if err != nil {
		return versionfile.Scheme{}, fmt.Errorf("compile calver format: %w", err)
	}

	return versionfile.CalVerScheme(calver), nil
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
