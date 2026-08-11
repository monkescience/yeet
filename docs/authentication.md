# Authentication

yeet reads provider credentials from environment variables. It never accepts tokens through flags or `.yeet.yaml`.

| Provider | Variables, in precedence order | Required access |
|---|---|---|
| GitHub | `GITHUB_TOKEN`, `GH_TOKEN` | `contents: write`, `pull-requests: write`, and `issues: write` |
| GitLab | `GITLAB_TOKEN`, `GL_TOKEN` | API access to create and update merge requests, branches, tags, labels, and releases |
| Azure DevOps | `AZURE_DEVOPS_SYSTEM_ACCESSTOKEN`, `AZURE_DEVOPS_EXT_PAT` | Repository access to create branches and tags, create and update pull requests, manage labels, and complete pull requests when auto-merge is enabled |

## GitHub

Export either token variable. `GITHUB_TOKEN` wins when both are set.

```sh
export GITHUB_TOKEN=github_pat_xxx
yeet release --dry-run
```

For GitHub Actions, a GitHub App installation token can provide the required repository permissions without granting write access to the workflow token. See the complete [GitHub Actions example](ci.md#github-actions-with-a-github-app).

## GitLab

Export either token variable. `GITLAB_TOKEN` wins when both are set.

```sh
export GITLAB_TOKEN=glpat-xxx
yeet release --dry-run
```

The token must be able to create and update merge requests, manage labels, push release branches and tags, and publish releases.

## Azure DevOps

Azure Pipelines should map `System.AccessToken` to yeet's provider-specific variable:

```yaml
env:
  AZURE_DEVOPS_SYSTEM_ACCESSTOKEN: $(System.AccessToken)
```

For local use or external CI, use the Azure DevOps CLI-compatible PAT variable:

```sh
export AZURE_DEVOPS_EXT_PAT=xxx
yeet release --dry-run
```

`AZURE_DEVOPS_SYSTEM_ACCESSTOKEN` takes precedence and uses bearer authentication. `AZURE_DEVOPS_EXT_PAT` uses basic authentication.

When `release.reviewers` is configured, a PAT also needs Identity (Read), `vso.identity`, so yeet can resolve reviewer names. Authorization for identity reads through a pipeline `System.AccessToken` depends on the organization and job authorization settings, so verify it in your Azure DevOps configuration.

## Host trust

yeet trusts a repository host only when it is a recognized public provider host, matches the Git remote host, or matches the provider's URL override. A checked-in `repository.*.host` cannot redirect a token by itself.

`GITHUB_URL`, `GITLAB_URL`, and `AZURE_DEVOPS_URL` override the API endpoint after repository resolution. They take precedence even for public hosts. Treat them as trusted operator input because yeet sends the provider token to the selected endpoint.

For custom domains, set `provider` explicitly. Automatic detection only recognizes `github.com`, `gitlab.com`, and `dev.azure.com`.

### GitHub Enterprise

For a configured custom host, yeet derives `https://<host>/api/v3/`. Override it when the API is elsewhere:

```sh
export GITHUB_TOKEN=github_pat_xxx
export GITHUB_URL=https://github.example.com/api/v3/
yeet release
```

### Self-managed GitLab

For a configured custom host, yeet derives `https://<host>/api/v4`. Override it for an instance under a relative path:

```sh
export GITLAB_TOKEN=glpat-xxx
export GITLAB_URL=https://example.com/gitlab/api/v4
yeet release
```

### Azure DevOps Server

Set the server URL explicitly, including any path prefix:

```sh
export AZURE_DEVOPS_EXT_PAT=xxx
export AZURE_DEVOPS_URL=https://devops.example.com/tfs
yeet release
```

## Related documentation

- [Documentation index](README.md)
- [CI setup](ci.md)
- [Troubleshooting](troubleshooting.md)
