# Changelog

## [v0.11.0-beta.1](https://github.com/monkescience/yeet/compare/v0.10.20...v0.11.0-beta.1) (2026-07-21)

### ⚠ BREAKING CHANGES

- **release:** read overrides from local commits ([6abbe05](https://github.com/monkescience/yeet/commit/6abbe0596bad3a025ef5dbb1bbe5566a3922438a))
- require a full local checkout for release history ([c56e484](https://github.com/monkescience/yeet/commit/c56e484ce75f9861d01b1763cf322acd6f6b602f))
### Features

- require a full local checkout for release history ([c56e484](https://github.com/monkescience/yeet/commit/c56e484ce75f9861d01b1763cf322acd6f6b602f))
- serve release history from the local git checkout ([3386945](https://github.com/monkescience/yeet/commit/3386945071e75809fb0db06167e3c0ae954d6125))
### Bug Fixes

- **release:** validate history before mutations ([6a42853](https://github.com/monkescience/yeet/commit/6a42853efb2e78480bbc8aa1a0540e5fcf343e63))
- **history:** avoid retaining per-ref ancestry ([a06493e](https://github.com/monkescience/yeet/commit/a06493e84d845189517fe9f6ddeee3e0f8c8121f))
- **release:** validate local tags against remote ([2cb97e8](https://github.com/monkescience/yeet/commit/2cb97e8081f566dc1e5607326f1e6a320a1a44a0))
### Performance Improvements

- **release:** cache changelog reads ([fb8c1d6](https://github.com/monkescience/yeet/commit/fb8c1d66dc94cb10a3dd2015b19a6f797a944e37))
- **history:** reuse remote tag snapshot ([c25e664](https://github.com/monkescience/yeet/commit/c25e664e1fb9e2695cece6512c9ab8c2a7f2d209))
- **release:** avoid duplicate tag lookup ([e581ebc](https://github.com/monkescience/yeet/commit/e581ebc5d12a2bceb5817366ded9b636c389b53e))
- **release:** avoid duplicate release lookup ([76745c8](https://github.com/monkescience/yeet/commit/76745c8dae08a7c2a08d5895427eb0540a025793))
- **release:** read base files from local commits ([4101947](https://github.com/monkescience/yeet/commit/41019474f69455ef15c48de7d2751b18811a7725))
- **release:** read overrides from local commits ([6abbe05](https://github.com/monkescience/yeet/commit/6abbe0596bad3a025ef5dbb1bbe5566a3922438a))
