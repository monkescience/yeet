# Telemetry

yeet uses limited anonymous telemetry to understand how its release workflows
are used and where maintenance effort is most useful. Telemetry counts command
executions. It does not identify users, installations, organizations, or
repositories.

The short version is:

- Telemetry is enabled by default.
- A repository can explicitly enable or disable telemetry in `.yeet.yaml`.
- `DO_NOT_TRACK=1` always disables telemetry.
- Only the `init` and `release` commands are measured.

## Choose the setting for a repository

Disable telemetry for one repository by committing this to `.yeet.yaml`:

```yaml
telemetry:
  enabled: false
```

This setting overrides the default. It is useful when everyone working with a
repository should get the same behavior.

To explicitly enable telemetry for a repository:

```yaml
telemetry:
  enabled: true
```

To disable telemetry for yeet and other tools that honor `DO_NOT_TRACK`, set:

```sh
export DO_NOT_TRACK=1
```

`DO_NOT_TRACK` also accepts `true`, `yes`, and `on` in any letter case. It always
wins over `.yeet.yaml`. yeet has no separate user-level telemetry setting.

The complete decision order is:

1. `DO_NOT_TRACK=1`, `true`, `yes`, or `on` disables telemetry.
2. Otherwise, yeet uses `telemetry.enabled` from `.yeet.yaml` when present.
3. If neither is set, telemetry is enabled.

If no repository configuration exists, telemetry remains enabled. If the
configuration is invalid or cannot be read, telemetry is disabled because yeet
cannot safely determine whether the repository opted out.

## Why yeet collects telemetry

Aggregate usage data helps us prioritize support for yeet versions, operating
systems, providers, release styles, and features. It also helps identify
reliability and performance problems.

This data does not show how many people or repositories use yeet. Repositories
that run yeet more often contribute more events. Opt-outs, offline runs, and
blocked requests contribute no events.

## Exactly what is sent

After `init` or `release` finishes, yeet attempts at most one
`Yeet.Command.executed` event with these fields:

| Field | Possible value | Purpose |
|---|---|---|
| `Yeet.eventDay` | UTC calendar date | Show broad usage trends without sending the local time. |
| `Yeet.version` | Official release version | Guide version support and upgrade messaging. Development and dirty versions are omitted. |
| `Yeet.os` | `linux`, `darwin`, `windows`, or another Go operating-system name | Prioritize platform testing and distribution. |
| `Yeet.arch` | `amd64`, `arm64`, or another Go architecture name | Prioritize build and distribution targets. |
| `Yeet.command` | `init` or `release` | Compare setup and release workflows. |
| `Yeet.outcome` | `success` or `failure` | Identify reliability work. |
| `Yeet.failure.category` | A failure kind such as `config_missing`, `config_invalid`, `config_exists`, `authentication`, `host_trust`, `repository`, `checkout`, `release_branch`, `release_state`, `merge_blocked`, `merge_timeout`, `reviewer`, `labels`, `network`, or `unexpected` | Only sent for failures. Show which kinds of failures dominate without sending error details. |

The event also sets TelemetryDeck's top-level `floatValue` field to the command
runtime in seconds, rounded to whole milliseconds, so average and percentile
runtimes can be charted.

When `release` successfully reads `.yeet.yaml`, the event can also contain:

| Field | Possible value | Purpose |
|---|---|---|
| `Yeet.release.provider` | `auto`, `github`, `gitlab`, or `azuredevops` | Prioritize provider support. Successful releases report the resolved provider. |
| `Yeet.release.layout` | `single` or `monorepo` | Understand monorepo usage. |
| `Yeet.release.versioning` | `semver`, `calver`, or `mixed` | Prioritize versioning support. |
| `Yeet.release.dryRun` | `true` or `false` | Understand preview usage. |
| `Yeet.release.channelsConfigured` | `true` or `false` | Understand prerelease-channel usage. |
| `Yeet.release.autoMerge` | `off`, `normal`, or `force` | Prioritize auto-merge safety and provider behavior. |

No user or session identifier is sent. TelemetryDeck documents that an empty
[`clientUser` disables user counting](https://telemetrydeck.com/docs/api/signals-reference/).

The `help`, `version`, and completion commands do not send telemetry. Invalid
commands and commands canceled before delivery do not send telemetry either.

## What is never sent

yeet does not send:

- A persistent user, installation, machine, repository, or session identifier
- Repository names, owners, remotes, hosts, URLs, paths, branches, tags,
  target names, or target counts
- Commit hashes, commit messages, changelogs, pull requests, reviewers, labels,
  or release names
- Command arguments, arbitrary flag values, environment-variable values, file
  names, or the working directory
- Credentials, tokens, request headers, error messages, logs, stack traces, or
  provider response bodies
- An IP address in the event payload, or a username, hostname, locale,
  timezone, device model, or operating-system version

## How events are delivered

Official release binaries and images send directly over HTTPS to the
[TelemetryDeck Ingest API](https://telemetrydeck.com/docs/ingest/v2/).

Telemetry is sent on a best-effort basis and is never queued or stored on disk.
Delivery failures do not change yeet's output, exit status, or release behavior.

TelemetryDeck documents that it does not store IP addresses. Its current
privacy information is available in the
[TelemetryDeck privacy FAQ](https://telemetrydeck.com/docs/guides/privacy-faq/).

## Related documentation

- [Configuration](configuration.md)
- [Documentation index](README.md)
