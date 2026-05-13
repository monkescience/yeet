package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

func (a *AzureDevOps) CreateBranch(ctx context.Context, name, base string) error {
	baseSHA, err := a.branchTipSHA(ctx, base)
	if err != nil {
		return fmt.Errorf("get base branch %q tip: %w", base, err)
	}

	if existing, _ := a.branchTipSHA(ctx, name); existing != "" {
		return nil
	}

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
			return nil
		}

		return fmt.Errorf("create branch %q: %w", name, err)
	}

	err = validateAzureDevOpsRefUpdateResults(name, results)
	if err != nil {
		return fmt.Errorf("create branch %q: %w", name, err)
	}

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
			return "", ErrFileNotFound
		}

		return "", fmt.Errorf("get file %q on branch %q: %w", path, branch, err)
	}

	defer func() {
		_, _ = io.Copy(io.Discard, body)
		_ = body.Close()
	}()

	contents, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("read file %q on branch %q: %w", path, branch, err)
	}

	return string(contents), nil
}

func (a *AzureDevOps) UpdateFiles(
	ctx context.Context,
	branch, base string,
	files map[string]string,
	message string,
) error {
	branchTip, err := a.ensureBranchExists(ctx, branch, base)
	if err != nil {
		return err
	}

	changes, err := a.buildPushChanges(ctx, branch, files)
	if err != nil {
		return err
	}

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

	return nil
}

// ensureBranchExists returns the current branch tip SHA, creating the branch
// from base when it does not yet exist.
func (a *AzureDevOps) ensureBranchExists(ctx context.Context, branch, base string) (string, error) {
	branchTip, err := a.branchTipSHA(ctx, branch)
	if err == nil {
		return branchTip, nil
	}

	if !errors.Is(err, errAzureDevOpsBranchMissing) {
		return "", fmt.Errorf("get branch %q tip: %w", branch, err)
	}

	baseTip, baseErr := a.branchTipSHA(ctx, base)
	if baseErr != nil {
		return "", fmt.Errorf("get base branch %q tip: %w", base, baseErr)
	}

	err = a.CreateBranch(ctx, branch, base)
	if err != nil {
		return "", err
	}

	return baseTip, nil
}

func (a *AzureDevOps) buildPushChanges(
	ctx context.Context,
	branch string,
	files map[string]string,
) ([]any, error) {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	changes := make([]any, 0, len(paths))
	rawText := git.ItemContentTypeValues.RawText

	for _, path := range paths {
		changeType, err := a.azureDevOpsFileChangeType(ctx, branch, path)
		if err != nil {
			return nil, err
		}

		ct := changeType
		fullPath := ensureLeadingSlash(path)
		content := files[path]

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

	return changes, nil
}

func (a *AzureDevOps) azureDevOpsFileChangeType(
	ctx context.Context,
	branch, path string,
) (git.VersionControlChangeType, error) {
	_, err := a.GetFile(ctx, branch, path)
	if err == nil {
		return git.VersionControlChangeTypeValues.Edit, nil
	}

	if errors.Is(err, ErrFileNotFound) {
		return git.VersionControlChangeTypeValues.Add, nil
	}

	return "", fmt.Errorf("probe file %q on branch %q: %w", path, branch, err)
}

var (
	errAzureDevOpsBranchMissing   = errors.New("branch missing")
	errAzureDevOpsRefUpdateFailed = errors.New("ref update failed")
)

func (a *AzureDevOps) branchTipSHA(ctx context.Context, branch string) (string, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return "", err
	}

	filter := "heads/" + branch

	response, err := gitClient.GetRefs(ctx, git.GetRefsArgs{
		RepositoryId: &a.repo,
		Project:      &a.project,
		Filter:       &filter,
	})
	if err != nil {
		return "", fmt.Errorf("list branch refs for %q: %w", branch, err)
	}

	wantName := "refs/heads/" + branch

	for _, ref := range response.Value {
		if ref.Name != nil && *ref.Name == wantName && ref.ObjectId != nil {
			return *ref.ObjectId, nil
		}
	}

	return "", errAzureDevOpsBranchMissing
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
