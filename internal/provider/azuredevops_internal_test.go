package provider

import (
	"encoding/base64"
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

func TestBuildAzureDevOpsCommitCriteria(t *testing.T) {
	t.Parallel()

	t.Run("unbounded history filters commits by branch", func(t *testing.T) {
		t.Parallel()

		// given: no boundary ref, so the full branch history is requested
		// when: building the commit criteria for a branch with no boundary
		criteria := buildAzureDevOpsCommitCriteria("main", "")

		// then: the branch is the item-version filter (not the compare version),
		// so Azure restricts to commits reachable from the branch rather than
		// returning every commit in the repository
		testastic.NotNil(t, criteria.ItemVersion)
		testastic.Equal(t, "main", *criteria.ItemVersion.Version)
		testastic.Equal(t, git.GitVersionTypeValues.Branch, *criteria.ItemVersion.VersionType)
		testastic.Nil(t, criteria.CompareVersion)
	})

	t.Run("tag boundary stops at the tag and walks from the branch head", func(t *testing.T) {
		t.Parallel()

		// given: a tag boundary
		// when: building the commit criteria
		criteria := buildAzureDevOpsCommitCriteria("main", "v1.0.0")

		// then: the tag is the item-version boundary and the branch is the
		// compare-version head, so Azure computes the graph range itself
		testastic.NotNil(t, criteria.ItemVersion)
		testastic.Equal(t, "v1.0.0", *criteria.ItemVersion.Version)
		testastic.Equal(t, git.GitVersionTypeValues.Tag, *criteria.ItemVersion.VersionType)
		testastic.NotNil(t, criteria.CompareVersion)
		testastic.Equal(t, "main", *criteria.CompareVersion.Version)
		testastic.Equal(t, git.GitVersionTypeValues.Branch, *criteria.CompareVersion.VersionType)
	})

	t.Run("commit sha boundary uses the commit version type", func(t *testing.T) {
		t.Parallel()

		// given: a 40-character commit SHA boundary
		sha := "a29bbfeda10bff1ba8ef28d0949b4e5ee84a49b7"

		// when: building the commit criteria
		criteria := buildAzureDevOpsCommitCriteria("main", sha)

		// then: the boundary is typed as a commit, not a tag
		testastic.NotNil(t, criteria.ItemVersion)
		testastic.Equal(t, sha, *criteria.ItemVersion.Version)
		testastic.Equal(t, git.GitVersionTypeValues.Commit, *criteria.ItemVersion.VersionType)
	})
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
