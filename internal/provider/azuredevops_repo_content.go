package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/monkescience/yeet/internal/forge"
)

func (a *AzureDevOps) createBranchAtSHA(ctx context.Context, name, baseSHA string) error {
	gitClient, err := a.client(ctx)
	if err != nil {
		return err
	}

	refUpdates := []git.GitRefUpdate{{
		Name:        new("refs/heads/" + name),
		OldObjectId: new(azureDevOpsZeroObjectID),
		NewObjectId: &baseSHA,
	}}

	results, err := gitClient.UpdateRefs(ctx, git.UpdateRefsArgs{
		RefUpdates:   &refUpdates,
		RepositoryId: &a.repo,
		Project:      &a.project,
	})
	if err != nil {
		if isAzureDevOpsBranchAlreadyExists(err) {
			slog.DebugContext(ctx, "azure devops: branch already exists",
				slog.String("branch", name),
				slog.Int("status", http.StatusConflict),
			)

			return nil
		}

		return fmt.Errorf("create branch %q: %w", name, err)
	}

	err = validateAzureDevOpsRefUpdateResults(name, results)
	if err != nil {
		return fmt.Errorf("create branch %q: %w", name, err)
	}

	slog.DebugContext(ctx, "azure devops: created branch",
		slog.String("branch", name),
		slog.String("base_sha", baseSHA),
	)

	return nil
}

func validateAzureDevOpsRefUpdateResults(branch string, results *[]git.GitRefUpdateResult) error {
	if results == nil || len(*results) == 0 {
		return fmt.Errorf("%w: missing result", errAzureDevOpsRefUpdateFailed)
	}

	for _, result := range *results {
		if result.Success != nil && *result.Success {
			return nil
		}

		if result.UpdateStatus != nil && *result.UpdateStatus == git.GitRefUpdateStatusValues.Succeeded {
			return nil
		}
	}

	result := (*results)[0]
	if result.CustomMessage != nil && *result.CustomMessage != "" {
		return fmt.Errorf("%w: %s", errAzureDevOpsRefUpdateFailed, *result.CustomMessage)
	}

	if result.UpdateStatus != nil && *result.UpdateStatus != "" {
		return fmt.Errorf("%w: %s", errAzureDevOpsRefUpdateFailed, *result.UpdateStatus)
	}

	return fmt.Errorf("%w for %q", errAzureDevOpsRefUpdateFailed, branch)
}

func (a *AzureDevOps) GetFile(ctx context.Context, branch, path string) (string, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return "", err
	}

	slog.DebugContext(ctx, "azure devops: reading file",
		slog.String("path", path),
		slog.String("ref", branch),
	)

	branchType := git.GitVersionTypeValues.Branch
	includeContent := true

	body, err := gitClient.GetItemText(ctx, git.GetItemTextArgs{
		RepositoryId: &a.repo,
		Project:      &a.project,
		Path:         &path,
		VersionDescriptor: &git.GitVersionDescriptor{
			Version:     &branch,
			VersionType: &branchType,
		},
		IncludeContent: &includeContent,
	})
	if err != nil {
		if isAzureDevOpsNotFound(err) {
			slog.DebugContext(ctx, "azure devops: file not found",
				slog.String("path", path),
				slog.String("ref", branch),
			)

			return "", forge.ErrFileNotFound
		}

		return "", fmt.Errorf("get file %q on branch %q: %w", path, branch, err)
	}

	defer func() {
		_, _ = io.Copy(io.Discard, body)
		_ = body.Close()
	}()

	return readAzureDevOpsFileBody(body, path, branch)
}

const azureDevOpsMaxFileBytes = 10 << 20

var errAzureDevOpsFileTooLarge = errors.New("file exceeds size limit")

