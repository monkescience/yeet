# Changelog generation

yeet generates a changelog from conventional commits. Two settings control the output: `include` lists the commit types that appear, and `sections` maps commit types to heading text.

By default only `feat`, `fix`, `perf`, and `revert` commits are included. The default `sections` map covers all common conventional commit types, so adding a type to `include` picks up its default heading without configuring `sections`:

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

```yaml
changelog:
  file: CHANGELOG.md
  include:
    - feat
    - fix
    - perf
    - revert
  sections:
    feat: Features
    fix: Bug Fixes
    perf: Performance Improvements
    revert: Reverts
```

Breaking changes always render under a `⚠ BREAKING CHANGES` heading, regardless of `include`, with the `BREAKING CHANGE` footer text as the description.

## References

yeet can link issue tracker references in generated changelogs. References are extracted from two sources: inline patterns matched in commit descriptions, and conventional commit footers.

```yaml
changelog:
  references:
    patterns:
      - pattern: "JIRA-\\d+"
        url: "https://jira.example.com/browse/{value}"
      - pattern: "#\\d+"
        url: ""  # plain text, GitHub auto-links these
    footers:
      Refs: "https://jira.example.com/browse/{value}"
      Closes: ""
```

**Inline patterns** match against the commit description using regex and replace matches with links. A commit like `feat: add OAuth2 support JIRA-123` produces:

```
- add OAuth2 support [JIRA-123](https://jira.example.com/browse/JIRA-123) (abc1234)
```

**Footer references** extract values from conventional commit footers and append them after the commit hash. A commit with a `Refs: JIRA-456` footer produces:

```
- add OAuth2 support (abc1234) ([JIRA-456](https://jira.example.com/browse/JIRA-456))
```

Use `{value}` as the placeholder in URL templates. An empty URL string renders the reference as plain text without linking. Both `patterns` and `footers` can be configured per target in monorepo setups.

Patterns are matched as substrings, so `JIRA-\d+` also matches inside `MYJIRA-123`. Anchor with `\b` (for example `\bJIRA-\d+\b`) when a pattern risks matching inside a larger token.

A single pattern covers an entire Jira instance, since every project shares the same `/browse/{KEY}` URL:

```yaml
changelog:
  references:
    patterns:
      - pattern: "[A-Z][A-Z0-9]+-\\d+"
        url: "https://jira.example.com/browse/{value}"
```

This links `PROJ-12`, `OPS-345`, and `ABC-9` alike. Add a separate entry per tracker only when issues live on different hosts.

Reference patterns are validated when the config is loaded, so an invalid regular expression fails fast in CI instead of being silently skipped during changelog generation.

## Commit overrides

Add override entries to the body of the final git commit message when its subject does not describe the releasable changes:

```text
chore: combine API changes

BEGIN_COMMIT_OVERRIDE
feat(auth): add OAuth token refresh

fix(api): return 401 for expired sessions
END_COMMIT_OVERRIDE
```

When yeet analyzes that commit, those conventional commit messages replace it for version bumping and changelog generation. The generated changelog still links to the original commit hash.

This can split one merged commit into multiple release notes, or introduce a breaking change:

```md
BEGIN_COMMIT_OVERRIDE
feat(auth)!: replace session cookie format

BREAKING CHANGE: existing session cookies are invalid after upgrade
END_COMMIT_OVERRIDE
```

The override block must exist in the final git commit message. Yeet does not read PR or MR bodies during commit analysis. A block written in a PR or MR body only works when the provider copies it into the final commit message before the commit reaches the release branch. This rule is the same for squash merges, merge commits, rebases, and direct pushes.
