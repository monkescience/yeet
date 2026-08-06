# Migrating from release-please

yeet is designed so a release-please repository can switch without re-tagging or rewriting history:

- The current version is read from existing git tags. When `tag_prefix` matches your existing tag
  format (`v` for `v1.2.3`, `api-v` for `api-v1.2.3`), yeet continues from the last
  release-please release.
- yeet defaults to the same `autorelease: pending` and `autorelease: tagged` labels, so no label
  cleanup is needed when those defaults are retained.
- Commit overrides use the same `BEGIN_COMMIT_OVERRIDE` / `END_COMMIT_OVERRIDE` markers, but yeet
  only reads them from the final git commit message. Existing blocks stored only in PR bodies do
  not work unless the provider copies them into that commit message.

yeet only recognizes release PRs created from its own `yeet/release-*` branches, so it never
picks up or modifies release-please PRs, even when they carry the shared labels.

## Steps

1. Finish in-flight release-please work: merge and tag the open release PR with release-please,
   or close it. yeet ignores release-please PRs, but an open one goes stale once the
   release-please workflow is removed.
2. Remove the release-please workflow, `release-please-config.json`, and
   `.release-please-manifest.json`. yeet needs no manifest, versions come from tags.
3. Run `yeet init` and port your settings using the mapping below.
4. Rename version markers in extra files from `x-release-please-*` to `x-yeet-*`.
5. Set up the token and workflow (see [Authentication](authentication.md) and
   [CI setup](ci.md)).
6. Verify with `yeet release --dry-run`. The reported current version should match your latest
   release-please tag.

## Config mapping

| release-please (`release-please-config.json`) | yeet (`.yeet.yaml`) |
|---|---|
| `packages.<path>` | `targets.<name>` with `type: path` and `path: <path>` |
| `include-v-in-tag`, `include-component-in-tag`, `tag-separator` | `tag_prefix` per target (e.g. `v`, `api-v`) |
| `bump-minor-pre-major` | `pre_major_breaking_bumps_minor` (default `true`) |
| `bump-patch-for-minor-pre-major` | `pre_major_features_bump_patch` (default `true`) |
| `changelog-path` | `changelog.file` |
| `changelog-sections` | `changelog.include` plus `changelog.sections` |
| `extra-files` | `version_files` (markers, or `format: json` with `json_pointer`) |
| `pull-request-header` / `pull-request-footer` | `release.pr_body_header` / `release.pr_body_footer` |
| `label` | `release.labels.pending` |
| `release-label` | `release.labels.tagged` |
| `extra-label` | `release.labels.extra` |
| `pull-request-title-pattern` | `release.pr_title` |
| `group-pull-request-title-pattern` | `release.pr_title_group` |
| `release-as` config option | `Release-As` commit footer only (see [Versioning](versioning.md#release-as-overrides)) |
| `prerelease` | `release.channels`, branch-scoped (see [Release PRs/MRs](release.md#prerelease-channels)) |
| `.release-please-manifest.json` | not needed, current versions come from git tags |
| `separate-pull-requests` | not supported, yeet always opens one combined PR/MR per base branch |

Translate release-please title patterns to Go `text/template` syntax. See
[Subject templates](release.md#subject-templates) for the available values. Change lifecycle label
names only after all in-flight release PRs/MRs have been closed or finalized.

yeet has no language-specific `release-type` strategies. Files that release-please updated
through a release type (for example `package.json`) must be listed explicitly in
`version_files`, using `format: json` with a `json_pointer` for JSON files (see
[Version files](configuration.md#version-files)).

release-please `extra-files` entries with a `jsonpath` map to `json_pointer` only for JSON
files. For YAML, TOML, and XML files, place yeet comment markers next to the value instead
(`#`, `#`, and `<!-- -->` respectively), since those formats support comments.

## Marker mapping

| release-please | yeet |
|---|---|
| `x-release-please-version` | `x-yeet-version` |
| `x-release-please-major` / `-minor` / `-patch` | `x-yeet-major` / `-minor` / `-patch` |
| `x-release-please-start-version` | `x-yeet-start-version` |
| `x-release-please-end` | `x-yeet-end` |
