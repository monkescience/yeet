package integration_test

import (
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseHostTrust(t *testing.T) {
	t.Parallel()

	t.Run("accepts a custom host matching the git remote", func(t *testing.T) {
		t.Parallel()

		// given: an explicit custom host that matches the repository remote.
		repoDir, shas := fixture.WriteRepoWithHistory(
			t,
			"https://github.example/acme/repo.git",
			"main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			},
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "acme",
			Repo:          "repo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.example",
			Owner:    "acme",
			Repo:     "repo",
		})

		// when: planning a release through the configured provider endpoint.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: host validation succeeds and the release plan is printed.
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(t, "testdata/release_autodetect/github_https/stdout.expected.txt", result.Stdout)
	})

	t.Run("rejects a custom host that differs from the git remote", func(t *testing.T) {
		t.Parallel()

		// given: an explicit custom host that differs from the repository remote.
		repoDir := fixture.WriteRepo(t, "https://other.example/acme/repo.git")
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.example",
			Owner:    "acme",
			Repo:     "repo",
		})

		// when: planning a release.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv("GITHUB_TOKEN=test-token"),
		)

		// then: yeet rejects the untrusted host before contacting the provider.
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_host_trust/mismatched_custom_host/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects a provider host containing a scheme", func(t *testing.T) {
		t.Parallel()

		// given: a provider host configured as a URL instead of a bare hostname.
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "https://github.example",
			Owner:    "acme",
			Repo:     "repo",
		})

		// when: planning a release.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv("GITHUB_TOKEN=test-token"),
		)

		// then: yeet reports the invalid host format.
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_host_trust/provider_host_with_scheme/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("rejects auto detection for an unsupported forge host", func(t *testing.T) {
		t.Parallel()

		// given: an automatic provider configuration on an unsupported forge host.
		repoDir := fixture.WriteRepo(t, "https://code.example/acme/repo.git")
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		// when: planning a release.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
		)

		// then: yeet explains which hosts support automatic detection.
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_host_trust/unsupported_auto_detect_host/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("redacts user info from an invalid remote", func(t *testing.T) {
		t.Parallel()

		// given: an invalid remote URL containing fixture user information.
		const remoteURL = "https://fixture-user:fixture-password@code.example"

		repoDir := fixture.WriteRepo(t, remoteURL)
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{Branch: "main"})

		// when: planning a release.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
		)

		// then: yeet reports the invalid remote without exposing its user information.
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_host_trust/redacted_invalid_remote/stderr.expected.txt",
			result.Stderr,
		)
		testastic.Equal(t, false, strings.Contains(result.Stderr, "fixture-password"))
	})

	t.Run("reports a missing remote when validating a custom host", func(t *testing.T) {
		t.Parallel()

		// given: a custom host configuration outside a Git repository.
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.example",
			Owner:    "acme",
			Repo:     "repo",
		})

		// when: planning a release.
		result := binary.RunWithOptions(
			t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(t.TempDir()),
			testastic.WithRunEnv("GITHUB_TOKEN=test-token"),
		)

		// then: yeet reports that the host could not be verified against a remote.
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_host_trust/missing_remote/stderr.expected.txt",
			result.Stderr,
		)
	})
}
