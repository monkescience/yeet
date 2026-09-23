package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-github/v91/github"
	"github.com/monkescience/yeet/internal/forge"
)

const (
	gitHubAutoMergeMutation = `mutation YeetEnablePullRequestAutoMerge($input: EnablePullRequestAutoMergeInput!) {
  enablePullRequestAutoMerge(input: $input) { pullRequest { id } }
}`
	gitHubEnqueueMutation = `mutation YeetEnqueuePullRequest($input: EnqueuePullRequestInput!) {
  enqueuePullRequest(input: $input) { mergeQueueEntry { id } }
}`
	gitHubMergeQueueQuery = `query YeetPullRequestMergeQueue(
  $owner: String!, $name: String!, $number: Int!, $branch: String!
) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) { isMergeQueueEnabled mergeQueueEntry { id } }
    mergeQueue(branch: $branch) { configuration { mergeMethod } }
  }
}`
)

type gitHubGraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type gitHubGraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type gitHubMergeQueueData struct {
	Repository *gitHubMergeQueueRepository `json:"repository"`
}

type gitHubMergeQueueRepository struct {
	PullRequest *struct {
		IsMergeQueueEnabled bool `json:"isMergeQueueEnabled"`
		MergeQueueEntry     *struct {
			ID string `json:"id"`
		} `json:"mergeQueueEntry"`
	} `json:"pullRequest"`
	MergeQueue *struct {
		Configuration struct {
			MergeMethod string `json:"mergeMethod"`
		} `json:"configuration"`
	} `json:"mergeQueue"`
}

func gitHubAutoMergeIdentityMissing(reference string) error {
	return &forge.AutoMergeUnsupportedError{
		Provider:  providerNameGitHub,
		Reference: reference,
		Problem:   "pull request is missing a node ID or head SHA",
	}
}

func (g *GitHub) EnsureAutoMerge(ctx context.Context, number int, opts forge.MergeReleasePROptions) error {
	pullRequest, _, err := g.client.PullRequests.Get(ctx, g.repo.Owner, g.repo.Name, number)
	if err != nil {
		return fmt.Errorf("get pull request #%d: %w", number, err)
	}

	current := gitHubMergeState(g.repo, number, pullRequest)

	if !isTrustedMergeState(current, opts.BaseBranch, mergeExpectedReleaseBranch(g.releaseBranch, opts.ReleaseBranch)) {
		return &forge.UntrustedReleasePRError{Reference: current.Reference}
	}

	if current.IsMerged {
		return nil
	}

	err = checkAutoMergeCandidate(current)
	if err != nil {
		return err
	}

	queueMethod, queueRequired, queued, err := g.gitHubMergeQueue(ctx, number, current.BaseBranch)
	if err != nil {
		return err
	}

	if pullRequest.AutoMerge != nil {
		if queueRequired {
			_, err = g.resolveGitHubAutoMergeMethod(ctx, opts.Method, queueMethod, queueRequired)
			if err != nil {
				return err
			}

			return nil
		}

		return checkGitHubExistingAutoMergeMethod(current.Reference, opts.Method, pullRequest.AutoMerge.GetMergeMethod())
	}

	method, err := g.resolveGitHubAutoMergeMethod(ctx, opts.Method, queueMethod, queueRequired)
	if err != nil {
		return err
	}

	pullRequestID := strings.TrimSpace(pullRequest.GetNodeID())
	if pullRequestID == "" || current.HeadSHA == "" {
		return gitHubAutoMergeIdentityMissing(current.Reference)
	}

	if queueRequired {
		if queued {
			return nil
		}

		if gitHubImmediatelyMergeable(pullRequest.GetMergeableState()) {
			return g.gitHubEnqueuePullRequest(ctx, current.Reference, pullRequestID, current.HeadSHA)
		}
	}

	return g.gitHubEnableAutoMerge(ctx, current.Reference, pullRequestID, current.HeadSHA, method)
}

