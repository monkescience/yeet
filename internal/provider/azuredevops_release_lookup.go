package provider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/monkescience/yeet/internal/forge"
)

func (a *AzureDevOps) GetReleaseByTag(ctx context.Context, tag string) (*forge.Release, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, forge.ErrNoRelease
	}

	slog.DebugContext(ctx, "azure devops: looking up release by tag", slog.String("tag", tag))

	objectID, err := a.lookupTagObjectID(ctx, tag)
	if err != nil {
		return nil, err
	}

	annotated, err := a.getAnnotatedTag(ctx, objectID)
	if err != nil {
		if isAzureDevOpsNotFound(err) {
			slog.DebugContext(ctx, "azure devops: tag exists but no annotation",
				slog.String("tag", tag),
				slog.String("object_id", objectID),
			)

			return &forge.Release{
				TagName:   tag,
				CommitSHA: objectID,
				Name:      tag,
				URL:       a.tagWebURL(tag),
			}, nil
		}

		return nil, fmt.Errorf("get annotated tag %q: %w", tag, err)
	}

	slog.DebugContext(ctx, "azure devops: release found",
		slog.String("tag", tag),
		slog.String("object_id", objectID),
	)

	return a.azureDevOpsAnnotatedTagRelease(tag, objectID, annotated), nil
}

func (a *AzureDevOps) CreateRelease(ctx context.Context, opts forge.ReleaseOptions) (*forge.Release, error) {
	ref := opts.Ref
	if !isFullCommitSHA(ref) {
		return nil, fmt.Errorf("create release: %w: %q", forge.ErrInvalidCommitSHA, ref)
	}

	slog.DebugContext(ctx, "azure devops: creating annotated tag",
		slog.String("tag", opts.TagName),
		slog.String("ref", ref),
	)

	gitClient, err := a.client(ctx)
	if err != nil {
		return nil, err
	}

	tagObject := git.GitAnnotatedTag{
		Name:    new(opts.TagName),
		Message: new(opts.Body),
		TaggedObject: &git.GitObject{
			ObjectId: new(ref),
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

	release := a.azureDevOpsAnnotatedTagRelease(opts.TagName, ref, created)
	release.Name = opts.Name

	slog.DebugContext(ctx, "azure devops: created annotated tag",
		slog.String("tag", opts.TagName),
		slog.String("object_id", ref),
	)

	return release, nil
}

func (a *AzureDevOps) lookupTagObjectID(ctx context.Context, tag string) (string, error) {
	ref, found, err := findRefByName(
		ctx,
		a.refPages(fmt.Sprintf("looking up tag ref %q", tag), "tags/"+tag, false),
		azureDevOpsTagRefPrefix+tag,
	)
	if err != nil {
		return "", err
	}

	objectID := strings.TrimSpace(derefString(ref.ObjectId))
	if !found || objectID == "" {
		return "", forge.ErrNoRelease
	}

	return objectID, nil
}

func (a *AzureDevOps) getAnnotatedTag(ctx context.Context, objectID string) (*git.GitAnnotatedTag, error) {
	if objectID == "" {
		return nil, fmt.Errorf("%w: empty annotated tag object id", forge.ErrEmptyCommitID)
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

func (a *AzureDevOps) azureDevOpsAnnotatedTagRelease(
	tagName, fallbackCommitSHA string,
	tag *git.GitAnnotatedTag,
) *forge.Release {
	name := derefString(tag.Name)
	if name == "" {
		name = tagName
	}

	commitSHA := fallbackCommitSHA
	if tag.TaggedObject != nil && strings.TrimSpace(derefString(tag.TaggedObject.ObjectId)) != "" {
		commitSHA = derefString(tag.TaggedObject.ObjectId)
	}

	return &forge.Release{
		TagName:   tagName,
		CommitSHA: commitSHA,
		Name:      name,
		Body:      derefString(tag.Message),
		URL:       a.tagWebURL(tagName),
	}
}

// The query string trips charmlog's logfmt quoting, which Azure pipeline logs
// then mis-linkify by appending the closing quote as %22. Manual copy works.
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
