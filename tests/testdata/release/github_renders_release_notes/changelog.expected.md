# Changelog

## [v2.0.0](http://127.0.0.1:{{regex `\d+`}}/testorg/testrepo/compare/v1.0.0...v2.0.0) ({{regex `\d{4}-\d{2}-\d{2}`}})

### ⚠ BREAKING CHANGES

- drop legacy flag ([{{regex `[0-9a-f]{7}`}}](http://127.0.0.1:{{regex `\d+`}}/testorg/testrepo/commit/{{regex `[0-9a-f]{40}`}}))
  > Remove `--legacy`.
  > Use `--mode` instead.
- **config:** rename targets to units ([{{regex `[0-9a-f]{7}`}}](http://127.0.0.1:{{regex `\d+`}}/testorg/testrepo/commit/{{regex `[0-9a-f]{40}`}}))
  > Rename `targets` to `units` in `.yeet.yaml`:
  >
  > ```yaml
  > units:
  >   - path: .
  > ```

### Features

- **release:** add auto-merge ([{{regex `[0-9a-f]{7}`}}](http://127.0.0.1:{{regex `\d+`}}/testorg/testrepo/commit/{{regex `[0-9a-f]{40}`}})) (#12)
  > Set `release.auto_merge: true` to merge release PRs automatically.
- **config:** rename targets to units ([{{regex `[0-9a-f]{7}`}}](http://127.0.0.1:{{regex `\d+`}}/testorg/testrepo/commit/{{regex `[0-9a-f]{40}`}}))

### Bug Fixes

- drop legacy flag ([{{regex `[0-9a-f]{7}`}}](http://127.0.0.1:{{regex `\d+`}}/testorg/testrepo/commit/{{regex `[0-9a-f]{40}`}}))
