package provider_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-github/v91/github"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/provider"
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"
)

func TestMissingTokenErrorPreservesClassificationAndFacts(t *testing.T) {
	t.Parallel()

	// given: a missing GitHub token with its supported environment variables
	err := &provider.MissingTokenError{
		Provider:  "github",
		Variables: []string{"GITHUB_TOKEN", "GH_TOKEN"},
	}

	// when: the error is presented or inspected by a caller
	var missingToken *provider.MissingTokenError

	found := errors.As(err, &missingToken)

	// then: the established text, sentinel, and token facts remain available
	testastic.Equal(t, "missing auth token: GITHUB_TOKEN or GH_TOKEN environment variable is required", err.Error())
	testastic.ErrorIs(t, err, provider.ErrMissingToken)
	testastic.True(t, found)
	testastic.Equal(t, "github", missingToken.Provider)
	testastic.SliceEqual(t, []string{"GITHUB_TOKEN", "GH_TOKEN"}, missingToken.Variables)
}

func TestSetupErrorPreservesCauseAndSafeFacts(t *testing.T) {
	t.Parallel()

	// given: a trusted-host setup failure with a known root cause
	cause := errors.New("remote lookup failed")
	err := &provider.SetupError{
		Host:     "gitlab.example.com",
		Remote:   "upstream",
		Provider: "gitlab",
		Problem:  "provider host is not trusted",
		Err:      cause,
	}

	// when: the error is inspected at the command boundary
	var setup *provider.SetupError

	found := errors.As(err, &setup)

	// then: the cause and presenter facts are available without parsing error text
	testastic.Equal(t, cause.Error(), err.Error())
	testastic.ErrorIs(t, err, cause)
	testastic.True(t, found)
	testastic.Equal(t, "gitlab.example.com", setup.Host)
	testastic.Equal(t, "upstream", setup.Remote)
	testastic.Equal(t, "gitlab", setup.Provider)
	testastic.Equal(t, "provider host is not trusted", setup.Problem)
}

func TestReviewerAndLabelErrorsPreserveIdentityAndFacts(t *testing.T) {
	t.Parallel()

	t.Run("reviewer", func(t *testing.T) {
		t.Parallel()

		// given: a reviewer error wrapping the established not-found sentinel
		cause := fmt.Errorf("%w: %q is not a project member", forge.ErrReviewerNotFound, "sam")
		err := &provider.ReviewerError{Reviewer: "sam", Err: cause}

		// when: the error is inspected by the command presenter
		var reviewer *provider.ReviewerError

		found := errors.As(err, &reviewer)

		// then: the exact text, sentinel, and reviewer fact remain available
		testastic.Equal(t, cause.Error(), err.Error())
		testastic.ErrorIs(t, err, forge.ErrReviewerNotFound)
		testastic.True(t, found)
		testastic.Equal(t, "sam", reviewer.Reviewer)
	})

	t.Run("label", func(t *testing.T) {
		t.Parallel()

		// given: a pending lifecycle label mismatch for a known merge request
		cause := fmt.Errorf(
			"%w: trusted merge request !42 on branch %q is missing configured pending label %q",
			forge.ErrReleasePRLabelMismatch,
			"release/main",
			"release: pending",
		)
		err := &provider.LabelError{
			Label:     "release: pending",
			Role:      "pending",
			Reference: "merge request !42",
			Branch:    "release/main",
			Err:       cause,
		}

		// when: the error is inspected by the command presenter
		var label *provider.LabelError

		found := errors.As(err, &label)

		// then: the exact text, sentinel, and label facts remain available
		testastic.Equal(t, cause.Error(), err.Error())
		testastic.ErrorIs(t, err, forge.ErrReleasePRLabelMismatch)
		testastic.True(t, found)
		testastic.Equal(t, "release: pending", label.Label)
		testastic.Equal(t, "pending", label.Role)
		testastic.Equal(t, "merge request !42", label.Reference)
		testastic.Equal(t, "release/main", label.Branch)
	})
}

func TestErrorStatusReadsSupportedProviderErrors(t *testing.T) {
	t.Parallel()

	t.Run("github", func(t *testing.T) {
		t.Parallel()

		// given: a wrapped GitHub SDK response error
		err := &github.ErrorResponse{Response: &http.Response{StatusCode: http.StatusUnauthorized}}

		// when: the provider status is requested
		status := provider.ErrorStatus(err)

		// then: the typed response status is returned
		testastic.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("gitlab", func(t *testing.T) {
		t.Parallel()

		// given: a wrapped GitLab SDK response error
		err := &gitlab.ErrorResponse{Response: &http.Response{StatusCode: http.StatusForbidden}}

		// when: the provider status is requested
		status := provider.ErrorStatus(err)

		// then: the typed response status is returned
		testastic.Equal(t, http.StatusForbidden, status)
	})

	t.Run("wrapped gitlab not found", func(t *testing.T) {
		t.Parallel()

		// given: a wrapped native GitLab not-found error without an HTTP response
		err := fmt.Errorf("lookup failed: %w", gitlab.ErrNotFound)

		// when: the provider status is requested
		status := provider.ErrorStatus(err)

		// then: the GitLab error's status code is returned
		testastic.Equal(t, http.StatusNotFound, status)
	})

	t.Run("azure devops", func(t *testing.T) {
		t.Parallel()

		// given: an Azure DevOps SDK error with a response status
		statusCode := http.StatusBadGateway
		err := azuredevops.WrappedError{StatusCode: &statusCode}

		// when: the provider status is requested
		status := provider.ErrorStatus(err)

		// then: the typed response status is returned
		testastic.Equal(t, http.StatusBadGateway, status)
	})

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()

		// given: an error without a supported provider response
		err := errors.New("transport failed")

		// when: the provider status is requested
		status := provider.ErrorStatus(err)

		// then: no status is inferred from arbitrary text
		testastic.Equal(t, 0, status)
	})
}
