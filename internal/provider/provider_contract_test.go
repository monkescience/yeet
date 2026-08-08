package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
)

type providerContractProviderFactory func(
	t *testing.T,
	server *httptest.Server,
	options ...provider.MergePollingOption,
) provider.Provider

type providerContractLabelHandlerFactory func(
	t *testing.T,
	store *providerContractLabelStore,
	registry providerContractLabelRegistry,
) http.Handler

// providerContractLabelRegistry describes how a forge's label definition lookup
// answers for a given name, so one handler covers a forge that has the label,
// one that does not, and one that cannot say.
type providerContractLabelRegistry struct {
	undefined   []string
	unreachable []string
}

// status reports the failure a definition lookup answers with, and whether it
// fails at all.
func (r providerContractLabelRegistry) status(name string) (int, bool) {
	switch {
	case slices.Contains(r.undefined, name):
		return http.StatusNotFound, true
	case slices.Contains(r.unreachable, name):
		return http.StatusInternalServerError, true
	default:
		return 0, false
	}
}

type providerContractHarness struct {
	name                      string
	newProvider               providerContractProviderFactory
	handler                   func(t *testing.T, scenario providerContractScenario) http.Handler
	labelHandler              providerContractLabelHandlerFactory
	expectedRepoURL           func(serverURL string) string
	expectedReleasePRURL      func(serverURL string) string
	expectedReleaseURL        func(serverURL string) string
	expectedReviewerError     string
	expectedPathPrefix        string
	rejectsUnknownExtraLabels bool
}

// providerContractLabelStore is the label set a contract handler has been told
// to put on a pull request, so a scenario can assert what a phase leaves behind
// rather than the requests it took to get there.
type providerContractLabelStore struct {
	ids    map[string]string
	labels []string
	next   int
	mu     sync.Mutex
}

func newProviderContractLabelStore() *providerContractLabelStore {
	return &providerContractLabelStore{ids: make(map[string]string)}
}

func (s *providerContractLabelStore) attach(names ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, name := range names {
		if s.indexOf(name) >= 0 {
			continue
		}

		s.next++
		s.ids[strings.ToLower(name)] = fmt.Sprintf("00000000-0000-0000-0000-%012d", s.next)
		s.labels = append(s.labels, name)
	}
}

func (s *providerContractLabelStore) detach(names ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, name := range names {
		if idx := s.indexOf(name); idx >= 0 {
			s.labels = slices.Delete(s.labels, idx, idx+1)
		}
	}
}

func (s *providerContractLabelStore) detachID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name, candidate := range s.ids {
		if candidate == id {
			if idx := s.indexOf(name); idx >= 0 {
				s.labels = slices.Delete(s.labels, idx, idx+1)
			}

			return
		}
	}
}

func (s *providerContractLabelStore) definitions() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	definitions := make([]map[string]any, 0, len(s.labels))
	for _, name := range s.labels {
		definitions = append(definitions, map[string]any{"id": s.ids[strings.ToLower(name)], "name": name})
	}

	return definitions
}

func (s *providerContractLabelStore) id(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.ids[strings.ToLower(name)]
}

func (s *providerContractLabelStore) sorted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := slices.Clone(s.labels)
	slices.Sort(names)

	return names
}

func (s *providerContractLabelStore) indexOf(name string) int {
	return slices.IndexFunc(s.labels, func(existing string) bool { return strings.EqualFold(existing, name) })
}

func splitProviderContractLabels(joined string) []string {
	if joined == "" {
		return nil
	}

	return strings.Split(joined, ",")
}

type providerContractScenario string

func defaultReleasePRLabels() provider.ReleasePRLabels {
	return provider.ReleasePRLabels{
		Pending: testReleaseLabelPending,
		Tagged:  testReleaseLabelTagged,
		Yeet:    true,
	}
}

func providerContractManagedLabels() provider.ReleasePRLabels {
	return provider.ReleasePRLabels{
		Pending: providerContractPendingLabel,
		Tagged:  providerContractTaggedLabel,
		Yeet:    true,
		Extra:   []string{"release", "automated"},
	}
}

