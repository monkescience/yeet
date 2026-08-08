package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestGitHubReleasePRStateTransitions(t *testing.T) {
	t.Parallel()

	t.Run("marks pull request pending", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub server that records label additions and deletions for PR 42
		var addLabels []string

		removedLabel := ""

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
				writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
			case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
				err := json.NewDecoder(r.Body).Decode(&addLabels)
				testastic.NoError(t, err)

				writeJSON(t, w, []map[string]any{{"name": testReleaseLabelPending}})
			case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
				removedLabel = decodedPathTail(t, r)
				http.NotFound(w, r)
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := newGitHubTestClient(t, server)

		gh := provider.NewGitHub(client, "o", "r")

		// when: MarkReleasePRPending is invoked for PR 42
		err := gh.MarkReleasePRPending(context.Background(), 42, defaultReleasePRLabels())

		// then: the managed and pending labels are added and the tagged label is removed
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{testReleaseLabelPending, "yeet"}, addLabels)
		testastic.Equal(t, testReleaseLabelTagged, removedLabel)

		// when: marking the same pull request pending with the managed label disabled
		labels := defaultReleasePRLabels()
		labels.Yeet = false
		err = gh.MarkReleasePRPending(context.Background(), 42, labels)

		// then: only the pending label is added
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{testReleaseLabelPending}, addLabels)
	})
}

func TestGitLabOpenReleaseMRLabelGuard(t *testing.T) {
	t.Parallel()

	// given: an open release MR carrying a label that differs from the configured one only by case
	guardSourceBranch := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/merge_requests" {
			t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())

			return
		}

		if r.URL.Query().Get("labels") != "" {
			writeJSON(t, w, []map[string]any{})

			return
		}

		guardSourceBranch = r.URL.Query().Get("source_branch")

		writeJSON(t, w, []map[string]any{{
			"iid":               10,
			"title":             "chore: release v2.0.0",
			"web_url":           "https://gitlab.com/o/r/-/merge_requests/10",
			"source_branch":     "yeet/release-main",
			"source_project_id": 10,
			"target_project_id": 10,
			"state":             "opened",
			"labels":            []string{"Autorelease: Pending"},
		}})
	}))
	defer server.Close()

	gl := newGitLabProvider(t, server)

	// when: finding open pending release MRs
	prs, err := gl.FindOpenPendingReleasePRs(context.Background(), "main", "autorelease: pending")

	// then: the guard scans only release branches and reports what the label filter did not match
	testastic.ErrorIs(t, err, provider.ErrReleasePRLabelMismatch)
	testastic.Equal(t, 0, len(prs))
	testastic.Equal(t, "yeet/release-main", guardSourceBranch)
}

func TestGitLabReleasePRLabelPreflight(t *testing.T) {
	t.Parallel()

	t.Run("allows lifecycle labels sharing a scope", func(t *testing.T) {
		t.Parallel()

		// given: sequential lifecycle labels in one GitLab exclusive scope
		var calls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
		}))
		defer server.Close()

		gl := newGitLabProvider(t, server)

		// when: preparing the release labels
		err := gl.PrepareReleasePRLabels(context.Background(), provider.ReleasePRLabels{
			Pending: "release::pending",
			Tagged:  "release::tagged",
		})

		// then: both sequential lifecycle states are accepted and prepared
		testastic.NoError(t, err)
		testastic.Equal(t, int32(2), calls.Load())
	})

	t.Run("rejects an extra label sharing a lifecycle scope", func(t *testing.T) {
		t.Parallel()

		// given: a permanent extra label sharing the pending lifecycle scope
		var calls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
		}))
		defer server.Close()

		gl := newGitLabProvider(t, server)
		labels := provider.ReleasePRLabels{
			Pending: "workflow::backend::pending",
			Tagged:  "release::tagged",
			Extra:   []string{"workflow::backend::automated"},
		}

		// when: preparing the release labels
		err := gl.PrepareReleasePRLabels(context.Background(), labels)

		// then: the permanent label conflict is rejected before a provider request
		testastic.ErrorContains(t, err, "share GitLab scope workflow::backend")
		testastic.Equal(t, int32(0), calls.Load())
	})

	t.Run("rejects reserved lifecycle labels", func(t *testing.T) {
		t.Parallel()

		for _, reserved := range []string{"Any", "nOnE"} {
			t.Run(reserved, func(t *testing.T) {
				t.Parallel()

				// given: a GitLab server and a reserved pending label name
				var calls atomic.Int32

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
				}))
				defer server.Close()

				gl := newGitLabProvider(t, server)

				// when: preparing the release labels
				err := gl.PrepareReleasePRLabels(context.Background(), provider.ReleasePRLabels{
					Pending: reserved,
					Tagged:  "release::tagged",
				})

				// then: the reserved filter value is rejected before a provider request
				testastic.ErrorContains(t, err, "reserved GitLab label filter value")

				_, err = gl.FindOpenPendingReleasePRs(context.Background(), "main", reserved)
				testastic.ErrorContains(t, err, "reserved GitLab label filter value")

				_, err = gl.FindMergedReleasePR(context.Background(), "main", reserved)
				testastic.ErrorContains(t, err, "reserved GitLab label filter value")
				testastic.Equal(t, int32(0), calls.Load())
			})
		}
	})
}

func TestGitHubMergeReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("blocks readiness checks unless force is enabled", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub server reporting PR 42 as open with a blocked mergeable state
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
				writeJSON(t, w, map[string]any{
					"number":          42,
					"state":           "open",
					"mergeable_state": "blocked",
					"draft":           false,
					"head": map[string]any{
						"ref":  "yeet/release-main",
						"repo": map[string]any{"full_name": "o/r"},
					},
					"base": map[string]any{"ref": "main"},
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := newGitHubTestClient(t, server)

		gh := provider.NewGitHub(client, "o", "r")

		// when: MergeReleasePR is invoked without the force option
		_, err := gh.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{})

		// then: ErrMergeBlocked is returned with the blocked mergeable state in the message
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
		testastic.Equal(t, "release PR merge blocked: pull request #42 mergeable_state=blocked", err.Error())
	})

	t.Run("forces merge when readiness is otherwise blocked", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub server reporting PR 42 as blocked with squash merging allowed on the repo
		var mergeRequest struct {
			MergeMethod string `json:"merge_method"`
			SHA         string `json:"sha"`
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
				writeJSON(t, w, map[string]any{
					"number":          42,
					"state":           "open",
					"mergeable_state": "blocked",
					"draft":           false,
					"head": map[string]any{
						"sha":  "head-sha",
						"ref":  "yeet/release-main",
						"repo": map[string]any{"full_name": "o/r"},
					},
					"base": map[string]any{"ref": "main"},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
				writeJSON(t, w, map[string]any{
					"allow_squash_merge": true,
				})
			case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/42/merge":
				err := json.NewDecoder(r.Body).Decode(&mergeRequest)
				testastic.NoError(t, err)

				writeJSON(t, w, map[string]any{"merged": true, "sha": "merge-sha"})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := newGitHubTestClient(t, server)

		gh := provider.NewGitHub(client, "o", "r")

		// when: MergeReleasePR is invoked with merge checks bypassed and auto method selection
		_, err := gh.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
			BypassMergeChecks: true,
			Method:            provider.MergeMethodAuto,
		})

		// then: the squash merge method is chosen and the head SHA is sent in the merge request
		testastic.NoError(t, err)
		testastic.Equal(t, string(provider.MergeMethodSquash), mergeRequest.MergeMethod)
		testastic.Equal(t, "head-sha", mergeRequest.SHA)
	})
}

