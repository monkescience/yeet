package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/forge"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var gitLabAutoMergeMinimumVersion = semver.MustParse("17.11.0")

func (g *GitLab) EnsureAutoMerge(ctx context.Context, number int, opts forge.MergeReleasePROptions) error {
	mergeRequest, _, err := g.client.MergeRequests.GetMergeRequest(
		g.projectID,
		int64(number),
		nil,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("get merge request !%d: %w", number, err)
	}

	reference := gitLabMergeRequestReference(number)
	if mergeRequest == nil {
		return blockedMerge(reference, forge.MergeBlockedReasonFailure, "fetch returned no merge request")
	}

	current := gitLabMergeState(reference, mergeRequest)
	if !isTrustedMergeState(current, opts.BaseBranch, mergeExpectedReleaseBranch(g.releaseBranch, opts.ReleaseBranch)) {
		return fmt.Errorf("%w: %s", forge.ErrUntrustedReleasePR, current.Reference)
	}

	if current.IsMerged {
		return nil
	}

	err = checkAutoMergeCandidate(current)
	if err != nil {
		return err
	}

	if mergeRequest.MergeWhenPipelineSucceeds {
		return g.ensureGitLabExistingAutoMerge(ctx, reference, opts.Method, mergeRequest)
	}

	err = g.checkGitLabAutoMergeVersion(ctx)
	if err != nil {
		return err
	}

	project, err := g.projectMergeSettings(ctx)
	if err != nil {
		return err
	}

	acceptOptions, err := gitLabAutoMergeOptions(project, opts.Method)
	if err != nil {
		return err
	}

	if project.MergeTrainsEnabled {
		return g.ensureGitLabMergeTrain(ctx, number, reference, current.HeadSHA, opts.Method, mergeRequest, acceptOptions)
	}

	return g.ensureGitLabAutoMerge(ctx, number, reference, current.HeadSHA, acceptOptions)
}

func (g *GitLab) ensureGitLabExistingAutoMerge(
	ctx context.Context,
	reference string,
	requested forge.MergeMethod,
	mergeRequest *gitlab.MergeRequest,
) error {
	err := checkGitLabExistingAutoMergeMethod(reference, requested, mergeRequest)
	if err != nil {
		return err
	}

	if requested == "" || requested == forge.MergeMethodAuto || requested == forge.MergeMethodSquash {
		return nil
	}

	project, err := g.projectMergeSettings(ctx)
	if err != nil {
		return err
	}

	_, err = gitLabAutoMergeOptions(project, requested)

	return err
}

func (g *GitLab) checkGitLabAutoMergeVersion(ctx context.Context) error {
	serverVersion, _, err := g.client.Version.GetVersion(gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("get GitLab version for provider-managed auto-merge: %w", err)
	}

	if serverVersion == nil {
		return fmt.Errorf("%w: GitLab returned no server version", forge.ErrAutoMergeUnsupported)
	}

	version := strings.TrimSpace(serverVersion.Version)
	version = strings.TrimSuffix(version, "-ee")
	version = strings.TrimSuffix(version, "-ce")

	parsed, err := semver.NewVersion(version)
	if err != nil {
		return fmt.Errorf(
			"%w: cannot parse GitLab version %q",
			forge.ErrAutoMergeUnsupported,
			serverVersion.Version,
		)
	}

	if parsed.LessThan(gitLabAutoMergeMinimumVersion) {
		return fmt.Errorf(
			"%w: GitLab %s is older than required version %s",
			forge.ErrAutoMergeUnsupported,
			parsed,
			gitLabAutoMergeMinimumVersion,
		)
	}

	return nil
}

