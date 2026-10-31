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

```yaml
versioning: calver
calver:
  format: YYYY.0M.0D.MICRO
```

| Component | Tokens |
|---|---|
| Year | `YYYY`, `YY`, `0Y` |
| Month | `M`, `0M` |
| ISO week | `W`, `0W` |
| Day | `D`, `0D` |
| Counter | `MICRO` |

A format needs exactly one year token and must end with `MICRO`.
Tokens starting with `0` are zero-padded.
`MM`, `WW`, and `DD` are deprecated aliases for `M`, `W`, and `D`. They are not zero-padded, and a format cannot use an alias together with its short form.
Week cannot be combined with month or day, and day requires month.

Weeks follow ISO 8601 and start on Monday.
In a week format, the year token is the ISO week-year, so the last days of December can belong to week 1 of the next year, and the first days of January to week 52 or 53 of the previous year.
For example, with `YYYY.0W.MICRO` a release on 1 January 2027 is `2026.53.1`.

Dates follow the configured [timezone](configuration.md#timezone).

## Related documentation

- [Writing commits](commits.md)
- [Configuration](configuration.md)
