package commands

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/release"
	"github.com/monkescience/yeet/internal/version"
	"github.com/monkescience/yeet/internal/versionfile"
)

type reportedFailure struct {
	diagnostic *diagnostic
	cause      *release.Failure
}

func reportReleaseError(ctx context.Context, failure *release.Failure) {
	reports := make([]reportedFailure, 0, len(failure.Failures()))
	byUnit := make(map[string]*diagnostic)
	suppressed := make(map[string]int)

	for _, failed := range failure.Failures() {
		unit := failed.Unit()
		if unit != "" && byUnit[unit] != nil {
			suppressed[unit]++

			continue
		}

		d := releaseDiagnostic(failed)

		if unit != "" {
			d.attrs = append([]slog.Attr{slog.String("unit", unit)}, d.attrs...)
			byUnit[unit] = &d
		}

		reports = append(reports, reportedFailure{diagnostic: &d, cause: failed})
	}

	for unit, count := range suppressed {
		byUnit[unit].attrs = append(byUnit[unit].attrs, slog.Int("further_failures", count))
	}

	for _, report := range reports {
		report.diagnostic.report(ctx, report.cause)
	}
}

func releaseDiagnostic(failure *release.Failure) diagnostic {
	d := releaseCategoryDiagnostic(failure.Kind(), failure.MergeReason())
	d.attrs = appendNonempty(d.attrs, slog.String("phase", failure.Phase()))

	for _, describe := range []func(*diagnostic, *release.Failure) bool{
		describeConfiguration,
		describeCheckout,
		describeProviderSetup,
		describeMergeState,
		describeProviderResource,
		describeProviderRequest,
		describeReleaseTarget,
		describeReleaseContent,
	} {
		if describe(&d, failure) {
			d.rawCause = false
		}
	}

	return d
}

func describeConfiguration(d *diagnostic, failure *release.Failure) bool {
	if failure.Kind() == release.FailureConfigMissing || failure.Kind() == release.FailureConfigInvalid {
		d.attrs = append(d.attrs, slog.String("path", failure.ConfigPath()))
	}

	invalid, ok := errors.AsType[*config.ValidationError](failure)
	if !ok {
		return false
	}

	d.explain(invalid.Problem)
	d.hint = ""

	if invalid.Line > 0 {
		d.attrs = append(d.attrs, slog.Int("line", invalid.Line), slog.Int("column", invalid.Column))
	}

	return true
}

func describeCheckout(d *diagnostic, failure *release.Failure) bool {
	checkout, ok := errors.AsType[*history.CheckoutError](failure)
	if !ok {
		return false
	}

	d.explain(checkout.Problem)
	d.attrs = appendNonempty(d.attrs,
		slog.String("branch", checkout.Branch), slog.String("current_branch", checkout.CurrentBranch),
		slog.String("ref", checkout.Ref), slog.String("local_head", checkout.LocalHead),
		slog.String("remote_head", checkout.RemoteHead))

	if hint := checkoutHint(checkout.Problem); hint != "" {
		d.hint = hint
	}

	return true
}

func checkoutHint(problem string) string {
	switch problem {
	case history.CheckoutProblemShallow:
		return "fetch the full history (fetch-depth: 0 on GitHub Actions, " +
			`GIT_DEPTH "0" on GitLab CI, fetchDepth: 0 on Azure Pipelines)`
	case history.CheckoutProblemBehindRemote:
		return "pull the latest commits of the release branch before releasing"
	case history.CheckoutProblemOtherBranch:
		return "check out the configured release branch"
	case history.CheckoutProblemNoRepository:
		return "run yeet from a git checkout"
	case history.CheckoutProblemTagUnavailable:
		return "fetch tags from the remote before releasing"
	default:
		return ""
	}
}

func describeProviderSetup(d *diagnostic, failure *release.Failure) bool {
	if token, ok := errors.AsType[*provider.MissingTokenError](failure); ok {
		d.attrs = append(d.attrs, slog.String("provider", token.Provider))
		d.hint = "set " + strings.Join(token.Variables, " or ")

		return true
	}

	setup, ok := errors.AsType[*provider.SetupError](failure)
	if !ok || failure.Kind() == release.FailureAuthentication {
		return false
	}

	d.attrs = appendNonempty(d.attrs, slog.String("provider", setup.Provider),
		slog.String("host", setup.Host), slog.String("remote", setup.Remote),
		slog.String("remote_host", setup.RemoteHost))

	if failure.Kind() != release.FailureConfigInvalid {
		d.explain(setup.Problem)
	}

	if setup.Hint != "" {
		d.hint = setup.Hint
	}

	return true
}

