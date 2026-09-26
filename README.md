# yeet

[![CI](https://github.com/monkescience/yeet/actions/workflows/ci.yaml/badge.svg)](https://github.com/monkescience/yeet/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/monkescience/yeet)](https://github.com/monkescience/yeet/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/monkescience/yeet)](go.mod)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/monkescience/yeet/badge)](https://scorecard.dev/viewer/?uri=github.com/monkescience/yeet)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/13506/badge)](https://www.bestpractices.dev/projects/13506)
[![License](https://img.shields.io/github/license/monkescience/yeet)](LICENSE)

Automate releases on **GitHub, GitLab, or Azure DevOps** from your [conventional commits](https://www.conventionalcommits.org/).

yeet calculates the next semver or calver version, updates your changelog, opens a release PR/MR, and publishes the release once you merge it.
It is a single static binary that you run in CI on every push to your release branch.

- One workflow for GitHub, GitLab, and Azure DevOps, including self-hosted installations
- Single repositories and monorepos
- Semver or calver, prerelease channels, and optional auto-merge
- One YAML file with a JSON schema for editor autocompletion

## Quick start

1. **Install and preview locally.**

   ```sh
   brew install monkescience/tap/yeet
   export GITHUB_TOKEN=github_pat_xxx # or GITLAB_TOKEN, AZURE_DEVOPS_EXT_PAT
   yeet init
   yeet release --dry-run
   ```

   `yeet init` creates `.yeet.yaml`. See [Authentication](docs/authentication.md) for token permissions.

2. **Add the release pipeline.** Copy the example for your provider from [CI setup](docs/ci.md), then commit it together with `.yeet.yaml`.

3. **Release.** Merge a `feat`, `fix`, or `perf` commit into the release branch.
   CI opens a release PR/MR. Merge it, and the next CI run creates the tag and the release.

## Install

```sh
brew install monkescience/tap/yeet
go install github.com/monkescience/yeet/cmd/yeet@v0.16.2 # x-yeet-version
docker run --rm ghcr.io/monkescience/yeet:v0.16.2 --help # x-yeet-version
```

On Windows, use [Scoop](https://scoop.sh):

```sh
scoop bucket add monkescience https://github.com/monkescience/scoop-bucket
scoop install yeet
```

Run `yeet completion bash|zsh|fish|powershell` for shell completions and `yeet --help` for all commands and flags.
To check a download, see [Artifact verification](docs/verification.md).

## Documentation

| Guide | Covers |
|---|---|
| [Authentication](docs/authentication.md) | Tokens, permissions, and self-hosted providers |
| [CI setup](docs/ci.md) | GitHub Actions, GitLab CI, and Azure Pipelines examples |
| [Configuration](docs/configuration.md) | `.yeet.yaml`, monorepo targets, and version files |
| [Writing commits](docs/commits.md) | Commit format, breaking changes, and release notes |
| [Versioning](docs/versioning.md) | Semver and calver rules |
| [Changelog](docs/changelog-generation.md) | Sections and issue links |
| [Release PRs and MRs](docs/release.md) | Labels, reviewers, templates, and prerelease channels |
| [Auto-merge](docs/auto-merge.md) | Merging release PRs/MRs automatically |
| [Troubleshooting](docs/troubleshooting.md) | Recovering from failed releases |
| [Migrating from release-please](docs/migrate-from-release-please.md) | Moving an existing repository to yeet |
| [Telemetry](docs/telemetry.md) | What anonymous usage data is sent and how to turn it off |

The complete `.yeet.yaml` reference is [`yeet.schema.json`](yeet.schema.json).

## Feedback and contributions

- [Report a bug](https://github.com/monkescience/yeet/issues/new?template=bug_report.yaml)
- [Request an enhancement](https://github.com/monkescience/yeet/issues/new?template=feature_request.yaml)
- [Contribute a change](CONTRIBUTING.md)
- [Report a vulnerability privately](SECURITY.md)