// readAzureDevOpsFileBody bounds the response read because GetItemText streams
// the raw body without a size limit, unlike the GitHub and GitLab SDKs.
func readAzureDevOpsFileBody(body io.Reader, path, branch string) (string, error) {
	contents, err := io.ReadAll(io.LimitReader(body, azureDevOpsMaxFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("read file %q on branch %q: %w", path, branch, err)
	}

	if len(contents) > azureDevOpsMaxFileBytes {
		return "", fmt.Errorf(
			"%w: file %q on branch %q is larger than %d bytes",
			errAzureDevOpsFileTooLarge, path, branch, azureDevOpsMaxFileBytes,
		)
	}

	return string(contents), nil
}

func (a *AzureDevOps) UpdateFiles(
	ctx context.Context,
	branch, base string,
	files map[string]forge.FileUpdate,
	message string,
) error {
	slog.DebugContext(ctx, "azure devops: updating files",
		slog.String("branch", branch),
		slog.String("base", base),
		slog.Int("files", len(files)),
	)

	branchTip, err := a.resetBranchToBase(ctx, branch, base)
	if err != nil {
		return err
	}

	changes := a.buildPushChanges(files)

	gitClient, err := a.client(ctx)
	if err != nil {
		return err
	}

	branchRef := "refs/heads/" + branch
	commitMessage := message

	push := git.GitPush{
		RefUpdates: &[]git.GitRefUpdate{{
			Name:        &branchRef,
			OldObjectId: &branchTip,
		}},
		Commits: &[]git.GitCommitRef{{
			Comment: &commitMessage,
			Changes: &changes,
		}},
	}

	_, err = gitClient.CreatePush(ctx, git.CreatePushArgs{
		Push:         &push,
		RepositoryId: &a.repo,
		Project:      &a.project,
	})
	if err != nil {
		return fmt.Errorf("push to branch %q: %w", branch, err)
	}

	slog.DebugContext(ctx, "azure devops: updated files",
		slog.String("branch", branch),
		slog.Int("files", len(files)),
	)

	return nil
}

// Points the release branch ref at the base branch tip so each release rewrite
// produces a single commit on top of base, mirroring GitHub and GitLab.
// Returns the resulting branch tip (i.e. the base tip).
func (a *AzureDevOps) resetBranchToBase(ctx context.Context, branch, base string) (string, error) {
	baseTip, err := a.branchTipSHA(ctx, base)
	if err != nil {
		return "", fmt.Errorf("get base branch %q tip: %w", base, err)
	}

	branchTip, err := a.branchTipSHA(ctx, branch)
	if errors.Is(err, errAzureDevOpsBranchMissing) {
		err = a.createBranchAtSHA(ctx, branch, baseTip)
		if err != nil {
			return "", err
		}

		return baseTip, nil
	}

	if err != nil {
		return "", fmt.Errorf("get branch %q tip: %w", branch, err)
	}

	if branchTip == baseTip {
		return baseTip, nil
	}

	gitClient, err := a.client(ctx)
	if err != nil {
		return "", err
	}

	refUpdates := []git.GitRefUpdate{{
		Name:        new("refs/heads/" + branch),
		OldObjectId: &branchTip,
		NewObjectId: &baseTip,
	}}

	results, err := gitClient.UpdateRefs(ctx, git.UpdateRefsArgs{
		RefUpdates:   &refUpdates,
		RepositoryId: &a.repo,
		Project:      &a.project,
	})
	if err != nil {
		return "", fmt.Errorf("reset branch %q to base: %w", branch, err)
	}

	err = validateAzureDevOpsRefUpdateResults(branch, results)
	if err != nil {
		return "", fmt.Errorf("reset branch %q to base: %w", branch, err)
	}

	return baseTip, nil
}

func (a *AzureDevOps) buildPushChanges(files map[string]forge.FileUpdate) []any {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	changes := make([]any, 0, len(paths))
	rawText := git.ItemContentTypeValues.RawText

	for _, path := range paths {
		update := files[path]

		changeType := git.VersionControlChangeTypeValues.Edit
		if !update.Exists {
			changeType = git.VersionControlChangeTypeValues.Add
		}

		ct := changeType
		fullPath := ensureLeadingSlash(path)
		content := update.Content

		changes = append(changes, git.GitChange{
			ChangeType: &ct,
			Item: &git.GitItem{
				Path: &fullPath,
			},
			NewContent: &git.ItemContent{
				Content:     &content,
				ContentType: &rawText,
			},
		})
	}

	return changes
}

var (
	errAzureDevOpsBranchMissing   = errors.New("branch missing")
	errAzureDevOpsRefUpdateFailed = errors.New("ref update failed")
)

func (a *AzureDevOps) branchTipSHA(ctx context.Context, branch string) (string, error) {
	ref, found, err := findRefByName(
		ctx,
		a.refPages(fmt.Sprintf("resolving branch tip %q", branch), "heads/"+branch, false),
		"refs/heads/"+branch,
	)
	if err != nil {
		return "", err
	}

	tip := strings.TrimSpace(derefString(ref.ObjectId))
	if !found || tip == "" {
		return "", errAzureDevOpsBranchMissing
	}

	return tip, nil
}

func ensureLeadingSlash(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}

	return "/" + path
}

func isAzureDevOpsBranchAlreadyExists(err error) bool {
	if err == nil {
		return false
	}

	if azureDevOpsStatusCode(err) == http.StatusConflict {
		return true
	}

	return strings.Contains(err.Error(), "already exists")
}
