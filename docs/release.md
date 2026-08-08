# Release PRs and MRs

Settings under `release:` apply to the combined release PR/MR, not to individual targets.

## PR/MR settings

```yaml
release:
  labels:
    pending: 'autorelease: pending'
    tagged: 'autorelease: tagged'
    yeet: true                         # set false to disable the managed yeet label
    extra:                            # applied when the PR/MR is created
      - release
      - automated
  pr_title: 'chore({{ .Target }}): release {{ .Tag }}'
  pr_title_group: 'chore({{ .Branch }}): release {{ .TargetCount }} components'
  commit_subject: 'chore({{ .Target }}): release {{ .Tag }}'
  commit_subject_group: 'chore({{ .Branch }}): release {{ .TargetCount }} components'
  auto_merge: true                   # equivalent to --auto-merge
  auto_merge_force: false            # equivalent to --auto-merge-force
  auto_merge_method: squash          # auto|squash|rebase|merge (default: auto)
  pr_body_header: "## Release"       # markdown before changelog in PR/MR body
  pr_body_footer: "_Automated._"     # markdown after changelog in PR/MR body
  pr_body_max_length: 0              # cap PR/MR body length in characters (0 = provider limit only)
  reviewers:                         # reviewers requested when the PR/MR is created
    - alice
```

### Labels

When yeet creates a release PR/MR, it adds `release.labels.pending`, the permanent `yeet` label,
and every `release.labels.extra` entry, then removes `release.labels.tagged` if present. Set
`release.labels.yeet` to `false` to disable the managed label. Refreshing an open release keeps its
current labels. It does not restore the managed or extra labels when a maintainer removes them, and
it does not remove manually added labels. Finalization adds the tagged label, removes the pending
label, and leaves every other label unchanged.

When GitHub or GitLab opens or adopts a release PR/MR, yeet creates missing lifecycle labels and the
managed `yeet` label with yeet's colors and descriptions. Extra labels must already exist in the
repository or project. Before auto-merge or release publication, yeet checks that the configured
tagged label still exists without creating it. If it was deleted, recreate it and retry the release.
Azure DevOps cannot inspect label definitions, so it skips this check and attaches configured labels
directly to the PR. A missing or rejected label fails the run rather than being silently dropped.

Do not rename or delete lifecycle labels while a release PR/MR is open, or after merge but before
finalization. Finish or close the in-flight release first. Yeet stores no lifecycle label history,
so a new pending name cannot safely identify the old release state, and a missing tagged label stops
finalization before publication on GitHub and GitLab.

Lifecycle label names are matched case-insensitively on GitHub and Azure DevOps. GitLab matches
them exactly, because GitLab treats labels differing only by case as distinct and filters them
server-side, so a case-insensitive client-side match would disagree with the server and let yeet
open a second release MR on the same branch.

Label names cannot contain a comma, and cannot be `any` or `none`, because GitLab uses all three in
its label filter syntax. Both rules are enforced by config validation, so `yeet release --dry-run`
reports them.

If a run is interrupted between creating the release PR/MR and labelling it, the next run adopts
the unlabelled PR/MR: it reapplies the lifecycle labels and reuses it. Adoption only applies when
the PR/MR carries no labels at all. One that carries some other label is treated as a lifecycle
mismatch and fails the run, since that indicates renamed configuration rather than an interrupted
run.

### Subject templates

`pr_title` uses Go `text/template` for a one-target release. It can access `.Branch`, `.Channel`,
`.Target`, `.Version`, and `.Tag`. `.Version` excludes the tag prefix, while `.Tag` is the complete
planned tag. `pr_title_group` applies to multi-target waves and can access `.Branch`, `.Channel`,
and `.TargetCount`. Target-specific fields are unavailable to group templates.

`commit_subject` and `commit_subject_group` customize the corresponding release branch commit
subjects with the same single-target and group data. PR/MR titles and commit subjects are
independent, so changing one does not silently change the other. Include `{{ .Branch }}` explicitly
where either output needs the branch.

Templates have no custom functions. They are parsed and checked during release setup. Rendered
values are trimmed and must be nonempty and single-line. Empty templates preserve the built-in
unscoped title or commit subject. Existing PR/MR titles are regenerated on the next release run.
Titles are never used to discover release PRs/MRs.

