package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

//nolint:funlen // Multi-boundary scanning and path hydration are clearer kept together.
func (a *AzureDevOps) GetCommitsSinceRefs(
	ctx context.Context,
	refs []string,
	branch string,
	includePaths bool,
) (CommitHistory, error) {
	normalizedRefs := normalizeCommitHistoryRefs(refs)
	if len(normalizedRefs) == 0 {
		return CommitHistory{EntriesByRef: map[string][]CommitEntry{}}, nil
	}

	boundaryRefsByID, hasUnboundedRef, err := resolveBoundaryRefs(
		ctx, normalizedRefs, a.resolveAzureDevOpsObjectID, a.maxConcurrentRequests)
	if err != nil {
		return CommitHistory{}, err
	}

	if len(boundaryRefsByID) == 0 && !hasUnboundedRef {
		return commitHistoryFromBoundaryPositions(normalizedRefs, nil, nil), nil
	}

	branch = strings.TrimSpace(branch)

	slog.DebugContext(ctx, "azure devops: fetching commits for refs",
		slog.Int("refs", len(normalizedRefs)),
		slog.String("branch", branch),
		slog.Bool("include_paths", includePaths),
	)

	gitClient, err := a.client(ctx)
	if err != nil {
		return CommitHistory{}, err
	}

	top := azureDevOpsRefPageSize
	criteria := buildAzureDevOpsCommitCriteria(branch, "")
	scanner := newCommitBoundaryScanner(boundaryRefsByID, hasUnboundedRef)

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
				return nil, fmt.Errorf("list commits: %w", err)
			}

			if pageCommits == nil {
				return nil, nil
			}

			return *pageCommits, nil
		},
		func(commit git.GitCommitRef) (bool, error) {
			return scanner.observe(derefString(commit.CommitId), derefString(commit.Comment)), nil
		},
	)
	if err != nil {
		return CommitHistory{}, err
	}

	entries := trimEntriesToReferencedRange(scanner.entries, scanner.positions, hasUnboundedRef)

	return a.commitHistoryWithPaths(
		ctx, normalizedRefs, entries, scanner.positions, includePaths, scanner.earlyTerminated(), hasUnboundedRef)
}

func (a *AzureDevOps) commitHistoryWithPaths(
	ctx context.Context,
	refs []string,
	entries []CommitEntry,
	positions map[string]int,
	includePaths bool,
	earlyTerminated bool,
	hasUnboundedRef bool,
) (CommitHistory, error) {
	if includePaths && len(entries) > 0 {
		err := a.fillAzureDevOpsCommitPaths(ctx, entries)
		if err != nil {
			return CommitHistory{}, err
		}
	}

	history := commitHistoryFromBoundaryPositions(refs, entries, positions)
	slog.DebugContext(ctx, "azure devops: fetched commits for refs",
		slog.Int("entries", len(entries)),
		slog.Int("missing_refs", len(history.MissingRefs)),
		slog.Bool("early_terminated", earlyTerminated),
		slog.Bool("unbounded_ref", hasUnboundedRef),
	)

	return history, nil
}

// Azure DevOps inverts the usual naming: CompareVersion is where it starts
// walking history (the head), and ItemVersion is the boundary it stops at.
// Swapping them returns nothing.
func buildAzureDevOpsCommitCriteria(branch, boundaryRef string) *git.GitQueryCommitsCriteria {
	criteria := &git.GitQueryCommitsCriteria{}

	if boundaryRef != "" {
		itemType := git.GitVersionTypeValues.Tag
		if isAzureDevOpsCommitSHA(boundaryRef) {
			itemType = git.GitVersionTypeValues.Commit
		}

		criteria.ItemVersion = &git.GitVersionDescriptor{
			Version:     new(boundaryRef),
			VersionType: &itemType,
		}
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

func (a *AzureDevOps) fillAzureDevOpsCommitPaths(ctx context.Context, entries []CommitEntry) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(a.maxConcurrentRequests)

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
