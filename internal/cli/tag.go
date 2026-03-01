package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/monkescience/yeet/internal/release"
	"github.com/spf13/cobra"
)

func tagCmd() *cobra.Command {
	var (
		tag       string
		changelog string
	)

	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Create a release tag and VCS release",
		Long: `Creates a git tag and a VCS release (GitHub Release or GitLab Release)
after a release PR/MR has been merged.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTag(cmd.Context(), tag, changelog)
		},
	}

	cmd.Flags().StringVar(&tag, "tag", "", "the tag to create (required)")
	cmd.Flags().StringVar(&changelog, "changelog", "", "changelog body for the release")

	err := cmd.MarkFlagRequired("tag")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}

	return cmd
}

func runTag(ctx context.Context, tag, changelogBody string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	p, err := newProvider(ctx, cfg)
	if err != nil {
		return err
	}

	r := release.New(cfg, p)

	result, err := r.Tag(ctx, tag, changelogBody)
	if err != nil {
		return fmt.Errorf("tag failed: %w", err)
	}

	if result.Release != nil {
		slog.InfoContext(ctx, "created release", "url", result.Release.URL)
	}

	return nil
}
