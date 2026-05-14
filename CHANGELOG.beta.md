# Changelog

## [v0.8.3-beta.1](https://github.com/monkescience/yeet/compare/v0.8.2...v0.8.3-beta.1) (2026-05-14)

### Features

- **provider:** add CompareURL method for provider-specific compare links ([6ddcdc7](https://github.com/monkescience/yeet/commit/6ddcdc76b20c0468774d6a2ccbc2fd6d671c579c))
- **release:** add debug logs for release planning and azure devops commits ([88ee49f](https://github.com/monkescience/yeet/commit/88ee49fc95214d30f97f8206847cca8ac873851f))
- **provider:** add azure devops release support ([7b6b800](https://github.com/monkescience/yeet/commit/7b6b800110b6b3ec4d2476fd2fa2cce565f1f85f))
### Bug Fixes

- **cli:** respect TTY and NO_COLOR for ANSI output ([1a7574a](https://github.com/monkescience/yeet/commit/1a7574a63faa44f2282b5f6d69d44b75287a2d45))
- **cli:** honor GITHUB_URL and GITLAB_URL on default host ([c53fff3](https://github.com/monkescience/yeet/commit/c53fff3f821ff3834a71635ee626d93f359bfda8))
- **provider:** swap azure devops itemVersion and compareVersion for commits since ([773a353](https://github.com/monkescience/yeet/commit/773a3536906bf57bd1ebd5ed25c233e84c89ed99))
- **provider:** fetch azure devops pr labels from dedicated endpoint ([2546a9d](https://github.com/monkescience/yeet/commit/2546a9d8ed51d4caf201374fbb1ac14631322c5a))
- **provider:** resolve azure devops tag refs and use web url for tags ([f78c883](https://github.com/monkescience/yeet/commit/f78c88353d115ace1f2ad1120f3935fd1a0263c3))
- **provider:** fetch full azure devops pr to read untruncated description ([b246f86](https://github.com/monkescience/yeet/commit/b246f8607d17a5072d14ed9bad21ba92f9c5a0f9))
- **provider:** rebase azure devops release branch onto base each run ([a296072](https://github.com/monkescience/yeet/commit/a29607206eb7b2009c25023f5915c10653ec5d90))
- **provider:** build azure devops pr url from provider state ([51c3a22](https://github.com/monkescience/yeet/commit/51c3a225450b79426f2f0202bd463c94c56a4a49))
- **provider:** use azure devops web url for pull requests ([1c9cbe1](https://github.com/monkescience/yeet/commit/1c9cbe1db0b81b08ffafae9e6e64d167852363a2))
- **provider:** remove azure devops labels by id ([3457fa4](https://github.com/monkescience/yeet/commit/3457fa424057213e2ddf6385d4eb6a9f296d681b))
- **provider:** create azure devops branches via refs ([fa19736](https://github.com/monkescience/yeet/commit/fa1973609e3da6ede57387d9c355a4e00f9b2c33))
- **cli:** support worktree config repositories ([0b13fe6](https://github.com/monkescience/yeet/commit/0b13fe64c60e8f1e3534ca8b46d3f1d13ba25068))
