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

Those four types are included by default. An included type without a `sections` entry uses the type with its first character capitalized.
Commit types are matched case-insensitively and normalized to lowercase. Use lowercase type names in
`include` and `sections`. See [Commit message format](versioning.md#commit-message-format) for the
required header separator and breaking-change syntax.
`sections` changes headings or supplies headings for additional included types:

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
| `breaking` | ⚠ BREAKING CHANGES |

Section headings must be unique, trimmed, single-line text without leading or closing Markdown `#` markers. Emoji and literal hashes in
text, such as `C#` and `Release###`, are supported.

Breaking changes appear under `changelog.sections.breaking`, regardless of `include`, using a valid
`BREAKING CHANGE` or `BREAKING-CHANGE` footer's description. With a header `!` and no valid breaking
footer, the header description is used instead. Trailing whitespace is removed from the rendered
footer description. Do not add `breaking` to `include` because breaking changes are included automatically.
The default heading is `⚠ BREAKING CHANGES`. A customized heading and the default heading are both recognized as generated content when a
release changelog is refreshed.

## Footer parsing

The first valid footer after a blank line starts the footer section. A footer consists of a token
and either `: ` or ` #`, such as `Reviewed-by: Alice` or `Refs #123`. Tokens use hyphens in place of
spaces, except for `BREAKING CHANGE`. Breaking footers require the colon-space separator.

Once the footer section starts, subsequent lines and blank-separated paragraphs continue the current
footer value until another valid footer token and separator appear. This applies to all footer keys,
including keys without configured reference rules. Markdown code fences do not escape footer syntax.

For example, `Additional context.` belongs to `Reviewed-by`, and both `Refs` and `Closes` are footers:

```text
fix: update API

Body paragraph.

Reviewed-by: Alice

Additional context.

Refs: #123

Closes #456
```

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

Footer reference keys are case-insensitive: `Refs`, `refs`, and `REFS` use the same configured rule.
Configuration keys that differ only by case are rejected, even when their URL templates are equal.
Keep a single spelling for each key. This validation also applies after per-target reference rules
are merged with top-level rules. To override an inherited rule, use exactly the same key spelling
in the target configuration.

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
