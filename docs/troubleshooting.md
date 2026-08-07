# Troubleshooting

`yeet release` keeps wrapped errors for debugging, but the top-level message points at the failure
category so you can pick the next fix quickly:

- `configuration file not found`: create `.yeet.yaml` with `yeet init` at the repo root or pass `--config`.
- `invalid configuration`: fix invalid values in `.yeet.yaml` before rerunning.
- `repository resolution failed`: set `provider` explicitly for custom or enterprise hosts. Set
  `repository` too when remote discovery cannot provide the host and path.
- `provider setup failed`: export the required token (`GITHUB_TOKEN`/`GH_TOKEN`,
  `GITLAB_TOKEN`/`GL_TOKEN`, `AZURE_DEVOPS_SYSTEM_ACCESSTOKEN`, or `AZURE_DEVOPS_EXT_PAT`)
  and, for self-hosted providers, verify `GITHUB_URL`, `GITLAB_URL`, or `AZURE_DEVOPS_URL`.
- `release execution failed: merge blocked`: the release PR/MR is still draft, has conflicts,
  lacks required approvals/checks, or requests a merge method the provider settings do not allow.
- `release execution failed: multiple pending release PRs/MRs found`: close or relabel stale
  entries carrying the configured `release.labels.pending` value until only one remains for the
  base branch. The default value is `autorelease: pending`.

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
