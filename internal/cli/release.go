package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/release"
	"github.com/spf13/cobra"
)

const (
	releaseHelpExample = `  yeet release --dry-run
	  yeet release --preview --dry-run
	  yeet release --auto-merge
	  yeet release --provider github --owner platform --repo yeet --dry-run`
	releasePreviewHelp        = "append preview build metadata with a short commit hash (for example 1.2.3+abc1234)"
	releaseAutoMergeHelp      = "automatically merge the release PR/MR and finalize the release in the same run"
	releaseAutoMergeForceHelp = "attempt auto-merge while bypassing yeet readiness checks; " +
		"still blocks draft/conflicts; provider rules may still apply"
)

func releaseCmd(bootstrap *bootstrapOptions) *cobra.Command {
	flags := &releaseFlagValues{}

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Finalize merged releases and manage release PRs/MRs",
		Long: `Analyzes conventional commits since the last release to determine the next
version, generate a changelog, and create or update a release PR/MR.

When a merged release PR/MR is waiting with the pending autorelease label,
this command first creates the tag/release from the latest changelog entry and
marks the PR/MR as tagged.`,
		Example: releaseHelpExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRelease(
				cmd.Context(),
				cmd.OutOrStdout(),
				bootstrap.configPath(),
				releaseOptionsFromCommand(cmd, *flags),
			)
		},
	}

	bindReleaseFlags(cmd, flags)

	return cmd
}

type releaseFlagValues struct {
	dryRun            bool
	preview           bool
	previewHashLength int
	providerType      string
	remote            string
	host              string
	owner             string
	repo              string
	project           string
	autoMerge         bool
	autoMergeForce    bool
	autoMergeMethod   string
}

func bindReleaseFlags(cmd *cobra.Command, flags *releaseFlagValues) {
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "preview the release without creating a PR/MR")
	cmd.Flags().BoolVar(&flags.preview, "preview", false, releasePreviewHelp)
	cmd.Flags().IntVar(
		&flags.previewHashLength,
		"preview-hash-length",
		release.DefaultPreviewHashLength,
		"length of the short commit hash used for preview build metadata",
	)
	cmd.Flags().StringVar(&flags.providerType, "provider", "", "override provider: github|gitlab")
	cmd.Flags().StringVar(&flags.remote, "remote", "", "override git remote used for repository auto-detection")
	cmd.Flags().StringVar(&flags.host, "host", "", "override repository host, such as github.com or gitlab.company.com")
	cmd.Flags().StringVar(
		&flags.owner,
		"owner",
		"",
		"override repository owner or namespace for github-style repositories",
	)
	cmd.Flags().StringVar(&flags.repo, "repo", "", "override repository name for github-style repositories")
	cmd.Flags().StringVar(&flags.project, "project", "", "override full GitLab project path, including subgroups")
	cmd.Flags().BoolVar(&flags.autoMerge, "auto-merge", false, releaseAutoMergeHelp)
	cmd.Flags().BoolVar(
		&flags.autoMergeForce,
		"auto-merge-force",
		false,
		releaseAutoMergeForceHelp,
	)
	cmd.Flags().StringVar(
		&flags.autoMergeMethod,
		"auto-merge-method",
		"",
		fmt.Sprintf(
			"merge method for auto-merge: auto|squash|rebase|merge (defaults to config value; built-in default: %s)",
			config.AutoMergeMethodAuto,
		),
	)
}

func releaseOptionsFromCommand(cmd *cobra.Command, flags releaseFlagValues) releaseRunOptions {
	return releaseRunOptions{
		dryRun:               flags.dryRun,
		preview:              flags.preview,
		previewHashLength:    flags.previewHashLength,
		provider:             flags.providerType,
		providerSet:          cmd.Flags().Changed("provider"),
		repositoryRemote:     flags.remote,
		repositoryRemoteSet:  cmd.Flags().Changed("remote"),
		repositoryHost:       flags.host,
		repositoryHostSet:    cmd.Flags().Changed("host"),
		repositoryOwner:      flags.owner,
		repositoryOwnerSet:   cmd.Flags().Changed("owner"),
		repositoryRepo:       flags.repo,
		repositoryRepoSet:    cmd.Flags().Changed("repo"),
		repositoryProject:    flags.project,
		repositoryProjectSet: cmd.Flags().Changed("project"),
		autoMerge:            flags.autoMerge,
		autoMergeSet:         cmd.Flags().Changed("auto-merge"),
		autoMergeForce:       flags.autoMergeForce,
		autoMergeForceSet:    cmd.Flags().Changed("auto-merge-force"),
		autoMergeMethod:      flags.autoMergeMethod,
		autoMergeMethodSet:   cmd.Flags().Changed("auto-merge-method"),
	}
}

