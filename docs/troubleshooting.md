# Troubleshooting

`yeet release` keeps the original cause for debugging. Every error starts with `release failed`,
then names the operational category and the recommended action:

- `configuration file "<path>" was not found`: create the file with `yeet init` or pass `--config`.
- `configuration file "<path>" is invalid`: fix the reported values before rerunning.
- `provider authentication is unavailable`: export one of the token variables named in the cause.
- `repository resolution failed`: check the configured provider settings and Git remote.
- `provider host trust validation failed`: align the configured host, Git remote, and provider URL
  override.
- `the local checkout is unusable or stale`: check out and fetch the configured release branch.
- `the release branch or prerelease channel is invalid`: run from the configured branch or select
  the configured channel.
- `multiple pending release changes were found`: close or relabel stale entries carrying the
  configured `release.labels.pending` value until only one remains for the base branch.
- `merge is blocked`: follow the reason-specific advice for conflicts, draft state, closure,
  repository policy, merge method, provider refusal, or unknown readiness.
- `merge finalization timed out`: inspect the provider state before retrying.
- `release reviewers could not be applied`: check identity, membership, permissions, and provider
  limits.
- `release labels are missing, mismatched, or rejected`: restore or create the configured labels.
- `unexpected failure`: use the preserved original cause for diagnosis.

## Release labels

If an extra label is missing, create every `release.labels.extra` entry in the GitHub repository or
GitLab project, then check its spelling and case before rerunning. Azure DevOps labels do not need
to be created in advance.

A lifecycle-label mismatch means a trusted yeet release branch carries some other label instead of
the configured pending label, which points at renamed lifecycle configuration. Restore the previous
lifecycle names first. Finish or close an open release PR/MR before changing them. For a merged but
unfinalized PR/MR, restore the old names and finalize it before changing the configuration. Yeet
cannot fall back automatically because it stores no label history.

A release PR/MR left with no labels at all is handled automatically. It means a previous run was
interrupted between creating the PR/MR and labelling it, so the next run reapplies the lifecycle
labels and reuses it. No manual relabelling is needed.

## Logging

`--verbose` (`-v`) enables debug logging and `--quiet` shows warnings and errors only (setting
both fails). `--no-color` disables colored output. When the flag is not set, the standard
`NO_COLOR`, `CLICOLOR`, and `CLICOLOR_FORCE` environment variables are honored.
