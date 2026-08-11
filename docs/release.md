# Release PRs and MRs

Settings under `release:` apply to the combined release PR/MR, not to individual targets.

The release lifecycle has three states:

| State | What `yeet release` does |
|---|---|
| No open release | Analyzes commits, updates changelogs and version files, then opens a release PR/MR with the pending label |
| Open pending release | Refreshes the same release branch and PR/MR with newly eligible commits |
| Merged pending release | Creates the tag and provider release, replaces the pending label with the tagged label, and publishes changelog notes |

With `--auto-merge` or `release.auto_merge`, yeet merges and finalizes in one run. `--auto-merge-force` skips yeet's readiness checks, but provider rules, required checks, approvals, draft state, conflicts, and permissions still apply.

## PR/MR settings

Add only the settings you need.

```yaml
release:
  auto_merge: true
  auto_merge_method: squash
  reviewers:
    - alice
```

```yaml
release:
  pr_body_header: "## Release"
  pr_body_footer: "_Automated._"
  pr_body_max_length: 4000
```

### Labels

```yaml
release:
  labels:
    pending: 'autorelease: pending'
    tagged: 'autorelease: tagged'
    yeet: true
    extra:
      - release
      - automated
```

| Event | Pending | Tagged | Managed and extra labels |
|---|---|---|---|
| Create release | Add | Remove if present | Add `yeet` unless disabled, then add every `extra` label |
| Refresh open release | Keep current state | Keep current state | Preserve manual changes, do not restore removed labels |
| Finalize release | Remove | Add | Leave every other label unchanged |

GitHub and GitLab create missing lifecycle labels and the managed `yeet` label when a release is opened or adopted. Extra labels must already exist. Before publication, yeet checks that the tagged label still exists. Azure DevOps cannot inspect label definitions, so it attaches configured labels directly.

GitHub and Azure DevOps match lifecycle names case-insensitively. GitLab matches exactly. Label names cannot contain a comma or equal `any` or `none`, because those values conflict with GitLab label filters.

If a run stops after creating a PR/MR but before applying labels, the next run adopts it only when it has no labels at all. A trusted release branch with other labels is treated as a lifecycle mismatch. For renamed, missing, or conflicting labels, follow the [label recovery table](troubleshooting.md#release-labels).

### Subject templates

```yaml
release:
  pr_title: 'chore({{ .Target }}): release {{ .Tag }}'
  pr_title_group: 'chore({{ .Branch }}): release {{ .TargetCount }} components'
  commit_subject: 'chore({{ .Target }}): release {{ .Tag }}'
  commit_subject_group: 'chore({{ .Branch }}): release {{ .TargetCount }} components'
```

Single-target templates receive `.Branch`, `.Channel`, `.Target`, `.Version`, and `.Tag`. Group templates receive `.Branch`, `.Channel`, and `.TargetCount`. `.Version` omits the tag prefix, while `.Tag` includes it.

Templates use Go `text/template` without custom functions. Rendered values must be nonempty and single-line. An empty setting keeps the built-in value. Titles and commit subjects are independent, and existing release titles are regenerated on refresh.

### Merge methods

`auto_merge_method`, or the one-run `--auto-merge-method` override, accepts `auto`, `squash`, `rebase`, or `merge`.

| Provider | `auto` behavior | Provider limitation |
|---|---|---|
| GitHub | Tries squash, then rebase, then merge | Selects the first method enabled in repository settings and fails when none are enabled |
| GitLab | Requests squash unless `squash_option` is `never` | Otherwise keeps the project's configured merge method |
| Azure DevOps | Requests squash | yeet cannot inspect merge-strategy capabilities |

An explicit method asks the provider for that strategy and fails if the provider rejects it.

### Reviewers

Reviewers are assigned only when the PR/MR is created. Refreshes preserve manual changes. Resolution or assignment failure stops the create flow.

| Provider | Identifier | Permission and limitation |
|---|---|---|
| GitHub | Username | Must be a repository collaborator. A PAT owner cannot review their own PR, so omit that user when a personal token creates the release |
| GitLab | Username | Must be a project member, including inherited members. Some tiers accept fewer reviewers, so yeet verifies the created MR |
| Azure DevOps | Email or display name | Requires Identity (Read), `vso.identity`. Ambiguous names fail, and display names can match groups, so prefer email |

### PR/MR body limits

`pr_body_max_length` caps the generated body. If the changelog does not fit but the fallback does, yeet keeps the header, footer, and hidden release manifest while replacing the changelog with a link notice. If the fallback also exceeds the limit, the run fails before updating the branch. `0` uses only the provider limit. Azure DevOps always enforces 4000 characters.

The default header is `## ٩(^ᴗ^)۶ release created`. Replacing `pr_body_footer` also replaces the default preview notice and attribution. An empty footer removes it. The body is regenerated on every run.

## Release notes

Final release notes come from the matching generated `CHANGELOG.md` entry, not the PR/MR body. Edit the changelog on the release branch to add manual notes:

````md
### Migration Notes

Run database migrations before deploying workers.
````

Refreshes preserve manual `###` sections, freeform text before the first section, and an outro separated from generated content by a blank line. Conventional-commit sections are regenerated, so manual edits inside or between them are replaced.

## Prerelease channels

Prerelease channels are branch-scoped and semver-only. `stable` is reserved.

```yaml
branch: main

release:
  channels:
    beta:
      branch: beta
      prerelease: beta
    rc:
      branch: rc
      prerelease: rc
```

A run on `beta` creates a beta release PR/MR and later publishes a version such as `v1.3.0-beta.1`. Stable releases ignore prerelease tags when selecting their baseline.

Channels use separate changelogs by default. `CHANGELOG.md` becomes `CHANGELOG.beta.md`, and `services/api/CHANGELOG.md` becomes `services/api/CHANGELOG.beta.md`. A single-target channel may set `changelog_file`. Multi-target configurations always use derived names. Version files still receive the prerelease version.

Runs fail on unconfigured branches. `--channel <name>` selects a configured channel, but non-dry runs still require its branch. `--dry-run` can preview stable or a selected channel from any branch.

## Related documentation

- [Documentation index](README.md)
- [Troubleshooting](troubleshooting.md)
- [Versioning](versioning.md)
- [Changelog generation](changelog-generation.md)
