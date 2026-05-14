package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestReleaseRepositoryValidation(t *testing.T) {
	t.Parallel()

	t.Run("rejects blank repository.remote", func(t *testing.T) {
		t.Parallel()

		// given: a config that empties repository.remote
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  remote: ""
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with a remote validation error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "repository.remote")
	})

	t.Run("rejects blank repository.host", func(t *testing.T) {
		t.Parallel()

		// given: a host set to whitespace
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: "   "
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr names the blank host
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "repository.host")
	})

	t.Run("rejects azuredevops config without project", func(t *testing.T) {
		t.Parallel()

		// given: an azure config missing project
		configPath := writeRawConfig(t, `provider: azuredevops
branch: main
repository:
  host: dev.azure.com
  organization: contoso
  repo: yeet
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says project is required
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "repository.project")
	})

	t.Run("rejects owner without repo for github", func(t *testing.T) {
		t.Parallel()

		// given: github config with owner but no repo
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says owner+repo must be set together
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "repository.owner")
	})

	t.Run("rejects mismatched project vs owner/repo", func(t *testing.T) {
		t.Parallel()

		// given: a config where project does not match owner/repo
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: alice
  repo: tools
  project: bob/tools
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says project must match
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "repository.project")
	})

	t.Run("rejects github owner containing slash", func(t *testing.T) {
		t.Parallel()

		// given: a github config where owner contains '/'
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: nested/owner
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says owner must not contain '/'
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "owner")
	})

	t.Run("rejects target.path empty", func(t *testing.T) {
		t.Parallel()

		// given: a target with empty path
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: ""
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says path must not be empty
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "path")
	})

	t.Run("rejects github project not in owner/repo form", func(t *testing.T) {
		t.Parallel()

		// given: a github config with a malformed project (too many segments)
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  project: a/b/c
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and stderr says project must be in owner/repo form
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "owner/repo")
	})

	t.Run("rejects target.tag_prefix empty", func(t *testing.T) {
		t.Parallel()

		// given: a target with empty tag_prefix
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: ""
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 (validation rejects empty tag_prefix or downstream fails)
		testastic.Equal(t, 1, result.ExitCode)
	})
}
