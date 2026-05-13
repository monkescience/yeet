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

func TestAzureDevOpsPullRequestWebURL(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pr   *git.GitPullRequest
		want string
	}{
		"builds web url from repository web url and id": {
			pr: &git.GitPullRequest{
				PullRequestId: new(1),
				Repository: &git.GitRepository{
					WebUrl: new("https://dev.azure.com/org/proj/_git/repo"),
				},
			},
			want: "https://dev.azure.com/org/proj/_git/repo/pullrequest/1",
		},
		"returns empty when repository is missing": {
			pr: &git.GitPullRequest{
				PullRequestId: new(1),
			},
			want: "",
		},
		"returns empty when web url is missing": {
			pr: &git.GitPullRequest{
				PullRequestId: new(1),
				Repository:    &git.GitRepository{},
			},
			want: "",
		},
		"returns empty when pull request id is missing": {
			pr: &git.GitPullRequest{
				Repository: &git.GitRepository{
					WebUrl: new("https://dev.azure.com/org/proj/_git/repo"),
				},
			},
			want: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			testastic.Equal(t, tc.want, azureDevOpsPullRequestWebURL(tc.pr))
		})
	}
}