func providerContractPagedTagRefs() []provider.TagRef {
	return []provider.TagRef{
		{Name: providerContractTag, CommitSHA: providerContractTagCommitSHA},
		{Name: providerContractPreviousTag, CommitSHA: providerContractPreviousTagCommitSHA},
		{Name: providerContractOlderTag, CommitSHA: providerContractOlderTagCommitSHA},
		{Name: providerContractOldestTag, CommitSHA: providerContractOldestTagCommitSHA},
	}
}

const (
	providerContractListTags                 providerContractScenario = "list tags"
	providerContractListTagsPaged            providerContractScenario = "list tags paged"
	providerContractBranchHead               providerContractScenario = "branch head"
	providerContractBranchHeadMissing        providerContractScenario = "branch head missing"
	providerContractGetReleaseByTag          providerContractScenario = "get release by tag"
	providerContractCreateReleasePR          providerContractScenario = "create release pr"
	providerContractCreateReleasePRReviewers providerContractScenario = "create release pr reviewers"
	providerContractUnknownReviewer          providerContractScenario = "unknown reviewer"
	providerContractUpdateReleasePR          providerContractScenario = "update release pr"
	providerContractFindOpenPRs              providerContractScenario = "find open prs"
	providerContractFindOpenPRsUnlabeled     providerContractScenario = "find open prs unlabeled"
	providerContractFindOpenPRsAdoptable     providerContractScenario = "find open prs adoptable"
	providerContractFindMergedPR             providerContractScenario = "find merged pr"
	providerContractMergeReleasePR           providerContractScenario = "merge release pr"
	providerContractAsyncMergeReleasePR      providerContractScenario = "async merge release pr"
	providerContractCreateBranch             providerContractScenario = "create branch"
	providerContractCreateRelease            providerContractScenario = "create release"
	providerContractGetFile                  providerContractScenario = "get file"
	providerContractUpdateFiles              providerContractScenario = "update files"
	providerContractMissingFile              providerContractScenario = "missing file"
	providerContractMissingRelease           providerContractScenario = "missing release"
	providerContractMissingPR                providerContractScenario = "missing pr"
	providerContractBlockedMerge             providerContractScenario = "blocked merge"
	providerContractUnsupportedMerge         providerContractScenario = "unsupported merge"
	providerContractTagPaginationLimit       providerContractScenario = "tag pagination limit"
	providerContractForcedMergeUntrusted     providerContractScenario = "forced merge untrusted"
	providerContractForcedMergeConflicted    providerContractScenario = "forced merge conflicted"
	providerContractMissingExtraLabelName                             = "missing"
	providerContractUnreachableLabelName                              = "flaky"
	providerContractReleaseTitle                                      = "chore: release v1.2.3"
	providerContractReleaseBody                                       = "release body"
	providerContractUpdatedReleaseBody                                = "updated release body"
	providerContractReleaseNotes                                      = "release notes"
	providerContractChangelogContent                                  = "# Changelog\n"
	providerContractReleaseBranch                                     = "release-main"
	providerContractPendingBranch                                     = "yeet/release-main"
	providerContractLookalikeBranch                                   = "yeet/release-main-attacker"
	providerContractFeatureBranch                                     = "feature/work"
	providerContractPendingLabel                                      = "release: waiting"
	providerContractTaggedLabel                                       = "release: complete"
	providerContractBaseBranch                                        = "main"
	providerContractTag                                               = "v1.2.3"
	providerContractTagCommitSHA                                      = "tag-commit-123"
	providerContractPreviousTag                                       = "v1.2.2"
	providerContractPreviousTagCommitSHA                              = "tag-commit-122"
	providerContractOlderTag                                          = "v1.1.0"
	providerContractOlderTagCommitSHA                                 = "tag-commit-110"
	providerContractOldestTag                                         = "v1.0.0"
	providerContractOldestTagCommitSHA                                = "tag-commit-100"
	providerContractHeadSHA                                           = "head-sha"
	providerContractMergeSHA                                          = "merge-sha"
	providerContractReviewerAlice                                     = "alice"
	providerContractReviewerBob                                       = "bob"
	providerContractUnknownReviewerName                               = "ghost"
	providerContractPRNumber                                          = 42
	providerContractForgedPRNumber                                    = 66
	providerContractLookalikePRNumber                                 = 67
	providerContractFeaturePRNumber                                   = 7
	providerContractForgedTitle                                       = "forged release"
	providerContractLookalikeTitle                                    = "lookalike release branch"
	providerContractFeatureTitle                                      = "feature work"
	providerContractUntrustedBody                                     = "untrusted body"
	providerContractReleasePRURL                                      = "https://example.com/pulls/42"
	providerContractReleaseURL                                        = "https://example.com/releases/v1.2.3"
)

