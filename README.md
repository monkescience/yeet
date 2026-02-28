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

[changelog]
file = "CHANGELOG.md"
include = ["feat", "fix", "perf"]

[changelog.sections]
feat = "Features"
fix = "Bug Fixes"
perf = "Performance Improvements"
breaking = "Breaking Changes"

[calver]
format = "YYYY.0M.MICRO"
```

### Options

| Key | Default | Description |
|---|---|---|
| `versioning` | `"semver"` | Versioning strategy: `"semver"` or `"calver"` |
| `branch` | `"main"` | Base branch for releases |
| `provider` | auto-detected | VCS provider: `"github"` or `"gitlab"` |
| `tag_prefix` | `"v"` | Prefix for version tags |
| `changelog.file` | `"CHANGELOG.md"` | Changelog file path |
| `changelog.include` | `["feat", "fix", "perf"]` | Commit types to include in the changelog |
| `changelog.sections` | see above | Mapping of commit types to section headings |
| `calver.format` | `"YYYY.0M.MICRO"` | CalVer format string |

## Commands

### `yeet init`

Creates a `.yeet.toml` with sensible defaults.

### `yeet release`

Analyzes conventional commits since the last release, calculates the next version, generates a changelog entry, and creates (or updates) a release PR/MR.

| Flag | Description |
|---|---|
| `--dry-run` | Preview the release without creating a PR/MR |

### `yeet tag`

Creates a git tag and VCS release after a release PR/MR has been merged.

| Flag | Description |
|---|---|
| `--tag` | The tag to create (required) |
| `--changelog` | Changelog body for the release |

## Authentication

yeet needs a token to interact with the VCS provider API.

**GitHub**: Set `GITHUB_TOKEN` or `GH_TOKEN` environment variable.

**GitLab**: Set `GITLAB_TOKEN` or `GL_TOKEN` environment variable. For self-hosted instances, also set `GITLAB_URL`.

## Versioning strategies

### Semantic Versioning (semver)

Follows [semver](https://semver.org/). Bump type is determined from commits:

- `feat` -> minor
- `fix`, `perf` -> patch
- Breaking changes (`!` or `BREAKING CHANGE` footer) -> major

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

## License

MIT
