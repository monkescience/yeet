# Changelog

## [v0.8.1](https://github.com/monkescience/yeet/compare/v0.8.0...v0.8.1) (2026-05-04)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.20.1 (#84) ([36c41df](https://github.com/monkescience/yeet/commit/36c41df243e71e30a3d5c1a0a039756345058ee7))

## [v0.8.0](https://github.com/monkescience/yeet/compare/v0.7.2...v0.8.0) (2026-05-02)

### ⚠ BREAKING CHANGES

- **versionfile:** validate marker scopes against versioning scheme ([da46040](https://github.com/monkescience/yeet/commit/da46040428dd2b797ed601a308e88bc27a64c769))
### Features

- **versionfile:** validate marker scopes against versioning scheme ([da46040](https://github.com/monkescience/yeet/commit/da46040428dd2b797ed601a308e88bc27a64c769))
### Bug Fixes

- **deps:** update module github.com/google/go-github/v84 to v85 (#81) ([9b20342](https://github.com/monkescience/yeet/commit/9b203429c343e924a2f62b206d869adbcd290a87))
- **deps:** update module github.com/monkescience/testastic to v0.3.4 (#79) ([e5b18b4](https://github.com/monkescience/yeet/commit/e5b18b47ea6a88f74b7bb1501b03c43b87731fb3))

### Migration Notes

`version_files` markers are now validated against the project's versioning scheme. Calver projects that previously used the positional aliases `x-yeet-major`, `x-yeet-minor`, or `x-yeet-patch` must rename them to the calver-native scopes; the validator's error names the replacement.

Allowed marker scopes:

| Scheme | Allowed scopes |
|---|---|
| semver | `version`, `major`, `minor`, `patch` |
| calver | `version`, `year`, `micro`, plus `month` / `week` / `day` only when the configured calver format includes that token |

Rename map for calver projects: `major` → `year`, `minor` → `month` (or `week` if the format uses `WW`), `patch` → `micro`. Block markers (`x-yeet-start-<scope>` … `x-yeet-end`) follow the same rules.

## [v0.7.2](https://github.com/monkescience/yeet/compare/v0.7.1...v0.7.2) (2026-05-01)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.20.0 (#76) ([7dc4f0f](https://github.com/monkescience/yeet/commit/7dc4f0fd7f01caa321b202ce75c56e85df6abde7))

## [v0.7.1](https://github.com/monkescience/yeet/compare/v0.7.0...v0.7.1) (2026-04-30)

### Bug Fixes

- **changelog:** separate prepended release entries ([b7fb76f](https://github.com/monkescience/yeet/commit/b7fb76fd002492e734fb96cb57044b6dfa7ebffd))

## [v0.7.0](https://github.com/monkescience/yeet/compare/v0.6.4...v0.7.0) (2026-04-30)

### ⚠ BREAKING CHANGES

- **release:** use changelog as release notes source ([092b6ed](https://github.com/monkescience/yeet/commit/092b6ed7dca652c269c72e56e5a2fc95ebfd5b78))
### Features

- **release:** use changelog as release notes source ([092b6ed](https://github.com/monkescience/yeet/commit/092b6ed7dca652c269c72e56e5a2fc95ebfd5b78))

### Migration Notes

Final GitHub/GitLab release notes now come only from the committed changelog entry. Move any custom notes from the release PR/MR body into the matching `CHANGELOG.md` entry before merging.

Add custom notes as separate `###` sections, such as `### Migration Notes`, so rerunning `yeet release` preserves them. Generated conventional-commit sections like `### Features` and `### Bug Fixes` may be regenerated on rerun.

## [v0.6.4](https://github.com/monkescience/yeet/compare/v0.6.3...v0.6.4) (2026-04-28)

### Bug Fixes

- **release:** append custom notes after changelog ([effeebf](https://github.com/monkescience/yeet/commit/effeebf7e5f08fb97d85d311c2845bd854db51c8))

## [v0.6.3](https://github.com/monkescience/yeet/compare/v0.6.2...v0.6.3) (2026-04-28)

### Features

- **release:** add prerelease channels ([6ccf678](https://github.com/monkescience/yeet/commit/6ccf6787ccad23bdf0299b74f078cd4d7d69859c))
### Bug Fixes

- **release:** tolerate normalized release markers ([4e9d33c](https://github.com/monkescience/yeet/commit/4e9d33ccdd3e8c14631dc354db58fd963c1dac9f))

## [v0.6.2](https://github.com/monkescience/yeet/compare/v0.6.1...v0.6.2) (2026-04-26)

### Bug Fixes

- **versionfile:** support custom calver markers ([b7cbab5](https://github.com/monkescience/yeet/commit/b7cbab5b3cc012b87c8c66b78f8fe01060405e5b))
- **config:** validate target versioning ([f3f98f8](https://github.com/monkescience/yeet/commit/f3f98f85370980a35079e3574d0bf7525cda44aa))

## [v0.6.1](https://github.com/monkescience/yeet/compare/v0.6.0...v0.6.1) (2026-04-26)

### Features

- **version:** support configurable calver formats ([be0326e](https://github.com/monkescience/yeet/commit/be0326ef1a7501b0a2c4c9c38c7da7371f0dbd88))
### Bug Fixes

- **deps:** update module github.com/monkescience/testastic to v0.3.0 (#66) ([b111e3d](https://github.com/monkescience/yeet/commit/b111e3d2e20d29ebae868f4e08887efe537e8275))

## [v0.6.0](https://github.com/monkescience/yeet/compare/v0.5.1...v0.6.0) (2026-04-26)

### Action Required
Provider auto-detection now only supports the public hosts `github.com` and `gitlab.com`.
If you use GitHub Enterprise, GitLab self-managed, or another custom domain, set `provider` explicitly in `.yeet.yaml`:

```yaml
provider: github
```
or
```yaml
provider: gitlab
```

### ⚠ BREAKING CHANGES

- **provider:** restrict auto-detection to public hosts ([d9bf83a](https://github.com/monkescience/yeet/commit/d9bf83a9139e02a28326331580ae8e52110e0b50))
### Features

- **release:** support editable release notes ([6c454d1](https://github.com/monkescience/yeet/commit/6c454d13f173d9ef3d0bd3e57a86c5733e2c7101))
### Bug Fixes

- **provider:** restrict auto-detection to public hosts ([d9bf83a](https://github.com/monkescience/yeet/commit/d9bf83a9139e02a28326331580ae8e52110e0b50))

## [v0.5.1](https://github.com/monkescience/yeet/compare/v0.5.0...v0.5.1) (2026-04-25)

### Features

- **release:** support commit override release notes ([69ebd2d](https://github.com/monkescience/yeet/commit/69ebd2d6cb72b23ee126ae5b9faa1b8a3e488326))
### Bug Fixes

- **deps:** update module github.com/charmbracelet/log to v2 (#61) ([256fc1d](https://github.com/monkescience/yeet/commit/256fc1dcb10b0efdd42d3156c4efac68fe85d580))
- **deps:** update module github.com/monkescience/testastic to v0.2.1 (#63) ([951ccaf](https://github.com/monkescience/yeet/commit/951ccafb9778b33a2fbbf4e1c915428a229457e8))

## [v0.5.0](https://github.com/monkescience/yeet/compare/v0.4.15...v0.5.0) (2026-04-22)

### ⚠ BREAKING CHANGES

- **versionfile:** reject misconfigured version files instead of silently skipping ([7e6991c](https://github.com/monkescience/yeet/commit/7e6991c943254dc143f374a3d73b97de60620cbf))
### Features

- **init:** write minimal config with target named after the directory ([15320df](https://github.com/monkescience/yeet/commit/15320dfbf708ca9d891651a05734855d3952e19a))
### Bug Fixes

- **versionfile:** require comment prefix on markers so prose mentions are skipped ([de7f086](https://github.com/monkescience/yeet/commit/de7f08615a4e452cd36e1abe479284ba013f2e5b))
- **versionfile:** reject misconfigured version files instead of silently skipping ([7e6991c](https://github.com/monkescience/yeet/commit/7e6991c943254dc143f374a3d73b97de60620cbf))

## [v0.4.15](https://github.com/monkescience/yeet/compare/v0.4.14...v0.4.15) (2026-04-21)

### Features

- print module checksum on go install builds ([8c8c1ca](https://github.com/monkescience/yeet/commit/8c8c1ca13ffe862acac40e8315bc1bc5207b5746))

## [v0.4.14](https://github.com/monkescience/yeet/compare/v0.4.13...v0.4.14) (2026-04-21)

### Features

- expose version metadata via debug.ReadBuildInfo fallback ([591f048](https://github.com/monkescience/yeet/commit/591f0483db9e80d932d208930caa34163c93fb38))

## [v0.4.13](https://github.com/monkescience/yeet/compare/v0.4.12...v0.4.13) (2026-04-21)

### Bug Fixes

- use OS.mac? guard for Homebrew cask xattr postflight ([96a9175](https://github.com/monkescience/yeet/commit/96a917540a08c7040d7837d95c0de82304be9589))

## [v0.4.12](https://github.com/monkescience/yeet/compare/v0.4.11...v0.4.12) (2026-04-21)

### Bug Fixes

- guard Homebrew cask xattr postflight with on_macos ([3e737d1](https://github.com/monkescience/yeet/commit/3e737d159004fffe85e71153d0f7f0b4a902803b))

## [v0.4.11](https://github.com/monkescience/yeet/compare/v0.4.10...v0.4.11) (2026-04-20)

### Features

- release Windows binaries via Scoop bucket ([50678e8](https://github.com/monkescience/yeet/commit/50678e80eb69d47d35c762c56da4a4361739e5b6))

## [v0.4.10](https://github.com/monkescience/yeet/compare/v0.4.9...v0.4.10) (2026-04-17)

### Features

- use charmbracelet/log for pretty CLI output and improve dry-run formatting ([b488f85](https://github.com/monkescience/yeet/commit/b488f85c7db7ee6b9d2474a70df3e025bb2e55fa))
- **ci:** enable auto-merge on homebrew-tap PRs ([48c4523](https://github.com/monkescience/yeet/commit/48c452365ff8ffcac0fcb96f995fccaa428356dd))
### Bug Fixes

- add pagination safety limits and warn on invalid changelog regex patterns ([ff7d485](https://github.com/monkescience/yeet/commit/ff7d48558aa368e1dc203b29cfd8300c8249fae4))

## [v0.4.9](https://github.com/monkescience/yeet/compare/v0.4.8...v0.4.9) (2026-04-17)

### Features

- add configurable bump_types mapping ([ef41b76](https://github.com/monkescience/yeet/commit/ef41b7601bb418c643f8a4f8089c0cecbdc3e9c5))

## [v0.4.8](https://github.com/monkescience/yeet/compare/v0.4.7...v0.4.8) (2026-04-10)

### Bug Fixes

- **deps:** migrate gitlab client to v2.17.0 ([50b2624](https://github.com/monkescience/yeet/commit/50b2624a07eb004fb3b78dc007ae9f0de4538421))
- **deps:** update module gitlab.com/gitlab-org/api/client-go to v2 (#44) ([1118f34](https://github.com/monkescience/yeet/commit/1118f34cf2285e62115adb4b9629f03737696380))
- **deps:** update module github.com/monkescience/testastic to v0.2.0 (#42) ([c9c4c44](https://github.com/monkescience/yeet/commit/c9c4c44faf36c91034c61840dc84a5509219cce2))
- **deps:** update module github.com/go-git/go-git/v5 to v5.17.2 (#40) ([ec0ff94](https://github.com/monkescience/yeet/commit/ec0ff94fa0f9119e31a30a012b1374c1e7a827ea))

## [v0.4.7](https://github.com/monkescience/yeet/compare/v0.4.6...v0.4.7) (2026-04-03)

### Bug Fixes

- **deps:** update module github.com/go-git/go-git/v5 to v5.17.1 [security] (#34) ([1384940](https://github.com/monkescience/yeet/commit/138494085361a5a66144bb05e6d651c666f29320))

## [v0.4.6](https://github.com/monkescience/yeet/compare/v0.4.5...v0.4.6) (2026-03-27)

### Features

- **changelog:** support issue/ticket reference linking ([91a30d6](https://github.com/monkescience/yeet/commit/91a30d6b5cfd3a147d91f423661e52e399ed0c39))
### Bug Fixes

- **provider:** add nil response checks in pagination loops ([1856264](https://github.com/monkescience/yeet/commit/185626493927cb1729b39c15a8da0e6997640745))
- **version:** normalize calver month padding and reject negative micro ([8330d4c](https://github.com/monkescience/yeet/commit/8330d4cbe10bb520b191262a80deededf19a4aa2))
- **version:** validate calver year and month segments ([98170b3](https://github.com/monkescience/yeet/commit/98170b35eb74c12427350412b4317c2a1c42e45e))
- **provider:** add pagination safety limit to tag and commit fetching ([a5d85f9](https://github.com/monkescience/yeet/commit/a5d85f93ed8cd76712c03261e51127a35887bde6))

## [v0.4.5](https://github.com/monkescience/yeet/compare/v0.4.4...v0.4.5) (2026-03-24)

### Bug Fixes

- **goreleaser:** strip quarantine attribute on macOS cask install ([3ee0710](https://github.com/monkescience/yeet/commit/3ee07103d3d674dc5e8aecec64e566b0b0a9d6cd))

## [v0.4.4](https://github.com/monkescience/yeet/compare/v0.4.3...v0.4.4) (2026-03-24)

### Bug Fixes

- **goreleaser:** add branch for homebrew cask PR creation ([54f1d96](https://github.com/monkescience/yeet/commit/54f1d96241f5d37ca836afa140f48229a3b06098))

## [v0.4.3](https://github.com/monkescience/yeet/compare/v0.4.2...v0.4.3) (2026-03-24)

### Bug Fixes

- **commit:** parse multi-line footer values per conventional commits spec ([cb80d29](https://github.com/monkescience/yeet/commit/cb80d290f2d1ce7ee80d95a09e761e2612ed3073))

## [v0.4.2](https://github.com/monkescience/yeet/compare/v0.4.1...v0.4.2) (2026-03-24)

### Features

- add Homebrew tap and binary releases via GoReleaser ([b9ce0c5](https://github.com/monkescience/yeet/commit/b9ce0c584ec2952c0e6599e53931eeb1872bb9fd))
### Bug Fixes

- **release:** update GitHub app ID and private key variables in release.yaml ([c885823](https://github.com/monkescience/yeet/commit/c8858237fbe869d34beaae67684af1f579eed2f0))
- **binaries:** update Homebrew tap app ID and private key variables ([5b80e51](https://github.com/monkescience/yeet/commit/5b80e512ead68cf3b9c06f317fd25d7836e7fe86))

## [v0.4.1](https://github.com/monkescience/yeet/compare/v0.4.0...v0.4.1) (2026-03-23)

### Features

- **version:** add configurable pre-major semver bump behavior ([21b7856](https://github.com/monkescience/yeet/commit/21b78562cef62ca199f9b5f412a5ee1afba82413))
### Bug Fixes

- **docs:** close unclosed block scope in version marker docs ([c11f364](https://github.com/monkescience/yeet/commit/c11f3645971e6dc5d574b6ddc480acb4b9a08384))
- **config:** resolve linter issues in resolveTarget ([3be49c5](https://github.com/monkescience/yeet/commit/3be49c51ad5c96a9cc695674641389f03261c9df))

## [v0.4.0](https://github.com/monkescience/yeet/compare/v0.3.0...v0.4.0) (2026-03-22)

### ⚠ BREAKING CHANGES

- **release:** remove backward-compatibility shims ([4895431](https://github.com/monkescience/yeet/commit/489543163055fc70a90205dcc2a12d841fa5014f))
- **release:** add monorepo release targets ([ae88ab3](https://github.com/monkescience/yeet/commit/ae88ab3220bc3dddb1225645f0c88d51eee4dfe0))
### Features

- **release:** add monorepo release targets ([ae88ab3](https://github.com/monkescience/yeet/commit/ae88ab3220bc3dddb1225645f0c88d51eee4dfe0))
### Bug Fixes

- **provider:** replace fragile string matching in CreateBranch error handling ([c955876](https://github.com/monkescience/yeet/commit/c95587683297e23f4f663018ca1a4c2285d4efa4))
### Performance Improvements

- **provider:** add HTTP resilience and parallel commit path fetching ([18e674c](https://github.com/monkescience/yeet/commit/18e674c5fae5abcc6fd015d08848235419c63eec))

## [v0.3.0](https://github.com/monkescience/yeet/compare/v0.2.2...v0.3.0) (2026-03-21)

### ⚠ BREAKING CHANGES

- **config:** make provider auto-detection explicit ([90aef14](https://github.com/monkescience/yeet/commit/90aef1490fa164f065dc0031b504784f7dfa70fa))
- **config:** switch config format from toml to yaml ([209ec3a](https://github.com/monkescience/yeet/commit/209ec3acccdb2169fe11ef5f60df494bcc59b45b))
### Features

- **config:** make provider auto-detection explicit ([90aef14](https://github.com/monkescience/yeet/commit/90aef1490fa164f065dc0031b504784f7dfa70fa))
- **config:** switch config format from toml to yaml ([209ec3a](https://github.com/monkescience/yeet/commit/209ec3acccdb2169fe11ef5f60df494bcc59b45b))
- **cli:** discover config from ancestor directories ([2ea9c27](https://github.com/monkescience/yeet/commit/2ea9c271633e4142881e74d5f5d772245e00b3b8))
- **cli:** add json log format option ([db7e957](https://github.com/monkescience/yeet/commit/db7e957a74f9191f7de94d9cf077eac462efa009))
- **cli:** support explicit repository targeting ([97722a5](https://github.com/monkescience/yeet/commit/97722a5366f30d58733402d07f14a3972f634baa))
- **cli:** add version command and log controls ([ebc53bd](https://github.com/monkescience/yeet/commit/ebc53bd5809ed82fbd9e1fed947c55e9ec391006))
### Bug Fixes

- **cli:** clarify release defaults and errors ([b64a72d](https://github.com/monkescience/yeet/commit/b64a72ddc742f987aada70c7318e6831edbc0647))

## [v0.2.2](https://github.com/monkescience/yeet/compare/v0.2.1...v0.2.2) (2026-03-16)

### Bug Fixes

- **cli:** honor custom config path during init ([5327c35](https://github.com/monkescience/yeet/commit/5327c35cd098c77366bf45dd65711b51005d1824))
- **release:** reuse exact releases and pick reachable baselines ([75df5e9](https://github.com/monkescience/yeet/commit/75df5e959836af7d86bc75c37147587ff423dbee))

## [v0.2.1](https://github.com/monkescience/yeet/compare/v0.2.0...v0.2.1) (2026-03-15)

### Bug Fixes

- **changelog:** normalize generated changelog header ([cdb3312](https://github.com/monkescience/yeet/commit/cdb33127fae590a841fcdb35e2f6476af23bb14a))

## [v0.2.0](https://github.com/monkescience/yeet/compare/v0.1.4...v0.2.0) (2026-03-10)

### ⚠ BREAKING CHANGES

- move config schema to project root ([9b18638](https://github.com/monkescience/yeet/commit/9b186382aaa66d74db42ccf6058c5d2d6318681e))

## [v0.1.4](https://github.com/monkescience/yeet/compare/v0.1.3...v0.1.4) (2026-03-10)

### Bug Fixes

- **cli:** read git remotes without git binary ([f050a5d](https://github.com/monkescience/yeet/commit/f050a5d2a220bfc74b81df7df442fcffc61e5a9c))
- **release:** pass target refs when creating releases ([5714831](https://github.com/monkescience/yeet/commit/5714831ef6fad288d6734230f909866aed542095))

## [v0.1.3](https://github.com/monkescience/yeet/compare/v0.1.2...v0.1.3) (2026-03-08)

### Features

- **provider:** support explicit repository targeting ([2e1fdcf](https://github.com/monkescience/yeet/commit/2e1fdcfb6334a534d2a68a0f849b62054188300e))
### Bug Fixes

- **release:** fail when previous release ref is unreachable ([a1315de](https://github.com/monkescience/yeet/commit/a1315de0873f29ec0f9faffbcaea8a0005ac794b))

## [v0.1.2](https://github.com/monkescience/yeet/compare/v0.1.1...v0.1.2) (2026-03-06)

### Features

- **config:** add TOML schema support ([0af2b52](https://github.com/monkescience/yeet/commit/0af2b520e86e31e43bfd0a03f8d1b33c9bcc6cf7))
- **release:** add auto-merge with force and method selection ([68566f5](https://github.com/monkescience/yeet/commit/68566f51b4ebd3ef452929a6b9a33530aa0b4b2c))

## [v0.1.1](https://github.com/monkescience/yeet/compare/v0.1.0...v0.1.1) (2026-03-01)

### Bug Fixes

- **release:** use head SHA compare link in PR body ([d3c66bf](https://github.com/monkescience/yeet/commit/d3c66bfd29ed30ddfac954cc58beabc1c5ef9006))
- **release:** preserve changelog history on release ([450baf1](https://github.com/monkescience/yeet/commit/450baf1b435d28ac88dc82cbf99613dd81031930))
- **release:** avoid duplicate PRs after finalize ([d064a32](https://github.com/monkescience/yeet/commit/d064a32b84d12496b1b1aadc65a8f9ce669f0ede))

## v0.1.0 (2026-03-01)

### ⚠ BREAKING CHANGES

- **release:** auto-finalize merged release PRs ([02a0c50](https://github.com/monkescience/yeet/commit/02a0c50804e8f3e70a11d4eaed4389c41bddff35))
### Features

- **release:** reuse single pending PR on stable branch ([b19caa6](https://github.com/monkescience/yeet/commit/b19caa6ecb8d81a93a076933dce17b9f1590065d))
- **release:** auto-finalize merged release PRs ([02a0c50](https://github.com/monkescience/yeet/commit/02a0c50804e8f3e70a11d4eaed4389c41bddff35))
- **release:** add configurable PR body header and footer ([921738e](https://github.com/monkescience/yeet/commit/921738e4f0d3a33446c44af969a6e9fde6d25fd6))
- **release:** support scoped subjects and force branch rewrites ([0710b6d](https://github.com/monkescience/yeet/commit/0710b6d1be290bfdcd97ad638396bff8ceae6a34))
- **release:** support Release-As footer with strict semver ([d4bfae6](https://github.com/monkescience/yeet/commit/d4bfae6809eb70846af304da0446df2510cdae53))
- **version:** scale semver bumps before 1.0.0 ([5fde279](https://github.com/monkescience/yeet/commit/5fde279676f631573333942830cf5b5d40b78930))
- **release:** add preview build metadata versions ([1e30149](https://github.com/monkescience/yeet/commit/1e30149d48daf0098ef39fc45756294ff044b4a8))
- **release:** update configured version files with yeet markers ([f2f184d](https://github.com/monkescience/yeet/commit/f2f184d06d863face1dd93bcb3df34b4fb66ae8a))
- add GITHUB_URL support for GitHub Enterprise ([3250b28](https://github.com/monkescience/yeet/commit/3250b283ad263ce60b0fd27c977025347168fb5b))
- **changelog:** add linked commits, compare URLs, and release-please style ([ee0e80a](https://github.com/monkescience/yeet/commit/ee0e80a5506195e0e516efe0fba17bc266780607))
- initial implementation of yeet CLI ([3533060](https://github.com/monkescience/yeet/commit/35330604c84b723170d7457a540889d6287b5259))
