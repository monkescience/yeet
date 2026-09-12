# Versioning

yeet uses top-level `versioning`, default `semver`. A monorepo target can override the strategy.

## Commit message format

yeet reads [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) in the form
`type(scope)!: description`, with an optional scope and `!`. Commit types are case-insensitive:
`feat:`, `Feat:`, and `FEAT:` all use the `feat` bump rules. Use lowercase types in configuration.
The authored scope and description retain their case.

A body or footer must be separated from the header by a blank line. If the second line is not
blank, yeet ignores the entire message for bump calculation and changelog generation, including
any `!` or `Release-As` signal. This also applies to entries inside commit override blocks.
For example:

```text
feat!: replace authentication API

Clients must use the new login endpoint.
```

A breaking footer must use uppercase `BREAKING CHANGE` or `BREAKING-CHANGE`, followed by a colon,
a space, and a nonempty description. The description may continue on subsequent lines, including
when the first line ends immediately after the separator space. Lowercase markers, missing
separator spaces, and whitespace-only descriptions do not mark a commit as breaking. A valid `!`
in the header still marks it as breaking even if a footer is malformed.

Use `--verbose` to see diagnostics for rejected message structure and breaking markers.

## Semantic Versioning (semver)

For versions at or above `1.0.0`:

| Commit | Default bump |
|---|---|
| `feat` | minor |
| `fix`, `perf` | patch |
| `!` or `BREAKING CHANGE` footer | major |

For versions below `1.0.0`, both pre-major options default to `true`:

| Commit | Default bump | Setting that restores normal semver behavior |
|---|---|---|
| `feat` | patch | `pre_major_features_bump_patch: false` makes it minor |
| `fix`, `perf` | patch | None |
| `!` or `BREAKING CHANGE` footer | minor | `pre_major_breaking_bumps_minor: false` makes it major |

These rules also apply to custom types in [Bump types](configuration.md#bump-types). Targets may override both pre-major settings.

### Release-As overrides

A `Release-As` commit footer overrides automatic semver calculation:

```text
chore: request a stable release

Release-As: 1.0.0
```

The value must be a stable semver version greater than the current version. The footer is case-insensitive and has no effect on calver targets. Every `Release-As` footer in one release must request the same version.

On a [prerelease channel](release.md#prerelease-channels), the footer selects the stable base and yeet adds the channel suffix, such as `1.0.0-beta.1`.

## Calendar Versioning (calver)

The default format is `YYYY.0M.MICRO`. `MICRO` increments within the selected calendar period and resets when that period changes.

Minimal configuration:

```yaml
versioning: calver
calver:
  format: YYYY.0M.0D.MICRO
```

| Component | Supported tokens |
|---|---|
| Year | `YYYY`, `YY`, `0Y` |
| Month | `MM`, `0M` |
| ISO week | `WW`, `0W` |
| Day | `DD`, `0D` |
| Counter | `MICRO` |

Tokens are dot-separated, the format includes exactly one year token, and `MICRO` is required as the final token. These combinations are incompatible:

- Week with month or day
- Day without month
- More than one token for the same calendar component

## Related documentation

- [Documentation index](README.md)
- [Configuration](configuration.md)
- [Changelog generation](changelog-generation.md)
- [Release PRs and MRs](release.md)
