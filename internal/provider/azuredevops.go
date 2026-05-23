package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

const azureDevOpsZeroObjectID = "0000000000000000000000000000000000000000"

var _ Provider = (*AzureDevOps)(nil)

type AzureDevOps struct {
	conn         *azuredevops.Connection
	baseURL      string
	organization string
	collection   string
	project      string
	repo         string

	clientOnce sync.Once
	gitClient  git.Client
	clientErr  error
}

// NewAzureDevOps constructs the provider client.
// baseURL must be the host-level base (e.g. https://dev.azure.com or a
// self-hosted host). The collection segment is appended internally. collection
// defaults to organization on cloud deployments. The httpClient's Timeout is
// honored by the SDK. Retry middleware is not propagated because the SDK
// constructs its own http.Client.
func NewAzureDevOps(
	httpClient *http.Client,
	baseURL, pat, organization, collection, project, repo string,
) *AzureDevOps {
	return newAzureDevOps(httpClient, baseURL, patConnection, pat, organization, collection, project, repo)
}

func NewAzureDevOpsWithSystemAccessToken(
	httpClient *http.Client,
	baseURL, token, organization, collection, project, repo string,
) *AzureDevOps {
	return newAzureDevOps(httpClient, baseURL, systemAccessTokenConnection, token, organization, collection, project, repo)
}

func newAzureDevOps(
	httpClient *http.Client,
	baseURL string,
	connectionFactory func(string, string) *azuredevops.Connection,
	token, organization, collection, project, repo string,
) *AzureDevOps {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	if collection == "" {
		collection = organization
	}

	connectionURL := fmt.Sprintf("%s/%s", baseURL, collection)
	conn := connectionFactory(connectionURL, token)

	if httpClient != nil && httpClient.Timeout > 0 {
		timeout := httpClient.Timeout
		conn.Timeout = &timeout
	}

	return &AzureDevOps{
		conn:         conn,
		baseURL:      baseURL,
		organization: organization,
		collection:   collection,
		project:      project,
		repo:         repo,
	}
}

func patConnection(connectionURL, token string) *azuredevops.Connection {
	return azuredevops.NewPatConnection(connectionURL, token)
}

func systemAccessTokenConnection(connectionURL, token string) *azuredevops.Connection {
	conn := azuredevops.NewAnonymousConnection(connectionURL)
	conn.AuthorizationString = "Bearer " + token

	return conn
}

func (a *AzureDevOps) RepoURL() string {
	return fmt.Sprintf("%s/%s/%s/_git/%s", a.baseURL, a.collection, a.project, a.repo)
}

func (a *AzureDevOps) PathPrefix() string {
	return ""
}

// CompareURL builds the branch-compare URL.
// Azure DevOps uses query parameters with prefixed version refs: GC for commit
// SHAs, GT for tag names.
func (a *AzureDevOps) CompareURL(fromRef, toRef string) string {
	return fmt.Sprintf(
		"%s/branchCompare?baseVersion=%s&targetVersion=%s",
		a.RepoURL(),
		azureDevOpsCompareRef(fromRef),
		azureDevOpsCompareRef(toRef),
	)
}

func azureDevOpsCompareRef(ref string) string {
	if isAzureDevOpsCommitSHA(ref) {
		return "GC" + ref
	}

	return "GT" + ref
}

// Construction performs an HTTP roundtrip to fetch resource areas, which is why
// it cannot happen in NewAzureDevOps (no context available there).
func (a *AzureDevOps) client(ctx context.Context) (git.Client, error) {
	a.clientOnce.Do(func() {
		gitClient, err := git.NewClient(ctx, a.conn)
		if err != nil {
			a.clientErr = fmt.Errorf("initialize azure devops git client: %w", err)

			return
		}

		a.gitClient = gitClient
	})

	if a.clientErr != nil {
		return nil, a.clientErr
	}

	return a.gitClient, nil
}

// The SDK wraps non-2xx responses in azuredevops.WrappedError with a
// StatusCode pointer. Returns 0 if no status code is available.
func azureDevOpsStatusCode(err error) int {
	if err == nil {
		return 0
	}

	var wrapped azuredevops.WrappedError
	if errors.As(err, &wrapped) && wrapped.StatusCode != nil {
		return *wrapped.StatusCode
	}

	var wrappedPtr *azuredevops.WrappedError
	if errors.As(err, &wrappedPtr) && wrappedPtr != nil && wrappedPtr.StatusCode != nil {
		return *wrappedPtr.StatusCode
	}

	return 0
}

func isAzureDevOpsNotFound(err error) bool {
	return azureDevOpsStatusCode(err) == http.StatusNotFound
}
