# yeet

[![CI](https://github.com/monkescience/yeet/actions/workflows/ci.yaml/badge.svg)](https://github.com/monkescience/yeet/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/monkescience/yeet)](https://github.com/monkescience/yeet/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/monkescience/yeet)](go.mod)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/monkescience/yeet/badge)](https://scorecard.dev/viewer/?uri=github.com/monkescience/yeet)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/13506/badge)](https://www.bestpractices.dev/projects/13506)
[![License](https://img.shields.io/github/license/monkescience/yeet)](LICENSE)

Release automation for **GitHub, GitLab, and Azure DevOps**, driven by [conventional commits](https://www.conventionalcommits.org/).

yeet analyzes commit history, calculates the next version (semver or calver), generates changelogs, opens a release PR/MR, and tags the release when it merges. It ships as a single static binary with no runtime dependencies.

## Why yeet

yeet provides a release-PR workflow across GitHub, GitLab, and Azure DevOps.

- Single binary and container image
- GitHub Enterprise, self-managed GitLab, and Azure DevOps Server support
- Semver and calver versioning
- Monorepo targets with one combined release PR/MR
- Built-in auto-merge and reviewer assignment
- Configurable changelogs, issue links, and version-file updates
- Branch-scoped prerelease channels
- One YAML configuration file with a JSON schema

Self-hosted setup is covered in [Authentication](docs/authentication.md).

## Install

```sh
brew install monkescience/tap/yeet
```

Or on Windows with [Scoop](https://scoop.sh):

```sh
scoop bucket add monkescience https://github.com/monkescience/scoop-bucket
scoop install yeet
```

Or with Go:

```sh
go install github.com/monkescience/yeet/cmd/yeet@v0.11.5 # x-yeet-version
```

Or use the published container image:

```sh
docker run --rm ghcr.io/monkescience/yeet:v0.11.5 --help # x-yeet-version
```

Shell completions are available via `yeet completion bash|zsh|fish|powershell`.

## Verify a release

Release archives and the container image are signed with [Sigstore](https://www.sigstore.dev)
keyless signing, and both carry GitHub build provenance attestations.

Verify an archive against the `.sigstore.json` bundle published next to it:

```sh
cosign verify-blob \
  --bundle yeet_linux_amd64.tar.gz.sigstore.json \
  --certificate-identity-regexp 'https://github.com/monkescience/yeet/.github/workflows/binaries.yaml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  yeet_linux_amd64.tar.gz
```

Verify the container image signature:

```sh
cosign verify \
  --certificate-identity-regexp 'https://github.com/monkescience/yeet/.github/workflows/image.yaml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/monkescience/yeet:v0.11.5 # x-yeet-version
```

Verify build provenance (which workflow and commit produced the artifact) with the GitHub CLI:

```sh
gh attestation verify yeet_linux_amd64.tar.gz --repo monkescience/yeet
gh attestation verify oci://ghcr.io/monkescience/yeet:v0.11.5 --repo monkescience/yeet # x-yeet-version
```

## Quick start

yeet talks to the provider API, so export a token first (`GITHUB_TOKEN`, `GITLAB_TOKEN`, or `AZURE_DEVOPS_EXT_PAT`, see [Authentication](docs/authentication.md)).

```sh
# Initialize config in your repo
yeet init

# Preview what the next release would look like
yeet release --dry-run

# Create a release PR/MR
yeet release

# Auto-merge and finalize in the same run
yeet release --auto-merge
```

## Command reference

| Command | Purpose |
|---|---|
| `yeet init` | Create a `.yeet.yaml` configuration file |
| `yeet release` | Preview or perform the release workflow |
| `yeet version` | Print build and version information |
| `yeet completion` | Generate shell completion scripts |

Run `yeet --help` for global flags and `yeet <command> --help` for each
command's inputs, options, environment variables, and examples. The
[configuration guide](docs/configuration.md) covers common settings and examples.
The [JSON schema](yeet.schema.json) is the complete reference for every `.yeet.yaml`
setting, default, and description.

## How it works

`yeet release` does slightly different work depending on repository state:

1. Before a release PR/MR exists, it scans conventional commits, calculates the next version,
   updates the changelog/version files, and opens a release PR/MR with the configured pending label
   (default `autorelease: pending`) and the managed `yeet` label. The managed label can be disabled
   with `release.labels.yeet: false`.
2. While that PR/MR is open, rerunning `yeet release` updates the same release branch instead of
   creating a second pending release.
3. After the release PR/MR is merged, the next `yeet release` run on the base branch
   creates the tag/provider release from the committed changelog entry and flips the label to
   the configured tagged label (default `autorelease: tagged`).

Final release notes are read from the matching `CHANGELOG.md` entry. To customize
release notes, edit that changelog entry on the release PR/MR branch. The PR/MR body itself is
regenerated by `yeet release` and should not be used for final release-note edits.

That label lifecycle is operational, not decorative: yeet uses the configured pending label to discover
merged releases that still need tagging, and it expects only one open pending release PR/MR per
base branch. If multiple pending PRs/MRs exist, `yeet release` fails and prints the conflicting
URLs so you can close or relabel stale entries.

When auto-merge is enabled (`--auto-merge` or `release.auto_merge` in config), yeet merges the
release PR/MR and finalizes the release in the same run. Force mode (`--auto-merge-force`) skips
yeet's own readiness gates but does not bypass provider branch protections, required checks,
approvals, or missing permissions.

## Documentation

Getting started:

- [Authentication](docs/authentication.md): tokens and self-hosted setup per provider
- [CI setup](docs/ci.md): GitHub Actions, GitLab CI, and Azure Pipelines examples
- [Migrating from release-please](docs/migrate-from-release-please.md): config mapping and switch-over steps
- [Troubleshooting](docs/troubleshooting.md): error categories and debug logging

Customization:

- [Configuration](docs/configuration.md): config file, repository targeting, monorepo targets, bump types, version files
- [Versioning](docs/versioning.md): semver, calver, and `Release-As` overrides
- [Changelog generation](docs/changelog-generation.md): sections, issue tracker references, commit overrides
- [Release PRs/MRs](docs/release.md): labels, title templates, body and merge settings, release notes, prerelease channels

## Feedback and contributions

- [Report a bug](https://github.com/monkescience/yeet/issues/new?template=bug_report.yaml)
- [Request an enhancement](https://github.com/monkescience/yeet/issues/new?template=feature_request.yaml)
- [Contribute a change](CONTRIBUTING.md)
- [Report a vulnerability privately](SECURITY.md)
