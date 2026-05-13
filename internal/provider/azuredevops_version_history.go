package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"golang.org/x/sync/errgroup"
)

const azureDevOpsTagRefPrefix = "refs/tags/"

const azureDevOpsRefPageSize = 100

func (a *AzureDevOps) GetLatestVersionRef(ctx context.Context) (string, error) {
	tags, err := a.ListTags(ctx)
	if err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "", ErrNoVersionRef
	}

	return tags[0], nil
}

func (a *AzureDevOps) ListTags(ctx context.Context) ([]string, error) {
	tags := make([]string, 0)

	err := a.paginateTagRefs(ctx, func(ref git.GitRef) (bool, error) {
		if ref.Name == nil {
			return false, nil
		}

		name := strings.TrimSpace(strings.TrimPrefix(*ref.Name, azureDevOpsTagRefPrefix))
		if name != "" {
			tags = append(tags, name)
		}

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return tags, nil
}

func (a *AzureDevOps) paginateTagRefs(
	ctx context.Context,
	handle func(git.GitRef) (bool, error),
) error {
	gitClient, err := a.client(ctx)
	if err != nil {
		return err
	}

	filter := "tags/"
	pageSize := azureDevOpsRefPageSize

	return paginateAzureDevOps(ctx, "listing tag refs",
		func(token string) ([]git.GitRef, string, error) {
			args := git.GetRefsArgs{
				RepositoryId: &a.repo,
				Project:      &a.project,
				Filter:       &filter,
				Top:          &pageSize,
			}

			if token != "" {
				args.ContinuationToken = new(token)
			}

			response, err := gitClient.GetRefs(ctx, args)
			if err != nil {
				return nil, "", fmt.Errorf("list tag refs: %w", err)
			}

			return response.Value, response.ContinuationToken, nil
		},
		handle,
	)
}

func (a *AzureDevOps) GetCommitsSince(
	ctx context.Context,
	ref, branch string,
	includePaths bool,
) ([]CommitEntry, error) {
	boundaryRef := strings.TrimSpace(ref)
	branch = strings.TrimSpace(branch)

	entries, err := a.fetchAzureDevOpsCommits(ctx, boundaryRef, branch)
	if err != nil {
		return nil, err
	}

	if includePaths && len(entries) > 0 {
		err = a.fillAzureDevOpsCommitPaths(ctx, entries)
		if err != nil {
			return nil, err
		}
	}

	return entries, nil
}

func (a *AzureDevOps) fetchAzureDevOpsCommits(
	ctx context.Context,
	boundaryRef, branch string,
) ([]CommitEntry, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]CommitEntry, 0)
	skip := 0
	top := azureDevOpsRefPageSize

	for range maxPaginationPages {
		err := ctx.Err()
		if err != nil {
			return nil, fmt.Errorf("paginate commits: %w", err)
		}

		criteria := buildAzureDevOpsCommitCriteria(branch, boundaryRef)

		pageCommits, err := gitClient.GetCommits(ctx, git.GetCommitsArgs{
			RepositoryId:   &a.repo,
			Project:        &a.project,
			SearchCriteria: criteria,
			Skip:           &skip,
			Top:            &top,
		})
		if err != nil {
			if boundaryRef != "" && azureDevOpsBoundaryError(err) {
				return nil, &CommitBoundaryNotFoundError{Ref: boundaryRef, Branch: branch}
			}

			return nil, fmt.Errorf("list commits: %w", err)
		}

		if pageCommits == nil {
			return entries, nil
		}

		for _, commit := range *pageCommits {
			entries = append(entries, CommitEntry{
				Hash:    derefString(commit.CommitId),
				Message: derefString(commit.Comment),
			})
		}

		if len(*pageCommits) < azureDevOpsRefPageSize {
			return entries, nil
		}

		skip += len(*pageCommits)
	}

	return nil, fmt.Errorf("%w: exceeded %d pages listing commits", ErrPaginationLimitExceeded, maxPaginationPages)
}

func buildAzureDevOpsCommitCriteria(branch, boundaryRef string) *git.GitQueryCommitsCriteria {
	criteria := &git.GitQueryCommitsCriteria{}

	branchType := git.GitVersionTypeValues.Branch

	if branch != "" {
		criteria.ItemVersion = &git.GitVersionDescriptor{
			Version:     new(branch),
			VersionType: &branchType,
		}
	}

	if boundaryRef != "" {
		compareType := git.GitVersionTypeValues.Commit
		criteria.CompareVersion = &git.GitVersionDescriptor{
			Version:     new(boundaryRef),
			VersionType: &compareType,
		}
	}

	return criteria
}

func azureDevOpsBoundaryError(err error) bool {
	status := azureDevOpsStatusCode(err)

	return status == 404 || status == 400
}

func (a *AzureDevOps) fillAzureDevOpsCommitPaths(ctx context.Context, entries []CommitEntry) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(maxConcurrentPathFetches)

	for idx := range entries {
		eg.Go(func() error {
			paths, err := a.commitPaths(egCtx, entries[idx].Hash)
			if err != nil {
				return err
			}

			entries[idx].Paths = paths

			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return fmt.Errorf("fetch commit paths: %w", err)
	}

	return nil
}

func (a *AzureDevOps) commitPaths(ctx context.Context, sha string) ([]string, error) {
	commitID := strings.TrimSpace(sha)
	if commitID == "" {
		return nil, fmt.Errorf("%w: empty commit id", ErrEmptyCommitID)
	}

	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	changes, err := gitClient.GetChanges(ctx, git.GetChangesArgs{
		CommitId:     &commitID,
		RepositoryId: &a.repo,
		Project:      &a.project,
		Top:          new(azureDevOpsRefPageSize),
	})
	if err != nil {
		return nil, fmt.Errorf("get changes for commit %q: %w", commitID, err)
	}

	if changes == nil || changes.Changes == nil {
		return []string{}, nil
	}

	paths := make([]string, 0, len(*changes.Changes))
	seen := make(map[string]struct{})

	for _, raw := range *changes.Changes {
		path := extractAzureDevOpsChangePath(raw)

		normalized := strings.TrimPrefix(strings.TrimSpace(path), "/")
		if normalized == "" {
			continue
		}

		if _, exists := seen[normalized]; exists {
			continue
		}

		seen[normalized] = struct{}{}
		paths = append(paths, normalized)
	}

	return paths, nil
}

// extractAzureDevOpsChangePath reads the item.path field from a change entry.
// The SDK types the change slice as []interface{} so we round-trip the entry
// through JSON to access the typed item path.
func extractAzureDevOpsChangePath(raw any) string {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return ""
	}

	var parsed struct {
		Item struct {
			Path string `json:"path"`
		} `json:"item"`
	}

	err = json.Unmarshal(encoded, &parsed)
	if err != nil {
		return ""
	}

	return parsed.Item.Path
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}
