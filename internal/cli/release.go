package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/release"
	"github.com/spf13/cobra"
)

func releaseCmd(bootstrap *bootstrapOptions) *cobra.Command {
	var (
		dryRun            bool
		preview           bool
		previewHashLength int
		autoMerge         bool
		autoMergeForce    bool
		autoMergeMethod   string
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
			runOptions := releaseRunOptions{
				dryRun:             dryRun,
				preview:            preview,
				previewHashLength:  previewHashLength,
				autoMerge:          autoMerge,
				autoMergeSet:       cmd.Flags().Changed("auto-merge"),
				autoMergeForce:     autoMergeForce,
				autoMergeForceSet:  cmd.Flags().Changed("auto-merge-force"),
				autoMergeMethod:    autoMergeMethod,
				autoMergeMethodSet: cmd.Flags().Changed("auto-merge-method"),
			}

			return runRelease(cmd.Context(), cmd.OutOrStdout(), bootstrap.configPath(), runOptions)
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
	cmd.Flags().BoolVar(&autoMerge, "auto-merge", false, "automatically merge release PR/MR and finalize release")
	cmd.Flags().BoolVar(
		&autoMergeForce,
		"auto-merge-force",
		false,
		"attempt auto-merge while bypassing readiness checks; still blocks draft/conflicts",
	)
	cmd.Flags().StringVar(
		&autoMergeMethod,
		"auto-merge-method",
		"",
		"merge method for auto-merge: auto|squash|rebase|merge",
	)

	return cmd
}

type releaseRunOptions struct {
	dryRun             bool
	preview            bool
	previewHashLength  int
	autoMerge          bool
	autoMergeSet       bool
	autoMergeForce     bool
	autoMergeForceSet  bool
	autoMergeMethod    string
	autoMergeMethodSet bool
}

func runRelease(ctx context.Context, output io.Writer, configPath string, options releaseRunOptions) error {
	logReleaseCommand(ctx, configPath, options)

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	applyReleaseOptions(cfg, options)

	err = cfg.Validate()
	if err != nil {
		return fmt.Errorf("invalid release options: %w", err)
	}

	p, err := newProvider(ctx, cfg)
	if err != nil {
		return err
	}

	r := release.New(cfg, p)

	result, err := r.Release(ctx, options.dryRun, options.preview, options.previewHashLength)
	if err != nil {
		return fmt.Errorf("release failed: %w", err)
	}

	if result.BumpType == commit.BumpNone {
		if result.Release != nil {
			slog.InfoContext(ctx, "release finalized; no new release needed", "tag", result.Release.TagName)

			return nil
		}

		slog.InfoContext(ctx, "no release needed")

		return nil
	}

	if options.dryRun {
		printDryRun(output, result)

		return nil
	}

	return nil
}

func logReleaseCommand(ctx context.Context, configPath string, options releaseRunOptions) {
	slog.DebugContext(ctx,
		"running release command",
		"config",
		configPath,
		"dry_run",
		options.dryRun,
		"preview",
		options.preview,
		"preview_hash_length",
		options.previewHashLength,
	)
}

func applyReleaseOptions(cfg *config.Config, options releaseRunOptions) {
	if options.autoMergeSet {
		cfg.Release.AutoMerge = options.autoMerge
	}

	if options.autoMergeForceSet {
		cfg.Release.AutoMergeForce = options.autoMergeForce
	}

	if options.autoMergeMethodSet {
		cfg.Release.AutoMergeMethod = options.autoMergeMethod
	}

	if cfg.Release.AutoMergeForce {
		cfg.Release.AutoMerge = true
	}
}

func printDryRun(w io.Writer, result *release.Result) {
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
