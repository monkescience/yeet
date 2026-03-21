# yeet

Automate releases based on [conventional commits](https://www.conventionalcommits.org/). Analyzes commit history, calculates the next version, generates changelogs, creates release PRs/MRs, and finalizes merged releases on GitHub or GitLab.

Inspired by [release-please](https://github.com/googleapis/release-please).

## Install

```sh
go install github.com/monkescience/yeet/cmd/yeet@latest
```

Or use the published container image:

```sh
docker run --rm ghcr.io/monkescience/yeet:vX.Y.Z --help
```

For CI, prefer pinning a specific release tag or digest instead of `latest`.

## Quick start

```sh
# Show the current build metadata
yeet version

# Initialize config in your repo
yeet init

# Or write the config to a custom path
yeet init --config .yeet.release.yaml

# Generate shell completion for your environment
yeet completion zsh

# Emit structured logs for CI or local debugging
yeet --log-format json --verbose release --dry-run

# Preview what the next release would look like
yeet --verbose release --dry-run

# Preview build version for testing artifacts (for example Helm charts)
yeet release --preview --dry-run

# Create a release PR/MR
yeet release

# Optionally auto-merge and finalize in the same run
yeet release --auto-merge

# After the PR/MR is merged, run the same command on main
# (usually from CI) to create the tag/release automatically
yeet release
```

## Configuration

yeet reads the nearest ancestor `.yeet.yaml` by default. Run `yeet init` to generate one at the repository root when inside a git repository, or in the current directory otherwise. Pass `--config` to use an explicit path:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json

versioning: semver
branch: main
provider: auto
tag_prefix: v
# version_files:
#   - VERSION.txt

changelog:
  file: CHANGELOG.md
  include:
    - feat
    - fix
    - perf
    - revert
  sections:
    feat: Features
    fix: Bug Fixes
    perf: Performance Improvements
    revert: Reverts
    docs: Documentation
    style: Styles
    refactor: Code Refactoring
    test: Tests
    build: Build System
    ci: Continuous Integration
    chore: Miscellaneous Chores
    breaking: Breaking Changes

calver:
  format: YYYY.0M.MICRO

release:
  subject_include_branch: false
  auto_merge: false
  auto_merge_force: false
  auto_merge_method: auto
  pr_body_header: "## ٩(^ᴗ^)۶ release created"
  pr_body_footer: "_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._"
```

`yeet init` includes a YAML language server schema modeline for editor validation and autocomplete.

### Editor schema

yeet publishes a JSON schema at `yeet.schema.json` for YAML-aware editors.

- YAML language server clients support `# yaml-language-server: $schema=...` modelines.
- You can pin the schema URL to a release tag for stricter reproducibility.

### Options

| Key | Default | Description |
|---|---|---|
| `versioning` | `"semver"` | Versioning strategy: `"semver"` or `"calver"` |
| `branch` | `"main"` | Base branch for releases |
| `provider` | `"auto"` | VCS provider: `"auto"`, `"github"`, or `"gitlab"` |
| `tag_prefix` | `"v"` | Prefix for version tags |
| `repository.remote` | `"origin"` | Git remote name used for repository auto-detection |
| `repository.host` | unset | Explicit repository host, such as `github.com` or `gitlab.company.com` |
| `repository.owner` | unset | Explicit owner or namespace for GitHub-style repositories |
| `repository.repo` | unset | Explicit repository name for GitHub-style repositories |
| `repository.project` | unset | Explicit full GitLab project path, including subgroups |
| `version_files` | `[]` | Extra files to update with yeet markers during `yeet release` |
| `release.subject_include_branch` | `false` | Include the target branch in generated release subjects (for example `chore(main): release 0.1.0`) used for PR/MR titles and release branch commits |
| `release.auto_merge` | `false` | Automatically merge the release PR/MR and finalize the release in the same `yeet release` run |
| `release.auto_merge_force` | `false` | Force auto-merge attempt by skipping yeet readiness gates for checks/approvals while still blocking draft/conflicts (implies `release.auto_merge = true`; provider rules and permissions may still block merge) |
| `release.auto_merge_method` | `"auto"` | Merge method preference for auto-merge: `auto`, `squash`, `rebase`, or `merge` |
| `release.pr_body_header` | `"## ٩(^ᴗ^)۶ release created"` | Optional markdown inserted before the changelog in release PR/MR bodies |
| `release.pr_body_footer` | `"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._"` | Optional markdown appended after the changelog in release PR/MR bodies |
| `changelog.file` | `"CHANGELOG.md"` | Changelog file path |
| `changelog.include` | `["feat", "fix", "perf", "revert"]` | Commit types to include in the changelog |
| `changelog.sections` | see above | Mapping of commit types to section headings. All conventional types are pre-configured; only types in `include` appear in the changelog |
| `calver.format` | `"YYYY.0M.MICRO"` | CalVer format string |

### Version file markers

`yeet release` updates only files listed in `version_files`. Each file must contain yeet markers.

```txt
# inline markers
VERSION = "1.2.3" # x-yeet-version
MAJOR = 1 # x-yeet-major
MINOR = 2 # x-yeet-minor
PATCH = 3 # x-yeet-patch

# block markers
# x-yeet-start-version
image: ghcr.io/acme/app:1.2.3
appVersion: "1.2.3"
# x-yeet-end
```

Marker behavior mirrors release-please generic markers, but uses the `x-yeet-*` prefix.

For calver repositories, yeet also supports aliases:

- `x-yeet-year` (alias of `x-yeet-major`)
- `x-yeet-month` (alias of `x-yeet-minor`)
- `x-yeet-micro` (alias of `x-yeet-patch`)
- `x-yeet-start-year|month|micro` ... `x-yeet-end`

## Commands

### `yeet version`

Prints the CLI build metadata.

```sh
yeet version
```

### `yeet completion`

Generates shell completion using Cobra's built-in completion support.

```sh
yeet completion bash
yeet completion zsh
yeet completion fish
yeet completion powershell
```

### `yeet init`

Creates a `.yeet.yaml` with sensible defaults at the repository root when inside a git repository, or in the current directory otherwise.

```sh
yeet init
yeet init --config .yeet.release.yaml
```

### Global flags

| Flag | Description |
|---|---|
| `--config` | Use an explicit config file path instead of default discovery/location |
| `--log-format` | Set log output format to `text` or `json` |
| `--verbose` | Enable debug logging |
| `--quiet` | Show warnings and errors only |

`--verbose` and `--quiet` cannot be used together.

### `yeet release`

Analyzes conventional commits since the last release, calculates the next version, generates a changelog entry, and creates (or updates) a release PR/MR.

Before creating/updating PRs, `yeet release` also checks for merged release PRs/MRs labeled
`autorelease: pending`, creates the corresponding tag/release from the latest changelog entry,
and flips the label to `autorelease: tagged`.

When auto-merge is enabled (`release.auto_merge = true` or `--auto-merge`), yeet also merges the
newly created/updated release PR/MR and finalizes the release in the same run.

Force mode (`release.auto_merge_force = true` or `--auto-merge-force`) skips yeet readiness
gates for checks/approvals and still attempts the merge, but it still blocks draft PRs/MRs and
conflicted PRs/MRs. It does not guarantee bypass; GitHub/GitLab branch protections, required
checks, approvals, and token permissions can still block merge.

Merge strategy is configurable with `release.auto_merge_method` or `--auto-merge-method`.

- GitHub `auto` preference order: `squash` -> `rebase` -> `merge` based on repository settings.
- GitLab always uses the project merge method; `squash` toggles MR squash when allowed.

| Flag | Description |
|---|---|
| `--dry-run` | Preview the release without creating a PR/MR |
| `--preview` | Append build metadata with short commit hash (for example `1.2.4+abc1234`) |
| `--preview-hash-length` | Length of the preview hash suffix (default: `7`) |
| `--provider` | Override provider detection with `auto`, `github`, or `gitlab` |
| `--remote` | Override the git remote used for repository auto-detection |
| `--host` | Override the repository host |
| `--owner` | Override the owner or namespace for GitHub-style repositories |
| `--repo` | Override the repository name for GitHub-style repositories |
| `--project` | Override the full GitLab project path, including subgroups |
| `--auto-merge` | Automatically merge the release PR/MR and finalize the release in the same command run |
| `--auto-merge-force` | Attempt auto-merge while bypassing yeet readiness checks/approvals (still blocks draft/conflicts; implies `--auto-merge`; provider rules and permissions still apply) |
| `--auto-merge-method` | Merge strategy for auto-merge: `auto`, `squash`, `rebase`, or `merge` (defaults to the configured `release.auto_merge_method`; built-in default: `auto`) |

### Repository targeting

yeet resolves the target repository in this order:

1. `yeet release` flags such as `--provider`, `--host`, `--owner`, `--repo`, and `--project`
2. explicit `.yeet.yaml` values
3. the configured `repository.remote`
4. the `origin` remote

When yeet cannot classify a remote host automatically, set the provider and repository explicitly.

GitHub Enterprise config example:

```yaml
provider: github

repository:
  host: github.company.com
  owner: platform
  repo: yeet
```

GitLab subgroup config example:

```yaml
provider: gitlab

repository:
  host: gitlab.company.com
  project: group/subgroup/service
```

Equivalent one-off CLI overrides:

```sh
# GitHub Enterprise
yeet release --provider github --host github.company.com --owner platform --repo yeet --dry-run

# GitLab subgroup project
yeet release --provider gitlab --host gitlab.company.com --project group/subgroup/service --dry-run
```

Preview mode is useful for testing deploy artifacts before a final release tag:

```sh
# semver example
yeet release --preview --dry-run
# Next version: 1.2.4+abc1234

# calver example
yeet release --preview --dry-run
# Next version: 2026.03.1+abc1234
```

When preview mode is enabled, yeet keeps a stable release PR branch based on the target branch
(for example `yeet/release-main`) so new commits update the same PR.

Preview runs create or update the release PR/MR, but they do not auto-merge or create a provider
release even when `release.auto_merge = true`.

You can also force an explicit semver version using a commit footer:

```text
Release-As: 1.0.0
```

When present in commits since the last release, `Release-As` overrides calculated semver bumps.
If multiple different `Release-As` values are present, yeet fails and asks you to resolve the conflict.

Release PR/MR labels follow release-please style:

- `autorelease: pending` while a release PR/MR is open or updated
- `autorelease: tagged` after merge + successful tag/release creation

yeet expects exactly one open `autorelease: pending` PR/MR per base branch. If multiple
pending PRs/MRs exist, `yeet release` fails and prints the conflicting PR/MR URLs so you
can close or relabel stale entries.

## Release workflow

`yeet release` does slightly different work depending on repository state:

1. Before a release PR/MR exists, it scans conventional commits, calculates the next version,
   updates the changelog/version files, and opens a release PR/MR labeled `autorelease: pending`.
2. While that PR/MR is open, rerunning `yeet release` updates the same release branch instead of
   creating a second pending release.
3. After the release PR/MR is merged, the next non-preview `yeet release` run on the base branch
   creates the tag/provider release from the latest changelog entry and flips the label to
   `autorelease: tagged`.

That label lifecycle is operational, not decorative: yeet uses `autorelease: pending` to discover
merged releases that still need tagging, and it expects only one open pending release PR/MR per
base branch.

### Preview mode vs stable release mode

- Stable mode (`yeet release`) prepares the real next release and is the only mode that finalizes
  merged release PRs/MRs into provider tags/releases.
- Preview mode (`yeet release --preview`) appends `+<shortsha>` build metadata so you can test
  artifacts before cutting the stable tag.
- Preview mode keeps a stable release branch per target branch (for example `yeet/release-main`) so
  new commits update the same PR/MR.
- Preview mode never auto-merges or publishes a provider release, even if auto-merge is enabled.

### Auto-merge caveats

- `--auto-merge` or `release.auto_merge = true` merges the release PR/MR and finalizes the release
  in the same non-preview run when provider rules allow it.
- `--auto-merge-force` only skips yeet's own readiness gates; it does not bypass GitHub/GitLab
  branch protections, required checks, approvals, or missing permissions.
- On GitHub, `auto` tries `squash`, then `rebase`, then `merge`, based on which merge methods the
  repository has enabled.
- On GitLab, the project merge method still wins; requesting `squash` only toggles squash when the
  project allows it.

## Authentication

yeet needs a provider API token whenever it creates or updates PRs/MRs, applies release labels, or
publishes releases.

### GitHub local development or PAT-based CI

Export either `GITHUB_TOKEN` or `GH_TOKEN`:

```sh
export GITHUB_TOKEN=ghp_xxx
yeet release --dry-run
```

For GitHub Enterprise, also set `GITHUB_URL` to the API base URL or let yeet derive it from the
configured repository host:

```sh
export GITHUB_TOKEN=ghp_xxx
export GITHUB_URL=https://github.example.com/api/v3/
yeet release
```

Use a token with enough repository access to:

- create and update pull requests
- create releases and tags
- apply labels to release PRs

On GitHub that maps to `contents: write`, `pull-requests: write`, and `issues: write`.

### GitHub Actions with a GitHub App

This repository does not use the default `GITHUB_TOKEN` for releases. The workflow in
`.github/workflows/release.yaml` does three key things:

1. checks out the repository with full history
2. generates a short-lived GitHub App installation token
3. runs `go run ./cmd/yeet release` with that token in `GITHUB_TOKEN`

The workflow-level permissions are:

- `contents: write`
- `pull-requests: write`
- `issues: write`

The GitHub App installation needs equivalent repository permissions because yeet creates release
PRs, labels them, and publishes releases.

```yaml
name: Release

on:
  push:
    branches:
      - main
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write
  issues: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - name: Generate GitHub App token
        id: generate-token
        uses: actions/create-github-app-token@v2
        with:
          app-id: ${{ vars.RELEASE_PLEASE_APP_ID }}
          private-key: ${{ secrets.RELEASE_PLEASE_APP_PRIVATE_KEY }}
          owner: ${{ github.repository_owner }}

      - name: Run yeet release
        run: go run ./cmd/yeet release
        env:
          GITHUB_TOKEN: ${{ steps.generate-token.outputs.token }}
```

If you copy this pattern, store the app ID as a repository variable and the private key as a
repository secret.

### GitLab local development or CI

Export either `GITLAB_TOKEN` or `GL_TOKEN`:

```sh
export GITLAB_TOKEN=glpat-xxx
yeet release --dry-run
```

For self-hosted GitLab, also set `GITLAB_URL`:

```sh
export GITLAB_TOKEN=glpat-xxx
export GITLAB_URL=https://gitlab.example.com/api/v4
yeet release
```

The token must be able to create merge requests, manage labels, and publish releases.

## Troubleshooting

`yeet release` keeps wrapped errors for debugging, but the top-level message points at the failure
category so you can pick the next fix quickly:

- `configuration file not found`: create `.yeet.yaml` with `yeet init` at the repo root or pass `--config`.
- `invalid configuration`: fix invalid values in `.yeet.yaml` before rerunning.
- `repository resolution failed`: set `provider` and/or `[repository]` explicitly when the remote
  host is unsupported or auto-detection cannot classify it.
- `provider setup failed`: export the required token (`GITHUB_TOKEN`/`GH_TOKEN` or
  `GITLAB_TOKEN`/`GL_TOKEN`) and, for self-hosted providers, verify `GITHUB_URL` or `GITLAB_URL`.
- `release execution failed: merge blocked`: the release PR/MR is still draft, has conflicts,
  lacks required approvals/checks, or requests a merge method the provider settings do not allow.
- `release execution failed: multiple pending release PRs/MRs found`: close or relabel stale
  `autorelease: pending` entries until only one open release PR/MR remains for the base branch.

If a GitHub App workflow fails before yeet starts, verify `RELEASE_PLEASE_APP_ID`,
`RELEASE_PLEASE_APP_PRIVATE_KEY`, and the app's repository permissions.

## CI examples

The published image is suitable for CI, but this repository's own release workflow runs the Go
entrypoint directly so it can generate a fresh GitHub App token first.

After yeet publishes a stable provider release, `.github/workflows/image.yaml` builds and pushes the
container image for that release tag. Preview runs and open release PRs do not trigger image
publishing because the image workflow only runs for published, non-prerelease provider releases.

### GitLab CI

```yaml
release:
  stage: release
  image:
    name: ghcr.io/monkescience/yeet:vX.Y.Z
    entrypoint: [""]
  variables:
    GIT_STRATEGY: fetch
    GIT_DEPTH: "0"
  script:
    - yeet release
  rules:
    - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
```

For GitLab, set `GITLAB_TOKEN` as a masked CI/CD variable. The `entrypoint: [""]` override is
required so GitLab runs the job script with `sh` instead of the image's default `yeet` entrypoint.

## Versioning strategies

### Semantic Versioning (semver)

Follows [semver](https://semver.org/) with pre-1.0 scaling.

For versions `>= 1.0.0`:

- `feat` -> minor
- `fix`, `perf` -> patch
- Breaking changes (`!` or `BREAKING CHANGE` footer) -> major

For versions `< 1.0.0`:

- `feat` -> patch
- `fix`, `perf` -> patch
- Breaking changes (`!` or `BREAKING CHANGE` footer) -> minor

This keeps pre-1.0 breaking changes from automatically jumping to `1.0.0`.

`Release-As` commit footers (for example `Release-As: 1.0.0`) override automatic semver bumping.
The value must be a stable semver version greater than the current version.

### Calendar Versioning (calver)

Uses `YYYY.0M.MICRO` format (e.g., `2026.02.1`). The micro counter resets when the year/month changes.

## Conventional commits

yeet parses commits following the [conventional commits](https://www.conventionalcommits.org/) specification:

```
type(scope): description

optional body

optional footer(s)
```

Breaking changes are indicated by a `!` after the type/scope or a `BREAKING CHANGE:` footer.

You can force a specific semver release version with:

```text
Release-As: 1.2.0
```

`Release-As` is case-insensitive and applies only to semver repositories. CalVer repositories ignore it.

## Changelog format

yeet generates changelogs that match the [release-please](https://github.com/googleapis/release-please) style:

```markdown
# Changelog

## [v1.2.0](https://github.com/owner/repo/compare/v1.1.0...v1.2.0) (2026-02-28)

### ⚠ BREAKING CHANGES

- **api:** /v1/users replaced by /v2/users ([pqr1234](https://github.com/owner/repo/commit/pqr1234...))

### Features

- **auth:** add OAuth2 login ([abc1234](https://github.com/owner/repo/commit/abc1234...))
- add user preferences page ([def5678](https://github.com/owner/repo/commit/def5678...))

### Bug Fixes

- **api:** handle null response body ([ghi9012](https://github.com/owner/repo/commit/ghi9012...))
```

yeet creates or normalizes `CHANGELOG.md` with a top-level `# Changelog` header. Version headers link to the compare diff, and commit hashes link to the individual commits. For the initial release (no previous tag), the version header is plain text.

## License

MIT
