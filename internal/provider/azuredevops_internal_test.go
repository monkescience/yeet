package provider

import (
	"encoding/base64"
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/monkescience/testastic"
)

func TestNewAzureDevOpsWithSystemAccessTokenUsesBearerAuth(t *testing.T) {
	t.Parallel()

	azureDevOpsProvider := NewAzureDevOpsWithSystemAccessToken(
		nil,
		"https://dev.azure.com",
		"system-token",
		"platform",
		"platform",
		"release-tools",
		"yeet",
	)

	testastic.Equal(t, "Bearer system-token", azureDevOpsProvider.conn.AuthorizationString)
}

func TestNewAzureDevOpsUsesPATBasicAuth(t *testing.T) {
	t.Parallel()

	azureDevOpsProvider := NewAzureDevOps(
		nil,
		"https://dev.azure.com",
		"pat-token",
		"platform",
		"platform",
		"release-tools",
		"yeet",
	)

	auth := strings.TrimPrefix(azureDevOpsProvider.conn.AuthorizationString, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(auth)
	testastic.NoError(t, err)
	testastic.Equal(t, ":pat-token", string(decoded))
}

func TestTrustedAzureDevOpsReleasePR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		repo        string
		pullRequest *git.GitPullRequest
		trusted     bool
	}{
		{
			name: "accepts exact same-repository release branch",
			repo: "yeet",
			pullRequest: &git.GitPullRequest{
				SourceRefName: new("refs/heads/yeet/release-main"),
				Repository:    &git.GitRepository{Name: new("yeet")},
			},
			trusted: true,
		},
		{
			name: "rejects another repository",
			repo: "yeet",
			pullRequest: &git.GitPullRequest{
				SourceRefName: new("refs/heads/yeet/release-main"),
				Repository:    &git.GitRepository{Name: new("yeet-attacker")},
			},
		},
		{
			name: "rejects missing repository",
			repo: "yeet",
			pullRequest: &git.GitPullRequest{
				SourceRefName: new("refs/heads/yeet/release-main"),
			},
		},
		{
			name: "rejects lookalike release branch",
			repo: "yeet",
			pullRequest: &git.GitPullRequest{
				SourceRefName: new("refs/heads/yeet/release-main-attacker"),
				Repository:    &git.GitRepository{Name: new("yeet")},
			},
		},
		{
			name: "rejects fork release branch",
			repo: "yeet",
			pullRequest: &git.GitPullRequest{
				SourceRefName: new("refs/heads/yeet/release-main"),
				Repository:    &git.GitRepository{Name: new("yeet")},
				ForkSource:    &git.GitForkRef{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: an Azure DevOps pull request candidate
			pullRequest := test.pullRequest
			azureDevOpsProvider := NewAzureDevOps(
				nil,
				"https://dev.azure.com",
				"pat-token",
				"platform",
				"platform",
				"release-tools",
				test.repo,
			)

			// when: checking the candidate against the configured base branch
			trusted := azureDevOpsProvider.isTrustedReleasePR(pullRequest, "main")

			// then: only the exact same-repository release branch is trusted
			testastic.Equal(t, test.trusted, trusted)
		})
	}
}

func TestTrustedAzureDevOpsReleasePRMatchesRepositoryConfiguredByID(t *testing.T) {
	t.Parallel()

	const repositoryID = "3f7c1b0e-4a2d-4c1e-9f3a-6b5d8e2a1c40"

	// given: a pull request payload carrying the repository id
	var pullRequest git.GitPullRequest

	testastic.NoError(t, json.Unmarshal([]byte(`{
		"sourceRefName": "refs/heads/yeet/release-main",
		"repository": {"id": "`+repositoryID+`", "name": "yeet"}
	}`), &pullRequest))

	azureDevOpsProvider := NewAzureDevOps(
		nil,
		"https://dev.azure.com",
		"pat-token",
		"platform",
		"platform",
		"release-tools",
		strings.ToUpper(repositoryID),
	)

	// when: checking the candidate against a provider configured with that id
	trusted := azureDevOpsProvider.isTrustedReleasePR(&pullRequest, "main")

	// then: the repository matches regardless of the configured id casing
	testastic.Equal(t, true, trusted)
}

func TestAzureDevOpsPullRequestWebURL(t *testing.T) {
	t.Parallel()

	azureDevOpsProvider := NewAzureDevOps(
		nil,
		"https://dev.azure.com",
		"pat-token",
		"contoso",
		"contoso",
		"platform",
		"yeet",
	)

	testastic.Equal(
		t,
		"https://dev.azure.com/contoso/platform/_git/yeet/pullrequest/42",
		azureDevOpsProvider.pullRequestWebURL(42),
	)
}

func TestAzureDevOpsCompareURL(t *testing.T) {
	t.Parallel()

	azureDevOpsProvider := NewAzureDevOps(
		nil,
		"https://dev.azure.com",
		"pat-token",
		"contoso",
		"contoso",
		"platform",
		"yeet",
	)

	const (
		base = "https://dev.azure.com/contoso/platform/_git/yeet/branchCompare"
		sha1 = "a29bbfeda10bff1ba8ef28d0949b4e5ee84a49b7"
		sha2 = "b246f8607d17a5072d14ed9bad21ba92f9c5a0f9"
	)

	tests := map[string]struct {
		fromRef string
		toRef   string
		want    string
	}{
		"tag to commit sha": {
			fromRef: "v0.1.0",
			toRef:   sha1,
			want:    base + "?baseVersion=GTv0.1.0&targetVersion=GC" + sha1,
		},
		"tag to tag": {
			fromRef: "v0.1.0",
			toRef:   "v0.2.0",
			want:    base + "?baseVersion=GTv0.1.0&targetVersion=GTv0.2.0",
		},
		"sha to sha": {
			fromRef: sha1,
			toRef:   sha2,
			want:    base + "?baseVersion=GC" + sha1 + "&targetVersion=GC" + sha2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// given: the fromRef and toRef inputs for this table case
			// when: CompareURL is invoked with those refs on the Azure DevOps provider
			// then: the produced URL matches the expected branchCompare URL for the case
			testastic.Equal(t, tc.want, azureDevOpsProvider.CompareURL(tc.fromRef, tc.toRef))
		})
	}
}

func TestAzureDevOpsReadFileBody(t *testing.T) {
	t.Parallel()

	t.Run("reads file within the limit", func(t *testing.T) {
		t.Parallel()

		// given: a file body below the size limit
		body := strings.NewReader("changelog contents")

		// when: reading the body
		contents, err := readAzureDevOpsFileBody(body, "CHANGELOG.md", "main")

		// then: the contents are returned
		testastic.NoError(t, err)
		testastic.Equal(t, "changelog contents", contents)
	})

	t.Run("rejects file exceeding the limit", func(t *testing.T) {
		t.Parallel()

		// given: a file body one byte over the size limit
		body := strings.NewReader(strings.Repeat("a", azureDevOpsMaxFileBytes+1))

		// when: reading the body
		_, err := readAzureDevOpsFileBody(body, "CHANGELOG.md", "main")

		// then: the read fails with the file size error
		testastic.ErrorIs(t, err, errAzureDevOpsFileTooLarge)
	})
}
