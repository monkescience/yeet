# Configuration

Run `yeet init` to create a `.yeet.yaml` with one target for the current repository:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json

targets:
  myrepo:
    type: path
    path: .
    tag_prefix: v
```

Most single-repository users edit only the target name, `path`, and `tag_prefix`. The target name appears in release output, `path` selects relevant commits, and `tag_prefix` controls tags such as `v1.2.3`.

The [JSON schema](../yeet.schema.json) is the complete reference for fields, defaults, and descriptions. It also powers editor validation and runtime config validation. Released builds write a schema URL pinned to their tag, while development builds write `main` as shown above.

yeet reads the nearest ancestor `.yeet.yaml`. Pass `--config` to read or create another path.

## Repository targeting

Repository detection uses these sources from highest to lowest priority:

1. CLI flags, `--provider`, `--remote`, `--host`, `--owner`, `--repo`, and `--project`
2. Explicit values under `repository:`
3. The configured `repository.remote`
4. The `origin` remote

The coordinate flags require an explicit `--provider`. GitLab uses `--project` rather than `--owner` and `--repo`. Azure DevOps does not accept `--owner`.

Automatic provider detection recognizes only `github.com`, `gitlab.com`, and `dev.azure.com`. Set `provider` for enterprise or self-hosted domains. Usually the remote still supplies the host and project path, so add `repository:` only when remote discovery is incomplete or must be overridden.

Exactly one provider subsection may be configured, and it must match `provider`:

```yaml
# GitHub (including Enterprise)
provider: github
repository:
  remote: upstream
  github:
    host: github.example.com
    owner: acme
    repo: widgets
    # project: acme/widgets
```

```yaml
# GitLab (self-managed)
provider: gitlab
repository:
  gitlab:
    host: gitlab.example.com
    project: group/sub/widgets
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
    # collection: DefaultCollection
```

Provider API URL overrides and host trust are documented in [Authentication](authentication.md#host-trust).

## Network requests

Provider requests time out after 30 seconds and make at most four total attempts by default. Durations use Go syntax such as `500ms`, `30s`, or `2m`:

```yaml
network:
  request_timeout: 45s
  retry:
    max_attempts: 5
    min_backoff: 1s
    max_backoff: 15s
```

Retries apply only when the request can be repeated safely, or when a provider returns a rate-limit response. The Azure DevOps SDK honors `request_timeout`, but manages its own transport and does not expose these retry bounds.

## Release timezone

`timezone` controls both CalVer calculations and generated changelog dates. It accepts `Local`, `UTC`, or an IANA location such as `Europe/Berlin`:

```yaml
timezone: America/Los_Angeles
```

The compatibility default is `Local`, which uses the machine's local timezone. A release run captures one timestamp, so every target in the run uses the same calendar date.

## Targets

yeet plans each target independently and combines all planned changes into one release PR/MR per base branch. Use `--target` repeatedly to limit a run.

### Single repository target

Use a path target rooted at `.`. Choose a prefix that matches existing tags so version discovery continues from the correct release.

```yaml
targets:
  widgets:
    type: path
    path: .
    tag_prefix: v
```

### Monorepo path targets

Each path target receives only commits that changed its path. `exclude_paths` removes subtrees from that match.

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
```

### Derived targets

A derived target releases when any included target releases. An optional `path` also lets it match direct commits.

```yaml
targets:
  api:
    type: path
    path: services/api
    tag_prefix: api-v
  web:
    type: path
    path: apps/web
    tag_prefix: web-v
  root:
    type: derived
    includes:
      - api
      - web
    path: .
    tag_prefix: v
```

Targets can override `versioning`, both pre-major settings, `version_files`, `changelog`, and `calver`. Release PR/MR settings remain top-level because they apply to the combined release.

## Bump types

By default, `feat` produces a minor bump and `fix` or `perf` produces a patch bump. Customize the mapping when the repository uses additional conventional commit types:

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

Unlisted types do not bump unless the commit is breaking. See [Versioning](versioning.md) for pre-1.0 behavior.

## Version files

`yeet release` changes only files listed in `version_files`. Plain string entries use comment markers:

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

Markers must be in a real `#`, `//`, `/* */`, `--`, `;`, or `<!-- -->` comment. Every listed marker file must contain a valid marker. Lines inside a block without a version value remain unchanged.

JSON files use a JSON Pointer because JSON has no comments:

```yaml
version_files:
  - path: package.json
    format: json
    json_pointer: /version
```

The pointer must resolve to a string. Nested values use RFC 6901 syntax such as `/packages/0/version`, and yeet preserves the existing JSON formatting.

| Scheme | Marker scopes |
|---|---|
| semver | `version`, `major`, `minor`, `patch` |
| calver | `version`, `year`, `micro`, plus `month`, `week`, or `day` when present in the configured format |

Block markers use `x-yeet-start-<scope>` and `x-yeet-end`. Calver substitution preserves token width, so `0M` produces a zero-padded month.

## Related documentation

- [Documentation index](README.md)
- [Versioning](versioning.md)
- [Changelog generation](changelog-generation.md)
- [Release PRs and MRs](release.md)