func TestGitHubUpdateFiles(t *testing.T) {
	t.Parallel()

	var treeRequest struct {
		BaseTree string `json:"base_tree"`
		Tree     []struct {
			Path    string `json:"path"`
			Mode    string `json:"mode"`
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"tree"`
	}

	var commitRequest struct {
		Message string   `json:"message"`
		Tree    string   `json:"tree"`
		Parents []string `json:"parents"`
	}

	var refRequest struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
			writeJSON(t, w, map[string]any{
				"ref":    "refs/heads/main",
				"object": map[string]any{"sha": "base-ref-sha"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/commits/base-ref-sha":
			writeJSON(t, w, map[string]any{
				"sha":  "base-ref-sha",
				"tree": map[string]any{"sha": "base-tree-sha"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/trees":
			err := json.NewDecoder(r.Body).Decode(&treeRequest)
			testastic.NoError(t, err)

			writeJSON(t, w, map[string]any{"sha": "new-tree-sha"})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/commits":
			err := json.NewDecoder(r.Body).Decode(&commitRequest)
			testastic.NoError(t, err)

			writeJSON(t, w, map[string]any{"sha": "new-commit-sha"})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/release-main":
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
			err := json.NewDecoder(r.Body).Decode(&refRequest)
			testastic.NoError(t, err)

			writeJSON(t, w, map[string]any{"ref": refRequest.Ref})
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newGitHubTestClient(t, server)

	gh := provider.NewGitHub(client, "o", "r")

	err := gh.UpdateFiles(context.Background(), "release-main", "main", map[string]provider.FileUpdate{
		"VERSION.txt":  {Content: "version=1.2.3"},
		"CHANGELOG.md": {Content: "# Changelog", Exists: true},
	}, "chore: release 1.2.3")

	testastic.NoError(t, err)
	testastic.Equal(t, "base-tree-sha", treeRequest.BaseTree)
	testastic.Equal(t, 2, len(treeRequest.Tree))
	testastic.Equal(t, "CHANGELOG.md", treeRequest.Tree[0].Path)
	testastic.Equal(t, "VERSION.txt", treeRequest.Tree[1].Path)
	testastic.Equal(t, "100644", treeRequest.Tree[0].Mode)
	testastic.Equal(t, "blob", treeRequest.Tree[0].Type)
	testastic.Equal(t, "chore: release 1.2.3", commitRequest.Message)
	testastic.Equal(t, "new-tree-sha", commitRequest.Tree)
	testastic.Equal(t, "base-ref-sha", strings.Join(commitRequest.Parents, ","))
	testastic.Equal(t, "refs/heads/release-main", refRequest.Ref)
	testastic.Equal(t, "new-commit-sha", refRequest.SHA)
}

func TestGitLabReleasePRStateTransitions(t *testing.T) {
	t.Parallel()

	t.Run("marks merge request pending", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab server that records label add and remove fields on MR 12 updates
		var updateRequest struct {
			AddLabels    string `json:"add_labels"`
			RemoveLabels string `json:"remove_labels"`
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
				writeJSON(t, w, map[string]any{"name": decodedPathTail(t, r)})
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/12":
				err := json.NewDecoder(r.Body).Decode(&updateRequest)
				testastic.NoError(t, err)

				writeJSON(t, w, map[string]any{"iid": 12})
			default:
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		gl := newGitLabProvider(t, server)

		// when: MarkReleasePRPending is invoked for MR 12
		err := gl.MarkReleasePRPending(context.Background(), 12, defaultReleasePRLabels())

		// then: the managed and pending labels are added and the tagged label is removed
		testastic.NoError(t, err)
		testastic.Equal(t, testReleaseLabelPending+",yeet", updateRequest.AddLabels)
		testastic.Equal(t, testReleaseLabelTagged, updateRequest.RemoveLabels)

		// when: marking the same merge request pending with the managed label disabled
		labels := defaultReleasePRLabels()
		labels.Yeet = false
		err = gl.MarkReleasePRPending(context.Background(), 12, labels)

		// then: only the pending label is added
		testastic.NoError(t, err)
		testastic.Equal(t, testReleaseLabelPending, updateRequest.AddLabels)
	})
}

func TestGitLabMergeReleasePR(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"checking", "unchecked", "preparing"} {
		t.Run("allows transient status "+status, func(t *testing.T) {
			t.Parallel()

			// given: a GitLab server reporting a transient merge status while recomputing readiness
			merged := false

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8":
					writeJSON(t, w, map[string]any{
						"iid":                   8,
						"state":                 "opened",
						"draft":                 false,
						"has_conflicts":         false,
						"detailed_merge_status": status,
						"source_branch":         "yeet/release-main",
						"target_branch":         "main",
						"source_project_id":     10,
						"target_project_id":     10,
					})
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
					writeJSON(t, w, map[string]any{
						"merge_method":  string(gitlabapi.NoFastForwardMerge),
						"squash_option": "default_off",
					})
				case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8/merge":
					merged = true

					writeJSON(t, w, map[string]any{"iid": 8, "state": "merged", "merge_commit_sha": "merge-sha"})
				default:
					t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
				}
			}))
			defer server.Close()

			gl := newGitLabProvider(t, server)

			// when: MergeReleasePR is invoked without force while readiness is recomputed
			_, err := gl.MergeReleasePR(context.Background(), 8, provider.MergeReleasePROptions{})

			// then: the transient status does not prevent the merge request
			testastic.NoError(t, err)
			testastic.True(t, merged)
		})
	}

	t.Run("blocks readiness checks unless force is enabled", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab server reporting MR 8 as opened with a not_approved merge status
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8":
				writeJSON(t, w, map[string]any{
					"iid":                   8,
					"state":                 "opened",
					"draft":                 false,
					"has_conflicts":         false,
					"detailed_merge_status": "not_approved",
					"source_branch":         "yeet/release-main",
					"target_branch":         "main",
					"source_project_id":     10,
					"target_project_id":     10,
				})
			default:
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		gl := newGitLabProvider(t, server)

		// when: MergeReleasePR is invoked without the force option
		_, err := gl.MergeReleasePR(context.Background(), 8, provider.MergeReleasePROptions{})

		// then: ErrMergeBlocked is returned with the detailed merge status in the message
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
		testastic.Equal(t, "release PR merge blocked: merge request !8 detailed_merge_status=not_approved", err.Error())
	})

	t.Run("forces merge and forwards squash option", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab server reporting MR 8 as blocked with squash always enabled at the project level
		var mergeRequest struct {
			SHA    string `json:"sha"`
			Squash bool   `json:"squash"`
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8":
				writeJSON(t, w, map[string]any{
					"iid":                   8,
					"state":                 "opened",
					"draft":                 false,
					"has_conflicts":         false,
					"detailed_merge_status": "not_approved",
					"sha":                   "head-sha",
					"source_branch":         "yeet/release-main",
					"target_branch":         "main",
					"source_project_id":     10,
					"target_project_id":     10,
				})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSON(t, w, map[string]any{
					"merge_method":  string(gitlabapi.NoFastForwardMerge),
					"squash_option": "always",
				})
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8/merge":
				err := json.NewDecoder(r.Body).Decode(&mergeRequest)
				testastic.NoError(t, err)

				writeJSON(t, w, map[string]any{"iid": 8, "state": "merged", "merge_commit_sha": "merge-sha"})
			default:
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		gl := newGitLabProvider(t, server)

		// when: MergeReleasePR is invoked with merge checks bypassed and the squash merge method
		_, err := gl.MergeReleasePR(context.Background(), 8, provider.MergeReleasePROptions{
			BypassMergeChecks: true,
			Method:            provider.MergeMethodSquash,
		})

		// then: the head SHA is forwarded and the squash flag is set on the merge request
		testastic.NoError(t, err)
		testastic.Equal(t, "head-sha", mergeRequest.SHA)
		testastic.True(t, mergeRequest.Squash)
	})
}

func TestGitLabUpdateFiles(t *testing.T) {
	t.Parallel()

	var commitRequest struct {
		Branch        string `json:"branch"`
		CommitMessage string `json:"commit_message"`
		StartBranch   string `json:"start_branch"`
		Force         bool   `json:"force"`
		Actions       []struct {
			Action   string `json:"action"`
			FilePath string `json:"file_path"`
			Content  string `json:"content"`
		} `json:"actions"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/commits":
			err := json.NewDecoder(r.Body).Decode(&commitRequest)
			testastic.NoError(t, err)

			writeJSON(t, w, map[string]any{"id": "new-commit"})
		default:
			t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	gl := newGitLabProvider(t, server)

	err := gl.UpdateFiles(context.Background(), "release-main", "main", map[string]provider.FileUpdate{
		"VERSION.txt":  {Content: "version=1.2.3"},
		"CHANGELOG.md": {Content: "# Changelog", Exists: true},
	}, "chore: release 1.2.3")

	testastic.NoError(t, err)
	testastic.Equal(t, "release-main", commitRequest.Branch)
	testastic.Equal(t, "main", commitRequest.StartBranch)
	testastic.Equal(t, "chore: release 1.2.3", commitRequest.CommitMessage)
	testastic.True(t, commitRequest.Force)
	testastic.Equal(t, 2, len(commitRequest.Actions))
	testastic.Equal(t, "CHANGELOG.md", commitRequest.Actions[0].FilePath)
	testastic.Equal(t, "update", commitRequest.Actions[0].Action)
	testastic.Equal(t, "VERSION.txt", commitRequest.Actions[1].FilePath)
	testastic.Equal(t, "create", commitRequest.Actions[1].Action)
}

func newGitLabProvider(t *testing.T, server *httptest.Server) *provider.GitLab {
	t.Helper()

	client, err := gitlabapi.NewClient(
		"",
		gitlabapi.WithBaseURL(server.URL),
		gitlabapi.WithHTTPClient(server.Client()),
		gitlabapi.WithoutRetries(),
	)
	testastic.NoError(t, err)

	return provider.NewGitLab(client, "o/r")
}

func decodedPathTail(t *testing.T, request *http.Request) string {
	t.Helper()

	segments := strings.Split(request.URL.EscapedPath(), "/")
	if len(segments) == 0 {
		t.Fatalf("unexpected path: %s", request.URL.EscapedPath())
	}

	label, err := url.PathUnescape(segments[len(segments)-1])
	testastic.NoError(t, err)

	return label
}

func isGitLabRawFilePath(request *http.Request, path string) bool {
	if request.URL.Query().Get("ref") != "main" {
		return false
	}

	prefix := "/api/v4/projects/o%2Fr/repository/files/"
	suffix := "/raw"

	escapedPath := request.URL.EscapedPath()
	if !strings.HasPrefix(escapedPath, prefix) || !strings.HasSuffix(escapedPath, suffix) {
		return false
	}

	rawPath := strings.TrimSuffix(strings.TrimPrefix(escapedPath, prefix), suffix)

	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return false
	}

	return decodedPath == path
}
