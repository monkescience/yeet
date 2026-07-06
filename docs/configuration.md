# Configuration

yeet reads the nearest ancestor `.yeet.yaml` by default. Run `yeet init` to generate one with sensible defaults, or pass `--config` to write to a custom path. The generated file includes a YAML language server schema modeline for editor validation and autocomplete.

The default `yeet init` output is intentionally minimal. It declares a single path target named after the repository directory, with everything else inheriting from schema defaults:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json

targets:
  myrepo:        # auto-derived from the repo directory name, falls back to "root"
    type: path
    path: .
    tag_prefix: v
```

All available options, defaults, and descriptions are defined in the [JSON schema](../yeet.schema.json). YAML-aware editors that support `# yaml-language-server: $schema=...` modelines will provide validation and autocomplete automatically. You can pin the schema URL to a release tag for stricter reproducibility.

## Repository targeting

yeet resolves the target repository from these sources, highest priority first:

1. CLI flags (`--provider`, `--remote`, `--host`, `--owner`, `--repo`, `--project`)
2. explicit `.yeet.yaml` values under `repository:`
3. the configured `repository.remote`
4. the `origin` remote

The repository override flags (`--host`, `--owner`, `--repo`, `--project`) require an explicit `--provider`. `--owner` and `--repo` are rejected for GitLab (use `--project`), and `--owner` is rejected for Azure DevOps.

Automatic provider detection intentionally only classifies the public hosts `github.com`,
`gitlab.com`, and `dev.azure.com`. For custom or enterprise domains, set the provider
explicitly. This avoids sending provider tokens to an arbitrary host based only on hostname
text. Repository host and path are discovered from `repository.remote`/`origin`. Set
`repository:` only when overriding remote discovery or when no usable remote exists.

When `repository:` is set, exactly one provider sub-section may be set, and it must match
the top-level `provider`. The available sub-sections are `repository.github`,
`repository.gitlab`, and `repository.azuredevops`:

```yaml
# GitHub (including Enterprise)
provider: github
repository:
  remote: upstream            # which git remote to inspect (default: origin)
  github:
    host: github.example.com  # override host for enterprise / mirrors
    owner: acme
    repo: widgets
    # project: acme/widgets   # alternative to owner + repo (owner/repo form)
```

```yaml
# GitLab (self-managed)
provider: gitlab
repository:
  gitlab:
    host: gitlab.example.com
    project: group/sub/widgets   # full project path, including subgroups
```

```yaml
# Azure DevOps
provider: azuredevops
repository:
  azuredevops:
    host: dev.azure.com
    organization: contoso
    project: MyProject
    repo: widgets
    # collection: DefaultCollection   # optional, defaults to organization
```

## Targets

yeet plans releases per target and creates one combined release PR/MR per base branch.
PR workflow settings remain top-level under `release:` and apply to the combined PR/MR, not individual targets.

Use `--target` to limit `yeet release` to specific targets (repeatable).

```yaml
targets:
  api:
    type: path
    path: services/api
    tag_prefix: api-v
    exclude_paths:
      - services/api/testdata

  web:
    type: path
    path: apps/web
    tag_prefix: web-v

  root:
    type: derived
    includes:
      - api
      - web
    path: .              # optional: also matches commits at repo root
    tag_prefix: v
```

Path targets support `exclude_paths` to ignore commits under specific subdirectories.
Derived targets aggregate included path targets and optionally match direct commits via `path`.

Each target can override these top-level settings: `versioning`, `pre_major_breaking_bumps_minor`, `pre_major_features_bump_patch`, `version_files`, `changelog`, and `calver`. Anything not overridden inherits the top-level value.

## Bump types

By default, `feat` commits bump minor and `fix`/`perf` commits bump patch. Override this mapping with `bump_types`:

```yaml
bump_types:
  minor:
    - feat
    - improvement
  patch:
    - fix
    - perf
    - deps
```

Types not listed produce no version bump. Breaking changes always bump major regardless of this mapping.

## Version files

`yeet release` updates only files listed in `version_files`. String entries use comment-based yeet markers.

```txt
# inline markers (semver project)
VERSION = "1.2.3" # x-yeet-version
MAJOR = 1 # x-yeet-major
MINOR = 2 # x-yeet-minor
PATCH = 3 # x-yeet-patch

# block markers
# x-yeet-start-version
image: ghcr.io/acme/app:1.2.3
appVersion: "1.2.3"
# x-yeet-end
```

Markers must sit in a real comment. yeet recognizes the `#`, `//`, `/* */`, `--`, `;`, and `<!-- -->` comment styles, so marker names mentioned in prose or inline code are ignored.

A file listed in `version_files` must contain at least one marker, otherwise the release fails. An inline marker on a line without a recognizable version value fails too. Lines inside a block that carry no version value are left untouched.

Entries can also be objects with an explicit `format` (`markers` or `json`). `format: markers` behaves like a plain string entry. JSON files cannot contain comments, so configure them with `format: json` and a `json_pointer`:

```yaml
version_files:
  - path: package.json
    format: json
    json_pointer: /version
```

The pointer must resolve to a JSON string. Nested values use standard RFC 6901 JSON Pointer syntax, for example `/packages/0/version`. The version file update preserves the existing JSON formatting and only replaces the targeted string value.

The marker surface depends on the project's versioning scheme. yeet validates each
marker against the configured scheme and the configured calver format. A marker
that doesn't apply to the scheme returns an error with a suggested replacement.

| Scheme | Allowed scopes |
|---|---|
| semver | `version`, `major`, `minor`, `patch` |
| calver | `version`, `year`, `micro`, plus `month` / `week` / `day` only when the configured format includes that token |

Examples by calver format:

- `YYYY.0M.MICRO` (default): `version`, `year`, `month`, `micro`
- `YYYY.WW.MICRO`: `version`, `year`, `week`, `micro`
- `YYYY.0M.0D.MICRO`: `version`, `year`, `month`, `day`, `micro`
- `YYYY.MICRO`: `version`, `year`, `micro`

Block markers use the same scope names: `x-yeet-start-<scope>` opens the block,
`x-yeet-end` closes it. Substitution width follows the format token, for example,
`0M` zero-pads the month to two digits, `MM` does not.
