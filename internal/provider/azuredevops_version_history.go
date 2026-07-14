package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
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
	slog.DebugContext(ctx, "azure devops: listing tags")

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

	slog.DebugContext(ctx, "azure devops: listed tags", slog.Int("count", len(tags)))

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

func (a *AzureDevOps) GetCommitsSinceRefs(
	ctx context.Context,
	refs []string,
	branch string,
	includePaths bool,
) (CommitHistory, error) {
	branch = strings.TrimSpace(branch)

	history, err := fetchCommitHistoryByRef(ctx, refs, a.maxConcurrentRequests,
		func(ctx context.Context, ref string) ([]CommitEntry, error) {
			return a.commitsSinceRef(ctx, ref, branch)
		},
	)
	if err != nil {
		return CommitHistory{}, err
	}

	if includePaths {
		if err := hydrateCommitHistoryPaths(ctx, history, a.maxConcurrentRequests, a.commitPaths); err != nil {
			return CommitHistory{}, err
		}
	}

	return history, nil
}

// commitsSinceRef returns the commits reachable from branch but not from ref,
// newest-first. It asks Azure DevOps for the graph-aware range directly
// (ItemVersion is the stop boundary, CompareVersion is the head) instead of
// walking the whole branch and slicing it client-side, which over-includes
// commits on non-linear histories. ref == "" walks the full branch history.
//
// A bounded query against a boundary that does not exist returns a 404/400,
// which surfaces as ErrRefNotFound so the batch records it as a missing ref
// rather than failing.
func (a *AzureDevOps) commitsSinceRef(
	ctx context.Context,
	ref, branch string,
) ([]CommitEntry, error) {
	boundaryRef := strings.TrimSpace(ref)

	slog.DebugContext(ctx, "azure devops: fetching commits",
		slog.String("branch", branch),
		slog.String("boundary_ref", boundaryRef),
	)

	entries, err := a.listAzureDevOpsCommits(ctx, branch, boundaryRef)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "azure devops: fetched commits", slog.Int("count", len(entries)))

	return entries, nil
}

// listAzureDevOpsCommits walks the graph-aware range boundaryRef..branch
// (boundaryRef == "" walks the full branch), newest-first. A boundary the API
// rejects with 404/400 becomes ErrRefNotFound so the batch records a missing
// ref rather than failing.
func (a *AzureDevOps) listAzureDevOpsCommits(
	ctx context.Context,
	branch, boundaryRef string,
) ([]CommitEntry, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]CommitEntry, 0)
	top := azureDevOpsRefPageSize
	criteria := buildAzureDevOpsCommitCriteria(branch, boundaryRef)

	err = paginateAzureDevOpsBySkip(ctx, "listing commits", top,
		func(skip int) ([]git.GitCommitRef, error) {
			pageCommits, err := gitClient.GetCommits(ctx, git.GetCommitsArgs{
				RepositoryId:   &a.repo,
				Project:        &a.project,
				SearchCriteria: criteria,
				Skip:           &skip,
				Top:            &top,
			})
			if err != nil {
				if boundaryRef != "" && azureDevOpsBoundaryError(err) {
					return nil, fmt.Errorf("%w: ref %q", ErrRefNotFound, boundaryRef)
				}

				return nil, fmt.Errorf("list commits: %w", err)
			}

			if pageCommits == nil {
				return nil, nil
			}

			return *pageCommits, nil
		},
		func(commit git.GitCommitRef) (bool, error) {
			entries = append(entries, CommitEntry{
				Hash:    derefString(commit.CommitId),
				Message: derefString(commit.Comment),
			})

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// buildAzureDevOpsCommitCriteria builds the GetCommits search criteria for the
// branch's history relative to boundaryRef.
//
// Bounded (boundaryRef != ""): Azure DevOps inverts the usual naming, so
// CompareVersion is where it starts walking history (the head) and ItemVersion
// is the boundary it stops at. Swapping them yields the wrong (inverse or empty)
// range.
//
// Unbounded (boundaryRef == ""): the branch must be the ItemVersion filter. A
// lone CompareVersion does not scope the query to the branch, so Azure returns
// repository-wide commits instead of the branch's own history.
func buildAzureDevOpsCommitCriteria(branch, boundaryRef string) *git.GitQueryCommitsCriteria {
	criteria := &git.GitQueryCommitsCriteria{}

	if boundaryRef == "" {
		if branch != "" {
			branchType := git.GitVersionTypeValues.Branch
			criteria.ItemVersion = &git.GitVersionDescriptor{
				Version:     new(branch),
				VersionType: &branchType,
			}
		}

		return criteria
	}

	itemType := git.GitVersionTypeValues.Tag
	if isAzureDevOpsCommitSHA(boundaryRef) {
		itemType = git.GitVersionTypeValues.Commit
	}

	criteria.ItemVersion = &git.GitVersionDescriptor{
		Version:     new(boundaryRef),
		VersionType: &itemType,
	}

	if branch != "" {
		branchType := git.GitVersionTypeValues.Branch
		criteria.CompareVersion = &git.GitVersionDescriptor{
			Version:     new(branch),
			VersionType: &branchType,
		}
	}

	return criteria
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

	paths := make([]string, 0)
	seen := make(map[string]struct{})
	pageSize := azureDevOpsRefPageSize

	err = paginateAzureDevOpsBySkip(ctx, fmt.Sprintf("listing changes for commit %q", commitID), pageSize,
		func(skip int) ([]any, error) {
			changes, err := gitClient.GetChanges(ctx, git.GetChangesArgs{
				CommitId:     &commitID,
				RepositoryId: &a.repo,
				Project:      &a.project,
				Top:          &pageSize,
				Skip:         &skip,
			})
			if err != nil {
				return nil, fmt.Errorf("get changes for commit %q: %w", commitID, err)
			}

			if changes == nil || changes.Changes == nil {
				return nil, nil
			}

			return *changes.Changes, nil
		},
		func(raw any) (bool, error) {
			for _, path := range extractAzureDevOpsChangePaths(raw) {
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

			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return paths, nil
}

// The SDK types the change slice as []interface{} so we round-trip the entry
// through JSON to access its current and previous paths.
func extractAzureDevOpsChangePaths(raw any) []string {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}

	var parsed struct {
		Item struct {
			Path string `json:"path"`
		} `json:"item"`
		OriginalPath     string `json:"originalPath"`
		SourceServerItem string `json:"sourceServerItem"`
	}

	err = json.Unmarshal(encoded, &parsed)
	if err != nil {
		return nil
	}

	return []string{parsed.Item.Path, parsed.OriginalPath, parsed.SourceServerItem}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}
