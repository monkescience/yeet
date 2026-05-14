package integration_test

import (
	"strings"
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

	t.Run("rejects target.path outside repository", func(t *testing.T) {
		t.Parallel()

		// given: a target path that escapes the repository root
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: ../outside
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and requires repo-relative target paths
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "repo-relative")
	})

	t.Run("rejects absolute target path", func(t *testing.T) {
		t.Parallel()

		// given: an absolute target path
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  default:
    type: path
    path: /tmp/app
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and requires repo-relative target paths
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "repo-relative")
	})

	t.Run("rejects exclude path outside target path", func(t *testing.T) {
		t.Parallel()

		// given: a target excluding a sibling path it does not own
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  api:
    type: path
    path: services/api
    tag_prefix: api-v
    exclude_paths:
      - services/web
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and requires excludes to be below their target path
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "exclude_paths")
		testastic.Contains(t, result.Stderr, "must be inside")
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

	t.Run("rejects duplicate target tag prefixes", func(t *testing.T) {
		t.Parallel()

		// given: two targets that would try to publish the same tag names
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  api:
    type: path
    path: services/api
    tag_prefix: service-v
  worker:
    type: path
    path: services/worker
    tag_prefix: service-v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and points at the duplicate tag prefix
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "tag_prefix")
		testastic.Contains(t, result.Stderr, "duplicates")
	})

	t.Run("rejects derived target including unknown target", func(t *testing.T) {
		t.Parallel()

		// given: a derived target that references a target that does not exist
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  api:
    type: path
    path: services/api
    tag_prefix: api-v
  all:
    type: derived
    tag_prefix: v
    includes:
      - missing
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 and names the invalid include
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "includes")
		testastic.Contains(t, result.Stderr, "missing")
	})

	t.Run("rejects derived target including another derived target", func(t *testing.T) {
		t.Parallel()

		// given: a derived target graph that nests another derived target
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  api:
    type: path
    path: services/api
    tag_prefix: api-v
  bundle:
    type: derived
    tag_prefix: bundle-v
    includes:
      - api
  all:
    type: derived
    tag_prefix: v
    includes:
      - bundle
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 because derived targets may only include path targets
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "must refer to a path target")
	})

	t.Run("rejects overlapping direct path ownership", func(t *testing.T) {
		t.Parallel()

		// given: one target owns services and another owns a child path under it
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  services:
    type: path
    path: services
    tag_prefix: services-v
  api:
    type: path
    path: services/api
    tag_prefix: api-v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 instead of allowing ambiguous target ownership
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "path ownership overlaps")
	})

	t.Run("allows direct path overlap excluded by parent target", func(t *testing.T) {
		t.Parallel()

		// given: a parent target explicitly excludes the child target's path
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  services:
    type: path
    path: services
    tag_prefix: services-v
    exclude_paths:
      - services/api
  api:
    type: path
    path: services/api
    tag_prefix: api-v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: validation allows the disjoint ownership and later fails only because no token/server exists
		if strings.Contains(result.Stderr, "path ownership overlaps") {
			t.Fatalf("expected overlap to be allowed, stderr: %s", result.Stderr)
		}
	})

	t.Run("rejects duplicate version file ownership across targets", func(t *testing.T) {
		t.Parallel()

		// given: two targets configured to edit the same version file
		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  owner: testorg
  repo: testrepo
targets:
  api:
    type: path
    path: services/api
    tag_prefix: api-v
    version_files:
      - services/api/VERSION.txt
  worker:
    type: path
    path: services/worker
    tag_prefix: worker-v
    version_files:
      - services/api/VERSION.txt
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 before one target can overwrite another target's file
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "version_files")
		testastic.Contains(t, result.Stderr, "duplicates")
	})
}
