# Find the right guide

Start with what you want to accomplish. Each guide includes the configuration and provider details you need for that task.

| I want to... | Start here |
|---|---|
| Publish my first automated release | Follow the local preview, CI setup, and first-release path in the [Quick start](../README.md#quick-start). |
| Give yeet access to my provider | Choose token variables, permissions, and trusted hosts in [Authentication](authentication.md). |
| Run yeet in CI | Copy the GitHub Actions, GitLab CI, or Azure Pipelines example from [CI setup](ci.md). |
| Release one repository | Start with the generated target in [Configuration](configuration.md#targets). |
| Release multiple packages from a monorepo | Define path and derived targets in [Configuration](configuration.md#targets). |
| Control version bumps and changelog sections | Choose a strategy in [Versioning](versioning.md), then configure [Changelog generation](changelog-generation.md). |
| Customize release PRs, reviewers, or prerelease channels | Use [Release PRs and MRs](release.md) for lifecycle and provider behavior. |
| Recover from a failed release | Match the error prefix or label state in [Troubleshooting](troubleshooting.md). |
| Verify a downloaded archive or container image | Check signatures and provenance with [Artifact verification](verification.md). |
| See or disable the anonymous analytics | Review the payload and opt-out options in [Telemetry](telemetry.md). |
| Move from release-please | Follow the ordered [Migration guide](migrate-from-release-please.md). |
| Contribute or report a vulnerability | Read [Contributing](../CONTRIBUTING.md) or use the private process in [Security Policy](../SECURITY.md). |

The complete `.yeet.yaml` field reference is [`yeet.schema.json`](../yeet.schema.json). CLI flags and environment variables are also available through `yeet --help` and `yeet <command> --help`.
