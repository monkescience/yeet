package provider

import (
	"errors"
	"net/http"

	"github.com/google/go-github/v91/github"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"
)

type RequestDetails struct {
	Provider  string
	Method    string
	Path      string
	RequestID string
	RateLimit bool
}

func ErrorRequestDetails(err error) RequestDetails {
	name, response := errorResponse(err) //nolint:bodyclose // SDK owns the response body

	details := RequestDetails{Provider: name}
	if response == nil {
		return details
	}

	details.RateLimit = response.StatusCode == http.StatusTooManyRequests ||
		(response.StatusCode == http.StatusForbidden && remainingRequests(response.Header) == "0")
	if _, ok := errors.AsType[*github.AbuseRateLimitError](err); ok {
		details.RateLimit = true
	}

	for _, header := range []string{"X-GitHub-Request-Id", "X-Request-Id"} {
		if value := response.Header.Get(header); value != "" {
			details.RequestID = value

			break
		}
	}

	if response.Request != nil {
		details.Method = response.Request.Method
		if response.Request.URL != nil {
			details.Path = response.Request.URL.EscapedPath()
		}
	}

	return details
}

func remainingRequests(header http.Header) string {
	for _, name := range []string{"X-RateLimit-Remaining", "RateLimit-Remaining"} {
		if value := header.Get(name); value != "" {
			return value
		}
	}

	return ""
}

func errorResponse(err error) (string, *http.Response) {
	if failure, ok := errors.AsType[*github.ErrorResponse](err); ok {
		return providerNameGitHub, failure.Response
	}

	if failure, ok := errors.AsType[*github.RateLimitError](err); ok {
		return providerNameGitHub, failure.Response
	}

	if failure, ok := errors.AsType[*github.AbuseRateLimitError](err); ok {
		return providerNameGitHub, failure.Response
	}

	if failure, ok := errors.AsType[*gitlab.ErrorResponse](err); ok {
		return providerNameGitLab, failure.Response
	}

	if _, ok := errors.AsType[azuredevops.WrappedError](err); ok {
		return providerNameAzureDevOps, nil
	}

	if _, ok := errors.AsType[*azuredevops.WrappedError](err); ok {
		return providerNameAzureDevOps, nil
	}

	return "", nil
}
