# Versioning

Set `versioning` to `semver` (default) or `calver`.
Monorepo targets can each choose their own strategy.

## Semver

From `1.0.0` onward:

| Commit | Bump |
|---|---|
| `feat` | minor |
| `fix`, `perf` | patch |
| Breaking (`!` or `BREAKING CHANGE`) | major |

Before `1.0.0`, bumps are one step smaller by default:

| Commit | Bump | To use normal semver |
|---|---|---|
| `feat` | patch | `pre_major_features_bump_patch: false` |
| `fix`, `perf` | patch | |
| Breaking | minor | `pre_major_breaking_bumps_minor: false` |

Other commit types do not bump the version unless they are breaking.
To change which types bump, see [Bump types](configuration.md#bump-types).
To pick a version yourself, use a [`Release-As` footer](commits.md#force-a-version).

## Calver

The default format is `YYYY.0M.MICRO`, for example `2026.09.3`.
`MICRO` counts releases within the period and resets when the period changes.

On a [prerelease channel](release.md#prerelease-channels), yeet adds the channel suffix to the version the next stable release would get, such as `2026.09.4-beta.1`.
The counter increases while that version stays the same and restarts at 1 when it changes, such as after a stable release or in a new period.
A stable release in the same period then takes that version, such as `2026.09.4`.

```yaml
versioning: calver
calver:
  format: YYYY.0M.0D.MICRO
```

| Component | Tokens |
|---|---|
| Year | `YYYY`, `YY`, `0Y` |
| Month | `MM`, `0M` |
| ISO week | `WW`, `0W` |
| Day | `DD`, `0D` |
| Counter | `MICRO` |

A format needs exactly one year token and must end with `MICRO`.
Tokens starting with `0` are zero-padded.
Week cannot be combined with month or day, and day requires month.

Dates follow the configured [timezone](configuration.md#timezone).

### Version label

To put a fixed label on every stable version, for example during a public beta, set `calver.label`:

```yaml
versioning: calver
calver:
  label: beta
```

Versions then look like `2026.09.1-beta` and `2026.09.2-beta`.
Labeled versions are normal releases: `MICRO` counts them, and the provider release is not marked as a prerelease.
To release without the label, remove it. `MICRO` continues from the latest tag, such as `2026.09.3` after `2026.09.2-beta`.

A label contains letters, digits, and hyphens, with at least one letter, and no dot.
So a tag such as `v2026.09.2-beta` is a labeled release, and `v2026.09.2-beta.1` is a prerelease.
Prerelease channel versions do not carry the label.

## Related documentation

- [Writing commits](commits.md)
- [Configuration](configuration.md)
