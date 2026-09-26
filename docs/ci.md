# CI setup

Run `yeet release` on every push to the release branch.
The job needs a full-history checkout (no shallow clone) of the latest commit on that branch.
yeet refuses to run on pull request and tag checkouts.

If a newer push lands while a run is in progress, yeet skips that run with a warning, and the run for the newer commit releases it.

## GitHub Actions

Create a GitHub App with the [required permissions](authentication.md#github) and install it on the repository.
Store its client ID as the `YEET_APP_ID` variable and its private key as the `YEET_APP_PRIVATE_KEY` secret.

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
        uses: docker://ghcr.io/monkescience/yeet:v0.17.0 # x-yeet-version
        with:
          args: release
        env:
          GITHUB_TOKEN: ${{ steps.generate-token.outputs.token }}
```

## GitLab CI

Create a token with the [required access](authentication.md#gitlab) and store it as a masked `GITLAB_TOKEN` CI/CD variable.
Only mark it protected if the job always runs on a protected branch.

```yaml
release:
  stage: release
  image:
    name: ghcr.io/monkescience/yeet:v0.17.0 # x-yeet-version
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

Grant the pipeline's build service the [required repository permissions](authentication.md#azure-devops).

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
        ghcr.io/monkescience/yeet:v0.17.0 release # x-yeet-version
    displayName: Run yeet
    env:
      AZURE_DEVOPS_SYSTEM_ACCESSTOKEN: $(System.AccessToken)
```

## Pinning the image

To pin the image by digest, append `@sha256:<digest>` for the same tag, and update both together when you upgrade.

## Related documentation

- [Authentication](authentication.md)
- [Troubleshooting](troubleshooting.md)
