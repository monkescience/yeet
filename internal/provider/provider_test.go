package provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	githubapi "github.com/google/go-github/v90/github"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	testReleaseLabelPending = "autorelease: pending"
	testReleaseLabelTagged  = "autorelease: tagged"
)

func TestParseRemote(t *testing.T) {
	t.Parallel()

	t.Run("github ssh", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub SSH remote URL
		url := "git@github.com:owner/repo.git"

		// when: parsing the remote
		info, err := provider.ParseRemoteForTest(url)

		// then: repository coordinates are extracted
		testastic.NoError(t, err)
		testastic.Equal(t, "github.com", info.Host)
		testastic.Equal(t, "owner", info.Owner)
		testastic.Equal(t, "repo", info.Repo)
		testastic.Equal(t, "owner/repo", info.Project)
	})

	t.Run("unknown remote error redacts user and password", func(t *testing.T) {
		t.Parallel()

		// given: an unparseable remote URL with user:password credentials
		url := "https://ci:secret-token@"

		// when: parsing the remote
		_, err := provider.ParseRemoteForTest(url)

		// then: the error names the URL with the entire userinfo redacted
		testastic.ErrorIs(t, err, provider.ErrUnknownRemote)
		testastic.Equal(t, "unable to parse remote URL: https://***@", err.Error())
	})

	t.Run("unknown remote error redacts username-only token", func(t *testing.T) {
		t.Parallel()

		// given: an unparseable remote URL with a token in the username position
		url := "https://ghp-secret-token@"

		// when: parsing the remote
		_, err := provider.ParseRemoteForTest(url)

		// then: the error hides the token
		testastic.ErrorIs(t, err, provider.ErrUnknownRemote)
		testastic.Equal(t, "unable to parse remote URL: https://***@", err.Error())
	})

	t.Run("github https", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub HTTPS remote URL
		url := "https://github.com/owner/repo.git"

		// when: parsing the remote
		info, err := provider.ParseRemoteForTest(url)

		// then: repository coordinates are extracted
		testastic.NoError(t, err)
		testastic.Equal(t, "github.com", info.Host)
		testastic.Equal(t, "owner", info.Owner)
		testastic.Equal(t, "repo", info.Repo)
		testastic.Equal(t, "owner/repo", info.Project)
	})

	t.Run("github enterprise https", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub Enterprise remote URL
		url := "https://github.company.com/platform/yeet.git"

		// when: parsing the remote
		info, err := provider.ParseRemoteForTest(url)

		// then: host and repository are preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "github.company.com", info.Host)
		testastic.Equal(t, "platform", info.Owner)
		testastic.Equal(t, "yeet", info.Repo)
	})

	t.Run("gitlab subgroup ssh", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab subgroup SSH remote URL
		url := "git@gitlab.com:group/subgroup/service.git"

		// when: parsing the remote
		info, err := provider.ParseRemoteForTest(url)

		// then: the full project path is preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab.com", info.Host)
		testastic.Equal(t, "group/subgroup", info.Owner)
		testastic.Equal(t, "service", info.Repo)
		testastic.Equal(t, "group/subgroup/service", info.Project)
	})

	t.Run("gitlab subgroup ssh url", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab SSH URL remote with a subgroup path
		url := "ssh://git@gitlab.company.com/group/subgroup/service.git"

		// when: parsing the remote
		info, err := provider.ParseRemoteForTest(url)

		// then: the host and full project path are preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab.company.com", info.Host)
		testastic.Equal(t, "group/subgroup", info.Owner)
		testastic.Equal(t, "service", info.Repo)
		testastic.Equal(t, "group/subgroup/service", info.Project)
	})

	t.Run("repo names with dots", func(t *testing.T) {
		t.Parallel()

		// given: a remote with a dotted repository name
		url := "https://gitlab.com/group/service.api.git"

		// when: parsing the remote
		info, err := provider.ParseRemoteForTest(url)

		// then: the dotted name is preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "group", info.Owner)
		testastic.Equal(t, "service.api", info.Repo)
		testastic.Equal(t, "group/service.api", info.Project)
	})

	t.Run("azure devops cloud https", func(t *testing.T) {
		t.Parallel()

		// given: a cloud Azure DevOps HTTPS remote
		url := "https://dev.azure.com/contoso/platform/_git/yeet"

		// when: parsing the remote
		info, err := provider.ParseRemoteForTest(url)

		// then: org, project, and repo are extracted under the cloud host
		testastic.NoError(t, err)
		testastic.Equal(t, "dev.azure.com", info.Host)
		testastic.Equal(t, "contoso", info.Organization)
		testastic.Equal(t, "platform", info.Project)
		testastic.Equal(t, "yeet", info.Repo)
	})

	t.Run("azure devops legacy visualstudio https", func(t *testing.T) {
		t.Parallel()

		// given: a legacy Azure DevOps remote where the org is the host subdomain
		url := "https://contoso.visualstudio.com/platform/_git/yeet"

		// when: parsing the remote
		info, err := provider.ParseRemoteForTest(url)

		// then: the host is normalized to dev.azure.com so the API base URL
		// resolves to dev.azure.com/{org}, not the broken {org}.visualstudio.com/{org}
		testastic.NoError(t, err)
		testastic.Equal(t, "dev.azure.com", info.Host)
		testastic.Equal(t, "contoso", info.Organization)
		testastic.Equal(t, "platform", info.Project)
		testastic.Equal(t, "yeet", info.Repo)
	})

	t.Run("azure devops ssh", func(t *testing.T) {
		t.Parallel()

		// given: an Azure DevOps SSH remote
		url := "git@ssh.dev.azure.com:v3/contoso/platform/yeet"

		// when: parsing the remote
		info, err := provider.ParseRemoteForTest(url)

		// then: org, project, and repo are extracted under the cloud host
		testastic.NoError(t, err)
		testastic.Equal(t, "dev.azure.com", info.Host)
		testastic.Equal(t, "contoso", info.Organization)
		testastic.Equal(t, "platform", info.Project)
		testastic.Equal(t, "yeet", info.Repo)
	})

	t.Run("invalid url", func(t *testing.T) {
		t.Parallel()

		// given: an unparseable URL
		url := "not-a-valid-url"

		// when: parsing the remote
		_, err := provider.ParseRemoteForTest(url)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnknownRemote)
	})
}

