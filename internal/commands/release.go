package commands

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/release"
	"github.com/monkescience/yeet/internal/telemetry"
	"github.com/spf13/cobra"
)

func releaseCmd(bootstrap *bootstrapOptions, manager *telemetry.Manager) *cobra.Command {
	flags := &releaseFlagValues{}

	var configFile string

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Finalize merged releases and manage release PRs/MRs",
		Args:  cobra.NoArgs,
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
			started := time.Now()
			options := releaseOptionsFromCommand(cmd, *flags)
			result, err := runRelease(
				cmd.Context(),
				newColorWriter(cmd.OutOrStdout(), bootstrap.noColor),
				strings.TrimSpace(configFile),
				options,
			)
			manager.RecordRelease(cmd.Context(), started, strings.TrimSpace(configFile), options, result, err)

			return err
		},
	}

	bindReleaseFlags(cmd, flags)
	cmd.Flags().StringVar(
		&configFile,
		"config",
		"",
		"path to config file (default: nearest ancestor .yeet.yaml)",
	)

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
	autoMergeMode   string
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
		"enable the configured auto-merge mode",
	)
	cmd.Flags().StringVar(
		&flags.autoMergeMode,
		"auto-merge-mode",
		"",
		fmt.Sprintf(
			"auto-merge execution mode: provider|direct "+
				"(defaults to the configured mode, or %s if unset)",
			config.AutoMergeModeProvider,
		),
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
		AutoMergeMode:     changedFlag(cmd, "auto-merge-mode", &flags.autoMergeMode),
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

func runRelease(
	ctx context.Context,
	output io.Writer,
	configPath string,
	options release.Options,
) (*release.Result, error) {
	result, err := release.Run(ctx, configPath, options)
	if err != nil {
		return result, err //nolint:wrapcheck // the release failure carries its own diagnostic facts
	}

	err = handleReleaseResult(ctx, output, result, options.DryRun)
	if err != nil {
		return result, fmt.Errorf("render release result: %w", err)
	}

	return result, nil
}

func handleReleaseResult(ctx context.Context, output io.Writer, result *release.Result, dryRun bool) error {
	if len(result.Plans) > 0 {
		if dryRun {
			return printDryRun(output, result)
		}

		return nil
	}

	if len(result.Releases) > 0 {
		slog.InfoContext(ctx, "release finalized with no new release needed",
			slog.String("tag", result.Releases[0].Release.TagName),
		)

		return nil
	}

	slog.InfoContext(ctx, "no release needed")

	return nil
}
