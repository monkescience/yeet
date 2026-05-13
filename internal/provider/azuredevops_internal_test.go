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
