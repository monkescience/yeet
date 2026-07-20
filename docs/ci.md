# CI setup

yeet needs the full commit history, so configure checkouts with fetch depth 0, and a provider
token (see [Authentication](authentication.md)).

For branch-triggered jobs, yeet determines the current branch from `GITHUB_REF`,
`GITHUB_REF_NAME`, `CI_COMMIT_BRANCH`, `BRANCH_NAME`, or `BUILD_SOURCEBRANCH` (checked in that
order) before falling back to git. Full GitHub and Azure refs must start with `refs/heads/`.
GitHub Actions and Azure Pipelines pull request and tag refs are rejected rather than treated as
release branches.

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
        uses: docker://ghcr.io/monkescience/yeet:v0.10.20 # x-yeet-version
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
    name: ghcr.io/monkescience/yeet:v0.10.20 # x-yeet-version
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
        -e BUILD_SOURCEBRANCH \
        ghcr.io/monkescience/yeet:v0.10.20 release # x-yeet-version
    displayName: Run yeet
    env:
      AZURE_DEVOPS_SYSTEM_ACCESSTOKEN: $(System.AccessToken)
```

## Local checkout requirement

yeet reads commit history and per-commit changed paths from the local git checkout instead of the
provider API. This makes release analysis fast even for monorepos with many targets, but the
checkout must faithfully represent the release branch:

- Full history: no shallow clone (`fetch-depth: 0`, `GIT_DEPTH "0"`, or `fetchDepth: 0` as shown in
  the checkout examples above).
- Tags fetched (`fetchTags: true` on Azure Pipelines; included in full fetches elsewhere).
- HEAD at the current remote head of the configured release branch.

yeet validates the checkout against the provider before analyzing and fails with an actionable
error when it is shallow, stale, or on the wrong branch.
