# Release PRs and MRs

Every CI run of `yeet release` moves the release one step forward:

| Situation | What yeet does |
|---|---|
| No open release | Updates changelogs and version files, then opens a release PR/MR labeled `autorelease: pending` |
| Release PR/MR is open | Refreshes it with any new commits |
| Release PR/MR was merged | Creates the tag and provider release, then relabels the PR/MR `autorelease: tagged` |

Most repositories need no `release:` settings. The options below are all optional.
To merge release PRs/MRs automatically, see [Auto-merge](auto-merge.md).

## Editing release notes

The provider release notes are the new `CHANGELOG.md` entry, not the PR/MR body.
To add a summary or extra sections, edit the changelog on the release branch:

```md
## [v2.0.0](https://github.com/org/repo/compare/v1.4.0...v2.0.0) (2026-09-25)

Units replace targets. Most configs need a one-line rename.

### Migration Notes

Run database migrations before deploying workers.

### ⚠ BREAKING CHANGES
```

Refreshes keep text before the first section, your own `###` sections, and a closing paragraph separated by a blank line.
Generated sections are rebuilt, so edits inside them are lost.
For notes on individual changes, use [release notes in commits](commits.md#release-notes).

## Monorepo release units

By default, one release PR/MR covers every target with changes.
For separate PRs/MRs, switch to independent mode:

```yaml
release:
  pull_request_mode: independent
  groups:
    backend:
      targets: [api, worker]
```

Each ungrouped target gets its own PR/MR, and each group shares one.
Selecting a grouped target with `--target` selects the whole group.
Independent units cannot write the same changelog or version file, so put such targets in one group.

## Reviewers

```yaml
release:
  reviewers:
    - alice
```

Reviewers are requested when the PR/MR is created. Later changes you make by hand are kept.

| Provider | Identifier | Notes |
|---|---|---|
| GitHub | Username | Must be a collaborator. A personal token's owner cannot review their own PR |
| GitLab | Username | Must be a project member |
| Azure DevOps | Email or display name | Prefer email. Needs the Identity (Read) scope |

## Labels

```yaml
release:
  labels:
    pending: 'autorelease: pending'
    tagged: 'autorelease: tagged'
    yeet: true
    extra: [release, automated]
```

yeet tracks releases through the `pending` and `tagged` labels. It also adds a `yeet` label unless disabled, plus any `extra` labels.
On GitHub and GitLab, extra labels must already exist. yeet creates the others.

Only rename `pending` or `tagged` when no release is open or waiting to be tagged. See [Troubleshooting](troubleshooting.md#release-labels).

## Titles, branches, and body

```yaml
release:
  pr_title: 'chore({{ .Target }}): release {{ .Tag }}'
  pr_title_group: 'chore({{ .Branch }}): release {{ .TargetCount }} components'
  commit_subject: 'chore({{ .Target }}): release {{ .Tag }}'
  commit_subject_group: 'chore({{ .Branch }}): release {{ .TargetCount }} components'
  name_template: '{{ .Target }} {{ .Version }}'
  branch_template: 'automation/release-{{ .Branch }}'
  pr_body_header: '## Release'
  pr_body_footer: '_Automated._'
```

Templates use Go [`text/template`](https://pkg.go.dev/text/template) syntax.

| Template | Fields |
|---|---|
| `pr_title`, `commit_subject`, `name_template` | `.Branch`, `.Channel`, `.Target`, `.Version`, `.Tag` |
| `pr_title_group`, `commit_subject_group` | `.Branch`, `.Channel`, `.TargetCount` |
| `branch_template` | `.Branch`, `.Channel`, `.Unit` |

`.Version` is `1.2.3`, `.Tag` includes the prefix, such as `v1.2.3`.
`name_template` sets the provider release name and defaults to the tag.
In independent mode, include `.Unit` in a custom `branch_template` so every unit gets its own branch.

When the changelog makes the body too long, yeet links to it instead. Set `pr_body_max_length` to use a lower limit.

## Prerelease channels

Release prereleases such as `v1.3.0-beta.1` from dedicated branches:

```yaml
branch: main

release:
  channels:
    beta:
      branch: beta
      prerelease: beta
```

- A channel uses its own changelog, such as `CHANGELOG.beta.md`. Single-target setups can set `changelog_file` instead.
- Stable releases ignore prerelease tags.
- yeet fails on branches that are neither `branch` nor a channel branch.
- `yeet release --dry-run --channel beta` previews a channel from any branch.

Channels require semver.

## Related documentation

- [Auto-merge](auto-merge.md)
- [Changelog](changelog-generation.md)
- [Troubleshooting](troubleshooting.md)
