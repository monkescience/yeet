# Writing commits

yeet reads [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) from your git history to decide the next version and build the changelog.

yeet reads only the final commit messages on the release branch, never PR/MR descriptions.
If you squash merge, configure the provider to copy the description into the commit message:

- GitHub: set the default squash merge message to the pull request title and description.
- GitLab: use a squash commit message template that contains `%{description}`.

## Format

```text
type(scope)!: description

Optional body.

Optional-Footer: value
```

- `scope` and `!` are optional.
- Types are case-insensitive, so `feat:` and `FEAT:` behave the same.
- Separate the body and footers from the header with a blank line. Otherwise yeet ignores the commit.
- Footers go last. A footer is a token followed by `: ` or ` #`, such as `Refs: JIRA-123` or `Closes #456`. Everything after the first footer counts as footer text.

By default `feat` bumps the minor version and `fix` or `perf` bumps the patch version.
See [Versioning](versioning.md) for the full rules.

Run `yeet release --dry-run --verbose` to see why a commit was ignored.

## Breaking changes

Mark a breaking change with `!` after the type or scope, or with a footer:

```text
feat!: replace authentication API

BREAKING CHANGE: clients must use the new login endpoint.
```

The footer must be written in uppercase as `BREAKING CHANGE:` or `BREAKING-CHANGE:`.
Its text appears in the changelog as the note for that change.

## Release notes

Add a `release-note` fence to the commit body to explain a change to your users.
It appears as a quote under the commit's changelog line:

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

A good note says what changed, who is affected, and what they must do.

- The info string must be exactly `release-note`.
- The note is Markdown. Use `####` or deeper headings only.
- When the note contains a code block, use more backticks for the outer fence, or use `~~~`.
- Multiple fences in one commit are joined in order.
- A fence takes precedence over the `BREAKING CHANGE` footer text.

If a fence is invalid, for example unclosed or empty, yeet warns, skips it, and continues the release.
You can still add the note by editing the changelog on the release branch.

## Force a version

A `Release-As` footer sets the next semver version:

```text
chore: request a stable release

Release-As: 1.0.0
```

The version must be stable and greater than the current version.
All `Release-As` footers in one release must agree.
On a [prerelease channel](release.md#prerelease-channels), yeet adds the channel suffix, such as `1.0.0-beta.1`.
Calver targets ignore the footer.

## Commit overrides

When a commit's subject does not describe what it releases, list the real changes in an override block:

```text
chore: combine API changes

BEGIN_COMMIT_OVERRIDE
feat(auth): add OAuth token refresh

fix(api)!: return 401 for expired sessions

BREAKING CHANGE: clients must handle 401 responses.
END_COMMIT_OVERRIDE
```

The entries replace the commit for versioning and the changelog, and keep its hash.
Each entry can have its own `release-note` fence.

## Related documentation

- [Versioning](versioning.md)
- [Changelog](changelog-generation.md)
