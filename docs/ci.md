# CI setup

yeet needs the full commit history, so configure checkouts with fetch depth 0, and a provider
token (see [Authentication](authentication.md)).

yeet determines the current branch from `GITHUB_REF_NAME`, `CI_COMMIT_BRANCH`, or `BRANCH_NAME`
(checked in that order) before falling back to git, so detached-HEAD checkouts in CI resolve to
the right branch.

## GitHub Actions with a GitHub App

This example uses a GitHub App installation token instead of the default `GITHUB_TOKEN`.
The app needs `contents: write`, `pull-requests: write`, and `issues: write` repository permissions.
Store the app ID as a repository variable and the private key as a repository secret.

```yaml
name: Release

on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: write
  issues: write
  pull-requests: write

concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6
        with:
          fetch-depth: 0

      - name: Generate GitHub App token
        id: generate-token
        uses: actions/create-github-app-token@1b10c78c7865c340bc4f6099eb2f838309f1e8c3 # v3
        with:
          app-id: ${{ vars.YEET_APP_ID }}
          private-key: ${{ secrets.YEET_APP_PRIVATE_KEY }}
          owner: ${{ github.repository_owner }}

      - name: Run yeet
        uses: docker://ghcr.io/monkescience/yeet:v0.10.11 # x-yeet-version
        with:
          args: release
        env:
          GITHUB_TOKEN: ${{ steps.generate-token.outputs.token }}
```

## GitLab CI

Set `GITLAB_TOKEN` as a masked CI/CD variable. The `entrypoint: [""]` override is required so
GitLab runs the job script with `sh` instead of the image's default `yeet` entrypoint.

```yaml
release:
  stage: release
  image:
    name: ghcr.io/monkescience/yeet:v0.10.11 # x-yeet-version
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

This example uses Azure Pipelines `System.AccessToken`. Map it explicitly into the step env.
yeet sends it to Azure DevOps as bearer auth.

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
    fetchTags: true

  - script: |
      docker run --rm \
        -v "$(Build.SourcesDirectory):/workspace" \
        -w /workspace \
        -e AZURE_DEVOPS_SYSTEM_ACCESSTOKEN \
        ghcr.io/monkescience/yeet:v0.10.11 release # x-yeet-version
    displayName: Run yeet
    env:
      AZURE_DEVOPS_SYSTEM_ACCESSTOKEN: $(System.AccessToken)
```

## Performance

For monorepos with many targets, yeet fetches commit history and per-commit changed paths from the
provider API in parallel. The number of concurrent requests per batch defaults to 8.

Set `YEET_MAX_CONCURRENT_REQUESTS` to a higher positive integer to speed up large runs, for example
on self-hosted instances with higher rate limits:

```sh
export YEET_MAX_CONCURRENT_REQUESTS=20
yeet release
```

Raising it too far can trip provider rate limits. yeet retries rate-limited requests with backoff, so
start conservatively and increase only if runs are still slow. A value that is not a positive integer
fails the run.
