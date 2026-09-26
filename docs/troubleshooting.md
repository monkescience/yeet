# Troubleshooting

Start with the advice in the error message. Rerun with `--verbose` (`-v`) for details, or `--quiet` for warnings and errors only.

## Auto-merge

| Problem | Fix |
|---|---|
| Provider auto-merge is unsupported | Azure DevOps and GitLab before 17.11 need `auto_merge_mode: direct` |
| The provider refuses to schedule the merge | Enable auto-merge in the repository settings and check the token permissions and branch rules. yeet does not fall back to `direct` |
| The PR/MR merged but no release was published | In `provider` mode, the next CI run on the base branch publishes it. Make sure that run happens |

See [Auto-merge](auto-merge.md).

## Release labels

yeet finds releases by their labels and does not remember old label names.

| Problem | Fix |
|---|---|
| The release PR/MR lost its labels | Rerun yeet. It relabels and reuses the PR/MR |
| Labels were renamed while a release was open | Restore the old names, finish that release, then rename |
| Several PRs/MRs have the pending label | Close or relabel the stale ones so only one remains per base branch |
| An extra label is missing | Create it in GitHub or GitLab. GitLab label names are case-sensitive |
| The tagged label is missing | Recreate it in GitHub or GitLab, then rerun |

## Related documentation

- [Authentication](authentication.md)
- [CI setup](ci.md)
- [Release PRs and MRs](release.md)
