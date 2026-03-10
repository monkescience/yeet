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
