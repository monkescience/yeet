package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/version"
	"github.com/monkescience/yeet/internal/versionfile"
)

type releaseBranchUpdater struct {
	core   *releaseCore
	source releaseSource
	files  releaseFileProvider
}

func newReleaseBranchUpdater(
	core *releaseCore,
	source releaseSource,
	files releaseFileProvider,
) *releaseBranchUpdater {
	return &releaseBranchUpdater{core: core, source: source, files: files}
}

func (u *releaseBranchUpdater) updateFiles(
	ctx context.Context,
	branch string,
	plans []TargetPlan,
	commitSubject string,
) error {
	r := u.core
	files := map[string]forge.FileUpdate{}
	changelogFiles := map[string]struct{}{}

	for _, plan := range plans {
		target, exists := r.targets[plan.ID]
		if !exists {
			return fmt.Errorf("%w: %s", errUnknownTarget, plan.ID)
		}

		changelogContent, err := u.releaseChangelogFileContent(
			ctx,
			files,
			changelogFiles,
			target,
			changelog.Render(plan.Entry),
		)
		if err != nil {
			return err
		}

		files[target.Changelog.File] = changelogContent
		changelogFiles[target.Changelog.File] = struct{}{}

		err = u.updateVersionFiles(ctx, files, target, plan.ID, plan.NextVersion)
		if err != nil {
			return err
		}
	}

	err := u.files.UpdateFiles(ctx, branch, r.cfg.Branch, files, commitSubject)
	if err != nil {
		return fmt.Errorf("update release branch files: %w", err)
	}

	return nil
}

func (u *releaseBranchUpdater) updateVersionFiles(
	ctx context.Context,
	files map[string]forge.FileUpdate,
	target config.ResolvedTarget,
	targetID string,
	nextVersion string,
) error {
	scheme, err := markerScheme(target)
	if err != nil {
		return fmt.Errorf("build marker scheme for target %s: %w", targetID, err)
	}

	for _, versionFile := range target.VersionFiles {
		content, fileErr := u.source.GetFile(ctx, versionFile.Path)
		if fileErr != nil {
			return fmt.Errorf("get version file %s: %w", versionFile.Path, fileErr)
		}

		updatedContent, changed, markerErr := applyVersionFile(content, nextVersion, scheme, versionFile)
		if markerErr != nil {
			return fmt.Errorf("update version file %s: %w", versionFile.Path, markerErr)
		}

		if !changed {
			slog.InfoContext(ctx, "version file already at target version", slog.String("path", versionFile.Path))

			continue
		}

		slog.DebugContext(ctx, "versionfile: rewrote",
			slog.String("path", versionFile.Path),
			slog.String("format", string(versionFile.Format)),
			slog.String("next_version", nextVersion),
		)

		setErr := setBranchFileContent(files, versionFile.Path, forge.FileUpdate{
			Content: updatedContent,
			Exists:  true,
		})
		if setErr != nil {
			return setErr
		}
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
	pendingFiles map[string]forge.FileUpdate,
	changelogFiles map[string]struct{},
	target config.ResolvedTarget,
	changelogEntry string,
) (forge.FileUpdate, error) {
	if existing, exists := pendingFiles[target.Changelog.File]; exists {
		if _, isChangelog := changelogFiles[target.Changelog.File]; !isChangelog {
			return forge.FileUpdate{}, fmt.Errorf("%w: %s", errConflictingFileUpdate, target.Changelog.File)
		}

		existing.Content = changelog.PrependEntry(existing.Content, changelogEntry)

		return existing, nil
	}

	existing, err := u.source.GetFile(ctx, target.Changelog.File)
	if err != nil {
		if errors.Is(err, forge.ErrFileNotFound) {
			return forge.FileUpdate{Content: changelog.Prepend("", changelogEntry)}, nil
		}

		return forge.FileUpdate{}, fmt.Errorf("get changelog file %s: %w", target.Changelog.File, err)
	}

	return forge.FileUpdate{Content: changelog.PrependEntry(existing, changelogEntry), Exists: true}, nil
}

func setBranchFileContent(files map[string]forge.FileUpdate, path string, update forge.FileUpdate) error {
	if existingUpdate, exists := files[path]; exists && existingUpdate != update {
		return fmt.Errorf("%w: %s", errConflictingFileUpdate, path)
	}

	files[path] = update

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
