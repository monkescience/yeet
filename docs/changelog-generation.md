# Changelog

yeet writes a changelog entry for every release and uses it as the provider release notes.

## Sections

`include` selects which commit types get a section. `sections` renames headings or adds new ones:

```yaml
changelog:
  file: CHANGELOG.md
  include:
    - feat
    - fix
    - perf
    - revert
    - docs
  sections:
    docs: Documentation
```

`feat`, `fix`, `perf`, and `revert` are included by default.

| Type | Default heading |
|---|---|
| `feat` | Features |
| `fix` | Bug Fixes |
| `perf` | Performance Improvements |
| `revert` | Reverts |
| `docs` | Documentation |
| `style` | Styles |
| `refactor` | Code Refactoring |
| `test` | Tests |
| `build` | Build System |
| `ci` | Continuous Integration |
| `chore` | Miscellaneous Chores |
| `breaking` | ⚠ BREAKING CHANGES |

Other types default to their name with a capital first letter.
`breaking` only renames the breaking changes section. Do not add it to `include`.

## What an entry looks like

```md
## [v2.0.0](https://github.com/org/repo/compare/v1.4.0...v2.0.0) (2026-09-25)

Units replace targets. Most configs need a one-line rename.

### ⚠ BREAKING CHANGES

- **config:** rename targets to units ([abc1234](https://github.com/org/repo/commit/abc1234))
  > Rename `targets` to `units` in `.yeet.yaml`.

### Features

- **config:** rename targets to units ([abc1234](https://github.com/org/repo/commit/abc1234))
- **release:** add auto-merge ([0a1b2c3](https://github.com/org/repo/commit/0a1b2c3))
  > Set `release.auto_merge: true` to merge release PRs automatically.
```

- The breaking changes section lists every breaking commit, even types not in `include`.
- Quoted lines come from [release notes](commits.md#release-notes) in commit messages.
- The intro paragraph is optional and written by you, see [Editing release notes](release.md#editing-release-notes).

## Issue links

Turn issue identifiers in commit descriptions or footers into links:

```yaml
changelog:
  references:
    patterns:
      - pattern: "\\b[A-Z][A-Z0-9]+-\\d+\\b"
        url: "https://jira.example.com/browse/{value}"
    footers:
      Refs: "https://jira.example.com/browse/{value}"
      Closes: ""
```

With this config, `feat: add OAuth2 support JIRA-123` becomes:

```md
- add OAuth2 support [JIRA-123](https://jira.example.com/browse/JIRA-123) (abc1234)
```

- `{value}` is replaced with the matched text. An empty URL keeps plain text.
- Patterns are regular expressions that match substrings, so anchor them with `\b`.
- Footer names are case-insensitive.

## Related documentation

- [Writing commits](commits.md)
- [Release PRs and MRs](release.md)
