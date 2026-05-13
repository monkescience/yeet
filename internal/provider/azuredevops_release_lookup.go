package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

func (a *AzureDevOps) GetReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, ErrNoRelease
	}

	objectID, err := a.lookupTagObjectID(ctx, tag)
	if err != nil {
		return nil, err
	}

	annotated, err := a.getAnnotatedTag(ctx, objectID)
	if err != nil {
		if isAzureDevOpsNotFound(err) {
			return &Release{TagName: tag, Name: tag, URL: a.tagWebURL(tag)}, nil
		}

		return nil, fmt.Errorf("get annotated tag %q: %w", tag, err)
	}

	return a.azureDevOpsAnnotatedTagRelease(tag, annotated), nil
}

func (a *AzureDevOps) TagExists(ctx context.Context, tag string) (bool, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false, nil
	}

	_, err := a.lookupTagObjectID(ctx, tag)
	if err != nil {
		if errors.Is(err, ErrNoRelease) || isAzureDevOpsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (a *AzureDevOps) CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error) {
	ref := strings.TrimSpace(opts.Ref)
	if ref == "" {
		return nil, fmt.Errorf("create release: %w: ref required", ErrEmptyCommitSHA)
	}

	objectID, err := a.resolveAzureDevOpsObjectID(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolve ref %q: %w", ref, err)
	}

	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	tagObject := git.GitAnnotatedTag{
		Name:    new(opts.TagName),
		Message: new(opts.Body),
		TaggedObject: &git.GitObject{
			ObjectId: new(objectID),
		},
	}

	created, err := gitClient.CreateAnnotatedTag(ctx, git.CreateAnnotatedTagArgs{
		TagObject:    &tagObject,
		Project:      &a.project,
		RepositoryId: &a.repo,
	})
	if err != nil {
		return nil, fmt.Errorf("create annotated tag: %w", err)
	}

	release := a.azureDevOpsAnnotatedTagRelease(opts.TagName, created)
	release.Name = opts.Name

	return release, nil
}

func (a *AzureDevOps) lookupTagObjectID(ctx context.Context, tag string) (string, error) {
	gitClient, err := a.client(ctx)
	if err != nil {
		return "", err
	}

	filter := "tags/" + tag

	response, err := gitClient.GetRefs(ctx, git.GetRefsArgs{
		RepositoryId: &a.repo,
		Project:      &a.project,
		Filter:       &filter,
	})
	if err != nil {
		return "", fmt.Errorf("get tag ref %q: %w", tag, err)
	}

	wantName := azureDevOpsTagRefPrefix + tag

	for _, ref := range response.Value {
		if ref.Name != nil && *ref.Name == wantName && ref.ObjectId != nil && *ref.ObjectId != "" {
			return *ref.ObjectId, nil
		}
	}

	return "", ErrNoRelease
}

func (a *AzureDevOps) getAnnotatedTag(ctx context.Context, objectID string) (*git.GitAnnotatedTag, error) {
	if objectID == "" {
		return nil, fmt.Errorf("%w: empty annotated tag object id", ErrEmptyCommitID)
	}

	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	tag, err := gitClient.GetAnnotatedTag(ctx, git.GetAnnotatedTagArgs{
		Project:      &a.project,
		RepositoryId: &a.repo,
		ObjectId:     &objectID,
	})
	if err != nil {
		return nil, fmt.Errorf("get annotated tag: %w", err)
	}

	return tag, nil
}

func (a *AzureDevOps) resolveAzureDevOpsObjectID(ctx context.Context, ref string) (string, error) {
	if isAzureDevOpsCommitSHA(ref) {
		return ref, nil
	}

	gitClient, err := a.client(ctx)
	if err != nil {
		return "", err
	}

	branchType := git.GitVersionTypeValues.Branch
	top := 1

	commits, err := gitClient.GetCommits(ctx, git.GetCommitsArgs{
		RepositoryId: &a.repo,
		Project:      &a.project,
		SearchCriteria: &git.GitQueryCommitsCriteria{
			ItemVersion: &git.GitVersionDescriptor{
				Version:     &ref,
				VersionType: &branchType,
			},
			Top: &top,
		},
		Top: &top,
	})
	if err != nil {
		return "", fmt.Errorf("resolve ref %q: %w", ref, err)
	}

	if commits == nil || len(*commits) == 0 {
		return "", fmt.Errorf("%w: ref %q", ErrEmptyCommitSHA, ref)
	}

	first := (*commits)[0]
	if first.CommitId == nil || *first.CommitId == "" {
		return "", fmt.Errorf("%w: ref %q", ErrEmptyCommitSHA, ref)
	}

	return *first.CommitId, nil
}

func (a *AzureDevOps) azureDevOpsAnnotatedTagRelease(tagName string, tag *git.GitAnnotatedTag) *Release {
	name := derefString(tag.Name)
	if name == "" {
		name = tagName
	}

	return &Release{
		TagName: tagName,
		Name:    name,
		Body:    derefString(tag.Message),
		URL:     a.tagWebURL(tagName),
	}
}

// tagWebURL returns the Azure DevOps "browse repo at tag" URL. The query
// string trips charmlog's logfmt quoting, which Azure pipeline logs then
// mis-linkify by appending the closing quote as %22; manual copy works.
func (a *AzureDevOps) tagWebURL(tag string) string {
	return fmt.Sprintf("%s?version=GT%s", a.RepoURL(), tag)
}

func isAzureDevOpsCommitSHA(ref string) bool {
	const minSHALength = 7

	const maxSHALength = 40

	if len(ref) < minSHALength || len(ref) > maxSHALength {
		return false
	}

	for _, r := range ref {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}

		return false
	}

	return true
}