func (g *GitHub) gitHubMergeQueue(
	ctx context.Context,
	number int,
	branch string,
) (forge.MergeMethod, bool, bool, error) {
	var data gitHubMergeQueueData

	variables := map[string]any{
		"owner":  g.repo.Owner,
		"name":   g.repo.Name,
		"number": number,
		"branch": branch,
	}

	err := g.gitHubGraphQL(ctx, gitHubMergeQueueQuery, variables, &data)
	if err != nil {
		return "", false, false, fmt.Errorf("get merge queue for branch %q: %w", branch, err)
	}

	if data.Repository == nil || data.Repository.PullRequest == nil {
		return "", false, false, fmt.Errorf(
			"%w: get merge queue for branch %q returned no pull request",
			errAutoMergeResponseInvalid,
			branch,
		)
	}

	if !data.Repository.PullRequest.IsMergeQueueEnabled {
		return "", false, false, nil
	}

	configured, err := gitHubConfiguredQueueMethod(branch, data.Repository)
	if err != nil {
		return "", false, false, err
	}

	return configured, true, data.Repository.PullRequest.MergeQueueEntry != nil, nil
}

func gitHubConfiguredQueueMethod(
	branch string,
	repository *gitHubMergeQueueRepository,
) (forge.MergeMethod, error) {
	if repository.MergeQueue == nil {
		return "", fmt.Errorf(
			"%w: merge queue for branch %q has no configuration",
			forge.ErrAutoMergeUnsupported,
			branch,
		)
	}

	configured := forge.MergeMethod(strings.ToLower(strings.TrimSpace(
		repository.MergeQueue.Configuration.MergeMethod,
	)))
	if configured != forge.MergeMethodMerge &&
		configured != forge.MergeMethodRebase &&
		configured != forge.MergeMethodSquash {
		return "", &forge.MergeMethodUnsupportedError{
			Method: forge.MergeMethod(repository.MergeQueue.Configuration.MergeMethod),
			Branch: branch,
		}
	}

	return configured, nil
}

func (g *GitHub) resolveGitHubAutoMergeMethod(
	ctx context.Context,
	requested, queueMethod forge.MergeMethod,
	queueRequired bool,
) (forge.MergeMethod, error) {
	if requested == "" {
		requested = forge.MergeMethodAuto
	}

	if !queueRequired {
		return g.resolveGitHubMergeMethod(ctx, requested)
	}

	if requested == forge.MergeMethodAuto {
		return queueMethod, nil
	}

	if requested != queueMethod {
		return "", blockedMerge("", forge.MergeBlockedReasonMethod, fmt.Sprintf(
			"merge method %q conflicts with merge queue method %q",
			requested,
			queueMethod,
		))
	}

	return requested, nil
}

func checkGitHubExistingAutoMergeMethod(reference string, requested forge.MergeMethod, configured string) error {
	if requested == "" || requested == forge.MergeMethodAuto {
		return nil
	}

	if strings.EqualFold(strings.TrimSpace(configured), string(requested)) {
		return nil
	}

	return blockedMerge(reference, forge.MergeBlockedReasonMethod, fmt.Sprintf(
		"already has auto-merge enabled with method %q instead of %q",
		strings.ToLower(strings.TrimSpace(configured)),
		requested,
	))
}

func gitHubImmediatelyMergeable(state string) bool {
	switch strings.TrimSpace(state) {
	case "clean", "has_hooks", "unstable":
		return true
	default:
		return false
	}
}

