# Authentication

yeet reads the provider token from an environment variable, never from flags or `.yeet.yaml`.

| Provider | Variable | Alternative |
|---|---|---|
| GitHub | `GITHUB_TOKEN` | `GH_TOKEN` |
| GitLab | `GITLAB_TOKEN` | `GL_TOKEN` |
| Azure DevOps | `AZURE_DEVOPS_SYSTEM_ACCESSTOKEN` (pipelines) | `AZURE_DEVOPS_EXT_PAT` (local) |

If both are set, the first one wins.

## GitHub

In GitHub Actions, use a GitHub App token, see the [workflow example](ci.md#github-actions).
Locally, a fine-grained personal access token works.

Grant these repository permissions:

| Permission | Access |
|---|---|
| Contents | Read and write |
| Pull requests | Read and write |
| Workflows | Write, only if yeet updates files under `.github/workflows/` |

The token must also be allowed to force-push the release branch (`yeet/release-*` by default) and, with auto-merge, to merge or enable auto-merge.

A classic token needs `repo` (or `public_repo` for public repositories). Add `read:org` for reviewers in an organization repository, and `workflow` for workflow files.

## GitLab

Use a project, group, or personal access token with the `api` scope and at least the Developer role.

If protected branch or tag rules match the release branch or release tags, allow the token to push and force-push the branch and to create the tags.
With auto-merge, it must also be allowed to merge into the base branch.

## Azure DevOps

In Azure Pipelines, map the job token:

```yaml
env:
  AZURE_DEVOPS_SYSTEM_ACCESSTOKEN: $(System.AccessToken)
```

Locally, use a PAT with Code (Read and write). Add Identity (Read) when `release.reviewers` is set.

Grant these permissions under Project settings > Repositories > the repository > Security.
For pipelines, grant them to `<project> Build Service (<organization>)`, or to `Project Collection Build Service (<organization>)` for collection-scoped jobs. For a PAT, grant them to its user.

- Read
- Contribute
- Contribute to pull requests
- Create branch
- Create tag
- Force push, at least on the release branch

## Self-hosted providers

Set `provider` in `.yeet.yaml`, see [Self-hosted providers](configuration.md#self-hosted-providers).
yeet derives the API URL from the host (`/api/v3/` for GitHub, `/api/v4` for GitLab).

To point yeet at a different API URL in one environment, set `GITHUB_URL`, `GITLAB_URL`, or `AZURE_DEVOPS_URL`:

```sh
export GITLAB_TOKEN=glpat-xxx
export GITLAB_URL=https://example.com/gitlab/api/v4
yeet release
```

yeet sends the token to that URL, so only set it in environments you control.

## Related documentation

- [CI setup](ci.md)
- [Troubleshooting](troubleshooting.md)
