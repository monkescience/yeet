# Authentication

yeet reads provider credentials from environment variables. It never accepts tokens through flags or `.yeet.yaml`.

| Provider | Variables, in precedence order | Minimum token or app access |
|---|---|---|
| GitHub | `GITHUB_TOKEN`, `GH_TOKEN` | Contents (write) and Pull requests (write) |
| GitLab | `GITLAB_TOKEN`, `GL_TOKEN` | `api` scope and Developer role |
| Azure DevOps | `AZURE_DEVOPS_SYSTEM_ACCESSTOKEN`, `AZURE_DEVOPS_EXT_PAT` | Code (read and write) for PATs, plus repository permissions |

## GitHub

Export either token variable. `GITHUB_TOKEN` wins when both are set.

```sh
export GITHUB_TOKEN=github_pat_xxx
yeet release --dry-run
```

For GitHub Actions, use a GitHub App installation token. See the complete [GitHub Actions example](ci.md#github-actions-with-a-github-app).

For a GitHub App or fine-grained personal access token, grant these repository permissions:

| Permission | Access | Used for |
|---|---|---|
| Contents | Read and write | Reading and updating release files and branches, then creating tags and releases |
| Pull requests | Read and write | Creating, updating, labeling, requesting reviewers for, and merging release pull requests |

Add Workflows (write) when a configured `version_files` or changelog path is under `.github/workflows/`.

Install the App on each repository it releases. Allow it to create and force-update the generated release branch. With auto-merge, allow it to merge into the base branch after required checks, approvals, and branch rules pass.

A classic personal access token needs `repo` for a private repository or `public_repo` for a public repository. For an organization-owned repository with `release.reviewers`, use `repo` and `read:org` even when the repository is public. Add `workflow` when yeet changes a workflow file. See [GitHub's REST permission table](https://docs.github.com/en/rest/authentication/permissions-required-for-github-apps).

## GitLab

Export either token variable. `GITLAB_TOKEN` wins when both are set.

```sh
export GITLAB_TOKEN=glpat-xxx
yeet release --dry-run
```

A project access token needs the `api` scope and Developer role under Settings > Access tokens. A group or personal access token needs the `api` scope and an identity with at least the Developer role in the project.

Repository access requirements:

- If a protected branch rule matches the generated release branch, allow the token identity to push and enable force pushes for that rule.
- If a protected tag rule matches a release tag, add the token role or identity to Allowed to create.
- With auto-merge, the target branch must allow the token identity to merge after its checks and approvals pass.

See GitLab's documentation for [`api` scope](https://docs.gitlab.com/security/tokens/access_token_scopes/), [project roles](https://docs.gitlab.com/user/permissions/), [protected branches](https://docs.gitlab.com/user/project/repository/branches/protected/), and [protected tags](https://docs.gitlab.com/user/project/protected_tags/).

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

For a PAT, select Code (Read and write), `vso.code_write`. When `release.reviewers` is configured, also select Identity (Read), `vso.identity`. The PAT user also needs the repository permissions below.

`System.AccessToken` uses a pipeline build service identity. For a project-scoped job token, grant the permissions below to `<project> Build Service (<organization>)`. For a collection-scoped token, grant them to `Project Collection Build Service (<organization>)`.

Set repository permissions under Project settings > Repositories > the target repository > Security:

| Permission | Scope | Used for |
|---|---|---|
| Read | Repository | Reading files, refs, tags, and pull requests |
| Contribute | Repository | Pushing release commits and completing pull requests |
| Contribute to pull requests | Repository | Creating, updating, and labeling pull requests |
| Create branch | Repository | Creating the generated release branch |
| Create tag | Repository | Publishing a release tag |
| Force push | Generated release branch | Resetting the release branch to the current base before each refresh |

The build service needs effective Force push permission on the generated release branch, either inherited when it creates the branch or granted explicitly. With auto-merge, it needs Contribute permission on the target branch and all branch policies must pass.

Authorization for identity reads through `System.AccessToken` depends on the organization and job authorization settings. Verify it when `release.reviewers` is configured. See Azure DevOps documentation for [job access tokens](https://learn.microsoft.com/en-us/azure/devops/pipelines/process/access-tokens), [token scopes](https://learn.microsoft.com/en-us/azure/devops/integrate/get-started/authentication/oauth#oauth-scopes), and [Git repository permissions](https://learn.microsoft.com/en-us/azure/devops/organizations/security/permissions#git-repository-object-level).

## Host trust

yeet trusts a repository host only when it is a recognized public provider host, matches the Git remote host, or matches the provider's environment URL override. A checked-in `repository.*.host` cannot redirect a token by itself.

Each provider subsection accepts checked-in `api_url` and `web_url` settings. Both require HTTPS. `api_url` must use the resolved repository hostname because yeet sends credentials to it. `web_url` controls links only and may use another HTTPS hostname.

`GITHUB_URL`, `GITLAB_URL`, and `AZURE_DEVOPS_URL` override a configured `api_url` after repository resolution. They take precedence even for public hosts. Treat them as trusted operator input because yeet sends the provider token to the selected endpoint. A configured `web_url` still controls generated links. Without `web_url`, links keep their legacy derivation from the environment override.

For custom domains, set `provider` explicitly. Automatic detection only recognizes `github.com`, `gitlab.com`, and `dev.azure.com`.

### GitHub Enterprise

For a configured custom host, yeet derives `https://<host>/api/v3/`. Configure a path prefix in `.yeet.yaml`:

```yaml
provider: github
repository:
  github:
    host: github.example.com
    api_url: https://github.example.com/root/api/v3/
    web_url: https://github.example.com/root
    owner: acme
    repo: widgets
```

An environment override remains available for operator-controlled routing:

```sh
export GITHUB_TOKEN=github_pat_xxx
export GITHUB_URL=https://github.example.com/api/v3/
yeet release
```

### Self-managed GitLab

For a configured custom host, yeet derives `https://<host>/api/v4`. Use `repository.gitlab.api_url` and `web_url` for a checked-in relative path, or override it for the current environment:

```sh
export GITLAB_TOKEN=glpat-xxx
export GITLAB_URL=https://example.com/gitlab/api/v4
yeet release
```

### Azure DevOps Server

Use `repository.azuredevops.api_url` and `web_url` for a checked-in server path, or set the server URL explicitly for the current environment:

```sh
export AZURE_DEVOPS_EXT_PAT=xxx
export AZURE_DEVOPS_URL=https://devops.example.com/tfs
yeet release
```

## Related documentation

- [Documentation index](README.md)
- [CI setup](ci.md)
- [Troubleshooting](troubleshooting.md)
