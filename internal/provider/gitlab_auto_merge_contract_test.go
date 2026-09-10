package provider_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
)

const gitLabAutoMergeFixtureRoot = "contracts/gitlab/auto_merge/"

func TestGitLabEnsureAutoMerge(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run("rejects null response from "+method, func(t *testing.T) {
			t.Parallel()

			// given: GitLab returns null when fetching or accepting the merge request
			next := gitLabOrdinaryAutoMergeHandler(t, gitLabAutoMergeFixtureRoot+"accept_scheduled.json")
			server := newGitLabAutoMergeServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == method && strings.Contains(r.URL.Path, "/merge_requests/42") {
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"null.json")

					return
				}

				next(w, r)
			})
			p := newGitLabContractProvider(t, server)

			// when: requesting provider-managed auto-merge
			err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

			// then: the missing response is reported without panicking
			testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
			testastic.ErrorContains(t, err, "returned no merge request")
		})
	}

	for _, projectFixture := range []string{"project.json", "project_free.json"} {
		t.Run("schedules without trains using "+projectFixture, func(t *testing.T) {
			t.Parallel()

			// given: an open trusted merge request and a project without merge trains
			server := newGitLabAutoMergeServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_open.json")
				case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/version"):
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"version.json")
				case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr"):
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+projectFixture)
				case isGitLabAutoMergeRequest(r, http.MethodPut, "/api/v4/projects/o%2Fr/merge_requests/42/merge"):
					assertGitLabAutoMergeRequest(t, r, true, gitLabSourceTipSHA)
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"accept_scheduled.json")
				default:
					fatalUnexpectedProviderRequest(t, "GitLab", r)
				}
			})

			p := newGitLabContractProvider(t, server)

			// when: provider-managed auto-merge is requested
			err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

			// then: the provider's legacy response field confirms scheduling
			testastic.NoError(t, err)
		})
	}

	t.Run("rejects a successful response that does not confirm scheduling", func(t *testing.T) {
		t.Parallel()

		// given: GitLab returns HTTP 200 without merging or enabling auto-merge
		server := newGitLabAutoMergeServer(t, gitLabOrdinaryAutoMergeHandler(
			t,
			gitLabAutoMergeFixtureRoot+"accept_unconfirmed.json",
		))
		p := newGitLabContractProvider(t, server)

		// when: provider-managed auto-merge is requested
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

		// then: the unconfirmed response is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
		testastic.ErrorContains(t, err, "merge request !42")
		assertGitLabAutoMergeFailureReason(t, err)
	})

	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name:    "merge error response",
			handler: gitLabOrdinaryAutoMergeHandler(t, gitLabAutoMergeFixtureRoot+"accept_refused.json"),
		},
		{
			name: "method not allowed response",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch {
				case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_open.json")
				case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/version"):
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"version.json")
				case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr"):
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"project.json")
				case isGitLabAutoMergeRequest(r, http.MethodPut, "/api/v4/projects/o%2Fr/merge_requests/42/merge"):
					http.Error(w, "merge request cannot be merged", http.StatusMethodNotAllowed)
				default:
					fatalUnexpectedProviderRequest(t, "GitLab", r)
				}
			},
		},
	} {
		t.Run("uses failure reason for "+tc.name, func(t *testing.T) {
			t.Parallel()

			// given: GitLab refuses the native auto-merge request
			server := newGitLabAutoMergeServer(t, tc.handler)
			p := newGitLabContractProvider(t, server)

			// when: provider-managed auto-merge is requested
			err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

			// then: the refusal distinguishes a provider failure from unknown readiness
			assertGitLabAutoMergeFailureReason(t, err)
		})
	}

	t.Run("accepts an immediate merge without publishing from the adapter", func(t *testing.T) {
		t.Parallel()

		// given: GitLab reports that the merge request merged immediately
		server := newGitLabAutoMergeServer(t, gitLabOrdinaryAutoMergeHandler(
			t,
			gitLabAutoMergeFixtureRoot+"accept_merged.json",
		))
		p := newGitLabContractProvider(t, server)

		// when: provider-managed auto-merge is requested
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

		// then: immediate provider completion is an accepted scheduling outcome
		testastic.NoError(t, err)
	})

	for _, status := range []int{http.StatusCreated, http.StatusAccepted} {
		t.Run("accepts merge train status "+http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			// given: a project with merge trains and no existing train entry
			server := newGitLabAutoMergeServer(t, gitLabMergeTrainHandler(t, status))
			p := newGitLabContractProvider(t, server)

			// when: provider-managed auto-merge is requested
			err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

			// then: GitLab's created and scheduled statuses are accepted
			testastic.NoError(t, err)
		})
	}

	t.Run("treats an existing merge train entry as idempotent", func(t *testing.T) {
		t.Parallel()

		// given: the merge request already has an active merge train entry
		var posts atomic.Int32

		server := newGitLabAutoMergeServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_open.json")
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/version"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"version.json")
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"project_train.json")
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_trains/merge_requests/42"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"train_entry.json")
			case r.Method == http.MethodPost:
				posts.Add(1)
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		})
		p := newGitLabContractProvider(t, server)

		// when: provider-managed auto-merge is requested again
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

		// then: the existing entry is accepted without duplicate enrollment
		testastic.NoError(t, err)
		testastic.Equal(t, int32(0), posts.Load())
	})

	t.Run("accepts the GitLab edition suffix at the minimum version", func(t *testing.T) {
		t.Parallel()

		// given: GitLab reports the minimum supported version with an edition suffix
		server := newGitLabAutoMergeServer(t, gitLabOrdinaryAutoMergeHandler(
			t,
			gitLabAutoMergeFixtureRoot+"accept_scheduled.json",
		))
		p := newGitLabContractProvider(t, server)

		// when: provider-managed auto-merge is requested
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

		// then: the edition suffix does not make the release look like a prerelease
		testastic.NoError(t, err)
	})

	t.Run("rejects GitLab before version 17.11 without mutation", func(t *testing.T) {
		t.Parallel()

		// given: GitLab reports a version before native auto-merge support
		var mutations atomic.Int32

		server := newGitLabAutoMergeServer(t, gitLabAutoMergeVersionHandler(
			t,
			gitLabAutoMergeFixtureRoot+"version_unsupported.json",
			http.StatusOK,
			&mutations,
		))
		p := newGitLabContractProvider(t, server)

		// when: provider-managed auto-merge is requested
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

		// then: the capability error is reported before any provider mutation
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrAutoMergeUnsupported)
		testastic.Equal(t, int32(0), mutations.Load())
	})

	for _, tc := range []struct {
		name    string
		fixture string
		status  int
	}{
		{name: "missing version", fixture: gitLabAutoMergeFixtureRoot + "version_missing.json", status: http.StatusOK},
		{
			name:    "version endpoint error",
			fixture: gitLabAutoMergeFixtureRoot + "version_error.json",
			status:  http.StatusInternalServerError,
		},
	} {
		t.Run(tc.name+" prevents mutation", func(t *testing.T) {
			t.Parallel()

			// given: GitLab cannot provide a usable server version
			var mutations atomic.Int32

			server := newGitLabAutoMergeServer(t, gitLabAutoMergeVersionHandler(
				t,
				tc.fixture,
				tc.status,
				&mutations,
			))
			p := newGitLabContractProvider(t, server)

			// when: provider-managed auto-merge is requested
			err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

			// then: version discovery fails before any provider mutation
			testastic.Error(t, err)
			testastic.Equal(t, int32(0), mutations.Load())
		})
	}

	t.Run("rejects explicit squash when the project disables it", func(t *testing.T) {
		t.Parallel()

		// given: an open merge request in a project that forbids squashing
		server := newGitLabAutoMergeServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_open.json")
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/version"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"version.json")
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"project_squash_never.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		})
		p := newGitLabContractProvider(t, server)

		// when: squash is explicitly requested
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodSquash))

		// then: the project conflict is reported as a method block
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
		testastic.ErrorContains(t, err, "merge method \"squash\" disabled by project squash_option=never")
	})

	t.Run("rejects an explicit method that conflicts with existing scheduling", func(t *testing.T) {
		t.Parallel()

		// given: an existing scheduled squash merge in a merge-commit project
		server := newGitLabAutoMergeServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_existing_squash.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		})
		p := newGitLabContractProvider(t, server)

		// when: merge commits are explicitly requested
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodMerge))

		// then: the existing squash option is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
		testastic.ErrorContains(t, err, "existing auto-merge options conflict with merge method \"merge\"")
	})

	t.Run("accepts existing scheduling without capability lookups", func(t *testing.T) {
		t.Parallel()

		// given: a merge request already scheduled with its current merge method
		server := newGitLabAutoMergeServer(t, func(w http.ResponseWriter, r *http.Request) {
			if isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42") {
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_existing_squash.json")

				return
			}

			fatalUnexpectedProviderRequest(t, "GitLab", r)
		})
		p := newGitLabContractProvider(t, server)

		// when: automatic provider-managed merging is requested again
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

		// then: existing scheduling succeeds without version or project discovery
		testastic.NoError(t, err)
	})

	for _, tc := range []struct {
		name       string
		project    string
		wantErr    error
		wantDetail string
	}{
		{
			name:       "rejects an incompatible existing rebase strategy",
			project:    "project.json",
			wantErr:    forge.ErrMergeBlocked,
			wantDetail: "merge method \"rebase\" incompatible with project merge_method=merge",
		},
		{
			name:    "accepts a compatible existing rebase strategy",
			project: "project_rebase.json",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: an unsquashed merge request already scheduled by GitLab
			server := newGitLabAutoMergeServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_existing_merge.json")
				case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr"):
					writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+tc.project)
				default:
					fatalUnexpectedProviderRequest(t, "GitLab", r)
				}
			})
			p := newGitLabContractProvider(t, server)

			// when: rebase merging is explicitly required
			err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodRebase))

			// then: project strategy compatibility controls the idempotent result
			if tc.wantErr != nil {
				testastic.ErrorIs(t, err, tc.wantErr)
				testastic.ErrorContains(t, err, tc.wantDetail)
			} else {
				testastic.NoError(t, err)
			}
		})
	}

	t.Run("rejects an untrusted source project before scheduling", func(t *testing.T) {
		t.Parallel()

		// given: a merge request whose source belongs to another project
		server := newGitLabAutoMergeServer(t, func(w http.ResponseWriter, r *http.Request) {
			if isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42") {
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_untrusted.json")

				return
			}

			fatalUnexpectedProviderRequest(t, "GitLab", r)
		})
		p := newGitLabContractProvider(t, server)

		// when: provider-managed auto-merge is requested
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

		// then: trust validation prevents a provider mutation
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrUntrustedReleasePR)
	})

	t.Run("rejects an empty head SHA before scheduling", func(t *testing.T) {
		t.Parallel()

		// given: a trusted merge request without a head SHA
		server := newGitLabAutoMergeServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_empty_sha.json")
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/version"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"version.json")
			case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr"):
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"project.json")
			default:
				fatalUnexpectedProviderRequest(t, "GitLab", r)
			}
		})
		p := newGitLabContractProvider(t, server)

		// when: provider-managed auto-merge is requested
		err := p.EnsureAutoMerge(t.Context(), 42, gitLabAutoMergeContractOptions(forge.MergeMethodAuto))

		// then: the missing reviewed head is rejected before the merge request mutation
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, forge.ErrEmptyCommitSHA)
	})
}

func newGitLabAutoMergeServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	return httptest.NewTestServer(t, handler)
}

func gitLabAutoMergeContractOptions(method forge.MergeMethod) forge.MergeReleasePROptions {
	return forge.MergeReleasePROptions{
		BaseBranch:    providerContractBaseBranch,
		ReleaseBranch: providerContractReleaseBranch,
		Method:        method,
	}
}

func isGitLabAutoMergeRequest(r *http.Request, method, path string) bool {
	return r.Method == method && r.URL.EscapedPath() == path
}

func gitLabOrdinaryAutoMergeHandler(t *testing.T, acceptanceFixture string) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
			writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_open.json")
		case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/version"):
			writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"version.json")
		case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr"):
			writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"project.json")
		case isGitLabAutoMergeRequest(r, http.MethodPut, "/api/v4/projects/o%2Fr/merge_requests/42/merge"):
			writeJSONFixture(t, w, acceptanceFixture)
		default:
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		}
	}
}

func gitLabMergeTrainHandler(t *testing.T, status int) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
			writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_open.json")
		case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/version"):
			writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"version.json")
		case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr"):
			writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"project_train.json")
		case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_trains/merge_requests/42"):
			w.WriteHeader(http.StatusNotFound)
			writeJSONFixture(t, w, "contracts/gitlab/_shared/not_found.json")
		case isGitLabAutoMergeRequest(r, http.MethodPost, "/api/v4/projects/o%2Fr/merge_trains/merge_requests/42"):
			assertGitLabAutoMergeRequest(t, r, true, gitLabSourceTipSHA)
			w.WriteHeader(status)

			if status == http.StatusCreated {
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"train_entries.json")
			} else {
				writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"train_entries_empty.json")
			}
		default:
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		}
	}
}

