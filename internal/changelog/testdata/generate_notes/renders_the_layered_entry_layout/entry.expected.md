## [v2.0.0](https://github.com/org/repo/compare/v1.4.0...v2.0.0) (2026-09-25)

Units replace targets. Most configs need a one-line rename.

### ⚠ BREAKING CHANGES

- **config:** rename targets to units ([abc1234](https://github.com/org/repo/commit/abc1234))
  > Rename `targets` to `units` in `yeet.yaml`:
  >
  > ```yaml
  > units:
  >   - path: .
  > ```
- drop Go 1.24 support ([def5678](https://github.com/org/repo/commit/def5678))
- remove legacy provider flag ([1a2b3c4](https://github.com/org/repo/commit/1a2b3c4))

### Features

- **release:** add auto-merge ([0a1b2c3](https://github.com/org/repo/commit/0a1b2c3))
  > Set `release.auto_merge: true` to merge release PRs automatically.
- **config:** rename targets to units ([abc1234](https://github.com/org/repo/commit/abc1234))
- drop Go 1.24 support ([def5678](https://github.com/org/repo/commit/def5678))
