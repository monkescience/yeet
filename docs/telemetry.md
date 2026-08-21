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

The repository can also record an explicit opt-in:

```yaml
telemetry:
  enabled: true
```

To disable telemetry for yeet and every other tool that supports the shared
standard, set:

```sh
export DO_NOT_TRACK=1
```

`DO_NOT_TRACK` also accepts `true`, `yes`, and `on`, without regard to case. It
always wins over `.yeet.yaml`. There is no user-scoped yeet telemetry file.

The complete decision order is:

1. A truthy `DO_NOT_TRACK` disables telemetry.
2. `telemetry.enabled` in `.yeet.yaml` selects the repository behavior.
3. Telemetry is enabled.

Missing repository configuration uses the enabled default. Invalid or unreadable
repository configuration disables telemetry because yeet cannot safely
determine whether the repository opted out.

## Why yeet collects telemetry

The maintainers use these execution counts to answer limited product questions:

- Which yeet versions and operating systems should receive the most testing and
  support?
- Which providers and release styles are actively used?
- Are failures or slow executions common enough to require focused work?
- Are features such as CalVer, prerelease channels, dry runs, and auto-merge
  worth continued investment?

The results cannot answer how many people or repositories use yeet. A
repository that runs yeet frequently contributes more events than one that runs
it occasionally. Opt-outs, offline runs, and blocked requests contribute no
events.

## Exactly what is sent

After `init` or `release` finishes, yeet attempts at most one
`Yeet.Command.executed` event with these fields:

| Field | Possible value | Purpose |
|---|---|---|
| `Yeet.eventDay` | UTC calendar date | Show broad usage trends without sending the local time. |
| `Yeet.version` | Official release version | Guide version support and upgrade messaging. Development and dirty versions are omitted. |
| `Yeet.os` | `linux`, `darwin`, `windows`, or another Go operating-system name | Prioritize platform testing and distribution. |
| `Yeet.command` | `init` or `release` | Compare setup and release workflows. |
| `Yeet.outcome` | `success`, `failure`, or `canceled` | Identify reliability work. |
| `Yeet.duration` | `under_1s`, `1s_to_5s`, `5s_to_30s`, `30s_to_120s`, or `over_120s` | Identify broad performance problems without collecting exact timings. |

When `release` successfully reads `.yeet.yaml`, the event can also contain:

| Field | Possible value | Purpose |
|---|---|---|
| `Yeet.release.provider` | `auto`, `github`, `gitlab`, or `azuredevops` | Prioritize provider support. |
| `Yeet.release.layout` | `single` or `monorepo` | Understand monorepo usage. |
| `Yeet.release.versioning` | `semver`, `calver`, or `mixed` | Prioritize versioning support. |
| `Yeet.release.dryRun` | `true` or `false` | Understand preview usage. |
| `Yeet.release.channelsConfigured` | `true` or `false` | Understand prerelease-channel usage. |
| `Yeet.release.autoMerge` | `off`, `normal`, or `force` | Prioritize auto-merge safety and provider behavior. |

The envelope contains the public TelemetryDeck app ID, the fixed event type,
and an empty `clientUser`. No session ID is sent. TelemetryDeck documents that
an [empty `clientUser` disables user counting](https://telemetrydeck.com/docs/api/signals-reference/).

The `help`, `version`, and completion commands do not send telemetry. Invalid
commands do not send telemetry either.

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
  timezone, CPU architecture, device model, or operating-system version

## How events are delivered

Official release binaries and images send directly over HTTPS to the
[TelemetryDeck Ingest API](https://telemetrydeck.com/docs/ingest/v2/).

Delivery has a 300 ms total timeout, no retries, no disk queue, no redirects,
and a 4 KiB payload limit. A network or ingestion failure never changes yeet's
command output, exit status, or release behavior.

TelemetryDeck documents that it does not store IP addresses. Its current
privacy information is available in the
[TelemetryDeck privacy FAQ](https://telemetrydeck.com/docs/guides/privacy-faq/).

## Related documentation

- [Configuration](configuration.md)
- [Documentation index](README.md)