func describeMergeState(d *diagnostic, failure *release.Failure) bool {
	described := false

	if blocked, ok := errors.AsType[*forge.MergeBlockedError](failure); ok {
		d.attrs = appendNonempty(d.attrs, slog.String("pull_request", blocked.Reference))

		if mergeDetailExplains(failure.MergeReason()) {
			d.explain(blocked.Detail)
		}

		described = true
	}

	if timeout, ok := errors.AsType[*provider.MergeNotFinalizedError](failure); ok {
		d.attrs = appendNonempty(d.attrs, slog.String("pull_request", timeout.Reference()))
		d.attrs = append(d.attrs, slog.Duration("timeout", timeout.Timeout()))

		described = true
	}

	return described
}

func mergeDetailExplains(reason release.MergeReason) bool {
	switch reason {
	case release.MergeReasonMethod, release.MergeReasonPolicy,
		release.MergeReasonProvider, release.MergeReasonUnknown:
		return true
	case release.MergeReasonConflicts, release.MergeReasonDraft, release.MergeReasonClosed:
		return false
	default:
		return false
	}
}

func describeProviderResource(d *diagnostic, failure *release.Failure) bool {
	described := false

	if branch, ok := errors.AsType[*provider.BranchUpdateError](failure); ok {
		d.message = "could not update release branch"
		d.explain(branch.Problem)
		d.attrs = append(d.attrs, slog.String("provider", "azuredevops"), slog.String("branch", branch.Branch))
		d.hint = "check branch policies and token permissions in the provider"

		described = true
	}

	if reviewer, ok := errors.AsType[*provider.ReviewerError](failure); ok {
		d.explain(reviewer.Problem)
		d.attrs = appendNonempty(d.attrs, slog.String("reviewer", reviewer.Reviewer))

		if len(reviewer.Reviewers) > 0 {
			d.attrs = append(d.attrs, slog.Any("reviewers", reviewer.Reviewers))
		}

		described = true
	}

	if label, ok := errors.AsType[*provider.LabelError](failure); ok {
		describeLabel(d, failure)
		d.attrs = appendNonempty(d.attrs, slog.String("label", label.Label), slog.String("role", label.Role),
			slog.String("pull_request", label.Reference), slog.String("branch", label.Branch),
			slog.String("scope", label.Scope), slog.String("conflicts_with", label.Conflict))

		described = true
	}

	return described
}

func describeLabel(d *diagnostic, failure *release.Failure) {
	switch {
	case errors.Is(failure, forge.ErrReleasePRLabelMissing):
		d.message = "configured release label does not exist"
		d.hint = "create the label in the provider or remove it from the configuration"
	case errors.Is(failure, forge.ErrReleasePRLabelMismatch):
		d.message = "release pull request is missing its configured lifecycle label"
		d.hint = "restore the configured label or close the pull request"
	case errors.Is(failure, forge.ErrReleasePRLabelsRejected):
		d.message = "provider rejected the release labels"
		d.hint = "check the label names against the provider label rules"
	}
}

func describeProviderRequest(d *diagnostic, failure *release.Failure) bool {
	status := provider.ErrorStatus(failure)
	if status == 0 {
		return false
	}

	details := provider.ErrorRequestDetails(failure)

	d.attrs = append(d.attrs, slog.Int("status", status))
	d.attrs = appendNonempty(d.attrs, slog.String("provider", details.Provider),
		slog.String("method", details.Method), slog.String("path", details.Path),
		slog.String("request_id", details.RequestID))

	if d.hint == "" {
		d.hint = providerStatusHint(status, details.RateLimit)
	}

	if failure.Kind() == release.FailureUnexpected {
		d.message = "provider request failed"
	}

	return true
}

func providerStatusHint(status int, rateLimit bool) string {
	if rateLimit || status == http.StatusTooManyRequests {
		return "wait for the provider rate limit to reset before retrying"
	}

	switch status {
	case http.StatusUnauthorized:
		return "check that the provider token is valid and has not expired"
	case http.StatusForbidden:
		return "check token permissions, repository access, and provider policy"
	case http.StatusNotFound:
		return "check the repository and resource exist and the token can access them"
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return "check the requested resource state and provider validation rules"
	default:
		if status >= http.StatusInternalServerError {
			return "check provider availability and inspect remote state before retrying"
		}

		return "inspect the failed request with --verbose"
	}
}