func (g *GitLab) ensureGitLabAutoMerge(
	ctx context.Context,
	number int,
	reference, headSHA string,
	acceptOptions *gitlab.AcceptMergeRequestOptions,
) error {
	if headSHA == "" {
		return fmt.Errorf("%w: %s", forge.ErrEmptyCommitSHA, reference)
	}

	acceptOptions.SHA = new(headSHA)

	accepted, response, err := g.client.MergeRequests.AcceptMergeRequest(
		g.projectID,
		int64(number),
		acceptOptions,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusMethodNotAllowed {
			return blockedMerge(reference, forge.MergeBlockedReasonFailure, "was refused: "+err.Error())
		}

		return fmt.Errorf("enable auto-merge for merge request !%d: %w", number, err)
	}

	if accepted == nil {
		return blockedMerge(reference, forge.MergeBlockedReasonFailure, "auto-merge returned no merge request")
	}

	mergeError := strings.TrimSpace(accepted.MergeError)
	if mergeError != "" {
		return blockedMerge(reference, forge.MergeBlockedReasonFailure, "was refused: "+mergeError)
	}

	if accepted.State == gitlabMergeRequestMergedState ||
		accepted.MergeWhenPipelineSucceeds {
		return nil
	}

	return blockedMerge(reference, forge.MergeBlockedReasonFailure, "was refused: auto-merge was not confirmed")
}

func gitLabAutoMergeOptions(
	project *gitlab.Project,
	requested forge.MergeMethod,
) (*gitlab.AcceptMergeRequestOptions, error) {
	options, err := gitLabAcceptMergeOptions(project, requested)
	if err != nil {
		return nil, err
	}

	if project.MergeTrainsEnabled && (requested == "" || requested == forge.MergeMethodAuto) {
		options.Squash = nil
	}

	if requested == forge.MergeMethodMerge || requested == forge.MergeMethodRebase {
		if project.SquashOption == gitlab.SquashOptionAlways {
			return nil, gitLabMergeMethodBlocked(fmt.Sprintf(
				"merge method %q conflicts with project squash_option=%s",
				requested,
				project.SquashOption,
			))
		}

		options.Squash = new(false)
	}

	options.AutoMerge = new(true)

	return options, nil
}

func (g *GitLab) ensureGitLabMergeTrain(
	ctx context.Context,
	number int,
	reference, headSHA string,
	requested forge.MergeMethod,
	mergeRequest *gitlab.MergeRequest,
	acceptOptions *gitlab.AcceptMergeRequestOptions,
) error {
	entry, response, err := g.client.MergeTrains.GetMergeRequestOnAMergeTrain(
		g.projectID,
		int64(number),
		gitlab.WithContext(ctx),
	)

	if err == nil && entry != nil {
		return checkGitLabExistingAutoMergeMethod(reference, requested, mergeRequest)
	}

	if err == nil {
		return fmt.Errorf(
			"%w: check %s merge train returned no entry",
			errAutoMergeResponseInvalid,
			reference,
		)
	}

	if response == nil || response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("check %s merge train: %w", reference, err)
	}

	trainOptions := &gitlab.AddMergeRequestToMergeTrainOptions{
		AutoMerge: new(true),
		Squash:    acceptOptions.Squash,
	}

	if headSHA == "" {
		return fmt.Errorf("%w: %s", forge.ErrEmptyCommitSHA, reference)
	}

	trainOptions.SHA = new(headSHA)

	entries, response, err := g.client.MergeTrains.AddMergeRequestToMergeTrain(
		g.projectID,
		int64(number),
		trainOptions,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("add %s to merge train: %w", reference, err)
	}

	return checkGitLabMergeTrainAcceptance(reference, entries, response)
}

func checkGitLabMergeTrainAcceptance(
	reference string,
	entries []*gitlab.MergeTrain,
	response *gitlab.Response,
) error {
	if response == nil ||
		(response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusAccepted) {
		return fmt.Errorf(
			"%w: add %s to merge train returned no acceptance status",
			errAutoMergeResponseInvalid,
			reference,
		)
	}

	if response.StatusCode == http.StatusCreated && len(entries) == 0 {
		return fmt.Errorf(
			"%w: add %s to merge train returned no entry",
			errAutoMergeResponseInvalid,
			reference,
		)
	}

	return nil
}

func checkGitLabExistingAutoMergeMethod(
	reference string,
	requested forge.MergeMethod,
	mergeRequest *gitlab.MergeRequest,
) error {
	if requested == "" || requested == forge.MergeMethodAuto {
		return nil
	}

	squashes := mergeRequest.SquashOnMerge
	if requested == forge.MergeMethodSquash && squashes {
		return nil
	}

	if requested != forge.MergeMethodSquash && !squashes {
		return nil
	}

	return blockedMerge(reference, forge.MergeBlockedReasonMethod, fmt.Sprintf(
		"existing auto-merge options conflict with merge method %q",
		strings.TrimSpace(string(requested)),
	))
}
