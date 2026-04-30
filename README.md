# yeet

Automate releases based on [conventional commits](https://www.conventionalcommits.org/). Analyzes commit history, calculates the next version, generates changelogs, creates release PRs/MRs, and finalizes merged releases on GitHub or GitLab.

Inspired by [release-please](https://github.com/googleapis/release-please).

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
go install github.com/monkescience/yeet/cmd/yeet@v0.7.0 # x-yeet-version
```

Or use the published container image:

```sh
docker run --rm ghcr.io/monkescience/yeet:v0.7.0 --help # x-yeet-version
```

## Quick start

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

Run `yeet --help` for the full list of commands and flags.

## How it works

`yeet release` does slightly different work depending on repository state:

1. Before a release PR/MR exists, it scans conventional commits, calculates the next version,
   updates the changelog/version files, and opens a release PR/MR labeled `autorelease: pending`.
2. While that PR/MR is open, rerunning `yeet release` updates the same release branch instead of
   creating a second pending release.
3. After the release PR/MR is merged, the next `yeet release` run on the base branch
   creates the tag/provider release from the committed changelog entry and flips the label to
   `autorelease: tagged`.

Final GitHub/GitLab release notes are read from the matching `CHANGELOG.md` entry. To customize
release notes, edit that changelog entry on the release PR/MR branch; the PR/MR body is only a
regenerated preview and guidance surface.

That label lifecycle is operational, not decorative: yeet uses `autorelease: pending` to discover
merged releases that still need tagging, and it expects only one open pending release PR/MR per
base branch. If multiple pending PRs/MRs exist, `yeet release` fails and prints the conflicting
URLs so you can close or relabel stale entries.

When auto-merge is enabled (`--auto-merge` or `release.auto_merge` in config), yeet merges the
release PR/MR and finalizes the release in the same run. Force mode (`--auto-merge-force`) skips
yeet's own readiness gates but does not bypass provider branch protections, required checks,
approvals, or missing permissions.

## Versioning strategies

### Semantic Versioning (semver)

Follows [semver](https://semver.org/) with configurable pre-1.0 behavior.

For versions `>= 1.0.0`:

- `feat` -> minor
- `fix`, `perf` -> patch
- Breaking changes (`!` or `BREAKING CHANGE` footer) -> major

For versions `< 1.0.0` (default behavior with `pre_major_breaking_bumps_minor: true` and `pre_major_features_bump_patch: true`):

- `feat` -> patch
- `fix`, `perf` -> patch
- Breaking changes (`!` or `BREAKING CHANGE` footer) -> minor

These type-to-bump defaults are configurable via `bump_types` (see [Bump types](#bump-types)).

This keeps pre-1.0 breaking changes from automatically jumping to `1.0.0`.

Set `pre_major_breaking_bumps_minor: false` to let breaking changes bump major (triggering 1.0.0),
or `pre_major_features_bump_patch: false` to let features bump minor as they do post-1.0.
These options can also be overridden per target in monorepo configurations.

`Release-As` commit footers (for example `Release-As: 1.0.0`) override automatic semver bumping.
The value must be a stable semver version greater than the current version. `Release-As` is
case-insensitive and applies only to semver repositories; calver repositories ignore it.

### Calendar Versioning (calver)

Uses `YYYY.0M.MICRO` format by default (e.g., `2026.02.1`). The `MICRO` counter increments within the configured calendar period and resets when that period changes.

Configure the format globally or per target:

```yaml
versioning: calver
calver:
  format: YYYY.0M.0D.MICRO
```

Supported date tokens are `YYYY`, `YY`, `0Y`, `MM`, `0M`, `WW`, `0W`, `DD`, and `0D`. `MICRO` is required as the final token so multiple releases in the same calendar period can produce unique versions. Tokens must be dot-separated; week tokens cannot be combined with month or day tokens, and day tokens require a month token.

## Configuration

yeet reads the nearest ancestor `.yeet.yaml` by default. Run `yeet init` to generate one with sensible defaults, or pass `--config` to write to a custom path. The generated file includes a YAML language server schema modeline for editor validation and autocomplete.

All available options, defaults, and descriptions are defined in the [JSON schema](yeet.schema.json). YAML-aware editors that support `# yaml-language-server: $schema=...` modelines will provide validation and autocomplete automatically. You can pin the schema URL to a release tag for stricter reproducibility.

### Repository targeting

yeet resolves the target repository from these sources, highest priority first:

1. CLI flags (`--provider`, `--host`, `--owner`, `--repo`, `--project`)
2. explicit `.yeet.yaml` values under `repository:`
3. the configured `repository.remote`
4. the `origin` remote

Automatic provider detection intentionally only classifies the public hosts `github.com` and
`gitlab.com`. For custom or enterprise domains, set the provider explicitly; this avoids
sending provider tokens to an arbitrary host based only on hostname text. Repository host and
path are discovered from `repository.remote`/`origin`; set `repository:` only when overriding
remote discovery or when no usable remote exists:

```yaml
# GitHub Enterprise
provider: github
```

```yaml
# GitLab self-managed
provider: gitlab
```

### Targets

yeet plans releases per target and creates one combined release PR/MR per base branch.
PR workflow settings remain top-level under `release:` and apply to the combined PR/MR, not individual targets.

Use `--target` to limit `yeet release` to specific targets (repeatable).

```yaml
targets:
  api:
    type: path
    path: services/api
    tag_prefix: api-v
    exclude_paths:
      - services/api/testdata

  web:
    type: path
    path: apps/web
    tag_prefix: web-v

  root:
    type: derived
    includes:
      - api
      - web
    path: .              # optional: also matches commits at repo root
    tag_prefix: v
```

Path targets support `exclude_paths` to ignore commits under specific subdirectories.
Derived targets aggregate included path targets and optionally match direct commits via `path`.

### Bump types

By default, `feat` commits bump minor and `fix`/`perf` commits bump patch. Override this mapping with `bump_types`:

```yaml
bump_types:
  minor:
    - feat
    - improvement
  patch:
    - fix
    - perf
    - deps
```

Types not listed produce no version bump. Breaking changes always bump major regardless of this mapping.

### Version files

`yeet release` updates only files listed in `version_files`. Each file must contain yeet markers.

```txt
# inline markers
VERSION = "0.7.0" # x-yeet-version
MAJOR = 0 # x-yeet-major
MINOR = 7 # x-yeet-minor
PATCH = 0 # x-yeet-patch

# block markers
# x-yeet-start-version
image: ghcr.io/acme/app:0.7.0
appVersion: "0.7.0"
# x-yeet-end
```

For calver repositories, yeet also supports aliases:

- `x-yeet-year` (alias of `x-yeet-major`)
- `x-yeet-month` (alias of `x-yeet-minor`)
- `x-yeet-micro` (alias of `x-yeet-patch`)
- `x-yeet-start-year|month|micro` for calver block markers
- `x-yeet-end` closes the block

### Changelog

yeet generates a changelog from conventional commits. Configure which commit types appear and how sections are labeled:

```yaml
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
```

Only types listed in `include` appear in the changelog. The `sections` map controls their heading text.

#### References

yeet can link issue tracker references in generated changelogs. References are extracted from two sources: inline patterns matched in commit descriptions, and conventional commit footers.

```yaml
changelog:
  references:
    patterns:
      - pattern: "JIRA-\\d+"
        url: "https://jira.example.com/browse/{value}"
      - pattern: "#\\d+"
        url: ""  # plain text, GitHub auto-links these
    footers:
      Refs: "https://jira.example.com/browse/{value}"
      Closes: ""
```

**Inline patterns** match against the commit description using regex and replace matches with links. A commit like `feat: add OAuth2 support JIRA-123` produces:

```
- add OAuth2 support [JIRA-123](https://jira.example.com/browse/JIRA-123) (abc1234)
```

**Footer references** extract values from conventional commit footers and append them after the commit hash. A commit with a `Refs: JIRA-456` footer produces:

```
- add OAuth2 support (abc1234) ([JIRA-456](https://jira.example.com/browse/JIRA-456))
```

Use `{value}` as the placeholder in URL templates. An empty URL string renders the reference as plain text without linking. Both `patterns` and `footers` can be configured per target in monorepo setups.

#### Commit overrides

If a merged PR/MR has a vague squash or merge commit message, edit that source PR/MR body to add override entries:

```md
BEGIN_COMMIT_OVERRIDE
feat(auth): add OAuth token refresh

fix(api): return 401 for expired sessions
END_COMMIT_OVERRIDE
```

When yeet analyzes the merge/squash commit, those conventional commit messages replace the commit message for version bumping and changelog generation. The generated changelog still links to the original commit hash.

This can split one merged commit into multiple release notes, or introduce a breaking change:

```md
BEGIN_COMMIT_OVERRIDE
feat(auth)!: replace session cookie format

BREAKING CHANGE: existing session cookies are invalid after upgrade
END_COMMIT_OVERRIDE
```

Commit overrides are read from the original merged PR/MR body. Manual edits to the generated release PR/MR body may be overwritten the next time yeet updates the release branch. Rebase-merged PRs are not overridden because one PR/MR can produce many commits and the association is ambiguous.

### Release PR/MR customization

```yaml
release:
  subject_include_branch: true       # include target branch in PR/MR subject
  pr_body_header: "## Release"       # markdown before changelog in PR/MR body
  pr_body_footer: "_Automated._"     # markdown after changelog in PR/MR body
```

Generated release PRs/MRs include a changelog preview and guidance. The PR/MR body may be
regenerated by `yeet release`, so do not use it for final release-note edits.

To customize final GitHub/GitLab release notes, edit the matching generated entry in
`CHANGELOG.md` on the release branch. You can add any markdown section, for example:

````md
### Migration Notes

Run database migrations before deploying workers.
````

When `yeet release` updates an existing release PR/MR, yeet preserves manual changelog sections
that are not part of the regenerated conventional-commit sections.

Migration note: older yeet versions used `BEGIN_YEET_RELEASE_NOTES` / `END_YEET_RELEASE_NOTES`
markers in the PR/MR body. Those blocks are now ignored during finalization; move custom notes into
the committed changelog entry before merging the release PR/MR.

### Prerelease channels

Prerelease channels are branch-scoped and semver-only. Configure each channel under `release.channels`:

```yaml
branch: main

release:
  channels:
    beta:
      branch: beta
      prerelease: beta
    rc:
      branch: rc
      prerelease: rc
```

On `main`, `yeet release` runs the stable release flow. On `beta`, it creates or updates a beta release PR/MR targeting `beta`; after that PR/MR is merged, the next run creates a provider prerelease such as `v1.3.0-beta.1`. Stable releases ignore prerelease tags when choosing the stable baseline.

Prerelease channels write to channel-specific changelogs by default, so stable `CHANGELOG.md` entries stay clean. For `CHANGELOG.md`, beta writes `CHANGELOG.beta.md`; for `services/api/CHANGELOG.md`, beta writes `services/api/CHANGELOG.beta.md`. Version files are still updated to the prerelease version on the channel branch.

`yeet release` fails on branches that are not configured as `branch` or a `release.channels.<name>.branch`. Use `--dry-run` for exploratory runs from other branches, or pass `--channel beta` to explicitly select a configured channel.

## Authentication

yeet needs a provider API token whenever it creates or updates PRs/MRs, applies release labels, or
publishes releases.

### GitHub

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

The token needs `contents: write`, `pull-requests: write`, and `issues: write` permissions.

### GitLab

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

## CI examples

### GitHub Actions with a GitHub App

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
        uses: docker://ghcr.io/monkescience/yeet:v0.7.0 # x-yeet-version
        with:
          args: release
        env:
          GITHUB_TOKEN: ${{ steps.generate-token.outputs.token }}
```

### GitLab CI

Set `GITLAB_TOKEN` as a masked CI/CD variable. The `entrypoint: [""]` override is required so
GitLab runs the job script with `sh` instead of the image's default `yeet` entrypoint.

```yaml
release:
  stage: release
  image:
    name: ghcr.io/monkescience/yeet:v0.7.0 # x-yeet-version
    entrypoint: [""]
  variables:
    GIT_STRATEGY: fetch
    GIT_DEPTH: "0"
  script:
    - yeet release
  rules:
    - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
```

## Troubleshooting

`yeet release` keeps wrapped errors for debugging, but the top-level message points at the failure
category so you can pick the next fix quickly:

- `configuration file not found`: create `.yeet.yaml` with `yeet init` at the repo root or pass `--config`.
- `invalid configuration`: fix invalid values in `.yeet.yaml` before rerunning.
- `repository resolution failed`: set `provider` explicitly for custom or enterprise hosts; set
  `repository` too when remote discovery cannot provide the host and path.
- `provider setup failed`: export the required token (`GITHUB_TOKEN`/`GH_TOKEN` or
  `GITLAB_TOKEN`/`GL_TOKEN`) and, for self-hosted providers, verify `GITHUB_URL` or `GITLAB_URL`.
- `release execution failed: merge blocked`: the release PR/MR is still draft, has conflicts,
  lacks required approvals/checks, or requests a merge method the provider settings do not allow.
- `release execution failed: multiple pending release PRs/MRs found`: close or relabel stale
  `autorelease: pending` entries until only one open release PR/MR remains for the base branch.
