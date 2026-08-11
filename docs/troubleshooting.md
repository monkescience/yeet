# Troubleshooting

Every release error starts with `release failed`, followed by an operational category and a recommended action. The original cause remains attached for debug logging.

| Error prefix or category | Action |
|---|---|
| `configuration file "<path>" was not found` | Run `yeet init` or pass the intended `--config` path |
| `configuration file "<path>" is invalid` | Fix the reported fields and values before rerunning |
| `provider authentication is unavailable` | Export one of the token variables named in the cause |
| `repository resolution failed` | Check the provider setting, repository overrides, and Git remote |
| `provider host trust validation failed` | Align the configured host, Git remote, and provider URL override |
| `the local checkout is unusable or stale` | Fetch full history and check out the current remote release branch |
| `the release branch or prerelease channel is invalid` | Run on a configured branch or select a configured channel |
| `multiple pending release changes were found` | Close or relabel stale pending releases until only one remains for the base branch |
| `merge is blocked` | Resolve the reported conflict, draft state, closure, policy, method, permission, or provider refusal |
| `merge finalization timed out` | Inspect the provider state before retrying |
| `release reviewers could not be applied` | Check identity, membership, permissions, and provider limits |
| `release labels are missing, mismatched, or rejected` | Use the decision table below |
| `unexpected failure` | Enable verbose logging and diagnose the preserved original cause |

## Release labels

| Observed state | Recovery |
|---|---|
| No labels | Rerun yeet. It adopts the trusted release branch, reapplies lifecycle labels, and reuses the PR/MR |
| Wrong lifecycle label | Restore the previous configured names. Finalize or close the in-flight release before changing label configuration |
| Multiple pending releases | Close or relabel stale entries until one PR/MR carries the pending label for the base branch |
| Missing extra label | Create every `release.labels.extra` value in GitHub or GitLab, then verify spelling and GitLab case. Azure DevOps needs no pre-created label |
| Missing tagged label | Recreate `release.labels.tagged` in GitHub or GitLab before retrying finalization |

yeet stores no lifecycle label history, so it cannot infer a renamed pending or tagged label. For a merged but unfinalized release, restore the old names and finalize it before adopting new names.

## Logging

Use `--verbose` or `-v` for debug logs. Use `--quiet` for warnings and errors only. Combining them is invalid. `--no-color` disables color, while the standard `NO_COLOR`, `CLICOLOR`, and `CLICOLOR_FORCE` variables apply when the flag is absent.

## Related documentation

- [Documentation index](README.md)
- [Authentication](authentication.md)
- [CI setup](ci.md)
- [Release PRs and MRs](release.md)
