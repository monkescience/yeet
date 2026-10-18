## release

### Features

* feat: add a thing
* feat: support configurable retry budgets for outbound webhook deliveries
* feat: expose per-target changelog paths in the release summary output

### Bug Fixes

* fix: keep trailing newlines when rewriting version files in place
* fix: report the failing unit name when a manifest target cannot be resolved

<!-- yeet-release-manifest
{"base_branch":"main","targets":[{"id":"default","type":"path","tag":"v1.1.0","changelog_file":"CHANGELOG.md"}]}
-->