func TestDetectType(t *testing.T) {
	t.Parallel()

	t.Run("detects github hosts", func(t *testing.T) {
		t.Parallel()

		// given: the public GitHub host

		// when: detecting its provider type
		providerType, err := provider.DetectTypeForTest("github.com")

		// then: GitHub is detected
		testastic.NoError(t, err)
		testastic.Equal(t, "github", providerType)
	})

	t.Run("detects gitlab hosts", func(t *testing.T) {
		t.Parallel()

		// given: the public GitLab host

		// when: detecting its provider type
		providerType, err := provider.DetectTypeForTest("gitlab.com")

		// then: GitLab is detected
		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab", providerType)
	})

	t.Run("fails on github custom hosts", func(t *testing.T) {
		t.Parallel()

		// given: a custom GitHub host

		// when: detecting its provider type
		_, err := provider.DetectTypeForTest("github.company.com")

		// then: the host is reported as unsupported
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
	})

	t.Run("fails on gitlab custom hosts", func(t *testing.T) {
		t.Parallel()

		// given: a custom GitLab host

		// when: detecting its provider type
		_, err := provider.DetectTypeForTest("gitlab.company.com")

		// then: the host is reported as unsupported
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
	})

	t.Run("fails on unsupported hosts", func(t *testing.T) {
		t.Parallel()

		// given: a host belonging to no supported provider

		// when: detecting its provider type
		_, err := provider.DetectTypeForTest("code.company.com")

		// then: the host is reported as unsupported
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
	})
}

func newGitHubTestClient(t *testing.T, server *httptest.Server) *githubapi.Client {
	t.Helper()

	baseURL := server.URL + "/"

	client, err := githubapi.NewClient(
		githubapi.WithHTTPClient(server.Client()),
		githubapi.WithURLs(&baseURL, &baseURL),
	)
	testastic.NoError(t, err)

	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(value)
	testastic.NoError(t, err)
}

func isGitHubCreateReleaseRequest(r *http.Request) bool {
	return r.Method == http.MethodPost &&
		r.URL.Path == "/repos/o/r/releases"
}

func isGitLabCreateReleaseRequest(r *http.Request) bool {
	return r.Method == http.MethodPost &&
		r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/releases"
}

func TestMaxPRBodyLength(t *testing.T) {
	t.Parallel()

	t.Run("azure devops enforces its 4000 character limit", func(t *testing.T) {
		t.Parallel()

		// given: an Azure DevOps provider
		az := provider.NewAzureDevOps(http.DefaultClient, "https://dev.azure.com", "pat", "org", "org", "proj", "repo")

		// when: reading its max PR body length
		limit := az.MaxPRBodyLength()

		// then: the Azure DevOps hard limit is reported
		testastic.Equal(t, 4000, limit)
	})

	t.Run("github reports no enforced limit", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub provider
		client, err := githubapi.NewClient()
		testastic.NoError(t, err)

		gh := provider.NewGitHub(client, "o", "r")

		// when: reading its max PR body length
		limit := gh.MaxPRBodyLength()

		// then: no limit is enforced
		testastic.Equal(t, 0, limit)
	})

	t.Run("gitlab reports no enforced limit", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab provider
		client, err := gitlabapi.NewClient("")
		testastic.NoError(t, err)

		gl := provider.NewGitLab(client, "o/r")

		// when: reading its max PR body length
		limit := gl.MaxPRBodyLength()

		// then: no limit is enforced
		testastic.Equal(t, 0, limit)
	})
}

var (
	_ forge.Provider = (*provider.GitHub)(nil)
	_ forge.Provider = (*provider.GitLab)(nil)
)

var _ commit.BumpType = commit.BumpMajor
