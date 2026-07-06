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
  `autorelease: pending` entries until only one open release PR/MR remains for the base branch.

## Logging

`--verbose` (`-v`) enables debug logging and `--quiet` shows warnings and errors only (setting
both fails). `--no-color` disables colored output. When the flag is not set, the standard
`NO_COLOR`, `CLICOLOR`, and `CLICOLOR_FORCE` environment variables are honored.