func describeReleaseTarget(d *diagnostic, failure *release.Failure) bool {
	described := false

	if selection, ok := errors.AsType[*release.SelectionError](failure); ok {
		d.attrs = appendNonempty(d.attrs, slog.String("target", selection.Target), slog.String("branch", selection.Branch),
			slog.String("expected_branch", selection.ExpectedBranch), slog.String("channel", selection.Channel),
			slog.String("ref", selection.Ref))

		if selection.Target != "" {
			d.message = "unknown release target"
			d.hint = "select a target configured in targets"
		}

		described = true
	}

	pending, hasPending := errors.AsType[*release.PendingReleaseError](failure)
	if hasPending && len(pending.References) > 0 {
		d.attrs = append(d.attrs, slog.Any("pending", pending.References))

		described = true
	}

	if conflict, ok := errors.AsType[*release.FileConflictError](failure); ok {
		d.attrs = appendNonempty(d.attrs, slog.String("path", conflict.Path))

		if len(conflict.Units) > 0 {
			d.attrs = append(d.attrs, slog.Any("units", conflict.Units))
		}

		described = true
	}

	if boundary, ok := errors.AsType[*forge.CommitBoundaryNotFoundError](failure); ok {
		d.message = "release history boundary is not reachable from branch"
		d.attrs = appendNonempty(d.attrs, slog.String("ref", boundary.Ref),
			slog.String("branch", boundary.Branch), slog.String("target", boundary.Target))
		d.hint = "check the release tag and branch ancestry"

		described = true
	}

	file, ok := errors.AsType[*release.VersionFileError](failure)
	if !ok {
		return described
	}

	d.message = "could not update version file"
	d.attrs = appendNonempty(d.attrs, slog.String("target", file.Target), slog.String("path", file.Path))
	describeVersionFileProblem(d, failure)

	return true
}

func describeVersionFileProblem(d *diagnostic, err error) {
	problems := []struct {
		cause   error
		problem string
	}{
		{versionfile.ErrNoMarkersFound, "file has no yeet version markers"},
		{versionfile.ErrUnclosedBlockMarker, "version marker block has no end marker"},
		{versionfile.ErrNestedBlockMarker, "version marker blocks are nested"},
		{versionfile.ErrMarkerNoMatch, "version marker has no matching version"},
		{versionfile.ErrMarkerSchemeMismatch, "version marker does not match the versioning scheme"},
		{versionfile.ErrInvalidJSON, "file contains invalid json"},
		{versionfile.ErrJSONPointerNotFound, "json pointer was not found"},
		{versionfile.ErrJSONPointerNonString, "json pointer must select a string"},
	}
	for _, problem := range problems {
		if !errors.Is(err, problem.cause) {
			continue
		}

		d.explain(problem.problem)

		d.hint = "check the file and its version_files configuration"
		if marker, ok := errors.AsType[*versionfile.MarkerError](err); ok {
			d.attrs = append(d.attrs, slog.String("marker", marker.Name), slog.Int("line", marker.Line))

			if len(marker.Suggestions) > 0 {
				d.hint = "use " + strings.Join(marker.Suggestions, " or ") + " for the configured versioning scheme"
			}
		}

		return
	}
}

func describeReleaseContent(d *diagnostic, failure *release.Failure) bool {
	described := false

	switch {
	case errors.Is(failure, version.ErrInvalidReleaseAs):
		d.message = "invalid release version override"
		d.attrs = append(d.attrs, slog.String("field", "Release-As"))
		d.hint = "use a stable version newer than the current version"
	case errors.Is(failure, version.ErrConflictingReleaseAs):
		d.message = "conflicting release version overrides"
		d.attrs = append(d.attrs, slog.String("field", "Release-As"))
		d.hint = "use the same Release-As version for commits in this release"
	}

	if releaseAs, ok := errors.AsType[*version.ReleaseAsError](failure); ok {
		d.explain(releaseAs.Problem)
		d.attrs = appendNonempty(d.attrs, slog.String("requested", releaseAs.Requested),
			slog.String("current", releaseAs.Current))

		described = true
	}

	if override, ok := errors.AsType[*release.CommitOverrideError](failure); ok {
		d.message = "invalid commit override"
		d.explain(override.Problem)
		d.attrs = appendNonempty(d.attrs, slog.String("commit", override.Commit),
			slog.String("marker", override.MissingMarker))

		d.hint = "include commit messages between the override markers"
		if override.MissingMarker != "" {
			d.hint = "close the override block with " + override.MissingMarker
		}

		described = true
	}

	manifest, ok := errors.AsType[*release.ManifestError](failure)
	if !ok {
		return described
	}

	d.message = "release pull request has an invalid manifest"
	d.attrs = appendNonempty(d.attrs, slog.String("pull_request", manifest.Reference))
	d.hint = "restore the release manifest in the pull request body"

	return true
}

