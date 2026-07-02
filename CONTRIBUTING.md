# Contributing

## Setup

Tool versions are managed with [mise](https://mise.jdx.dev):

```sh
mise install
```

## Build and test

```sh
make build      # build the binary
make test       # unit + blackbox tests
make lint       # golangci-lint
make fmt        # format
```

Run `make help` for all targets. CI enforces lint, tidy, tests with a coverage
threshold, and a build smoke test on every PR.

## Commits

Commit messages must follow [conventional commits](https://www.conventionalcommits.org/)
in the form `type(scope): description`. A ruleset on `main` rejects
non-conforming messages. `feat`, `fix`, and `perf` commits trigger a release,
other types do not.

## Releases

Releases are fully automated, yeet releases itself. Do not bump versions,
edit `CHANGELOG.md`, or create tags manually.
