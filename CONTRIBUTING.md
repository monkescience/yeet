# Contributing

For larger changes, open an issue first so the approach is agreed on before
you invest the work.

Contributions are submitted as GitHub pull requests. External contributors
should fork the repository and create a branch in their fork. Repository
collaborators may create a branch directly in this repository. Make a focused
change, explain its user-visible and security impact in the pull request, and
keep unrelated changes in separate pull requests.

## Setup

Tool versions are managed with [mise](https://mise.jdx.dev):

```sh
mise install
```

## Acceptance requirements

Contributions must build, be formatted, pass lint checks, and pass the full
test suite. Changes to user-visible behavior must update the relevant
documentation. New functionality must include automated tests, and bug fixes
must include a regression test. If an automated test is impractical, explain
why and describe the manual verification in the pull request.

Run the required checks before opening a pull request:

```sh
make fmt        # format
make lint       # golangci-lint
make test       # unit + blackbox tests
make build      # build the binary
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
