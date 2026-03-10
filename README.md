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
# Initialize config in your repo
yeet init

# Preview what the next release would look like
yeet release --dry-run

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

yeet uses a `.yeet.toml` file in your project root. Run `yeet init` to generate one with defaults:

```toml
#:schema https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json

versioning = "semver"
branch = "main"
tag_prefix = "v"
# version_files = ["VERSION.txt"]

[changelog]
file = "CHANGELOG.md"
include = ["feat", "fix", "perf", "revert"]

[changelog.sections]
feat = "Features"
fix = "Bug Fixes"
perf = "Performance Improvements"
revert = "Reverts"
docs = "Documentation"
style = "Styles"
refactor = "Code Refactoring"
test = "Tests"
build = "Build System"
ci = "Continuous Integration"
chore = "Miscellaneous Chores"
breaking = "Breaking Changes"

[calver]
format = "YYYY.0M.MICRO"

[release]
subject_include_branch = false
auto_merge = false
auto_merge_force = false
auto_merge_method = "auto"
pr_body_header = "## ٩(^ᴗ^)۶ release created"
pr_body_footer = "_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._"
```

`yeet init` includes a TOML schema directive for editor validation and autocomplete.

### Editor schema

yeet publishes a JSON schema at `yeet.schema.json` for TOML-aware editors.

- Taplo/Even Better TOML reads the `#:schema ...` directive automatically.
- You can pin the schema URL to a release tag for stricter reproducibility.

### Options

| Key | Default | Description |
|---|---|---|
| `versioning` | `"semver"` | Versioning strategy: `"semver"` or `"calver"` |
| `branch` | `"main"` | Base branch for releases |
| `provider` | auto-detected | VCS provider: `"github"` or `"gitlab"` |
| `tag_prefix` | `"v"` | Prefix for version tags |
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

### `yeet init`

Creates a `.yeet.toml` with sensible defaults.

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
| `--auto-merge` | Automatically merge the release PR/MR and finalize the release in the same command run |
| `--auto-merge-force` | Attempt auto-merge while bypassing yeet readiness checks/approvals (still blocks draft/conflicts; implies `--auto-merge`; provider rules and permissions still apply) |
| `--auto-merge-method` | Merge strategy for auto-merge: `auto`, `squash`, `rebase`, or `merge` |

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

## Authentication

yeet needs a token to interact with the VCS provider API.

**GitHub**: Set `GITHUB_TOKEN` or `GH_TOKEN` environment variable. For GitHub Enterprise, also set `GITHUB_URL`.

**GitLab**: Set `GITLAB_TOKEN` or `GL_TOKEN` environment variable. For self-hosted instances, also set `GITLAB_URL`.

## CI examples

The published image includes `sh` and puts `yeet` on `PATH`, so CI jobs can run `yeet` directly.

### GitHub Actions

```yaml
name: Release

on:
  push:
    branches:
      - main

permissions:
  contents: write
  pull-requests: write
  issues: write

jobs:
  release:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/monkescience/yeet:vX.Y.Z
    steps:
      - name: Checkout
        uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - name: Run yeet
        run: yeet release
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

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

For GitLab, set `GITLAB_TOKEN` as a masked CI/CD variable. The `entrypoint: [""]` override is required so GitLab runs the job script with `sh` instead of the image's default `yeet` entrypoint.

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
## [v1.2.0](https://github.com/owner/repo/compare/v1.1.0...v1.2.0) (2026-02-28)

### ⚠ BREAKING CHANGES

- **api:** /v1/users replaced by /v2/users ([pqr1234](https://github.com/owner/repo/commit/pqr1234...))

### Features

- **auth:** add OAuth2 login ([abc1234](https://github.com/owner/repo/commit/abc1234...))
- add user preferences page ([def5678](https://github.com/owner/repo/commit/def5678...))

### Bug Fixes

- **api:** handle null response body ([ghi9012](https://github.com/owner/repo/commit/ghi9012...))
```

Version headers link to the compare diff, and commit hashes link to the individual commits. For the initial release (no previous tag), the version header is plain text.

## License

MIT
