# Documentation

Choose the task you want to complete.

| Task | Where to go |
|---|---|
| Set up the first automated release | Follow the local preview, CI setup, and first-release path in the [Quick start](../README.md#quick-start). |
| Configure authentication | Choose provider token variables, permissions, and trusted hosts in [Authentication](authentication.md). |
| Add release automation | Copy the GitHub Actions, GitLab CI, or Azure Pipelines example from [CI setup](ci.md). |
| Configure a single repository | Start from the generated target in [Configuration](configuration.md#targets). |
| Configure a monorepo | Define path and derived targets in [Configuration](configuration.md#targets). |
| Customize versions and changelogs | Choose [Versioning](versioning.md), then configure [Changelog generation](changelog-generation.md). |
| Configure release PRs, reviewers, and channels | Use [Release PRs and MRs](release.md) for lifecycle and provider behavior. |
| Troubleshoot a failed release | Match the error prefix or label state in [Troubleshooting](troubleshooting.md). |
| Verify downloaded artifacts | Check signatures and provenance with [Artifact verification](verification.md). |
| Migrate from release-please | Follow the ordered [Migration guide](migrate-from-release-please.md). |
| Contribute or report a vulnerability | Read [Contributing](../CONTRIBUTING.md) or use the private process in [Security Policy](../SECURITY.md). |

The complete `.yeet.yaml` field reference is [`yeet.schema.json`](../yeet.schema.json). CLI flags and environment variables are also available through `yeet --help` and `yeet <command> --help`.