func TestProviderContract(t *testing.T) {
	t.Parallel()

	for _, harness := range providerContractHarnesses() {
		t.Run(harness.name, func(t *testing.T) {
			t.Parallel()

			// given: the current provider harness defining server fixtures and provider construction
			// when: each contract scenario subtest exercises a provider method
			// then: every scenario satisfies the shared provider contract for this harness

			t.Run("exposes repository metadata", func(t *testing.T) {
				t.Parallel()

				// given: a provider server for the current harness
				server := httptest.NewServer(harness.handler(t, providerContractListTags))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: the repository URL and path prefix are read from the provider
				// then: the values match the harness expectations
				testastic.Equal(t, harness.expectedRepoURL(server.URL), p.RepoURL())
				testastic.Equal(t, harness.expectedPathPrefix, p.PathPrefix())
			})

			t.Run("lists tag commit targets", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning tags with their peeled commit targets
				server := httptest.NewServer(harness.handler(t, providerContractListTags))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: tag refs are requested
				refs, err := p.ListTagRefs(context.Background())

				// then: names and commit targets are preserved in provider order
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, refs[0].Name)
				testastic.Equal(t, providerContractTagCommitSHA, refs[0].CommitSHA)
			})

			t.Run("lists every tag across every page", func(t *testing.T) {
				t.Parallel()

				// given: a provider server serving the tag list across two pages
				server := httptest.NewServer(harness.handler(t, providerContractListTagsPaged))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: tag refs are requested
				refs, err := p.ListTagRefs(context.Background())

				// then: both pages contribute every ref in provider order
				testastic.NoError(t, err)
				testastic.SliceEqual(t, providerContractPagedTagRefs(), refs)
			})

			t.Run("resolves branch head commit", func(t *testing.T) {
				t.Parallel()

				// given: a provider server exposing the base branch head
				server := httptest.NewServer(harness.handler(t, providerContractBranchHead))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetBranchHead is invoked for the base branch
				head, err := p.GetBranchHead(context.Background(), providerContractBaseBranch)

				// then: the branch head commit SHA is returned
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractHeadSHA, head)
			})

			t.Run("reports missing branch as ref not found", func(t *testing.T) {
				t.Parallel()

				// given: a provider server without the requested branch
				server := httptest.NewServer(harness.handler(t, providerContractBranchHeadMissing))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetBranchHead is invoked for a branch that does not exist
				_, err := p.GetBranchHead(context.Background(), "missing-branch")

				// then: the sentinel ref-not-found error is surfaced
				testastic.Error(t, err)
				testastic.True(t, errors.Is(err, provider.ErrRefNotFound))
			})

			t.Run("gets release by tag", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a release for the contract tag
				server := httptest.NewServer(harness.handler(t, providerContractGetReleaseByTag))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetReleaseByTag is invoked for the contract tag
				release, err := p.GetReleaseByTag(context.Background(), providerContractTag)

				// then: the release metadata matches the harness expectations
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, release.TagName)
				testastic.Equal(t, "release notes", release.Body)
				testastic.Equal(t, harness.expectedReleaseURL(server.URL), release.URL)
			})

			t.Run("creates release pull request", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting a new release pull request for the release branch
				server := httptest.NewServer(harness.handler(t, providerContractCreateReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateReleasePR is invoked with the contract title, body, and branches
				pr, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
					Title:         providerContractReleaseTitle,
					Body:          providerContractReleaseBody,
					BaseBranch:    providerContractBaseBranch,
					ReleaseBranch: providerContractReleaseBranch,
				})

				// then: the created pull request reflects the supplied options and the harness URL
				testastic.NoError(t, err)
				testastic.Equal(t, 42, pr.Number)
				testastic.Equal(t, providerContractReleaseTitle, pr.Title)
				testastic.Equal(t, providerContractReleaseBody, pr.Body)
				testastic.Equal(t, providerContractReleaseBranch, pr.Branch)
				testastic.Equal(t, harness.expectedReleasePRURL(server.URL), pr.URL)
			})

			t.Run("creates release pull request with reviewers", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting a new release pull request and reviewer assignment
				server := httptest.NewServer(harness.handler(t, providerContractCreateReleasePRReviewers))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateReleasePR is invoked with two reviewers
				pr, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
					Title:         providerContractReleaseTitle,
					Body:          providerContractReleaseBody,
					BaseBranch:    providerContractBaseBranch,
					ReleaseBranch: providerContractReleaseBranch,
					Reviewers:     []string{providerContractReviewerAlice, providerContractReviewerBob},
				})

				// then: the pull request is created and the reviewer requests reach the server
				testastic.NoError(t, err)
				testastic.Equal(t, 42, pr.Number)
			})

			t.Run("fails when a reviewer cannot be assigned", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that cannot resolve or assign the requested reviewer
				server := httptest.NewServer(harness.handler(t, providerContractUnknownReviewer))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateReleasePR is invoked with an unknown reviewer
				_, err := p.CreateReleasePR(context.Background(), provider.ReleasePROptions{
					Title:         providerContractReleaseTitle,
					Body:          providerContractReleaseBody,
					BaseBranch:    providerContractBaseBranch,
					ReleaseBranch: providerContractReleaseBranch,
					Reviewers:     []string{providerContractUnknownReviewerName},
				})

				// then: the error names the reviewer and wraps the not-found sentinel
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrReviewerNotFound)
				testastic.Equal(t, harness.expectedReviewerError, err.Error())
			})

			t.Run("updates release pull request", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting updates to an existing release pull request
				server := httptest.NewServer(harness.handler(t, providerContractUpdateReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: UpdateReleasePR is invoked with a new body and reviewers for PR 42
				err := p.UpdateReleasePR(context.Background(), 42, provider.ReleasePROptions{
					Title:     providerContractReleaseTitle,
					Body:      "updated release body",
					Reviewers: []string{providerContractReviewerAlice},
				})

				// then: the update completes without touching reviewers
				testastic.NoError(t, err)
			})

			t.Run("finds open pending release pull requests", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a single open pending release PR targeting the base branch
				server := httptest.NewServer(harness.handler(t, providerContractFindOpenPRs))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: FindOpenPendingReleasePRs is invoked for the base branch
				prs, err := p.FindOpenPendingReleasePRs(
					context.Background(),
					providerContractBaseBranch,
					providerContractPendingLabel,
				)

				// then: a single PR is returned with the expected number and pending branch
				testastic.NoError(t, err)
				testastic.Equal(t, 1, len(prs))
				testastic.Equal(t, 42, prs[0].Number)
				testastic.Equal(t, providerContractPendingBranch, prs[0].Branch)
			})

			t.Run("rejects a trusted release pull request labelled with a different lifecycle label", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a trusted release PR carrying a stale lifecycle label
				server := httptest.NewServer(harness.handler(t, providerContractFindOpenPRsUnlabeled))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: FindOpenPendingReleasePRs is invoked for the base branch
				prs, err := p.FindOpenPendingReleasePRs(
					context.Background(),
					providerContractBaseBranch,
					providerContractPendingLabel,
				)

				// then: the run aborts with the lifecycle mismatch sentinel instead of skipping the PR
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrReleasePRLabelMismatch)
				testastic.Equal(t, 0, len(prs))
			})

			t.Run("adopts a trusted release pull request that carries no labels at all", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a trusted release PR left unlabelled by an interrupted run
				server := httptest.NewServer(harness.handler(t, providerContractFindOpenPRsAdoptable))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: FindOpenPendingReleasePRs is invoked for the base branch
				prs, err := p.FindOpenPendingReleasePRs(
					context.Background(),
					providerContractBaseBranch,
					providerContractPendingLabel,
				)

				// then: the PR is returned for adoption rather than aborting the run
				testastic.NoError(t, err)
				testastic.Equal(t, 1, len(prs))
				testastic.Equal(t, 42, prs[0].Number)
				testastic.True(t, prs[0].NeedsPendingLabel)
			})

			t.Run("finds merged release pull request", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a recently merged release PR for the base branch
				server := httptest.NewServer(harness.handler(t, providerContractFindMergedPR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: FindMergedReleasePR is invoked for the base branch
				pr, err := p.FindMergedReleasePR(
					context.Background(),
					providerContractBaseBranch,
					providerContractPendingLabel,
				)

				// then: the merged PR is returned with the expected number, branch, merge SHA, and body
				testastic.NoError(t, err)
				testastic.Equal(t, 42, pr.Number)
				testastic.Equal(t, providerContractPendingBranch, pr.Branch)
				testastic.Equal(t, providerContractMergeSHA, pr.MergeCommitSHA)
				testastic.Equal(t, providerContractReleaseBody, pr.Body)
			})

			t.Run("carries the managed label set of the phase it was put in", func(t *testing.T) {
				t.Parallel()

				// given: a provider server tracking the labels on PR 42
				store := newProviderContractLabelStore()

				server := httptest.NewServer(harness.labelHandler(t, store, providerContractLabelRegistry{}))
				defer server.Close()

				p := harness.newProvider(t, server)

				labels := providerContractManagedLabels()

				// when: the pull request is put in the pending phase
				err := p.SetReleasePRLabels(context.Background(), 42, labels, provider.ReleasePRPhasePending)

				// then: it carries the pending label, the extras and the yeet marker, and not tagged
				testastic.NoError(t, err)
				testastic.SliceEqual(t, []string{
					"automated",
					"release",
					providerContractPendingLabel,
					provider.ReleaseLabelYeet,
				}, store.sorted())

				// when: the pull request is put in the tagged phase
				err = p.SetReleasePRLabels(context.Background(), 42, labels, provider.ReleasePRPhaseTagged)

				// then: the lifecycle label flips and everything else stays put
				testastic.NoError(t, err)
				testastic.SliceEqual(t, []string{
					"automated",
					"release",
					providerContractTaggedLabel,
					provider.ReleaseLabelYeet,
				}, store.sorted())
			})

			t.Run("merges release pull request", func(t *testing.T) {
				t.Parallel()

				// given: a provider server reporting PR 42 as ready to merge
				server := httptest.NewServer(harness.handler(t, providerContractMergeReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: MergeReleasePR is invoked with the auto merge method on PR 42
				mergeSHA, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
					Method: provider.MergeMethodAuto,
				})

				// then: the merge completes and returns the merge commit
				testastic.NoError(t, err)
				testastic.Equal(t, "merge-sha", mergeSHA)
			})

			t.Run("finalizes an asynchronous merge", func(t *testing.T) {
				t.Parallel()

				// given: a provider accepting the merge before the commit is applied
				server := httptest.NewServer(harness.handler(t, providerContractAsyncMergeReleasePR))
				defer server.Close()

				p := harness.newProvider(t, server, provider.WithMergePolling(time.Millisecond, 5*time.Second))

				// when: MergeReleasePR is invoked with the auto merge method on PR 42
				mergeSHA, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
					Method: provider.MergeMethodAuto,
				})

				// then: no provisional commit is returned before the merge is applied
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractMergeSHA, mergeSHA)
			})

			t.Run("creates branch", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting a new branch off the base branch
				server := httptest.NewServer(harness.handler(t, providerContractCreateBranch))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateBranch is invoked for the release branch with the base branch as source
				err := p.CreateBranch(context.Background(), providerContractReleaseBranch, providerContractBaseBranch)

				// then: the branch is created without error
				testastic.NoError(t, err)
			})

			t.Run("creates release", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting a new prerelease against the base branch
				server := httptest.NewServer(harness.handler(t, providerContractCreateRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: CreateRelease is invoked with the contract tag and release notes
				release, err := p.CreateRelease(context.Background(), provider.ReleaseOptions{
					TagName:    providerContractTag,
					Ref:        providerContractBaseBranch,
					Name:       providerContractTag,
					Body:       "release notes",
					Prerelease: true,
				})

				// then: the returned release matches the requested tag, body, and harness URL
				testastic.NoError(t, err)
				testastic.Equal(t, providerContractTag, release.TagName)
				testastic.Equal(t, "release notes", release.Body)
				testastic.Equal(t, harness.expectedReleaseURL(server.URL), release.URL)
			})

			t.Run("reads file content", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a CHANGELOG.md file on the base branch
				server := httptest.NewServer(harness.handler(t, providerContractGetFile))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetFile is invoked for CHANGELOG.md on the base branch
				content, err := p.GetFile(context.Background(), providerContractBaseBranch, "CHANGELOG.md")

				// then: the file content matches the fixture
				testastic.NoError(t, err)
				testastic.Equal(t, "# Changelog\n", content)
			})

			t.Run("updates release files", func(t *testing.T) {
				t.Parallel()

				// given: a provider server accepting file updates on the release branch
				server := httptest.NewServer(harness.handler(t, providerContractUpdateFiles))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: UpdateFiles is invoked with CHANGELOG.md and VERSION.txt against the release branch
				err := p.UpdateFiles(
					context.Background(),
					providerContractReleaseBranch,
					providerContractBaseBranch,
					map[string]provider.FileUpdate{
						"CHANGELOG.md": {Content: "# Changelog\n", Exists: true},
						"VERSION.txt":  {Content: "version=1.2.3\n"},
					},
					"chore: release v1.2.3",
				)

				// then: the file updates are committed without error
				testastic.NoError(t, err)
			})

			t.Run("returns file not found error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that reports MISSING.md as not found on the base branch
				server := httptest.NewServer(harness.handler(t, providerContractMissingFile))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetFile is invoked for MISSING.md on the base branch
				_, err := p.GetFile(context.Background(), providerContractBaseBranch, "MISSING.md")

				// then: ErrFileNotFound is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrFileNotFound)
			})

			t.Run("returns release not found error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that reports no release for the contract tag
				server := httptest.NewServer(harness.handler(t, providerContractMissingRelease))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: GetReleaseByTag is invoked for the contract tag
				_, err := p.GetReleaseByTag(context.Background(), providerContractTag)

				// then: ErrNoRelease is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrNoRelease)
			})

			t.Run("returns release pull request not found error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that reports no merged release PR on the base branch
				server := httptest.NewServer(harness.handler(t, providerContractMissingPR))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: FindMergedReleasePR is invoked for the base branch
				_, err := p.FindMergedReleasePR(
					context.Background(),
					providerContractBaseBranch,
					providerContractPendingLabel,
				)

				// then: ErrNoPR is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrNoPR)
			})

			t.Run("returns blocked merge error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server reporting PR 42 as not ready to merge
				server := httptest.NewServer(harness.handler(t, providerContractBlockedMerge))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: MergeReleasePR is invoked without the force option on PR 42
				_, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{})

				// then: ErrMergeBlocked is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
			})

			t.Run("returns unsupported merge method error", func(t *testing.T) {
				t.Parallel()

				// given: a provider server prepared for a merge attempt with an unsupported method
				server := httptest.NewServer(harness.handler(t, providerContractUnsupportedMerge))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: MergeReleasePR is invoked with the unsupported "octopus" merge method
				_, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
					Method: provider.MergeMethod("octopus"),
				})

				// then: ErrMergeMethodUnsupported is returned
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrMergeMethodUnsupported)
			})

			t.Run("resolves a configured extra label the forge does not define", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that does not define the configured extra label
				store := newProviderContractLabelStore()

				server := httptest.NewServer(harness.labelHandler(
					t,
					store,
					providerContractLabelRegistry{undefined: []string{providerContractMissingExtraLabelName}},
				))
				defer server.Close()

				p := harness.newProvider(t, server)

				labels := provider.ReleasePRLabels{
					Pending: providerContractPendingLabel,
					Tagged:  providerContractTaggedLabel,
					Extra:   []string{providerContractMissingExtraLabelName},
				}

				// when: the pull request is put in the pending phase
				err := p.SetReleasePRLabels(context.Background(), 42, labels, provider.ReleasePRPhasePending)

				// then: a forge that exposes label definitions rejects the unknown label
				// and mutates nothing, and one that creates labels on attach carries it
				if harness.rejectsUnknownExtraLabels {
					testastic.Error(t, err)
					testastic.ErrorIs(t, err, provider.ErrReleasePRLabelMissing)
					testastic.Equal(t, 0, len(store.sorted()))

					return
				}

				testastic.NoError(t, err)
				testastic.SliceEqual(t, []string{
					providerContractMissingExtraLabelName,
					providerContractPendingLabel,
				}, store.sorted())
			})

			t.Run("tags without validating creation-only extras", func(t *testing.T) {
				t.Parallel()

				store := newProviderContractLabelStore()
				store.attach(
					providerContractPendingLabel,
					provider.ReleaseLabelYeet,
					providerContractMissingExtraLabelName,
				)

				server := httptest.NewServer(harness.labelHandler(
					t,
					store,
					providerContractLabelRegistry{undefined: []string{providerContractMissingExtraLabelName}},
				))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.SetReleasePRLabels(context.Background(), 42, provider.ReleasePRLabels{
					Pending: providerContractPendingLabel,
					Tagged:  providerContractTaggedLabel,
					Yeet:    true,
					Extra:   []string{providerContractMissingExtraLabelName},
				}, provider.ReleasePRPhaseTagged)

				testastic.NoError(t, err)
				testastic.SliceEqual(t, []string{
					providerContractMissingExtraLabelName,
					providerContractTaggedLabel,
					provider.ReleaseLabelYeet,
				}, store.sorted())
			})

			t.Run("preflights tagged label existence without mutation", func(t *testing.T) {
				t.Parallel()

				store := newProviderContractLabelStore()

				server := httptest.NewServer(harness.labelHandler(t, store, providerContractLabelRegistry{}))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.PreflightReleasePRTagging(
					context.Background(),
					providerContractTaggedLabel,
				)

				testastic.NoError(t, err)
				testastic.Equal(t, 0, len(store.sorted()))
			})

			t.Run("preflight rejects a missing tagged label when definitions are available", func(t *testing.T) {
				t.Parallel()

				store := newProviderContractLabelStore()

				server := httptest.NewServer(harness.labelHandler(
					t,
					store,
					providerContractLabelRegistry{undefined: []string{providerContractTaggedLabel}},
				))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.PreflightReleasePRTagging(
					context.Background(),
					providerContractTaggedLabel,
				)

				if harness.rejectsUnknownExtraLabels {
					testastic.Error(t, err)
					testastic.ErrorIs(t, err, provider.ErrReleasePRLabelMissing)

					if err != nil {
						testastic.Equal(
							t,
							`release PR label does not exist: tagged label "`+providerContractTaggedLabel+`"`,
							err.Error(),
						)
					}
				} else {
					testastic.NoError(t, err)
				}

				testastic.Equal(t, 0, len(store.sorted()))
			})

			t.Run("preflight separates an unreachable tagged label from a missing one", func(t *testing.T) {
				t.Parallel()

				store := newProviderContractLabelStore()

				server := httptest.NewServer(harness.labelHandler(
					t,
					store,
					providerContractLabelRegistry{unreachable: []string{providerContractTaggedLabel}},
				))
				defer server.Close()

				p := harness.newProvider(t, server)

				err := p.PreflightReleasePRTagging(
					context.Background(),
					providerContractTaggedLabel,
				)

				if harness.rejectsUnknownExtraLabels {
					testastic.Error(t, err)
					testastic.False(t, errors.Is(err, provider.ErrReleasePRLabelMissing))
					testastic.ErrorContains(t, err, `get label "`+providerContractTaggedLabel+`"`)
				} else {
					testastic.NoError(t, err)
				}

				testastic.Equal(t, 0, len(store.sorted()))
			})

			t.Run("separates an unreachable extra label from a missing one", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that fails the extra label lookup
				store := newProviderContractLabelStore()

				server := httptest.NewServer(harness.labelHandler(
					t,
					store,
					providerContractLabelRegistry{unreachable: []string{providerContractUnreachableLabelName}},
				))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: the pending phase is applied with an extra label the forge cannot report on
				err := p.SetReleasePRLabels(context.Background(), 42, provider.ReleasePRLabels{
					Pending: providerContractPendingLabel,
					Tagged:  providerContractTaggedLabel,
					Extra:   []string{providerContractUnreachableLabelName},
				}, provider.ReleasePRPhasePending)

				// then: an unreachable label is never reported as one the operator must create
				if !harness.rejectsUnknownExtraLabels {
					testastic.NoError(t, err)

					return
				}

				testastic.Error(t, err)
				testastic.False(t, errors.Is(err, provider.ErrReleasePRLabelMissing))
				testastic.ErrorContains(t, err, `get label "`+providerContractUnreachableLabelName+`"`)
			})

			t.Run("stops listing tags at the pagination limit", func(t *testing.T) {
				t.Parallel()

				// given: a provider server that always announces another page of tags
				server := httptest.NewServer(harness.handler(t, providerContractTagPaginationLimit))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: tag refs are listed from an effectively infinite repository
				_, err := p.ListTagRefs(context.Background())

				// then: the pagination limit is enforced instead of looping forever
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrPaginationLimitExceeded)
			})

			t.Run("refuses an untrusted release pull request even when merge checks are bypassed", func(t *testing.T) {
				t.Parallel()

				// given: a provider server returning a PR on the release branch from another repository
				server := httptest.NewServer(harness.handler(t, providerContractForcedMergeUntrusted))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: MergeReleasePR is invoked with merge checks bypassed on PR 42
				mergeSHA, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
					BypassMergeChecks: true,
				})

				// then: the trust check refuses the merge and no commit is reported
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrUntrustedReleasePR)
				testastic.Equal(t, "", mergeSHA)
			})

			t.Run("refuses a conflicted release pull request even when merge checks are bypassed", func(t *testing.T) {
				t.Parallel()

				// given: a provider server reporting PR 42 as conflicted, whose handler
				// fails the test if a merge is attempted
				server := httptest.NewServer(harness.handler(t, providerContractForcedMergeConflicted))
				defer server.Close()

				p := harness.newProvider(t, server)

				// when: MergeReleasePR is invoked with merge checks bypassed on PR 42
				mergeSHA, err := p.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
					BypassMergeChecks: true,
				})

				// then: bypassing policy never bypasses conflicts and no commit is reported
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
				testastic.Equal(t, "", mergeSHA)
			})
		})
	}
}

