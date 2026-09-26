# Telemetry

yeet sends limited, anonymous usage data to help prioritize platforms, providers, and features, and to spot reliability problems.
It never identifies users, machines, organizations, or repositories.

## Turn it off

For one repository, commit this to `.yeet.yaml`:

```yaml
telemetry:
  enabled: false
```

For everything on your machine or CI runner, set `DO_NOT_TRACK=1`. It always wins over `.yeet.yaml`.

Telemetry is on by default. It is also off when `.yeet.yaml` is invalid, because yeet cannot tell whether you opted out.

## What is sent

After `init` or `release` finishes, yeet sends at most one event to [TelemetryDeck](https://telemetrydeck.com/docs/guides/privacy-faq/) over HTTPS.
Other commands send nothing. Nothing is stored on disk, and a failed delivery never affects the release.

| Field | Values |
|---|---|
| `Yeet.eventDay` | UTC date |
| `Yeet.version` | Official release version only |
| `Yeet.os`, `Yeet.arch` | Such as `linux` and `amd64` |
| `Yeet.command` | `init` or `release` |
| `Yeet.outcome` | `success` or `failure` |
| `Yeet.failure.category` | On failure, a category such as `authentication`, `network`, or `config_invalid`. Never the error message |
| `floatValue` | Command duration in seconds |

For `release`, the event can also contain:

| Field | Values |
|---|---|
| `Yeet.release.provider` | `auto`, `github`, `gitlab`, or `azuredevops` |
| `Yeet.release.layout` | `single` or `monorepo` |
| `Yeet.release.versioning` | `semver`, `calver`, or `mixed` |
| `Yeet.release.dryRun` | `true` or `false` |
| `Yeet.release.channelsConfigured` | `true` or `false` |
| `Yeet.release.autoMerge` | `off`, `provider`, or `direct` |

## What is never sent

- User, machine, installation, session, or repository identifiers
- Repository names, hosts, URLs, paths, branches, tags, or target names
- Commits, changelogs, PRs/MRs, reviewers, labels, or release names
- Command arguments, environment variables, file names, or the working directory
- Tokens, error messages, logs, or provider responses
- An IP address in the event, or a hostname, username, locale, or timezone

## Related documentation

- [Configuration](configuration.md)
