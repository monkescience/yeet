# Changelog

## [v0.14.7](https://github.com/monkescience/yeet/compare/v0.14.6...v0.14.7) (2026-09-02)

### Bug Fixes

- **deps:** update module github.com/yuin/goldmark to v2 ([213c567](https://github.com/monkescience/yeet/commit/213c5671079dd2a3346c8291f014b661d97d1624))
- **changelog:** ignore preamble headings ([0c689f2](https://github.com/monkescience/yeet/commit/0c689f2188846c11d1cdbc0b1b3fdedfb6e306bb))
- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.60.0 (#251) ([3148e11](https://github.com/monkescience/yeet/commit/3148e1112674e2f81fa0f58ba7c6090d092e9048))
- **deps:** update module github.com/monkescience/testastic to v0.4.4 (#248) ([4238777](https://github.com/monkescience/yeet/commit/42387771d9a6fea00be3b715ac3d84810eac1b89))

## [v0.14.6](https://github.com/monkescience/yeet/compare/v0.14.5...v0.14.6) (2026-09-01)

### Bug Fixes

- **deps:** update x/crypto to v0.55.0 ([5c14034](https://github.com/monkescience/yeet/commit/5c140345d2ed81f6b55ea532145264fb1b4c5072))
- **commands:** show dry-run link destinations ([2368b17](https://github.com/monkescience/yeet/commit/2368b17e95a264e1888ee830861710d8b247de15))

## [v0.14.5](https://github.com/monkescience/yeet/compare/v0.14.4...v0.14.5) (2026-08-31)

### Bug Fixes

- **provider:** handle truncated GitHub trees ([5d44d01](https://github.com/monkescience/yeet/commit/5d44d010e3ca450941b2e6c58fb5fbd73b4b6f06))

## [v0.14.4](https://github.com/monkescience/yeet/compare/v0.14.3...v0.14.4) (2026-08-30)

### Bug Fixes

- **provider:** bind release merge to base branch ([8996241](https://github.com/monkescience/yeet/commit/8996241385ad1e9c1bf384a69c691207eb8fb48c))

## [v0.14.3](https://github.com/monkescience/yeet/compare/v0.14.2...v0.14.3) (2026-08-29)

### Features

- **release:** support independent monorepo PRs ([e91a94e](https://github.com/monkescience/yeet/commit/e91a94ede511ef8254512c916fed5477e6c5a6ef))

## [v0.14.2](https://github.com/monkescience/yeet/compare/v0.14.1...v0.14.2) (2026-08-27)

### Features

- **release:** improve dry-run output ([2d41cfa](https://github.com/monkescience/yeet/commit/2d41cfab7b0abbc046f4dc222e41b43884849e91))
- **release:** centralize rendered release text ([d0cebb2](https://github.com/monkescience/yeet/commit/d0cebb2d13f1f4d5014a692af074432b0479cd91))

## [v0.14.1](https://github.com/monkescience/yeet/compare/v0.14.0...v0.14.1) (2026-08-22)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.58.2 (#238) ([ee29608](https://github.com/monkescience/yeet/commit/ee296085212746cc7b0c6dfa3268ac8c5f1b1481))

## [v0.14.0](https://github.com/monkescience/yeet/compare/v0.13.8...v0.14.0) (2026-08-22)

### Migration Notes

Official yeet binaries and images now send limited anonymous telemetry for the `init` and
`release` commands by default.

Disable telemetry for a repository:

```yaml
telemetry:
  enabled: false
```

Or disable it globally for tools that support `DO_NOT_TRACK`:

```sh
export DO_NOT_TRACK=1
```

See [Telemetry](docs/telemetry.md) for the complete payload, behavior, and privacy details.

### ⚠ BREAKING CHANGES

- add anonymous usage telemetry ([654cfe5](https://github.com/monkescience/yeet/commit/654cfe58850f719d6133546674d26d67c5c97e38))

### Features

- add anonymous usage telemetry ([654cfe5](https://github.com/monkescience/yeet/commit/654cfe58850f719d6133546674d26d67c5c97e38))

## [v0.13.8](https://github.com/monkescience/yeet/compare/v0.13.7...v0.13.8) (2026-08-21)

### Bug Fixes

- **deps:** update module charm.land/lipgloss/v2 to v2.0.6 (#229) ([71a7115](https://github.com/monkescience/yeet/commit/71a711581ec5636c3d8ec7d2e1ad72067f60a868))
- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.58.0 (#232) ([d562c9a](https://github.com/monkescience/yeet/commit/d562c9af3b285bd373976d1e47422eba7c9060a7))
- **deps:** update module github.com/charmbracelet/x/ansi to v0.11.8 (#230) ([8e05ee4](https://github.com/monkescience/yeet/commit/8e05ee4952f981f96628fdf944d3ea01a56e94dc))

## [v0.13.7](https://github.com/monkescience/yeet/compare/v0.13.6...v0.13.7) (2026-08-16)

### Bug Fixes

- **tools:** bump mise Go version to 1.26.6 ([52bf1ab](https://github.com/monkescience/yeet/commit/52bf1ab609b9b84477085c334f5cdb0ad512ab7f))
- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.56.0 (#225) ([624b830](https://github.com/monkescience/yeet/commit/624b830c99a474a90744c42cad706d81d0bdaa04))

## [v0.13.6](https://github.com/monkescience/yeet/compare/v0.13.5...v0.13.6) (2026-08-13)

### Features

- **release:** configure provider release names ([81c8bb6](https://github.com/monkescience/yeet/commit/81c8bb6667bef295930651f254f1f014109746b2))
- **config:** support provider API and web URLs ([f77e17f](https://github.com/monkescience/yeet/commit/f77e17f7d15f39f6315412d2f5ef688803d360fa))
- **config:** support release timezones ([b5fb403](https://github.com/monkescience/yeet/commit/b5fb40348de015e78a62e9817d87464540d637c7))
- **config:** configure release merge polling ([c0f0fad](https://github.com/monkescience/yeet/commit/c0f0fad8a4c361d337a794ea7c41276ac2951594))
- **config:** configure provider network behavior ([64467de](https://github.com/monkescience/yeet/commit/64467de4309d4977cce2a5b5ef84d0b10c0e2dd7))
- **config:** support release branch templates ([66ba723](https://github.com/monkescience/yeet/commit/66ba723812a0e672159943fed7f7f5141d9f1a54))

### Bug Fixes

- **config:** validate changelog section headings ([7e5c23c](https://github.com/monkescience/yeet/commit/7e5c23cae8dc11318fd116ae9a4682835757dbab))
- **release:** normalize trusted manifest paths ([7f4810d](https://github.com/monkescience/yeet/commit/7f4810d52eb7e165647f1c9cd334b190788fe68b))
- **provider:** apply configured request timeouts ([3d71c70](https://github.com/monkescience/yeet/commit/3d71c700e098a91f5984dbb4c902467f0484a2a3))
- **config:** handle Windows repository paths ([6a5905d](https://github.com/monkescience/yeet/commit/6a5905d8efb3b09ed2e20576ae30db062c7188fd))
- **changelog:** honor configured breaking heading ([e82eb0d](https://github.com/monkescience/yeet/commit/e82eb0d8b397b7921eeee6b38112a25012ae1b75))
- **provider:** preserve GitHub file modes ([a979ce9](https://github.com/monkescience/yeet/commit/a979ce9a6fa177c0b92635da7da68c29af4fd82b))
- **config:** validate repository file paths ([3f0f738](https://github.com/monkescience/yeet/commit/3f0f7385f0a427f8c17139c5ac8598893a4ef415))
- **config:** reject blank stable branches ([5bd2f7c](https://github.com/monkescience/yeet/commit/5bd2f7cef4fc0a851769bb22522fd83282c2a4e6))
- **cli:** reject positional arguments ([4645dfd](https://github.com/monkescience/yeet/commit/4645dfd7a329e843ebeb79f9f4f5b048405e7768))
- **cli:** scope config flag to config commands ([e4cd2c1](https://github.com/monkescience/yeet/commit/e4cd2c11e0e2494048ec9d9575fed29bde79aa11))
- **cli:** clean up help indentation ([21f4c99](https://github.com/monkescience/yeet/commit/21f4c99315d39c36bf09617f08649d131c242e8b))

## [v0.13.5](https://github.com/monkescience/yeet/compare/v0.13.4...v0.13.5) (2026-08-13)

### Features

- **version:** move release-as and prerelease progression behind the version strategy ([be4ee9f](https://github.com/monkescience/yeet/commit/be4ee9f49247fa8fb5b50970e1f7273ee11f5f75))

### Bug Fixes

- **provider:** reject undated merged GitHub PRs ([211dd6a](https://github.com/monkescience/yeet/commit/211dd6abed216a23e192b45827ad18266cc50b07))
- **changelog:** preserve parent sections by identity ([75faf7e](https://github.com/monkescience/yeet/commit/75faf7ec183b6bc82cf0a305ea300162e554898f))
- **provider:** report a withheld merge time before re-reading further candidates ([1d950d6](https://github.com/monkescience/yeet/commit/1d950d6c0f2ee886608b10533b5cf4a6988ae436))
- **release:** restore the scheme fallbacks the strategy dispatch dropped ([c126073](https://github.com/monkescience/yeet/commit/c12607379acb20e046e76f692e34b4c777e6d12c))
- **changelog:** keep the release date on derived entries ([41213f1](https://github.com/monkescience/yeet/commit/41213f15f84713019c0ee0cfc4db6899fee1c283))

### Performance Improvements

- **release:** check channel membership once ([9e27f3d](https://github.com/monkescience/yeet/commit/9e27f3d617616c7ff97ce128dd2c3dbcd9ab8aa1))

## [v0.13.4](https://github.com/monkescience/yeet/compare/v0.13.3...v0.13.4) (2026-08-11)

### Bug Fixes

- **provider:** restore github tag creation ([0338023](https://github.com/monkescience/yeet/commit/03380237d9b0831196235dc4ae31d2d8c2ca75d4))

## [v0.13.3](https://github.com/monkescience/yeet/compare/v0.13.2...v0.13.3) (2026-08-11)

### Bug Fixes

- **provider:** harden release workflows ([e1e98d2](https://github.com/monkescience/yeet/commit/e1e98d288fb28da003a5a51e9e2a7218c8932a09))

## [v0.13.2](https://github.com/monkescience/yeet/compare/v0.13.1...v0.13.2) (2026-08-10)

### Bug Fixes

- **deps:** update module github.com/google/go-github/v89 to v90 (#217) ([e206dc9](https://github.com/monkescience/yeet/commit/e206dc975f774db9d2175ba5e2481f7a2d43fdbb))
- **version:** compare calver periods chronologically ([62268ed](https://github.com/monkescience/yeet/commit/62268ed8cc07fa6d4d4daa062f9b299e4a385200))
- **config:** reject blank changelog headings ([f8d76f2](https://github.com/monkescience/yeet/commit/f8d76f2d848ada39853f5bbf5fa79c6a83ef5911))
- **config:** reject duplicate changelog includes ([988b262](https://github.com/monkescience/yeet/commit/988b2620a1522cca28db66ebbc14562c110023ab))
- **changelog:** normalize closing heading hashes ([09a2de0](https://github.com/monkescience/yeet/commit/09a2de0778dcda939c41fe1a04acf8103c18b9ff))
- **changelog:** ignore indented code headings ([486b59a](https://github.com/monkescience/yeet/commit/486b59a6519afd061c8dad8d72af38c6a3bd4cb9))
- **release:** enforce PR body character limits ([187a648](https://github.com/monkescience/yeet/commit/187a64845b6a376c2f5c644ed59facd2c7bf3cc0))
- **changelog:** ignore fenced release headings ([958043b](https://github.com/monkescience/yeet/commit/958043bfbdef38f2f78fdffef4551820876a1ef1))
- **version:** reject future calver periods ([4e34fb7](https://github.com/monkescience/yeet/commit/4e34fb7615b1187b3d9bb4919a1d52653a046d73))

## [v0.13.1](https://github.com/monkescience/yeet/compare/v0.13.0...v0.13.1) (2026-08-10)

### Features

- **release:** classify failures with actionable remediation ([6052c53](https://github.com/monkescience/yeet/commit/6052c5388db9221052ad0f574de23cc3c478263a))

## [v0.13.0](https://github.com/monkescience/yeet/compare/v0.12.1...v0.13.0) (2026-08-08)

yeet now validates `.yeet.yaml` against
[`yeet.schema.json`](https://github.com/monkescience/yeet/blob/main/yeet.schema.json) before release
planning. Configurations that do not match the schema must be corrected before upgrading.

Object-form `version_files` entries must now set `format` explicitly. Use `markers` for files
containing yeet comment markers, or `json` with a `json_pointer` for JSON files.

```yaml
# Before
version_files:
  - path: VERSION

# v0.13.0
version_files:
  - path: VERSION
    format: markers
```

### ⚠ BREAKING CHANGES

- **config:** enforce the JSON schema as the config shape contract at load time ([b0c292d](https://github.com/monkescience/yeet/commit/b0c292d45c8411c26546e0478e61150e43670424))

### Features

- **config:** enforce the JSON schema as the config shape contract at load time ([b0c292d](https://github.com/monkescience/yeet/commit/b0c292d45c8411c26546e0478e61150e43670424))

### Bug Fixes

- **changelog:** preserve freeform release notes ([27efb95](https://github.com/monkescience/yeet/commit/27efb95c93406ec6c01965662ef938b38ba8a9b1))
- **provider:** stop polling terminal Azure merge refusals ([bf5bd8f](https://github.com/monkescience/yeet/commit/bf5bd8f98935ba7eca5d05c2f80998b576999a32))
- **release:** validate remote mutation prerequisites ([2878057](https://github.com/monkescience/yeet/commit/287805787d64ebad25397e98d1c5fe75c0fb7f2d))
- **provider:** share one merge driver and report why a merge is blocked ([fc2f3a4](https://github.com/monkescience/yeet/commit/fc2f3a478021dbfc3ea328855a46d1dfbb88c722))
- **release:** stop re-planning a published version from a stale forge tag list ([9d6e4d7](https://github.com/monkescience/yeet/commit/9d6e4d7a25b93da191c0fca02e7bb7a0a7b23afb))
- **changelog:** stop inheriting child target sections absent from the release wave ([c9c8166](https://github.com/monkescience/yeet/commit/c9c8166c6544e80135efe9d1ef49fc36bce45792))
- **provider:** page GitLab project member lookups for reviewer validation ([a6b665e](https://github.com/monkescience/yeet/commit/a6b665e4c41722c738091f6362bc377983dfa9e2))
- **provider:** page Azure DevOps ref lookups instead of scanning one page ([97357d0](https://github.com/monkescience/yeet/commit/97357d0765afd5e8a857d0dc5822dd70714cf828))
- **provider:** assert Azure DevOps release PRs belong to the configured repository ([07d62dc](https://github.com/monkescience/yeet/commit/07d62dcf92392f3029caf5c486b5cffb23100ab2))
- **provider:** trust only the host of the provider URL environment variable ([6bd891d](https://github.com/monkescience/yeet/commit/6bd891dadc3b7b4afc73b1b0acc32ed48d3adf1a))

## [v0.12.1](https://github.com/monkescience/yeet/compare/v0.12.0...v0.12.1) (2026-08-07)

### Bug Fixes

- **changelog:** separate section headings from the preceding section ([603c95f](https://github.com/monkescience/yeet/commit/603c95f808b3787c5856f9a6753157ce8bb24e4a))
- **release:** never inherit generator-owned changelog sections on refresh ([f353173](https://github.com/monkescience/yeet/commit/f3531739cb0a68f2b4c0926d41fdee2b55bc26a6))
- **release:** stop published changelog sections seeding the next entry ([04bc254](https://github.com/monkescience/yeet/commit/04bc25416aee6206bbe805af7b08c8718db374de))
- **release:** refresh remote tags before the post-finalize analysis ([ef05b26](https://github.com/monkescience/yeet/commit/ef05b266f0ad91bdec75571d10450bf418fc5605))

## [v0.12.0](https://github.com/monkescience/yeet/compare/v0.11.5...v0.12.0) (2026-08-07)

### ⚠ BREAKING CHANGES

- **gitlab:** prefer squash for the auto merge method ([00d7c0e](https://github.com/monkescience/yeet/commit/00d7c0ee85f91bf8e62bd55975f747aee0652b2f))
- configure release PR labels, titles, and commit subjects ([7f19b2c](https://github.com/monkescience/yeet/commit/7f19b2cac4a7d7f2042352975ced7c7b83d7e99c))
### Features

- configure release PR labels, titles, and commit subjects ([7f19b2c](https://github.com/monkescience/yeet/commit/7f19b2cac4a7d7f2042352975ced7c7b83d7e99c))
### Bug Fixes

- **provider:** recolor release labels with the Monokai Pro palette ([6ccc6c7](https://github.com/monkescience/yeet/commit/6ccc6c77287e11225b6a7a1d96ba6248010bdb89))
- **provider:** wait for the forge to finalize an accepted merge ([f4bfe05](https://github.com/monkescience/yeet/commit/f4bfe05a417ad21a5b59cb715a7573bc5014def2))
- **release:** keep finalized releases when auto-merge publishes a wave ([d8aceb4](https://github.com/monkescience/yeet/commit/d8aceb46a753ec07ed06528a337aca5548ca0427))
- **release:** reject a changelog colliding with a version file ([e431a06](https://github.com/monkescience/yeet/commit/e431a068a8b78d2968c48c24ba6d7005af936bb5))
- **release:** read preserved changelog edits from the manifest path ([413f05f](https://github.com/monkescience/yeet/commit/413f05fcee46679bdc46b711de6b68ba1e1f899c))
- **gitlab:** prefer squash for the auto merge method ([00d7c0e](https://github.com/monkescience/yeet/commit/00d7c0ee85f91bf8e62bd55975f747aee0652b2f))
- adopt unlabelled release PRs and harden release label handling ([aa1564e](https://github.com/monkescience/yeet/commit/aa1564ebb571b44e749172713d6d74b5a0f9a1f8))
- recover release runs from analysis and label lookup failures ([b0162b5](https://github.com/monkescience/yeet/commit/b0162b50bcac28eca7cb378613727998a5ae284a))

## [v0.11.5](https://github.com/monkescience/yeet/compare/v0.11.4...v0.11.5) (2026-07-31)

### Bug Fixes

- **deps:** update module github.com/go-git/go-git/v6 to v6.0.0-alpha.5 (#201) ([810ea05](https://github.com/monkescience/yeet/commit/810ea0544203751291e62b0591c66b20707e352f))
- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.55.1 (#202) ([20bb36b](https://github.com/monkescience/yeet/commit/20bb36b86c91c305b99062f9c4225b94b23f6185))

## [v0.11.4](https://github.com/monkescience/yeet/compare/v0.11.3...v0.11.4) (2026-07-25)

### Bug Fixes

- **deps:** update module github.com/monkescience/testastic to v0.4.1 (#195) ([46ead83](https://github.com/monkescience/yeet/commit/46ead83eef5c0f075fdecbc528306d61a081d1ae))
### Performance Improvements

- **commit:** fold footer continuations in linear time ([8eaf127](https://github.com/monkescience/yeet/commit/8eaf1271adfecd7a249cff2b6e82249d59d2d31e))

## [v0.11.3](https://github.com/monkescience/yeet/compare/v0.11.2...v0.11.3) (2026-07-24)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.51.0 (#193) ([89fc757](https://github.com/monkescience/yeet/commit/89fc757946e2c020e23cd8b508c0761aa5aab26f))
- **release:** wait for merged GitLab accept response ([53cfa02](https://github.com/monkescience/yeet/commit/53cfa02f23a03e8072472f26757a642a5e4e7578))
- **release:** handle GitLab fast-forward merge refs ([3321d72](https://github.com/monkescience/yeet/commit/3321d72030bbab268f594a8b57b30cc668f98101))
- **release:** require merged commit for publishing ([280c8c1](https://github.com/monkescience/yeet/commit/280c8c1d27a8f433f51b7288a048e1c3baf3d39c))
- **release:** reject provisional Azure merge commits ([a80152b](https://github.com/monkescience/yeet/commit/a80152bd9271994f24e95a97148dc083beceb31d))
- **release:** wait for final Azure merge commit ([fbb41fe](https://github.com/monkescience/yeet/commit/fbb41fe1816e31030c196cfda31a80b0a8c769f6))
### Performance Improvements

- **release:** use merge response commit ([f830b35](https://github.com/monkescience/yeet/commit/f830b354e98be8c1e6d34e14edcfdb8a6414866f))
- **release:** let file updates create branches ([4d44827](https://github.com/monkescience/yeet/commit/4d448273600a17fd3542e14e3ef506b67108a05d))
- **release:** skip redundant pending relabel ([4b45f77](https://github.com/monkescience/yeet/commit/4b45f7741408971fd50661bd2f953beabf400247))
- **release:** read finalized changelogs locally ([07916c3](https://github.com/monkescience/yeet/commit/07916c37d62d4614e0eca39f9e021126751b3784))
- **release:** skip redundant latest release lookup ([7606f40](https://github.com/monkescience/yeet/commit/7606f404472b9e04c08e28e8ee2ca9923575c770))

## [v0.11.2](https://github.com/monkescience/yeet/compare/v0.11.1...v0.11.2) (2026-07-22)

### Bug Fixes

- **commit:** validate conventional headers strictly ([d1b6bd5](https://github.com/monkescience/yeet/commit/d1b6bd575d40df91cb426dac2a706902c86ffece))

## [v0.11.1](https://github.com/monkescience/yeet/compare/v0.11.0...v0.11.1) (2026-07-22)

### Features

- **logging:** add sanitized provider HTTP traces ([3c18859](https://github.com/monkescience/yeet/commit/3c1885933ffe036ba66370a3fd4ce2bff1c41226))
### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.50.0 (#187) ([2431c6e](https://github.com/monkescience/yeet/commit/2431c6efc553b5d081013abe1acfc6e7a683cf1c))

## [v0.11.0](https://github.com/monkescience/yeet/compare/v0.10.20...v0.11.0) (2026-07-21)

### Migration Notes

yeet now reads release history from the local Git checkout. Release jobs must use a full,
non-shallow checkout whose HEAD matches the remote release branch.

#### GitHub Actions

```yaml
- uses: actions/checkout@<pinned-commit>
  with:
    fetch-depth: 0
```

#### GitLab CI

```yaml
variables:
  GIT_STRATEGY: fetch
  GIT_DEPTH: "0"
```

#### Azure Pipelines

```yaml
- checkout: self
  fetchDepth: 0
```

Commit overrides are now read from the final Git commit message. Overrides stored only in PR or MR
descriptions are ignored.

```text
chore: combine API changes

BEGIN_COMMIT_OVERRIDE
feat(auth): add OAuth token refresh
fix(api): return 401 for expired sessions
END_COMMIT_OVERRIDE
```

See [CI setup](docs/ci.md) and [commit overrides](docs/changelog-generation.md) for complete examples.

### ⚠ BREAKING CHANGES

- **release:** read overrides from local commits ([c754117](https://github.com/monkescience/yeet/commit/c75411781c44c218f86c9eb40f417f68d0df2aff))
- require a full local checkout for release history ([2429976](https://github.com/monkescience/yeet/commit/242997650d7b71505414df64d4872a6e3ed70d45))

### Features

- require a full local checkout for release history ([2429976](https://github.com/monkescience/yeet/commit/242997650d7b71505414df64d4872a6e3ed70d45))
- serve release history from the local git checkout ([284c4fd](https://github.com/monkescience/yeet/commit/284c4fdb60575aa22d43389175ce9933f906c107))

### Bug Fixes

- clarify CLI messages ([8203253](https://github.com/monkescience/yeet/commit/8203253e5d2d5e15d2532b7b847af8b59db5be62))
- **release:** preserve custom changelog section positions ([0ad9a4a](https://github.com/monkescience/yeet/commit/0ad9a4a63c3cc36a33665612d1005d24e3b8bf4c))
- **release:** validate history before mutations ([6d8361f](https://github.com/monkescience/yeet/commit/6d8361f258154364b0dfb244d46b02913d768949))
- **history:** avoid retaining per-ref ancestry ([38bf26f](https://github.com/monkescience/yeet/commit/38bf26fa1872d103d9005e6ee1d686da69aa0d26))
- **release:** validate local tags against remote ([7b9a791](https://github.com/monkescience/yeet/commit/7b9a7917bd7aa39c1c8c6ca1fda6c6437a9e3246))

### Performance Improvements

- **release:** cache changelog reads ([802675b](https://github.com/monkescience/yeet/commit/802675bfad311bfcf08c47e6d226cc79a32f7a88))
- **history:** reuse remote tag snapshot ([ef88f8f](https://github.com/monkescience/yeet/commit/ef88f8f257651c1b98916bdedb4905c3406b4cfa))
- **release:** avoid duplicate tag lookup ([77459a7](https://github.com/monkescience/yeet/commit/77459a798490adcbb3789fe6108cf635130a6598))
- **release:** avoid duplicate release lookup ([fb1ab7b](https://github.com/monkescience/yeet/commit/fb1ab7bd33161ec8c22e1bb4152e980ab99be512))
- **release:** read base files from local commits ([08ceeb1](https://github.com/monkescience/yeet/commit/08ceeb13fab36f074a09c207000d4e018bb76028))
- **release:** read overrides from local commits ([c754117](https://github.com/monkescience/yeet/commit/c75411781c44c218f86c9eb40f417f68d0df2aff))

## [v0.10.20](https://github.com/monkescience/yeet/compare/v0.10.19...v0.10.20) (2026-07-18)

### Bug Fixes

- **deps:** update module github.com/google/go-github/v88 to v89 (#179) ([badfdde](https://github.com/monkescience/yeet/commit/badfdde58537314c6b7fe74d88599c6e7a0995a1))

## [v0.10.19](https://github.com/monkescience/yeet/compare/v0.10.18...v0.10.19) (2026-07-18)

### Bug Fixes

- **tests:** isolate branch environment ([81c28ac](https://github.com/monkescience/yeet/commit/81c28ac3cbabdb912f3481d3fcfe2009daef6682))
- **release:** reject forged release pull requests ([d25d25d](https://github.com/monkescience/yeet/commit/d25d25d479c72808d3c187b728f6fbbbfec1ef11))
- **azuredevops:** paginate pull requests consistently ([d2effa3](https://github.com/monkescience/yeet/commit/d2effa32865682a0fa160345656f249eedd736b3))
- **commands:** allow CI pull request dry runs ([e690057](https://github.com/monkescience/yeet/commit/e6900577885003a329533374cd7b145f19522715))
- **provider:** accept exact pagination capacity ([7f58192](https://github.com/monkescience/yeet/commit/7f581922e0a38d7ccf3a3acfb88cbd3723a97c52))
- **commands:** reject GitHub non-branch refs ([df55a52](https://github.com/monkescience/yeet/commit/df55a52c9029db4fbafc77ea253985857b7eb732))
- **commands:** reject Azure non-branch refs ([fbd5af7](https://github.com/monkescience/yeet/commit/fbd5af71d1334f7164423eea1e82a5f11a5cd5f9))
- **azuredevops:** ignore folder change paths ([aaf3b98](https://github.com/monkescience/yeet/commit/aaf3b985500fa2afffbb7aac3d5ccdc8aa31882b))
- **commands:** detect Azure Pipelines branch ([9d4de52](https://github.com/monkescience/yeet/commit/9d4de520520cd8d2b4016e7c14486c162e5da0cd))
- **azuredevops:** preserve renamed commit paths ([0593ddc](https://github.com/monkescience/yeet/commit/0593ddc97eef7f51e5bc77a04517a657ab9bccf4))
- **azuredevops:** paginate commit changes ([323f312](https://github.com/monkescience/yeet/commit/323f31297d0a1a4c123243a56b800fde5a37700b))
- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.46.0 (#178) ([1a28a6a](https://github.com/monkescience/yeet/commit/1a28a6aa41d6a02a04a21705621457692b32657d))

## [v0.10.18](https://github.com/monkescience/yeet/compare/v0.10.17...v0.10.18) (2026-07-13)

### Performance Improvements

- **provider:** cache commit path lookups across overlapping refs ([638c21f](https://github.com/monkescience/yeet/commit/638c21f740dfac56108fda0285e9199c49fd69e0))

## [v0.10.17](https://github.com/monkescience/yeet/compare/v0.10.16...v0.10.17) (2026-07-11)

### Bug Fixes

- **azuredevops:** normalize configured legacy hosts ([618aca4](https://github.com/monkescience/yeet/commit/618aca46d7e53e08a59ce559f180cbb9069830fb))
- **cli:** honor disabled auto merge flag ([61d58cf](https://github.com/monkescience/yeet/commit/61d58cfdb1fdd4a98c2a14bc09796feddbb40e04))
- **config:** allow shared inherited version files ([69bdd75](https://github.com/monkescience/yeet/commit/69bdd756851391a495042d47842fbd834348fd4e))
- **gitlab:** allow transient merge statuses ([5662df7](https://github.com/monkescience/yeet/commit/5662df754f405d0663555845d1949209fec45b02))
- **gitlab:** preserve failed reviewer merge requests ([00eec24](https://github.com/monkescience/yeet/commit/00eec243ec8948524d4b1d73ebc10eb5b4d9c756))
- **gitlab:** reject unreachable version refs ([73c33bd](https://github.com/monkescience/yeet/commit/73c33bd3e13f8d9505597c8f6593d8a611f91ea9))

## [v0.10.16](https://github.com/monkescience/yeet/compare/v0.10.15...v0.10.16) (2026-07-11)

### Bug Fixes

- **changelog:** insert new entries before first release heading ([0d1d61d](https://github.com/monkescience/yeet/commit/0d1d61dae748df8f9cd1999ea1775a4051ce315f))
- **changelog:** preserve multiline footer spacing ([7ee994c](https://github.com/monkescience/yeet/commit/7ee994c74bf7162a34935189946f264d8610aac8))
- **commit:** parse compact breaking footers ([f517786](https://github.com/monkescience/yeet/commit/f517786a3ab16b4fc99bd170e240f6c8b7689356))
- **commit:** parse footers from final block ([4f7a3e1](https://github.com/monkescience/yeet/commit/4f7a3e1b1d7ddf4c54ea9c1e9fad8f66c5f5de75))
- **release:** find merged release PR via search instead of closed-PR scan ([427a1f4](https://github.com/monkescience/yeet/commit/427a1f4331c019ae2fbd874fd418726dba95e93f))
- **release:** preserve changelog edits across retags ([e9f6eb9](https://github.com/monkescience/yeet/commit/e9f6eb954618b5bb7349472a9e55bb910cbc585a))
- **release:** tag auto-merged release commit ([01b3cc9](https://github.com/monkescience/yeet/commit/01b3cc9014943f079cb4b75f35c7b8f640ab5d14))
- **release:** preserve channel on derived versions ([3166b10](https://github.com/monkescience/yeet/commit/3166b10ba367a2d4bd7a005eced25e0ca7744ee6))

## [v0.10.15](https://github.com/monkescience/yeet/compare/v0.10.14...v0.10.15) (2026-07-11)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.45.0 (#170) ([ef855ef](https://github.com/monkescience/yeet/commit/ef855ef527f2db464dd1d182b5b1d590baba9f4a))

## [v0.10.14](https://github.com/monkescience/yeet/compare/v0.10.13...v0.10.14) (2026-07-09)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.44.0 (#165) ([08fc63d](https://github.com/monkescience/yeet/commit/08fc63d2602c745eb03d420200bab9ea8c921323))
- **deps:** update module golang.org/x/sync to v0.22.0 (#166) ([65ea93d](https://github.com/monkescience/yeet/commit/65ea93ddbd89d1f2d8ca8dd21ce61b1e2b9681f7))

## [v0.10.13](https://github.com/monkescience/yeet/compare/v0.10.12...v0.10.13) (2026-07-09)

### Bug Fixes

- **build:** pin go toolchain to go1.26.5 ([2aa9683](https://github.com/monkescience/yeet/commit/2aa9683a22f1e0a0b021feb9fb072dd0ec284578))
- **provider:** only send auth token to trusted hosts ([7b716c5](https://github.com/monkescience/yeet/commit/7b716c56beff64e796505e096852acc4a6057752))
- **changelog:** strip control chars before escaping comment markers ([2378321](https://github.com/monkescience/yeet/commit/2378321641407a90019e3e03de46f5bc20482d31))

## [v0.10.12](https://github.com/monkescience/yeet/compare/v0.10.11...v0.10.12) (2026-07-06)

### Bug Fixes

- **commands:** let explicit GITHUB_URL and GITLAB_URL override host derivation ([fcb190b](https://github.com/monkescience/yeet/commit/fcb190bee34968ed3fe113c5988cf3c65b9673d0))
- **deps:** update module charm.land/lipgloss/v2 to v2.0.5 (#155) ([91026ee](https://github.com/monkescience/yeet/commit/91026ee2aab88efb2d3fb937dcf22471bb042df7))

## [v0.10.11](https://github.com/monkescience/yeet/compare/v0.10.10...v0.10.11) (2026-07-02)

### Bug Fixes

- **provider:** bound azure devops file reads to 10 MiB ([659f8f2](https://github.com/monkescience/yeet/commit/659f8f257fe86890c4da69e930be8650a5d22a36))
- **provider:** redact remote url credentials in unknown remote errors ([b3d35a4](https://github.com/monkescience/yeet/commit/b3d35a4cf9adcb5df470df598d332c436df8ad37))

## [v0.10.10](https://github.com/monkescience/yeet/compare/v0.10.9...v0.10.10) (2026-07-02)

### Features

- **release:** request reviewers on release PR creation ([bb55bee](https://github.com/monkescience/yeet/commit/bb55beee19fc3b899481aefbc26797126df6c36c))
### Bug Fixes

- **release:** drop author check that breaks GitHub App tokens ([0a2b0e9](https://github.com/monkescience/yeet/commit/0a2b0e9ceac90ecef8f874d857c069c7abe369ab))

## [v0.10.9](https://github.com/monkescience/yeet/compare/v0.10.8...v0.10.9) (2026-07-01)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.43.0 (#150) ([e0cdd20](https://github.com/monkescience/yeet/commit/e0cdd2030d87188ccfe47ced9bc0c5e034eee38c))
- **changelog:** sanitize commit text to block manifest and escape injection ([9d604df](https://github.com/monkescience/yeet/commit/9d604df9a107b3a1ec1b28ffc80707eb49381dd5))
- **release:** reject release PR bodies with multiple manifest markers ([5b91ac8](https://github.com/monkescience/yeet/commit/5b91ac8bbc781a5420b3a4b6130d56b75b786701))

## [v0.10.8](https://github.com/monkescience/yeet/compare/v0.10.7...v0.10.8) (2026-06-29)

### Features

- **release:** cap PR body length and enforce Azure DevOps 4000-char limit ([68f65ab](https://github.com/monkescience/yeet/commit/68f65ab79d0ad20aba0cbf797d1c43f88ca4b51e))
### Bug Fixes

- **provider:** scope unbounded azure commit listing to the branch ([7e3d1b5](https://github.com/monkescience/yeet/commit/7e3d1b5a71217841469c0978a31f4235b1ead240))
- **provider:** resolve legacy visualstudio.com remotes to dev.azure.com ([617d8e1](https://github.com/monkescience/yeet/commit/617d8e1ae9152b59ae888e24d0d8d002f073a836))
- **release:** eliminate duplicate debug log messages ([fb03a25](https://github.com/monkescience/yeet/commit/fb03a25ab3dc18926c2ee442caa287aa2249f0f0))
- **provider:** query commit ranges per ref to stop changelog over-inclusion ([900e29b](https://github.com/monkescience/yeet/commit/900e29b0bdf41565e9293cc4802e8cfd126f756e))

## [v0.10.7](https://github.com/monkescience/yeet/compare/v0.10.6...v0.10.7) (2026-06-26)

### Bug Fixes

- **gitlab:** select the most recently merged release MR instead of the most recently updated ([07ee0ed](https://github.com/monkescience/yeet/commit/07ee0ed7f7d32f199e8182a6a9dec738780c6f95))
- **gitlab:** paginate CommitPullRequestBody to find merge requests past the first page ([bb6ea44](https://github.com/monkescience/yeet/commit/bb6ea44b0e8164b3074914037009a61f201e0d70))

## [v0.10.6](https://github.com/monkescience/yeet/compare/v0.10.5...v0.10.6) (2026-06-23)

### Bug Fixes

- **deps:** update golang.org/x/crypto to v0.53.0 ([386bf0c](https://github.com/monkescience/yeet/commit/386bf0c73aeadb4ca9c3b9ef6a8f166ec9286971))
- **deps:** update module go.yaml.in/yaml/v4 to v4.0.0-rc.6 (#141) ([c987b50](https://github.com/monkescience/yeet/commit/c987b50a7fa0f2c3a7022e4d7eeb2b7cdde10928))

## [v0.10.5](https://github.com/monkescience/yeet/compare/v0.10.4...v0.10.5) (2026-06-19)

### Bug Fixes

- **deps:** update module go.yaml.in/yaml/v4 to v4.0.0-rc.5 (#137) ([bdd8549](https://github.com/monkescience/yeet/commit/bdd85499862a8058f864b9595eab1094efe76e1a))
- **deps:** update module github.com/monkescience/testastic to v0.4.0 (#138) ([32b9b23](https://github.com/monkescience/yeet/commit/32b9b23917727dfbe2c204b872bdb73a82967c16))
- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.39.0 (#139) ([adfb712](https://github.com/monkescience/yeet/commit/adfb7126dc1de0d7e2c8fc390f43d12943792c9b))
- **deps:** update module charm.land/lipgloss/v2 to v2.0.4 (#136) ([881bbca](https://github.com/monkescience/yeet/commit/881bbca8a3f6c7f81b6a41db494152f16692a873))
- **deps:** update module golang.org/x/sync to v0.21.0 (#133) ([d7d4918](https://github.com/monkescience/yeet/commit/d7d4918c59192a3b7fabda6c434cc390ce537c13))

## [v0.10.4](https://github.com/monkescience/yeet/compare/v0.10.3...v0.10.4) (2026-06-04)

### Bug Fixes

- **release:** match indented changelog headings when extracting entries ([eaeb9a5](https://github.com/monkescience/yeet/commit/eaeb9a59d3972051994b3fdabb2e0636cca63703))
### Performance Improvements

- **provider:** query azure devops pull requests by commit instead of listing all ([5fdac89](https://github.com/monkescience/yeet/commit/5fdac89f96fd3d1dbca5a2036709fcb755fe0397))

## [v0.10.3](https://github.com/monkescience/yeet/compare/v0.10.2...v0.10.3) (2026-06-04)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.36.0 (#128) ([7d526a4](https://github.com/monkescience/yeet/commit/7d526a4206144e8ef9cc75c013bf600a8a1d12b5))

## [v0.10.2](https://github.com/monkescience/yeet/compare/v0.10.1...v0.10.2) (2026-05-30)

### Features

- **config:** validate changelog reference patterns at load ([4eda23f](https://github.com/monkescience/yeet/commit/4eda23fb484251e3ac8f07f6c829bfbba551f086))
- **provider:** make max concurrent API requests configurable ([895b7bb](https://github.com/monkescience/yeet/commit/895b7bb2fe92470afbdada1a48f85b7c28803a65))

## [v0.10.1](https://github.com/monkescience/yeet/compare/v0.10.0...v0.10.1) (2026-05-29)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.32.0 (#121) ([6f0c8ef](https://github.com/monkescience/yeet/commit/6f0c8ef44c99a6d1d84f30b0a922f6de85694ce2))

## [v0.10.0](https://github.com/monkescience/yeet/compare/v0.9.3...v0.10.0) (2026-05-29)

### ⚠ BREAKING CHANGES

- **commands:** replace --version flag with expanded version subcommand ([562d620](https://github.com/monkescience/yeet/commit/562d620973d8754e9a87435f4024da799883cb95))
### Features

- **release:** share monorepo history index across targets ([0fead31](https://github.com/monkescience/yeet/commit/0fead31e0d5ddc4b1d5791fc12f24dc11dbe7542))
### Bug Fixes

- **release:** keep git-trailer footers attached in commit overrides ([d65cce0](https://github.com/monkescience/yeet/commit/d65cce0f327c963f905219f3642d871f9c3bbf87))
- **deps:** update module github.com/google/go-github/v86 to v88 (#120) ([01e7fb8](https://github.com/monkescience/yeet/commit/01e7fb8d11c84c68c0f097d6bde5783f2a58724b))
- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.31.0 (#118) ([7f82ef0](https://github.com/monkescience/yeet/commit/7f82ef07df3f882b74e108de7d08b246548b511e))
- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.26.1 (#113) ([48fa486](https://github.com/monkescience/yeet/commit/48fa48631711d701241b99501d351927262aca43))
- **deps:** update module github.com/go-git/go-git/v6 to v6.0.0-alpha.4 [security] (#110) ([f03c7c5](https://github.com/monkescience/yeet/commit/f03c7c54b52a810a100a672014e26cfad681ea5b))
### Performance Improvements

- **release:** reuse shared scan for per-target fallback lookups ([a8444b0](https://github.com/monkescience/yeet/commit/a8444b0e7c051b9ca2f3232c7dd0159f49ba829f))

## [v0.9.3](https://github.com/monkescience/yeet/compare/v0.9.2...v0.9.3) (2026-05-16)

### Features

- **cli:** register --version flag on root command ([ed57dd8](https://github.com/monkescience/yeet/commit/ed57dd85db5fd996121d9c2b2c199e94297a2e5c))

## [v0.9.2](https://github.com/monkescience/yeet/compare/v0.9.1...v0.9.2) (2026-05-16)

### Bug Fixes

- **deps:** update module github.com/google/go-github/v85 to v86 (#106) ([34e3e9a](https://github.com/monkescience/yeet/commit/34e3e9acaf9ed1801c386626ac6f70536d3ba0ae))
- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.24.1 (#105) ([ac852bf](https://github.com/monkescience/yeet/commit/ac852bf71f47dce57c69e3f2f51f665938bae96a))

## [v0.9.1](https://github.com/monkescience/yeet/compare/v0.9.0...v0.9.1) (2026-05-15)

### Bug Fixes

- **deps:** update module gitlab.com/gitlab-org/api/client-go/v2 to v2.21.0 (#98) ([a1bbcc3](https://github.com/monkescience/yeet/commit/a1bbcc3a27a316d45e9dc40951c68c899b69aad2))
- **deps:** update module github.com/masterminds/semver/v3 to v3.5.0 (#97) ([5cb7b83](https://github.com/monkescience/yeet/commit/5cb7b834b1e6df986c0a4c7fae8eb87dca173c3c))

## [v0.9.0](https://github.com/monkescience/yeet/compare/v0.8.4...v0.9.0) (2026-05-14)

### ⚠ BREAKING CHANGES

- **config:** nest repository settings under per-provider sub-sections ([fd49da6](https://github.com/monkescience/yeet/commit/fd49da650965af395c7a72189ad3d9b9ab3236b2))
### Features

- **config:** nest repository settings under per-provider sub-sections ([fd49da6](https://github.com/monkescience/yeet/commit/fd49da650965af395c7a72189ad3d9b9ab3236b2))

### Migration Notes

- **Repository config schema is restructured.** The flat repository fields (`host`, `owner`, `repo`, `project`, `organization`, `collection`) have been moved under per-provider sub-sections: `repository.github`, `repository.gitlab`, and `repository.azuredevops`. Only the sub-section matching the top-level `provider` may be set; `provider: auto` allows no sub-section at all. Old configs fail to load with a "field not found" parse error pointing at the legacy key. Migrate `.yeet.yaml` as follows:

  ```yaml
  # before
  provider: github
  repository:
    host: github.com
    owner: acme
    repo: widgets

  # after
  provider: github
  repository:
    github:
      host: github.com
      owner: acme
      repo: widgets
  ```

  ```yaml
  # before
  provider: gitlab
  repository:
    host: gitlab.company.com
    project: group/sub/widgets

  # after
  provider: gitlab
  repository:
    gitlab:
      host: gitlab.company.com
      project: group/sub/widgets
  ```

  ```yaml
  # before
  provider: azuredevops
  repository:
    host: dev.azure.com
    organization: contoso
    project: MyProject
    repo: widgets

  # after
  provider: azuredevops
  repository:
    azuredevops:
      host: dev.azure.com
      organization: contoso
      project: MyProject
      repo: widgets
  ```

- **CLI flags are stricter.** Repository field flags (`--host`, `--owner`, `--repo`, `--project`) now require an explicit `--provider` (or `provider:` in config). `--owner`/`--repo` are valid only for `--provider github`. `--owner` is rejected for `--provider azuredevops`.

## [v0.8.4](https://github.com/monkescience/yeet/compare/v0.8.3...v0.8.4) (2026-05-14)

### Features

- add debug logs for provider calls and core stages ([a974a23](https://github.com/monkescience/yeet/commit/a974a2332c6cb4c684173a79c220dbe43550f736))

## [v0.8.3](https://github.com/monkescience/yeet/compare/v0.8.2...v0.8.3) (2026-05-14)

### Features

- **release:** add provider compare links and diagnostics ([bfe26d1](https://github.com/monkescience/yeet/commit/bfe26d1d6daa920e8271bc6736b80a0a51750c96))
- **provider:** add azure devops release support ([c2becd3](https://github.com/monkescience/yeet/commit/c2becd3cd9443e9e7baa820140445055ce458c31))
### Bug Fixes

- **cli:** respect custom hosts and terminal color settings ([4215b91](https://github.com/monkescience/yeet/commit/4215b91e2cd61261bc2d748968320f8e65c93eea))
- **cli:** support worktree config repositories ([6c216ba](https://github.com/monkescience/yeet/commit/6c216bacd58be42687e821955d7beaa3441f7452))

## [v0.8.2](https://github.com/monkescience/yeet/compare/v0.8.1...v0.8.2) (2026-05-11)

### Features

- **version-files:** support json pointers ([14bd2af](https://github.com/monkescience/yeet/commit/14bd2afac5d35a3e7768ede6d8d2b82c76a218f7))
### Bug Fixes

- **deps:** update go-git to v5.19.0 ([9f0a477](https://github.com/monkescience/yeet/commit/9f0a477120cd5283f6e32059c05084ca3d4f72e8))

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
