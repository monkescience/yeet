package provider

import (
	"encoding/base64"
	"strings"
	"testing"

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
