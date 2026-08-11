# Versioning

yeet uses top-level `versioning`, default `semver`. A monorepo target can override the strategy.

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