func gitLabAutoMergeVersionHandler(
	t *testing.T,
	versionFixture string,
	versionStatus int,
	mutations *atomic.Int32,
) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/projects/o%2Fr/merge_requests/42"):
			writeJSONFixture(t, w, gitLabAutoMergeFixtureRoot+"pr_open.json")
		case isGitLabAutoMergeRequest(r, http.MethodGet, "/api/v4/version"):
			w.WriteHeader(versionStatus)
			writeJSONFixture(t, w, versionFixture)
		case r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete:
			mutations.Add(1)
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		default:
			fatalUnexpectedProviderRequest(t, "GitLab", r)
		}
	}
}

func assertGitLabAutoMergeRequest(t *testing.T, r *http.Request, autoMerge bool, sha string) {
	t.Helper()

	var request struct {
		AutoMerge *bool  `json:"auto_merge"`
		SHA       string `json:"sha"`
	}
	decodeJSONRequest(t, r, &request)
	testastic.True(t, request.AutoMerge != nil)

	if request.AutoMerge != nil {
		testastic.Equal(t, autoMerge, *request.AutoMerge)
	}

	testastic.Equal(t, sha, request.SHA)
}

func assertGitLabAutoMergeFailureReason(t *testing.T, err error) {
	t.Helper()

	var blocked *forge.MergeBlockedError
	testastic.ErrorAs(t, err, &blocked)

	if blocked != nil {
		testastic.Equal(t, forge.MergeBlockedReasonFailure, blocked.Reason)
	}
}
