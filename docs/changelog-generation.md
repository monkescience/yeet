# Changelog generation

yeet generates changelogs from conventional commits.
The minimal configuration selects which commit types appear:

```yaml
changelog:
  file: CHANGELOG.md
  include:
    - feat
    - fix
    - perf
    - revert
```

Those four types are included by default.
An included type without a `sections` entry uses the type with its first character capitalized.
Use lowercase type names in `include` and `sections`.
Commit types match regardless of case.
See [Commit message format](versioning.md#commit-message-format) for the commit syntax.

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

Section headings must be unique single-line text without leading or trailing Markdown `#` markers.
`breaking` sets the heading of the [breaking changes](#breaking-changes) section.
It is not a commit type, so do not add it to `include`.

## Entry layout

Every entry has the same order:

1. The version heading.
2. A manual intro written on the release branch. See [Release notes](release.md#release-notes).
3. `### ⚠ BREAKING CHANGES` with every breaking change. It appears only when the release has a breaking commit.
4. One section per commit type, with one line per change.

A change with a [release note](#release-notes) shows the note as a quote under its line.

```md
## [v2.0.0](https://github.com/org/repo/compare/v1.4.0...v2.0.0) (2026-09-25)

Units replace targets. Most configs need a one-line rename.

### ⚠ BREAKING CHANGES

- **config:** rename targets to units ([abc1234](https://github.com/org/repo/commit/abc1234))
  > Rename `targets` to `units` in `.yeet.yaml`.
- drop Go 1.24 support ([def5678](https://github.com/org/repo/commit/def5678))

### Features

- **config:** rename targets to units ([abc1234](https://github.com/org/repo/commit/abc1234))
- drop Go 1.24 support ([def5678](https://github.com/org/repo/commit/def5678))
- **release:** add auto-merge ([0a1b2c3](https://github.com/org/repo/commit/0a1b2c3))
  > Set `release.auto_merge: true` to merge release PRs automatically.
```

## Breaking changes

The breaking changes section lists every breaking commit in commit order, including commits whose type is not in `include`.
Each line uses the commit description, and the commit's note appears as a quote under it.
A breaking commit whose type is in `include` also appears in its type section, without the note.
Do not add `breaking` to `include`.

## Release notes

Add a fenced code block with the info string `release-note` to the commit body to explain a change:

`````text
feat(config)!: rename targets to units

````release-note
Rename `targets` to `units` in `.yeet.yaml`:

```yaml
units:
  - path: .
```
````
`````

The fence content is Markdown and is quoted under the commit's changelog line.
HTML comment markers outside code are escaped, while code examples keep their literal text.
Notes of breaking commits appear in the breaking changes section.
Notes of other commits appear in their type section.
A monorepo commit renders its note in the entry of every target it touches.

Fence rules:

- The info string must be exactly `release-note`.
- Use more backticks for the outer fence than for inner code blocks, or use a `~~~` outer fence.
- Use `####` or deeper headings inside a note. Levels 1 to 3 would render larger than the section headings.
- Fences can appear at the top level or inside a list or blockquote.
- Valid fences in one commit are joined in order, even when another fence is invalid.
- Fence content never becomes footers, including content in an unclosed fence.
- Only text outside note fences contributes to breaking detection and `Release-As`.
  Original blank-line boundaries are preserved. A footer immediately after a closing fence
  still needs a blank line before it to start the footer section.
- A fence on a commit that is not in the changelog is ignored.

A note has one source, chosen in this order:

1. The `release-note` fence content.
2. Otherwise the `BREAKING CHANGE` or `BREAKING-CHANGE` footer, with its line breaks kept. Headings, unclosed code fences, and unclosed HTML blocks are escaped.
3. Otherwise the commit has no note.

yeet skips an invalid fence when it has no closing fence, is empty, contains a level 1 to 3
heading, contains an unclosed code block, or contains an unclosed HTML block.
For commits included in the changelog, a warning names the commit and explains how to fix the fence.
An unclosed note that hides `BEGIN_COMMIT_OVERRIDE` also warns for excluded commits.
That override stays ignored until the release-note fence is closed before the marker.
The release continues and keeps any other valid fences.
When no valid fence remains, a `BREAKING CHANGE` footer outside the fences becomes the note.
An unclosed fence consumes the rest of its Markdown block, so footer-like examples inside it
do not change the version.
An unclosed inner code block usually means the outer fence needs more backticks.
Add a skipped note by editing the changelog on the release branch.

A good note answers three questions:

- What changed?
- Who is affected?
- What must they do?

yeet reads notes only from final git commit messages.
With squash merges, configure the provider to put the PR or MR description into the commit message so that a fence in the description reaches yeet:

- GitHub: set the default squash merge commit message to the pull request title and description.
- GitLab: use a squash commit message template that contains `%{description}`.

## Footer parsing

Footers go at the end of the commit message, after a blank line.
A footer is a token followed by `: ` or ` #`, such as `Refs: JIRA-123` or `Closes #456`.
Use hyphens instead of spaces in tokens, except for `BREAKING CHANGE`.
Everything after the first footer belongs to a footer, so put body text before it:

```text
fix: update API

Body paragraph.

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

Use `{value}` in URL templates.
An empty URL keeps plain text.
Patterns match substrings, so anchor identifiers with `\b` when they could appear inside a larger token.

Footer reference keys are case-insensitive: `Refs`, `refs`, and `REFS` use the same rule.
Use one spelling per key, including when a target overrides a top-level rule.

One pattern can cover every project on a Jira host:

```yaml
changelog:
  references:
    patterns:
      - pattern: "\\b[A-Z][A-Z0-9]+-\\d+\\b"
        url: "https://jira.example.com/browse/{value}"
```

Add separate entries only when trackers use different hosts.
Invalid regular expressions fail during configuration loading.
Reference settings can be overridden per target.

## Commit overrides

Put override blocks in the body of the final git commit when its subject does not describe the releasable changes:

```text
chore: combine API changes

BEGIN_COMMIT_OVERRIDE
feat(auth): add OAuth token refresh

fix(api): return 401 for expired sessions
END_COMMIT_OVERRIDE
```

The entries replace that commit for bump calculation and changelog generation while retaining its original hash.
Each entry can contain its own `release-note` fence.
Commit-like lines inside a note fence stay in that note.
Ordinary code fences do not hide override entry boundaries.
Fences outside the override block are ignored.
Override markers inside closed release notes are treated as note content.
An unclosed note outside an override block cannot introduce an override.
Inside an override block, `END_COMMIT_OVERRIDE` also ends an unclosed note.
Overrides can also introduce a breaking change:

```text
BEGIN_COMMIT_OVERRIDE
feat(auth)!: replace session cookie format

BREAKING CHANGE: existing session cookies are invalid after upgrade
END_COMMIT_OVERRIDE
```

The block must exist in the final commit message.
yeet does not read PR or MR bodies.
This applies to squash merges, merge commits, rebases, and direct pushes.

## Related documentation

- [Documentation index](README.md)
- [Configuration](configuration.md)
- [Versioning](versioning.md)
- [Release notes](release.md#release-notes)