type releaseRunOptions struct {
	dryRun               bool
	preview              bool
	previewHashLength    int
	provider             string
	providerSet          bool
	repositoryRemote     string
	repositoryRemoteSet  bool
	repositoryHost       string
	repositoryHostSet    bool
	repositoryOwner      string
	repositoryOwnerSet   bool
	repositoryRepo       string
	repositoryRepoSet    bool
	repositoryProject    string
	repositoryProjectSet bool
	autoMerge            bool
	autoMergeSet         bool
	autoMergeForce       bool
	autoMergeForceSet    bool
	autoMergeMethod      string
	autoMergeMethodSet   bool
}

func runRelease(ctx context.Context, output io.Writer, configPath string, options releaseRunOptions) error {
	logReleaseCommand(ctx, configPath, options)

	cfg, err := loadConfig(configPath)
	if err != nil {
		return wrapReleaseConfigError(configPath, err)
	}

	applyReleaseOptions(cfg, options)

	err = cfg.Validate()
	if err != nil {
		return fmt.Errorf("invalid release options: %w", err)
	}

	repository, err := resolveRepository(ctx, cfg, getGitRemoteURL)
	if err != nil {
		return fmt.Errorf("repository resolution failed: %w", err)
	}

	cfg.Provider = repository.Provider

	p, err := createProvider(repository)
	if err != nil {
		return fmt.Errorf("provider setup failed: %w", err)
	}

	r := release.New(cfg, p)

	result, err := r.Release(ctx, options.dryRun, options.preview, options.previewHashLength)
	if err != nil {
		return wrapReleaseExecutionError(err)
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

func wrapReleaseConfigError(configPath string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"configuration file not found: %s; run `yeet init` or pass --config: %w",
			configPath,
			err,
		)
	}

	if errors.Is(err, config.ErrInvalidConfig) {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	return fmt.Errorf("configuration failed: %w", err)
}

func wrapReleaseExecutionError(err error) error {
	if errors.Is(err, release.ErrInvalidPreviewHashLength) {
		return fmt.Errorf("invalid release options: %w", err)
	}

	if errors.Is(err, provider.ErrMergeBlocked) {
		return fmt.Errorf(
			"release execution failed: merge blocked; resolve PR/MR readiness or use --auto-merge-force when appropriate: %w",
			err,
		)
	}

	if errors.Is(err, release.ErrMultiplePendingReleasePRs) {
		return fmt.Errorf(
			"release execution failed: multiple pending release PRs/MRs found; close or relabel stale entries: %w",
			err,
		)
	}

	return fmt.Errorf("release execution failed: %w", err)
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
		"provider_override_set",
		options.providerSet,
		"remote_override_set",
		options.repositoryRemoteSet,
		"host_override_set",
		options.repositoryHostSet,
		"owner_override_set",
		options.repositoryOwnerSet,
		"repo_override_set",
		options.repositoryRepoSet,
		"project_override_set",
		options.repositoryProjectSet,
	)
}

func applyReleaseOptions(cfg *config.Config, options releaseRunOptions) {
	applyRepositoryReleaseOptions(cfg, options)

	applyReleaseBehaviorOptions(cfg, options)
}

func applyRepositoryReleaseOptions(cfg *config.Config, options releaseRunOptions) {
	previousProvider := cfg.Provider

	if options.providerSet {
		cfg.Provider = options.provider
	}

	if options.repositoryRemoteSet {
		cfg.Repository.Remote = options.repositoryRemote
	}

	if options.repositoryHostSet {
		cfg.Repository.Host = options.repositoryHost
	} else if providerChanged(previousProvider, cfg.Provider) {
		cfg.Repository.Host = ""
	}

	if options.repositoryOwnerSet {
		cfg.Repository.Owner = options.repositoryOwner
	}

	if options.repositoryRepoSet {
		cfg.Repository.Repo = options.repositoryRepo
	}

	if options.repositoryProjectSet {
		cfg.Repository.Project = options.repositoryProject

		clearRepositoryOwnerRepoForProject(cfg, options)
	}

	if !options.repositoryProjectSet && (options.repositoryOwnerSet || options.repositoryRepoSet) {
		if strings.TrimSpace(cfg.Repository.Owner) != "" && strings.TrimSpace(cfg.Repository.Repo) != "" {
			cfg.Repository.Project = ""
		}
	}
}

func providerChanged(previous config.ProviderType, next string) bool {
	if previous == "" || next == "" {
		return false
	}

	return previous != next
}

func clearRepositoryOwnerRepoForProject(cfg *config.Config, options releaseRunOptions) {
	if options.repositoryProject == "" {
		return
	}

	if !options.repositoryOwnerSet {
		cfg.Repository.Owner = ""
	}

	if !options.repositoryRepoSet {
		cfg.Repository.Repo = ""
	}
}

func applyReleaseBehaviorOptions(cfg *config.Config, options releaseRunOptions) {
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
