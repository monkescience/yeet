# yeet

Automate releases based on [conventional commits](https://www.conventionalcommits.org/). Analyzes commit history, calculates the next version, generates changelogs, and creates release PRs/MRs on GitHub or GitLab.

Inspired by [release-please](https://github.com/googleapis/release-please).

## Install

```sh
go install github.com/monkescience/yeet/cmd/yeet@latest
```

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

# After the PR/MR is merged, tag the release
yeet tag --tag v1.2.0
```

## Configuration

yeet uses a `.yeet.toml` file in your project root. Run `yeet init` to generate one with defaults:

```toml
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
pr_body_header = "## ٩(^ᴗ^)۶ release created"
pr_body_footer = "_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._"
```

### Options

| Key | Default | Description |
|---|---|---|
| `versioning` | `"semver"` | Versioning strategy: `"semver"` or `"calver"` |
| `branch` | `"main"` | Base branch for releases |
| `provider` | auto-detected | VCS provider: `"github"` or `"gitlab"` |
| `tag_prefix` | `"v"` | Prefix for version tags |
| `version_files` | `[]` | Extra files to update with yeet markers during `yeet release` |
| `release.subject_include_branch` | `false` | Include the target branch in generated release subjects (for example `chore(main): release 0.1.0`) used for PR/MR titles and release branch commits |
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

| Flag | Description |
|---|---|
| `--dry-run` | Preview the release without creating a PR/MR |
| `--preview` | Append build metadata with short commit hash (for example `1.2.4+abc1234`) |
| `--preview-hash-length` | Length of the preview hash suffix (default: `7`) |

Preview mode is useful for testing deploy artifacts before a final release tag:

```sh
# semver example
yeet release --preview --dry-run
# Next version: 1.2.4+abc1234

# calver example
yeet release --preview --dry-run
# Next version: 2026.03.1+abc1234
```

When preview mode is enabled, yeet keeps a stable release PR branch based on the base version
(for example `yeet/release-v1.2.4`) so new commits update the same PR.

You can also force an explicit semver version using a commit footer:

```text
Release-As: 1.0.0
```

When present in commits since the last release, `Release-As` overrides calculated semver bumps.
If multiple different `Release-As` values are present, yeet fails and asks you to resolve the conflict.

### `yeet tag`

Creates a git tag and VCS release after a release PR/MR has been merged.

| Flag | Description |
|---|---|
| `--tag` | The tag to create (required) |
| `--changelog` | Changelog body for the release |

`yeet tag` rejects preview-style tags (for example `v1.2.4+abc1234` or `v1.2.4-rc.1`) so tags stay reserved for final releases.

## Authentication

yeet needs a token to interact with the VCS provider API.

**GitHub**: Set `GITHUB_TOKEN` or `GH_TOKEN` environment variable. For GitHub Enterprise, also set `GITHUB_URL`.

**GitLab**: Set `GITLAB_TOKEN` or `GL_TOKEN` environment variable. For self-hosted instances, also set `GITLAB_URL`.

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
