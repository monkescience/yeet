package provider

import (
	"context"
	"errors"
)

// latestVersionRefWithReleaseFallback returns the tag of the latest release,
// falling back to the most recent tag when the provider reports ErrNoRelease.
// It is shared by providers that expose a release concept (GitHub, GitLab).
// Providers without one resolve the latest version ref from tags directly.
func latestVersionRefWithReleaseFallback(
	ctx context.Context,
	latestReleaseTag func(context.Context) (string, error),
	listTags func(context.Context) ([]string, error),
) (string, error) {
	tag, err := latestReleaseTag(ctx)
	if err == nil {
		return tag, nil
	}

	if !errors.Is(err, ErrNoRelease) {
		return "", err
	}

	tags, err := listTags(ctx)
	if err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "", ErrNoVersionRef
	}

	return tags[0], nil
}