func (g *GitHub) gitHubEnableAutoMerge(
	ctx context.Context,
	reference, pullRequestID, headSHA string,
	method forge.MergeMethod,
) error {
	input := map[string]any{
		"pullRequestId":   pullRequestID,
		"expectedHeadOid": headSHA,
		"mergeMethod":     strings.ToUpper(string(method)),
	}

	var data struct {
		EnablePullRequestAutoMerge *struct {
			PullRequest *struct {
				ID string `json:"id"`
			} `json:"pullRequest"`
		} `json:"enablePullRequestAutoMerge"`
	}

	err := g.gitHubGraphQL(ctx, gitHubAutoMergeMutation, map[string]any{"input": input}, &data)
	if err != nil {
		return fmt.Errorf("enable auto-merge for %s: %w", reference, err)
	}

	if data.EnablePullRequestAutoMerge == nil ||
		data.EnablePullRequestAutoMerge.PullRequest == nil ||
		strings.TrimSpace(data.EnablePullRequestAutoMerge.PullRequest.ID) == "" {
		return fmt.Errorf(
			"%w: enable auto-merge for %s returned no accepted pull request",
			errAutoMergeResponseInvalid,
			reference,
		)
	}

	return nil
}

func (g *GitHub) gitHubEnqueuePullRequest(
	ctx context.Context,
	reference, pullRequestID, headSHA string,
) error {
	input := map[string]any{
		"pullRequestId":   pullRequestID,
		"expectedHeadOid": headSHA,
	}

	var data struct {
		EnqueuePullRequest *struct {
			MergeQueueEntry *struct {
				ID string `json:"id"`
			} `json:"mergeQueueEntry"`
		} `json:"enqueuePullRequest"`
	}

	err := g.gitHubGraphQL(ctx, gitHubEnqueueMutation, map[string]any{"input": input}, &data)
	if err != nil {
		return fmt.Errorf("enqueue %s: %w", reference, err)
	}

	if data.EnqueuePullRequest == nil ||
		data.EnqueuePullRequest.MergeQueueEntry == nil ||
		strings.TrimSpace(data.EnqueuePullRequest.MergeQueueEntry.ID) == "" {
		return fmt.Errorf(
			"%w: enqueue %s returned no accepted merge queue entry",
			errAutoMergeResponseInvalid,
			reference,
		)
	}

	return nil
}

func (g *GitHub) gitHubGraphQL(
	ctx context.Context,
	query string,
	variables map[string]any,
	data any,
) error {
	req, err := g.client.NewRequest(ctx, http.MethodPost, "graphql", gitHubGraphQLRequest{
		Query:     query,
		Variables: variables,
	})
	if err != nil {
		return fmt.Errorf("prepare GraphQL request: %w", err)
	}

	req.URL, err = url.Parse(g.graphqlURL)
	if err != nil {
		return fmt.Errorf("parse GraphQL URL: %w", err)
	}

	var response gitHubGraphQLResponse

	httpResponse, err := g.client.Do(req, &response)
	if err != nil {
		if httpResponse != nil && httpResponse.StatusCode >= http.StatusBadRequest &&
			httpResponse.StatusCode < http.StatusInternalServerError {
			message := err.Error()
			if failure, ok := errors.AsType[*github.ErrorResponse](err); ok {
				message = failure.Message
			}

			return blockedMergeMessage("", forge.MergeBlockedReasonFailure,
				fmt.Sprintf("github GraphQL request returned HTTP %d", httpResponse.StatusCode), message)
		}

		return fmt.Errorf("github GraphQL request: %w", err)
	}

	if len(response.Errors) > 0 {
		messages := make([]string, 0, len(response.Errors))
		for _, graphQLError := range response.Errors {
			messages = append(messages, strings.TrimSpace(graphQLError.Message))
		}

		return fmt.Errorf("%w: %s", errGitHubGraphQLRequest, strings.Join(messages, ", "))
	}

	if len(response.Data) == 0 || string(response.Data) == "null" {
		return fmt.Errorf("%w: GitHub GraphQL request returned no data", errAutoMergeResponseInvalid)
	}

	err = json.Unmarshal(response.Data, data)
	if err != nil {
		return fmt.Errorf("decode GitHub GraphQL data: %w", err)
	}

	return nil
}
