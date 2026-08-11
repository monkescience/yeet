# Migrating from release-please

## Steps

1. Finish the open release-please PR and tag its release, or close it.
2. Remove the release-please workflow, `release-please-config.json`, and `.release-please-manifest.json`.
3. Run `yeet init`, then port settings with the table below.
4. Rename extra-file markers from `x-release-please-*` to `x-yeet-*`.
5. Configure the provider token and pipeline using [Authentication](authentication.md) and [CI setup](ci.md).
6. Run `yeet release --dry-run`. Confirm that each current version matches the latest release-please tag.

yeet continues from existing git tags. Set each `tag_prefix` to match the prior format, such as `v` for `v1.2.3` or `api-v` for `api-v1.2.3`. The default `autorelease: pending` and `autorelease: tagged` labels are compatible with release-please.

yeet ignores release-please PR branches and only adopts branches named `yeet/release-*`. An unfinished release-please PR therefore becomes stale after its workflow is removed.

Commit override markers are compatible, but yeet reads them only from final git commit messages. A block that exists only in a PR body has no effect unless the provider copies it into the final commit.

## Config mapping

| release-please (`release-please-config.json`) | yeet (`.yeet.yaml`) |
|---|---|
| `packages.<path>` | `targets.<name>` with `type: path` and `path: <path>` |
| `include-v-in-tag`, `include-component-in-tag`, `tag-separator` | Per-target `tag_prefix`, such as `v` or `api-v` |
| `bump-minor-pre-major` | `pre_major_breaking_bumps_minor`, default `true` |
| `bump-patch-for-minor-pre-major` | `pre_major_features_bump_patch`, default `true` |
| `changelog-path` | `changelog.file` |
| `changelog-sections` | `changelog.include` plus `changelog.sections` |
| `extra-files` | `version_files` markers, or `format: json` with `json_pointer` |
| `pull-request-header` / `pull-request-footer` | `release.pr_body_header` / `release.pr_body_footer` |
| `label` | `release.labels.pending` |
| `release-label` | `release.labels.tagged` |
| `extra-label` | `release.labels.extra` |
| `pull-request-title-pattern` | `release.pr_title` |
| `group-pull-request-title-pattern` | `release.pr_title_group` |
| `release-as` config option | `Release-As` commit footer only |
| `prerelease` | Branch-scoped `release.channels` |
| `.release-please-manifest.json` | Not needed, versions come from git tags |
| `separate-pull-requests` | Not supported, yeet creates one combined PR/MR per base branch |

Translate title patterns to Go `text/template` syntax using [Subject templates](release.md#subject-templates). Change lifecycle label names only after all in-flight releases are closed or finalized.

yeet has no language-specific `release-type` strategies. List every versioned file explicitly. JSON files use `format: json` and `json_pointer`. YAML, TOML, and XML files use comment markers. See [Version files](configuration.md#version-files).

## Marker mapping

| release-please | yeet |
|---|---|
| `x-release-please-version` | `x-yeet-version` |
| `x-release-please-major` / `-minor` / `-patch` | `x-yeet-major` / `-minor` / `-patch` |
| `x-release-please-start-version` | `x-yeet-start-version` |
| `x-release-please-end` | `x-yeet-end` |

## Related documentation

- [Documentation index](README.md)
- [Authentication](authentication.md)
- [CI setup](ci.md)
- [Configuration](configuration.md)
