# Configuration

`yeet init` creates a `.yeet.yaml` with one target for the current repository:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json

targets:
  myrepo:
    type: path
    path: .
    tag_prefix: v
```

For a single repository, you usually only adjust `tag_prefix` so it matches your existing tags, such as `v` for `v1.2.3`.
yeet continues from the latest matching tag.

yeet uses the nearest `.yeet.yaml` in the current or a parent directory. Pass `--config` to use another file.
The [JSON schema](../yeet.schema.json) lists every field and default, and gives your editor validation and autocompletion.

## Targets

A target is something yeet versions and tags.
By default, all targets with changes share one release PR/MR. For one PR/MR per target, see [Monorepo release units](release.md#monorepo-release-units).
Use `--target <name>` to release only some targets.

### Monorepo

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
    includes: [api, web]
    path: .
    tag_prefix: v
```

A `path` target only picks up commits that touch its path, minus `exclude_paths`.
A `derived` target releases whenever one of its `includes` releases, and optionally for commits under its own `path`.

Targets can override `versioning`, `calver`, `changelog`, `version_files`, and the pre-major settings.

## Version files

yeet updates only the files listed in `version_files`.
Mark the version in a comment on the same line, or wrap lines in a block:

```yaml
version_files:
  - README.md
  - chart/Chart.yaml
```

```txt
VERSION = "1.2.3" # x-yeet-version

# x-yeet-start-version
image: ghcr.io/acme/app:1.2.3
appVersion: "1.2.3"
# x-yeet-end
```

Markers work in `#`, `//`, `/* */`, `--`, `;`, and `<!-- -->` comments, and every listed file must contain one.

| Versioning | Marker scopes |
|---|---|
| semver | `version`, `major`, `minor`, `patch` |
| calver | `version`, `year`, `micro`, plus `month`, `week`, or `day` if the format has them |

Use `x-yeet-<scope>` inline, or `x-yeet-start-<scope>` with `x-yeet-end` for a block.

JSON has no comments, so point at the value instead:

```yaml
version_files:
  - path: package.json
    format: json
    json_pointer: /version
```

## Bump types

Add commit types that should bump the version:

```yaml
bump_types:
  minor: [feat, improvement]
  patch: [fix, perf, deps]
```

See [Versioning](versioning.md) for how bumps work.

## Timezone

`timezone` sets the date used for calver versions and changelog entries.
It accepts `Local` (default), `UTC`, or an IANA name such as `Europe/Berlin`.
Set it explicitly so local previews and CI produce the same date.

## Self-hosted providers

yeet detects `github.com`, `gitlab.com`, and `dev.azure.com` from the `origin` remote.
For any other host, set `provider`:

```yaml
provider: gitlab
```

Add a `repository` section only when the remote does not provide everything, for example a path prefix:

```yaml
provider: github
repository:
  remote: upstream
  github:
    host: github.example.com
    api_url: https://github.example.com/root/api/v3/
    web_url: https://github.example.com/root
    owner: acme
    repo: widgets
```

GitLab uses `project: group/sub/repo`. Azure DevOps uses `organization`, `project`, `repo`, and optionally `collection`.
`api_url` and `web_url` must be HTTPS URLs on the repository host.
See [Authentication](authentication.md#self-hosted-providers) for tokens and environment overrides.

## Network

Slow or unreliable provider connections can use a longer timeout or more retries:

```yaml
network:
  request_timeout: 45s
  retry:
    max_attempts: 5
```

Defaults are a 30 second timeout and 4 attempts. Azure DevOps ignores `retry`.

## Related documentation

- [Versioning](versioning.md)
- [Changelog](changelog-generation.md)
- [Release PRs and MRs](release.md)
- [Telemetry](telemetry.md)
