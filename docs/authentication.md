# Authentication

yeet needs a provider API token whenever it creates or updates PRs/MRs, applies release labels, or
publishes releases.

## GitHub

Export either `GITHUB_TOKEN` or `GH_TOKEN` (`GITHUB_TOKEN` wins when both are set):

```sh
export GITHUB_TOKEN=ghp_xxx
yeet release --dry-run
```

For GitHub Enterprise, configure the repository host and yeet derives the API base URL as
`https://<host>/api/v3/`, or set `GITHUB_URL` explicitly:

```sh
export GITHUB_TOKEN=ghp_xxx
export GITHUB_URL=https://github.example.com/api/v3/
yeet release
```

A configured non-`github.com` host takes precedence over `GITHUB_URL`, so the env var only
applies when no custom host is configured.

The token needs `contents: write`, `pull-requests: write`, and `issues: write` permissions.

## GitLab

Export either `GITLAB_TOKEN` or `GL_TOKEN` (`GITLAB_TOKEN` wins when both are set):

```sh
export GITLAB_TOKEN=glpat-xxx
yeet release --dry-run
```

For self-hosted GitLab, configure the repository host and yeet derives the API base URL as
`https://<host>/api/v4`, or set `GITLAB_URL` explicitly:

```sh
export GITLAB_TOKEN=glpat-xxx
export GITLAB_URL=https://gitlab.example.com/api/v4
yeet release
```

As with GitHub, a configured non-`gitlab.com` host takes precedence over `GITLAB_URL`.

The token must be able to create merge requests, manage labels, and publish releases.

## Azure DevOps

In Azure Pipelines, map `System.AccessToken` to yeet's Azure DevOps-specific env var:

```yaml
env:
  AZURE_DEVOPS_SYSTEM_ACCESSTOKEN: $(System.AccessToken)
```

For local use or external CI, use the Azure DevOps CLI-compatible PAT variable:

```sh
export AZURE_DEVOPS_EXT_PAT=xxx
yeet release --dry-run
```

`AZURE_DEVOPS_SYSTEM_ACCESSTOKEN` is sent as bearer auth and takes precedence when both variables
are set. `AZURE_DEVOPS_EXT_PAT` is sent as basic auth.

For Azure DevOps Server/self-hosted, also set `AZURE_DEVOPS_URL`:

```sh
export AZURE_DEVOPS_EXT_PAT=xxx
export AZURE_DEVOPS_URL=https://devops.example.com
yeet release
```

Unlike GitHub and GitLab, `AZURE_DEVOPS_URL` takes precedence over the host configured in the
repository settings.

The pipeline build service identity or PAT needs repository permissions to create branches,
create/update pull requests, manage pull request labels, complete pull requests when auto-merge
is enabled, and create tags.

When `release.reviewers` is configured, the PAT additionally needs the Identity (Read) scope
(`vso.identity`), because reviewer names are resolved through the identities API. Whether a
pipeline `System.AccessToken` is authorized for identity reads (especially with "Limit job
authorization scope" enabled) has not been verified.