`auto_merge_method` (or `--auto-merge-method`) selects the merge strategy yeet asks the provider to use. `auto` prefers squash, then rebase, then a merge commit, taking the first strategy the provider permits. What each provider permits differs. GitHub picks the first method enabled in repository settings and fails when none are enabled. GitLab requests a squash unless the project sets `squash_option: never`, and otherwise leaves the project's own merge method in force. Azure DevOps requests a squash, since it exposes no merge-strategy capability yeet can inspect. `squash`, `rebase`, and `merge` request that strategy explicitly. The flag overrides the config value for a single run.

`reviewers` requests reviews from the listed users when the release PR/MR is created. Reviewers are applied on create only, so later `yeet release` runs never overwrite manual reviewer changes on an open release PR/MR. A reviewer that cannot be resolved or assigned fails the release run before the PR/MR is created. Per provider:

- GitHub: entries are usernames. Each must be a repository collaborator, validated before the PR is created. When running with a personal access token, do not list the token owner: GitHub rejects the PR author as reviewer, and that failure happens only after the PR exists, leaving an unlabeled release PR that later runs cannot pick up. Releases run through a GitHub App or Actions token are not affected.
- GitLab: entries are usernames and must be project members (inherited group members count). GitLab silently applies fewer reviewers in some cases (the Free tier supports only a single MR reviewer), so yeet verifies the created MR and fails the run when a requested reviewer was dropped.
- Azure DevOps: entries are an email (unique name) or display name, resolved via the identities API. This needs the Identity (Read) PAT scope (`vso.identity`). A name matching multiple identities fails the run, and display names can also match groups, so prefer emails.

`pr_body_max_length` caps the generated PR/MR body. When the body would exceed the cap, yeet drops the changelog from the body entirely and replaces it with a short notice pointing to the changelog file (the header, footer, and the hidden manifest marker that identifies the release are always preserved). `0` applies no extra cap. Azure DevOps rejects PR descriptions longer than 4000 characters, so yeet always enforces that hard limit on Azure DevOps regardless of this setting. The full notes still live in the changelog file committed to the release branch.

The default `pr_body_header` is `## ٩(^ᴗ^)۶ release created`. The default `pr_body_footer` carries an "auto-generated preview" notice alongside the yeet attribution. Overriding `pr_body_footer` replaces both. Setting it to an empty string removes the footer entirely. The PR/MR body is regenerated by `yeet release`, so do not use it for final release-note edits.

## Release notes

To customize final release notes, edit the matching generated entry in
`CHANGELOG.md` on the release branch. Add custom notes as `###` sections, for example:

````md
### Migration Notes

Run database migrations before deploying workers.
````

When `yeet release` updates an existing release PR/MR, yeet preserves manual `###` sections
that are not part of the regenerated conventional-commit sections. Only `###` sections survive
regeneration: text placed directly under the version heading without a `###` heading is dropped.

## Prerelease channels

Prerelease channels are branch-scoped and semver-only. Configure each channel under `release.channels`. Any channel name except `stable` is allowed, `stable` is reserved:

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

On `main`, `yeet release` runs the stable release flow. On `beta`, it creates or updates a beta release PR/MR targeting `beta`. After that PR/MR is merged, the next run creates a provider prerelease such as `v1.3.0-beta.1`. Stable releases ignore prerelease tags when choosing the stable baseline.

Prerelease channels write to channel-specific changelogs by default, so stable `CHANGELOG.md` entries stay clean. For `CHANGELOG.md`, beta writes `CHANGELOG.beta.md`. For `services/api/CHANGELOG.md`, beta writes `services/api/CHANGELOG.beta.md`. A channel can set `changelog_file` to write to an explicit path instead. That override only applies to single-target configurations, multi-target setups always use the derived names. Version files are still updated to the prerelease version on the channel branch.

`yeet release` fails on branches that are not configured as `branch` or a `release.channels.<name>.branch`. Passing `--channel <name>` selects a configured channel explicitly, but outside of dry runs the current branch must still match that channel's `branch`. Use `--dry-run` to preview from any branch: alone it previews the stable flow, combined with `--channel <name>` it previews that channel.
