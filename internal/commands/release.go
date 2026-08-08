package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/release"
	"github.com/monkescience/yeet/internal/ui"
	"github.com/spf13/cobra"
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
marks the PR/MR as tagged.

Settings resolve in priority order: command-line flag, then the .yeet.yaml
config file, then the built-in default. A .yeet.yaml file is required (run
"yeet init" to create one).

Authentication tokens are read only from environment variables, never from
flags or config:
  GitHub:       GITHUB_TOKEN or GH_TOKEN (optional GITHUB_URL for Enterprise)
  GitLab:       GITLAB_TOKEN or GL_TOKEN (optional GITLAB_URL for self-hosted)
  Azure DevOps: AZURE_DEVOPS_SYSTEM_ACCESSTOKEN or AZURE_DEVOPS_EXT_PAT
                (optional AZURE_DEVOPS_URL)

Commit history is read from the local git checkout. The checkout must be
complete (not shallow) and match the remote release branch.`,
		Example: `  yeet release --dry-run
	  yeet release --target api --target web --dry-run
	  yeet release --channel beta
	  yeet release --auto-merge
	  yeet release --provider github --owner platform --repo yeet --dry-run`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRelease(
				cmd.Context(),
				newColorWriter(cmd.OutOrStdout(), bootstrap.noColor),
				bootstrap.configPath(),
				releaseOptionsFromCommand(cmd, *flags),
			)
		},
	}

	bindReleaseFlags(cmd, flags)

	return cmd
}

type releaseFlagValues struct {
	dryRun          bool
	providerType    string
	remote          string
	host            string
	owner           string
	repo            string
	project         string
	autoMerge       bool
	autoMergeForce  bool
	autoMergeMethod string
	channel         string
	targets         []string
}

func bindReleaseFlags(cmd *cobra.Command, flags *releaseFlagValues) {
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "show the planned release without creating a PR/MR")
	cmd.Flags().StringVar(&flags.providerType, "provider", "", "override provider: auto|github|gitlab|azuredevops")
	cmd.Flags().StringVar(&flags.remote, "remote", "", "override git remote used for repository auto-detection")
	cmd.Flags().StringVar(
		&flags.host,
		"host",
		"",
		"override repository host, such as github.com or gitlab.company.com (github, gitlab, azuredevops)",
	)
	cmd.Flags().StringVar(
		&flags.owner,
		"owner",
		"",
		"override repository owner or namespace (github)",
	)
	cmd.Flags().StringVar(&flags.repo, "repo", "", "override repository name (github, azuredevops)")
	cmd.Flags().StringVar(
		&flags.project,
		"project",
		"",
		"override full project path, including subgroups (github, gitlab, azuredevops)",
	)
	cmd.Flags().BoolVar(
		&flags.autoMerge,
		"auto-merge",
		false,
		"automatically merge the release PR/MR and finalize the release in the same run",
	)
	cmd.Flags().BoolVar(
		&flags.autoMergeForce,
		"auto-merge-force",
		false,
		"attempt auto-merge while bypassing yeet readiness checks. "+
			"Draft status and conflicts still block merging. Provider rules may still apply",
	)
	cmd.Flags().StringVar(
		&flags.autoMergeMethod,
		"auto-merge-method",
		"",
		fmt.Sprintf(
			"merge method for auto-merge: auto|squash|rebase|merge "+
				"(defaults to the configured method, or %s if unset)",
			config.AutoMergeMethodAuto,
		),
	)
	cmd.Flags().StringVar(
		&flags.channel,
		"channel",
		"",
		"run a configured prerelease channel, defaulting to the channel matching the current branch",
	)
	cmd.Flags().StringArrayVar(
		&flags.targets,
		"target",
		nil,
		"limit analysis to one or more configured targets (repeatable)",
	)
}

func releaseOptionsFromCommand(cmd *cobra.Command, flags releaseFlagValues) release.Options {
	return release.Options{
		DryRun:            flags.dryRun,
		Provider:          changedFlag(cmd, "provider", &flags.providerType),
		RepositoryRemote:  changedFlag(cmd, "remote", &flags.remote),
		RepositoryHost:    changedFlag(cmd, "host", &flags.host),
		RepositoryOwner:   changedFlag(cmd, "owner", &flags.owner),
		RepositoryRepo:    changedFlag(cmd, "repo", &flags.repo),
		RepositoryProject: changedFlag(cmd, "project", &flags.project),
		AutoMerge:         changedFlag(cmd, "auto-merge", &flags.autoMerge),
		AutoMergeForce:    changedFlag(cmd, "auto-merge-force", &flags.autoMergeForce),
		AutoMergeMethod:   changedFlag(cmd, "auto-merge-method", &flags.autoMergeMethod),
		Channel:           changedFlag(cmd, "channel", &flags.channel),
		Targets:           append([]string(nil), flags.targets...),
	}
}

func changedFlag[T any](cmd *cobra.Command, name string, value *T) *T {
	if !cmd.Flags().Changed(name) {
		return nil
	}

	return value
}

func runRelease(ctx context.Context, output io.Writer, configPath string, options release.Options) error {
	result, err := release.Run(ctx, configPath, options)
	if err != nil {
		var configErr *release.ConfigError
		if errors.As(err, &configErr) {
			return wrapReleaseConfigError(configErr.Path(), err)
		}

		var executionErr *release.ExecutionError
		if errors.As(err, &executionErr) {
			return wrapReleaseExecutionError(err)
		}

		return err //nolint:wrapcheck // preserve stage-specific user-facing error verbatim
	}

	return handleReleaseResult(ctx, output, result, options.DryRun)
}

func handleReleaseResult(ctx context.Context, output io.Writer, result *release.Result, dryRun bool) error {
	if len(result.Plans) > 0 {
		if dryRun {
			printDryRun(output, result)
		}

		return nil
	}

	if len(result.Releases) > 0 {
		slog.InfoContext(ctx, "release finalized with no new release needed",
			slog.String("tag", result.Releases[0].TagName),
		)

		return nil
	}

	slog.InfoContext(ctx, "no release needed")

	return nil
}

func wrapReleaseConfigError(configPath string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"configuration file not found: %s. Run `yeet init` or pass --config: %w",
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
	if errors.Is(err, provider.ErrMergeBlocked) {
		return fmt.Errorf(
			"release execution failed: merge blocked. Resolve pull request or merge request readiness, "+
				"or use --auto-merge-force when appropriate: %w",
			err,
		)
	}

	if errors.Is(err, release.ErrMultiplePendingReleasePRs) {
		return fmt.Errorf(
			"release execution failed: multiple pending release changes found "+
				"(pull requests or merge requests). Close or relabel stale entries: %w",
			err,
		)
	}

	return fmt.Errorf("release execution failed: %w", err)
}

func printDryRun(w io.Writer, result *release.Result) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, ui.Header.Render("Dry Run"))
	_, _ = fmt.Fprintln(w)

	if len(result.Plans) == 0 {
		_, _ = fmt.Fprintln(w, ui.Faint.Render("No changed targets."))

		return
	}

	for i, plan := range result.Plans {
		if i > 0 {
			_, _ = fmt.Fprintln(w)
		}

		printDryRunTarget(w, plan)
	}
}

func printDryRunTarget(w io.Writer, plan release.TargetPlan) {
	_, _ = fmt.Fprintf(w, "  %s  %s\n",
		ui.Label.Render("Target"),
		plan.ID,
	)

	versionLine := fmt.Sprintf("%s → %s", plan.CurrentVersion, ui.Value.Render(plan.NextVersion))
	_, _ = fmt.Fprintf(w, "  %s  %s %s\n",
		ui.Label.Render("Version"),
		versionLine,
		ui.Faint.Render("("+string(plan.BumpType)+")"),
	)

	_, _ = fmt.Fprintf(w, "  %s  %s\n",
		ui.Label.Render("Tag"),
		ui.Value.Render(plan.NextTag),
	)

	_, _ = fmt.Fprintf(w, "  %s  %d\n",
		ui.Label.Render("Commits"),
		plan.CommitCount,
	)

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "  %s\n", ui.Faint.Render(ui.Separator))
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, changelog.Render(plan.Entry))
}