func appendNonempty(attrs []slog.Attr, values ...slog.Attr) []slog.Attr {
	for _, attr := range values {
		if attr.Value.String() != "" {
			attrs = append(attrs, attr)
		}
	}

	return attrs
}

func releaseCategoryDiagnostic(kind release.FailureKind, reason release.MergeReason) diagnostic {
	switch kind {
	case release.FailureConfigMissing:
		return diagnostic{message: "configuration file was not found", hint: "run yeet init or pass --config"}
	case release.FailureConfigInvalid:
		return diagnostic{message: "invalid configuration", hint: "fix the reported configuration values"}
	case release.FailureAuthentication:
		return diagnostic{message: "missing authentication token"}
	case release.FailureRepository:
		return diagnostic{
			message: "could not resolve repository",
			hint:    "check provider settings and the configured git remote",
		}
	case release.FailureHostTrust:
		return diagnostic{
			message: "provider host could not be trusted",
			hint:    "align the configured host, git remote, and provider url override",
		}
	case release.FailureCheckout:
		return diagnostic{
			message: "local checkout cannot be used for release",
			hint:    "fetch full history and check out the current remote release branch",
		}
	case release.FailureReleaseBranch:
		return diagnostic{message: "invalid release branch or channel", hint: "use a configured branch or channel"}
	case release.FailureReleaseState:
		return diagnostic{
			message: "multiple pending release pull requests",
			hint:    "close or relabel stale pending release pull requests",
		}
	case release.FailureMergeBlocked:
		return mergeDiagnostic(reason)
	case release.FailureMergeTimeout:
		return diagnostic{message: "merge finalization timed out", hint: "inspect provider state before retrying"}
	case release.FailureAutoMergeUnsupported:
		return diagnostic{
			message: "provider-managed auto-merge is unsupported",
			hint:    "check provider prerequisites or use --auto-merge-mode direct",
		}
	case release.FailureReviewer:
		return diagnostic{
			message: "release reviewer could not be applied",
			hint:    "check reviewer identity, membership, permissions, and provider limits",
		}
	case release.FailureLabels:
		return diagnostic{
			message: "release labels could not be applied",
			hint:    "restore or create the configured labels",
		}
	case release.FailureFileConflict:
		return diagnostic{
			message: "release writes conflict on one file",
			hint:    "configure separately addressable files or place the targets in one atomic group",
		}
	case release.FailureUnexpected:
		return diagnostic{message: "release could not be completed", rawCause: true}
	default:
		return diagnostic{message: "release could not be completed", rawCause: true}
	}
}

func mergeDiagnostic(reason release.MergeReason) diagnostic {
	const inspectRequest = "inspect the release pull request in the provider"

	switch reason {
	case release.MergeReasonConflicts:
		return diagnostic{message: "merge is blocked by conflicts", hint: "resolve conflicts on the release branch"}
	case release.MergeReasonDraft:
		return diagnostic{
			message: "release pull request is a draft",
			hint:    "mark the release pull request ready to merge",
		}
	case release.MergeReasonClosed:
		return diagnostic{
			message: "release pull request is closed",
			hint:    "reopen it or let the next run open a new one",
		}
	case release.MergeReasonPolicy:
		return diagnostic{message: "merge is blocked by repository policy", hint: "satisfy required approvals and checks"}
	case release.MergeReasonMethod:
		return diagnostic{
			message: "merge method is unavailable",
			hint:    "enable the method in the provider settings or choose another --auto-merge-method",
		}
	case release.MergeReasonProvider:
		return diagnostic{message: "provider refused the merge", hint: inspectRequest}
	case release.MergeReasonUnknown:
		return diagnostic{message: "merge readiness is unknown", hint: inspectRequest}
	default:
		return diagnostic{message: "merge readiness is unknown", hint: inspectRequest}
	}
}
