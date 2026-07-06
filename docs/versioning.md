# Versioning

yeet supports two versioning strategies, selected with the top-level `versioning` key (default `semver`). Targets can override it individually in monorepo setups.

## Semantic Versioning (semver)

Follows [semver](https://semver.org/) with configurable pre-1.0 behavior.

For versions `>= 1.0.0`:

- `feat` -> minor
- `fix`, `perf` -> patch
- Breaking changes (`!` or `BREAKING CHANGE` footer) -> major

For versions `< 1.0.0` (default behavior with `pre_major_breaking_bumps_minor: true` and `pre_major_features_bump_patch: true`):

- `feat` -> patch
- `fix`, `perf` -> patch
- Breaking changes (`!` or `BREAKING CHANGE` footer) -> minor

These type-to-bump defaults are configurable via `bump_types` (see [Bump types](configuration.md#bump-types)).

This keeps pre-1.0 breaking changes from automatically jumping to `1.0.0`.

Set `pre_major_breaking_bumps_minor: false` to let breaking changes bump major (triggering 1.0.0),
or `pre_major_features_bump_patch: false` to let features bump minor as they do post-1.0.
These options can also be overridden per target in monorepo configurations.

### Release-As overrides

`Release-As` commit footers (for example `Release-As: 1.0.0`) override automatic semver bumping.
The value must be a stable semver version greater than the current version. `Release-As` is
case-insensitive and applies only to semver repositories. Calver repositories ignore it.

Commits in the same release must agree on the version: two commits with different `Release-As`
values fail the run.

On a [prerelease channel](release.md#prerelease-channels), `Release-As` sets the stable base
version and the channel run produces `<version>-<channel>.1`.

## Calendar Versioning (calver)

Uses `YYYY.0M.MICRO` format by default (e.g., `2026.02.1`). The `MICRO` counter increments within the configured calendar period and resets when that period changes.

Configure the format globally or per target:

```yaml
versioning: calver
calver:
  format: YYYY.0M.0D.MICRO
```

Supported date tokens are `YYYY`, `YY`, `0Y`, `MM`, `0M`, `WW`, `0W`, `DD`, and `0D`. `MICRO` is required as the final token so multiple releases in the same calendar period can produce unique versions. Tokens must be dot-separated. Week tokens cannot be combined with month or day tokens, and day tokens require a month token.
