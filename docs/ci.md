# CI setup

yeet reads commit history and changed paths from the local checkout. Every provider pipeline therefore needs full history and a checkout at the current remote head of the release branch. yeet rejects shallow, stale, pull request, and tag checkouts before release analysis.

For branch-triggered jobs, yeet reads `GITHUB_REF`, `GITHUB_REF_NAME`, `CI_COMMIT_BRANCH`, `BRANCH_NAME`, or `BUILD_SOURCEBRANCH`, in that order, before falling back to git. Full GitHub and Azure refs must start with `refs/heads/`.

The copyable examples use a release tag so yeet can keep them synchronized through `x-yeet-version`. If your policy requires an immutable image reference, resolve the digest for that exact tag and append `@sha256:<digest>`. Update the tag and digest together. When they disagree, container tooling follows the digest and can silently run the older image.

## GitHub Actions with a GitHub App

The workflow token stays read-only. The GitHub App installation provides `contents`, `pull-requests`, and `issues` write access. Install the App on the release repository, store its client ID as the `YEET_APP_ID` repository variable, and store its private key as the `YEET_APP_PRIVATE_KEY` repository secret.

```yaml
name: Release

on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: yeet-release-${{ github.ref }}
  cancel-in-progress: false

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
        with:
          fetch-depth: 0

      - name: Generate GitHub App token
        id: generate-token
        uses: actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1 # v3
        with:
          client-id: ${{ vars.YEET_APP_ID }}
          private-key: ${{ secrets.YEET_APP_PRIVATE_KEY }}
          owner: ${{ github.repository_owner }}
          repositories: ${{ github.event.repository.name }}

      - name: Run yeet
        uses: docker://ghcr.io/monkescience/yeet:v0.13.4 # x-yeet-version
        with:
          args: release
        env:
          GITHUB_TOKEN: ${{ steps.generate-token.outputs.token }}
```

## GitLab CI

Set `GITLAB_TOKEN` as a masked CI/CD variable. The empty entrypoint lets GitLab run the job script with `sh`.

```yaml
release:
  stage: release
  image:
    name: ghcr.io/monkescience/yeet:v0.13.4 # x-yeet-version
    entrypoint: [""]
  variables:
    GIT_STRATEGY: fetch
    GIT_DEPTH: "0"
  script:
    - yeet release
  rules:
    - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
```

## Azure Pipelines

This pipeline maps `System.AccessToken` explicitly and forwards the branch ref into the container.

```yaml
trigger:
  branches:
    include:
      - main

pr: none

pool:
  vmImage: ubuntu-latest

steps:
  - checkout: self
    fetchDepth: 0

  - script: |
      docker run --rm \
        -v "$(Build.SourcesDirectory):/workspace" \
        -w /workspace \
        -e AZURE_DEVOPS_SYSTEM_ACCESSTOKEN \
        -e BUILD_SOURCEBRANCH \
        ghcr.io/monkescience/yeet:v0.13.4 release # x-yeet-version
    displayName: Run yeet
    env:
      AZURE_DEVOPS_SYSTEM_ACCESSTOKEN: $(System.AccessToken)
```

## Local checkout requirement

| Provider | Full-history setting | Release branch signal | Credential mapping |
|---|---|---|---|
| GitHub Actions | `fetch-depth: 0` | `GITHUB_REF` or `GITHUB_REF_NAME` | App token to `GITHUB_TOKEN` |
| GitLab CI | `GIT_DEPTH: "0"` | `CI_COMMIT_BRANCH` | Masked `GITLAB_TOKEN` |
| Azure Pipelines | `fetchDepth: 0` | `BUILD_SOURCEBRANCH` | `System.AccessToken` to `AZURE_DEVOPS_SYSTEM_ACCESSTOKEN` |

The checkout must match the provider's current remote head. A full history alone does not make a stale or detached checkout usable.

## Related documentation

- [Documentation index](README.md)
- [Authentication](authentication.md)
- [Troubleshooting](troubleshooting.md)
