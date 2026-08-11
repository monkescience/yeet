# Changelog generation

yeet generates changelogs from conventional commits. The minimal configuration selects which commit types appear:

```yaml
changelog:
  file: CHANGELOG.md
  include:
    - feat
    - fix
    - perf
    - revert
```

Those four types are included by default. `sections` changes their headings or supplies headings for additional included types:

```yaml
changelog:
  include:
    - feat
    - fix
    - docs
  sections:
    feat: Features
    fix: Bug Fixes
    docs: Documentation
```

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

Breaking changes always appear under `⚠ BREAKING CHANGES`, regardless of `include`, using the `BREAKING CHANGE` footer text.

## References

Reference rules turn issue identifiers in commit descriptions or footers into links:

```yaml
changelog:
  references:
    patterns:
      - pattern: "\\bJIRA-\\d+\\b"
        url: "https://jira.example.com/browse/{value}"
      - pattern: "#\\d+"
        url: ""
    footers:
      Refs: "https://jira.example.com/browse/{value}"
      Closes: ""
```

Given `feat: add OAuth2 support JIRA-123`, the pattern produces:

```md
- add OAuth2 support [JIRA-123](https://jira.example.com/browse/JIRA-123) (abc1234)
```

A `Refs: JIRA-456` footer appends:

```md
([JIRA-456](https://jira.example.com/browse/JIRA-456))
```

Use `{value}` in URL templates. An empty URL keeps plain text. Patterns match substrings, so anchor identifiers with `\b` when they could appear inside a larger token.

One pattern can cover every project on a Jira host:

```yaml
changelog:
  references:
    patterns:
      - pattern: "\\b[A-Z][A-Z0-9]+-\\d+\\b"
        url: "https://jira.example.com/browse/{value}"
```

Add separate entries only when trackers use different hosts. Invalid regular expressions fail during configuration loading. Reference settings can be overridden per target.

## Commit overrides

Put override blocks in the body of the final git commit when its subject does not describe the releasable changes:

```text
chore: combine API changes

BEGIN_COMMIT_OVERRIDE
feat(auth): add OAuth token refresh

fix(api): return 401 for expired sessions
END_COMMIT_OVERRIDE
```

The entries replace that commit for bump calculation and changelog generation while retaining its original hash. Overrides can also introduce a breaking change:

```text
BEGIN_COMMIT_OVERRIDE
feat(auth)!: replace session cookie format

BREAKING CHANGE: existing session cookies are invalid after upgrade
END_COMMIT_OVERRIDE
```

The block must exist in the final commit message. yeet does not read PR or MR bodies. This applies to squash merges, merge commits, rebases, and direct pushes.

## Related documentation

- [Documentation index](README.md)
- [Configuration](configuration.md)
- [Versioning](versioning.md)
- [Release notes](release.md#release-notes)
