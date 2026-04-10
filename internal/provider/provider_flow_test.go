package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	githubapi "github.com/google/go-github/v84/github"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestGitHubReleasePRStateTransitions(t *testing.T) {
	t.Parallel()

	t.Run("marks pull request pending", func(t *testing.T) {
		t.Parallel()

		var addLabels []string

		removedLabel := ""

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
				writeJSON(t, w, map[string]any{"name": pathLabel(t, r)})
			case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
				err := json.NewDecoder(r.Body).Decode(&addLabels)
				testastic.NoError(t, err)

				writeJSON(t, w, []map[string]any{{"name": provider.ReleaseLabelPending}})
			case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
				removedLabel = pathLabel(t, r)
				http.NotFound(w, r)
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		err := gh.MarkReleasePRPending(context.Background(), 42)

		testastic.NoError(t, err)
		testastic.Equal(t, provider.ReleaseLabelPending, strings.Join(addLabels, ","))
		testastic.Equal(t, provider.ReleaseLabelTagged, removedLabel)
	})

	t.Run("marks pull request tagged", func(t *testing.T) {
		t.Parallel()

		var addLabels []string

		removedLabel := ""

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/labels/"):
				writeJSON(t, w, map[string]any{"name": pathLabel(t, r)})
			case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/7/labels":
				err := json.NewDecoder(r.Body).Decode(&addLabels)
				testastic.NoError(t, err)

				writeJSON(t, w, []map[string]any{{"name": provider.ReleaseLabelTagged}})
			case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/7/labels/"):
				removedLabel = pathLabel(t, r)

				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		err := gh.MarkReleasePRTagged(context.Background(), 7)

		testastic.NoError(t, err)
		testastic.Equal(t, provider.ReleaseLabelTagged, strings.Join(addLabels, ","))
		testastic.Equal(t, provider.ReleaseLabelPending, removedLabel)
	})
}

func TestGitHubMergeReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("blocks readiness checks unless force is enabled", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
				writeJSON(t, w, map[string]any{
					"number":          42,
					"state":           "open",
					"mergeable_state": "blocked",
					"draft":           false,
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		err := gh.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{})

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
		testastic.ErrorContains(t, err, "mergeable_state=blocked")
	})

	t.Run("forces merge when readiness is otherwise blocked", func(t *testing.T) {
		t.Parallel()

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
					"head":            map[string]any{"sha": "head-sha"},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
				writeJSON(t, w, map[string]any{
					"allow_squash_merge": true,
				})
			case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/42/merge":
				err := json.NewDecoder(r.Body).Decode(&mergeRequest)
				testastic.NoError(t, err)

				writeJSON(t, w, map[string]any{"merged": true})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		err := gh.MergeReleasePR(context.Background(), 42, provider.MergeReleasePROptions{
			Force:  true,
			Method: provider.MergeMethodAuto,
		})

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

	client := githubapi.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")

	gh := provider.NewGitHub(client, "o", "r")

	err := gh.UpdateFiles(context.Background(), "release-main", "main", map[string]string{
		"VERSION.txt":  "version=1.2.3",
		"CHANGELOG.md": "# Changelog",
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

		var updateRequest struct {
			AddLabels    string `json:"add_labels"`
			RemoveLabels string `json:"remove_labels"`
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
				writeJSON(t, w, map[string]any{"name": pathLabel(t, r)})
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

		err := gl.MarkReleasePRPending(context.Background(), 12)

		testastic.NoError(t, err)
		testastic.Equal(t, provider.ReleaseLabelPending, updateRequest.AddLabels)
		testastic.Equal(t, provider.ReleaseLabelTagged, updateRequest.RemoveLabels)
	})

	t.Run("marks merge request tagged", func(t *testing.T) {
		t.Parallel()

		var updateRequest struct {
			AddLabels    string `json:"add_labels"`
			RemoveLabels string `json:"remove_labels"`
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/labels/"):
				writeJSON(t, w, map[string]any{"name": pathLabel(t, r)})
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/5":
				err := json.NewDecoder(r.Body).Decode(&updateRequest)
				testastic.NoError(t, err)

				writeJSON(t, w, map[string]any{"iid": 5})
			default:
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		gl := newGitLabProvider(t, server)

		err := gl.MarkReleasePRTagged(context.Background(), 5)

		testastic.NoError(t, err)
		testastic.Equal(t, provider.ReleaseLabelTagged, updateRequest.AddLabels)
		testastic.Equal(t, provider.ReleaseLabelPending, updateRequest.RemoveLabels)
	})
}

func TestGitLabMergeReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("blocks readiness checks unless force is enabled", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8":
				writeJSON(t, w, map[string]any{
					"iid":                   8,
					"state":                 "opened",
					"draft":                 false,
					"has_conflicts":         false,
					"detailed_merge_status": "not_approved",
				})
			default:
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		gl := newGitLabProvider(t, server)

		err := gl.MergeReleasePR(context.Background(), 8, provider.MergeReleasePROptions{})

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
		testastic.ErrorContains(t, err, "detailed_merge_status=not_approved")
	})

	t.Run("forces merge and forwards squash option", func(t *testing.T) {
		t.Parallel()

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
				})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSON(t, w, map[string]any{
					"merge_method":  string(gitlabapi.NoFastForwardMerge),
					"squash_option": "always",
				})
			case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/8/merge":
				err := json.NewDecoder(r.Body).Decode(&mergeRequest)
				testastic.NoError(t, err)

				writeJSON(t, w, map[string]any{"iid": 8})
			default:
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		gl := newGitLabProvider(t, server)

		err := gl.MergeReleasePR(context.Background(), 8, provider.MergeReleasePROptions{
			Force:  true,
			Method: provider.MergeMethodSquash,
		})

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
		case r.Method == http.MethodGet && isGitLabRawFilePath(r, "CHANGELOG.md"):
			_, err := w.Write([]byte("# Changelog"))
			testastic.NoError(t, err)
		case r.Method == http.MethodGet && isGitLabRawFilePath(r, "VERSION.txt"):
			http.NotFound(w, r)
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

	err := gl.UpdateFiles(context.Background(), "release-main", "main", map[string]string{
		"VERSION.txt":  "version=1.2.3",
		"CHANGELOG.md": "# Changelog",
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

func pathLabel(t *testing.T, request *http.Request) string {
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
