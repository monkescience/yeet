# Auto-merge

yeet can merge the release PR/MR for you:

```yaml
release:
  auto_merge: true
  auto_merge_mode: provider
  auto_merge_method: squash
```

Use `--auto-merge`, `--auto-merge-mode`, and `--auto-merge-method` to override these for one run.

## Modes

| Mode | What happens | Supported on |
|---|---|---|
| `provider` (default) | The provider merges once its checks, approvals, merge queue, or merge train pass. The next CI run on the base branch publishes the release | GitHub, GitLab 17.11 or newer |
| `direct` | yeet checks that the PR/MR is ready, merges it, and publishes the release in the same run | GitHub, GitLab, Azure DevOps |

Provider mode needs native auto-merge enabled in the repository settings, and a token that may use it.
Direct mode needs a token that may merge into the base branch.
See [Authentication](authentication.md) for permissions.

Turning auto-merge off does not cancel a merge the provider already scheduled. Cancel it in the provider.

## Merge methods

`auto_merge_method` is `auto` (default), `squash`, `rebase`, or `merge`.

- `auto` prefers squash and follows the method of a required merge queue or train.
- An explicit method fails if the provider or its queue does not allow it.

In direct mode, a slow provider can need longer to report the merge. Raise the wait time if yeet reports a merge timeout:

```yaml
release:
  merge_polling:
    timeout: 5m
```

## Related documentation

- [Release PRs and MRs](release.md)
- [Troubleshooting](troubleshooting.md#auto-merge)
