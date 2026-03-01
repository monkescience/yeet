package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/monkescience/yeet/internal/release"
	"github.com/spf13/cobra"
)

func releaseCmd() *cobra.Command {
	var (
		dryRun            bool
		preview           bool
		previewHashLength int
	)

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Finalize merged releases and manage release PRs/MRs",
		Long: `Analyzes conventional commits since the last release to determine the next
version, generate a changelog, and create or update a release PR/MR.

When a merged release PR/MR is waiting with the pending autorelease label,
this command first creates the tag/release from the latest changelog entry and
marks the PR/MR as tagged.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRelease(cmd.Context(), dryRun, preview, previewHashLength)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the release without creating a PR/MR")
	cmd.Flags().BoolVar(&preview, "preview", false, "append build metadata with commit hash (e.g. 1.2.3+abc1234)")
	cmd.Flags().IntVar(
		&previewHashLength,
		"preview-hash-length",
		release.DefaultPreviewHashLength,
		"length of short commit hash used for preview metadata",
	)

	return cmd
}

func runRelease(ctx context.Context, dryRun, preview bool, previewHashLength int) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	p, err := newProvider(ctx, cfg)
	if err != nil {
		return err
	}

	r := release.New(cfg, p)

	result, err := r.Release(ctx, dryRun, preview, previewHashLength)
	if err != nil {
		return fmt.Errorf("release failed: %w", err)
	}

	if result.BumpType == "none" {
		if result.Release != nil {
			slog.InfoContext(ctx, "release finalized; no new release needed", "tag", result.Release.TagName)

			return nil
		}

		slog.InfoContext(ctx, "no release needed")

		return nil
	}

	if dryRun {
		printDryRun(os.Stdout, result)

		return nil
	}

	return nil
}

func printDryRun(w *os.File, result *release.Result) {
	_, _ = fmt.Fprintln(w, "--- Dry Run ---")
	_, _ = fmt.Fprintf(w, "Current version: %s\n", result.CurrentVersion)
	_, _ = fmt.Fprintf(w, "Next version:    %s\n", result.NextVersion)
	_, _ = fmt.Fprintf(w, "Next tag:        %s\n", result.NextTag)
	_, _ = fmt.Fprintf(w, "Bump type:       %s\n", result.BumpType)
	_, _ = fmt.Fprintf(w, "Commits:         %d\n", result.CommitCount)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Changelog:")
	_, _ = fmt.Fprintln(w, result.Changelog)
}