func providerContractHarnesses() []providerContractHarness {
	return []providerContractHarness{
		{
			name:                      "github",
			newProvider:               newGitHubContractProvider,
			handler:                   newGitHubContractHandler,
			labelHandler:              newGitHubContractLabelHandler,
			expectedRepoURL:           func(serverURL string) string { return serverURL + "/o/r" },
			expectedReleasePRURL:      func(_ string) string { return providerContractReleasePRURL },
			expectedReleaseURL:        func(_ string) string { return providerContractReleaseURL },
			expectedReviewerError:     `reviewer not found: "ghost" is not a repository collaborator`,
			expectedPathPrefix:        "",
			rejectsUnknownExtraLabels: true,
		},
		{
			name:                      "gitlab",
			newProvider:               newGitLabContractProvider,
			handler:                   newGitLabContractHandler,
			labelHandler:              newGitLabContractLabelHandler,
			expectedRepoURL:           func(serverURL string) string { return serverURL + "/o/r" },
			expectedReleasePRURL:      func(_ string) string { return providerContractReleasePRURL },
			expectedReleaseURL:        func(_ string) string { return providerContractReleaseURL },
			expectedReviewerError:     `reviewer not found: "ghost" is not a project member`,
			expectedPathPrefix:        "/-",
			rejectsUnknownExtraLabels: true,
		},
		{
			name:                 "azuredevops",
			newProvider:          newAzureDevOpsContractProvider,
			handler:              newAzureDevOpsContractHandler,
			labelHandler:         newAzureDevOpsContractLabelHandler,
			expectedRepoURL:      azureDevOpsContractExpectedRepoURL,
			expectedReleasePRURL: func(s string) string { return azureDevOpsContractExpectedRepoURL(s) + "/pullrequest/42" },
			expectedReleaseURL: func(s string) string {
				return azureDevOpsContractExpectedRepoURL(s) + "?version=GT" + providerContractTag
			},
			expectedReviewerError: `reviewer not found: "ghost"`,
			expectedPathPrefix:    "",
		},
	}
}

func decodeJSONRequest(t *testing.T, r *http.Request, value any) {
	t.Helper()

	err := json.NewDecoder(r.Body).Decode(value)
	testastic.NoError(t, err)
}

func writeJSONFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	writeFixture(t, w, name)
}

func writeTextFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writeFixture(t, w, name)
}

func writeFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	testastic.NoError(t, err)

	_, err = w.Write(data)
	testastic.NoError(t, err)
}

func fatalUnexpectedProviderRequest(t *testing.T, providerName string, r *http.Request) {
	t.Helper()

	t.Fatalf("unexpected %s request: %s %s", providerName, r.Method, r.URL.String())
}
