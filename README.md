# yeet

[![CI](https://github.com/monkescience/yeet/actions/workflows/ci.yaml/badge.svg)](https://github.com/monkescience/yeet/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/monkescience/yeet)](https://github.com/monkescience/yeet/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/monkescience/yeet)](go.mod)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/monkescience/yeet/badge)](https://scorecard.dev/viewer/?uri=github.com/monkescience/yeet)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/13506/badge)](https://www.bestpractices.dev/projects/13506)
[![License](https://img.shields.io/github/license/monkescience/yeet)](LICENSE)

Automate releases on **GitHub, GitLab, or Azure DevOps** from your [conventional commits](https://www.conventionalcommits.org/).

With yeet, you can calculate the next semver or calver version, generate a changelog, open a release PR/MR, and publish the release after it merges. yeet ships as a single static binary with no runtime dependencies.

yeet is designed to run from CI on every push to a release branch. The local CLI is the setup and preview tool. Your CI workflow performs the recurring release work.

## Quick start

### 1. Preview locally

For a GitHub repository, install yeet, export a token, generate the default config, and preview the release:

```sh
brew install monkescience/tap/yeet
export GITHUB_TOKEN=github_pat_xxx
yeet init
yeet release --dry-run
```

Use `GITLAB_TOKEN` or `AZURE_DEVOPS_EXT_PAT` for another provider. See [Authentication](docs/authentication.md) for token permissions and CI variables.

`yeet init` creates only `.yeet.yaml`. The generated config is provider-neutral and auto-detects the public provider from the Git remote.

### 2. Automate it in CI

Add the matching release pipeline from [CI setup](docs/ci.md). GitHub repositories normally use the [GitHub Actions workflow](docs/ci.md#github-actions-with-a-github-app). GitLab and Azure DevOps use the equivalent examples on that page.

Commit `.yeet.yaml` and the workflow. From then on, CI runs `yeet release` on pushes to the configured release branch.

### 3. Create the first release

Merge a `feat`, `fix`, or `perf` conventional commit into the release branch. CI opens the release PR/MR. Merge that release, then the next CI run creates the tag and provider release.

## Install

Other installation options:

```sh
go install github.com/monkescience/yeet/cmd/yeet@v0.14.11 # x-yeet-version
docker run --rm ghcr.io/monkescience/yeet:v0.14.11 --help # x-yeet-version
```

On Windows, install with [Scoop](https://scoop.sh):

```sh
scoop bucket add monkescience https://github.com/monkescience/scoop-bucket
scoop install yeet
```

Shell completions are available through `yeet completion bash|zsh|fish|powershell`.

## Why yeet

- One release workflow across GitHub, GitLab, and Azure DevOps
- Monorepo targets with combined or independent release PRs/MRs
- Semver, calver, changelog, reviewer, and prerelease configuration
- GitHub Enterprise, self-managed GitLab, and Azure DevOps Server support
- One YAML file backed by a JSON schema

## Command reference

| Command | Purpose |
|---|---|
| `yeet init` | Create a `.yeet.yaml` configuration file |
| `yeet release` | Preview or perform the release workflow |
| `yeet version` | Print build and version information |
| `yeet completion` | Generate shell completion scripts |

Run `yeet --help` or `yeet <command> --help` for generated CLI help. The [configuration guide](docs/configuration.md) covers common tasks, and [`yeet.schema.json`](yeet.schema.json) is the complete field reference.

## How it works

1. **Plan:** yeet analyzes conventional commits and opens a labelled release PR/MR.
2. **Refresh:** later runs update that open release instead of creating another one.
3. **Finalize:** after merge, the next run creates the tag and provider release, then marks the PR/MR as tagged.

See [Release PRs and MRs](docs/release.md) for lifecycle, labels, auto-merge, reviewers, release notes, and prerelease channels.

## Verify a release

Archives and container images are signed with Sigstore and carry GitHub build provenance. Follow [Artifact verification](docs/verification.md) for the expected identities and copyable verification commands.

## Documentation

| Task | Guide |
|---|---|
| Add provider CI | [CI setup](docs/ci.md) |
| Configure a repository or monorepo | [Configuration](docs/configuration.md) |
| Customize versions and changelogs | [Versioning](docs/versioning.md) and [Changelog generation](docs/changelog-generation.md) |
| Review or disable anonymous analytics | [Telemetry](docs/telemetry.md) |
| Recover from a failed release | [Troubleshooting](docs/troubleshooting.md) |
| Migrate from release-please | [Migration guide](docs/migrate-from-release-please.md) |

Open the [complete documentation index](docs/README.md) to find every task and advanced guide.

## Feedback and contributions

- [Report a bug](https://github.com/monkescience/yeet/issues/new?template=bug_report.yaml)
- [Request an enhancement](https://github.com/monkescience/yeet/issues/new?template=feature_request.yaml)
- [Contribute a change](CONTRIBUTING.md)
- [Report a vulnerability privately](SECURITY.md)
