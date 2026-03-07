package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	githubapi "github.com/google/go-github/v84/github"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go"
)

func TestParseRemote(t *testing.T) {
	t.Parallel()

	t.Run("github ssh", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub SSH remote URL
		url := "git@github.com:owner/repo.git"

		// when: parsing the remote
		info, err := provider.ParseRemote(url)

		// then: repository coordinates are extracted
		testastic.NoError(t, err)
		testastic.Equal(t, "github.com", info.Host)
		testastic.Equal(t, "owner", info.Owner)
		testastic.Equal(t, "repo", info.Repo)
		testastic.Equal(t, "owner/repo", info.Project)
	})

	t.Run("github https", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub HTTPS remote URL
		url := "https://github.com/owner/repo.git"

		// when: parsing the remote
		info, err := provider.ParseRemote(url)

		// then: repository coordinates are extracted
		testastic.NoError(t, err)
		testastic.Equal(t, "github.com", info.Host)
		testastic.Equal(t, "owner", info.Owner)
		testastic.Equal(t, "repo", info.Repo)
		testastic.Equal(t, "owner/repo", info.Project)
	})

	t.Run("github enterprise https", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub Enterprise remote URL
		url := "https://github.company.com/platform/yeet.git"

		// when: parsing the remote
		info, err := provider.ParseRemote(url)

		// then: host and repository are preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "github.company.com", info.Host)
		testastic.Equal(t, "platform", info.Owner)
		testastic.Equal(t, "yeet", info.Repo)
	})

	t.Run("gitlab subgroup ssh", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab subgroup SSH remote URL
		url := "git@gitlab.com:group/subgroup/service.git"

		// when: parsing the remote
		info, err := provider.ParseRemote(url)

		// then: the full project path is preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab.com", info.Host)
		testastic.Equal(t, "group/subgroup", info.Owner)
		testastic.Equal(t, "service", info.Repo)
		testastic.Equal(t, "group/subgroup/service", info.Project)
	})

	t.Run("repo names with dots", func(t *testing.T) {
		t.Parallel()

		// given: a remote with a dotted repository name
		url := "https://gitlab.com/group/service.api.git"

		// when: parsing the remote
		info, err := provider.ParseRemote(url)

		// then: the dotted name is preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "group", info.Owner)
		testastic.Equal(t, "service.api", info.Repo)
		testastic.Equal(t, "group/service.api", info.Project)
	})

	t.Run("invalid url", func(t *testing.T) {
		t.Parallel()

		// given: an unparseable URL
		url := "not-a-valid-url"

		// when: parsing the remote
		_, err := provider.ParseRemote(url)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnknownRemote)
	})
}

func TestDetectProviderType(t *testing.T) {
	t.Parallel()

	t.Run("detects github hosts", func(t *testing.T) {
		t.Parallel()

		providerType, err := provider.DetectProviderType("github.company.com")

		testastic.NoError(t, err)
		testastic.Equal(t, "github", providerType)
	})

	t.Run("detects gitlab hosts", func(t *testing.T) {
		t.Parallel()

		providerType, err := provider.DetectProviderType("gitlab.company.com")

		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab", providerType)
	})

	t.Run("fails on unsupported hosts", func(t *testing.T) {
		t.Parallel()

		_, err := provider.DetectProviderType("code.company.com")

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
	})
}

func TestParseCommits(t *testing.T) {
	t.Parallel()

	t.Run("parses commit entries", func(t *testing.T) {
		t.Parallel()

		// given: raw commit entries
		entries := []provider.CommitEntry{
			{Hash: "abc123", Message: "feat: add new feature"},
			{Hash: "def456", Message: "fix(auth): resolve token issue"},
			{Hash: "ghi789", Message: "not a conventional commit"},
		}

		// when: parsing commits
		commits := provider.ParseCommits(entries)

		// then: all commits are parsed
		testastic.Equal(t, 3, len(commits))
		testastic.Equal(t, "feat", commits[0].Type)
		testastic.Equal(t, "fix", commits[1].Type)
		testastic.Equal(t, "auth", commits[1].Scope)
		testastic.Equal(t, "", commits[2].Type)
	})
}

func TestParseCommitsEmpty(t *testing.T) {
	t.Parallel()

	// given: no commit entries
	var entries []provider.CommitEntry

	// when: parsing commits
	commits := provider.ParseCommits(entries)

	// then: empty result
	testastic.Equal(t, 0, len(commits))
}

func TestGitHubGetCommitsSince(t *testing.T) {
	t.Parallel()

	t.Run("returns commits from requested branch until boundary across pages", func(t *testing.T) {
		t.Parallel()

		const (
			branch = "release/main"
			ref    = "v1.0.0"
		)

		var listCalls atomic.Int32

		var resolveCalls atomic.Int32

		var server *httptest.Server

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+ref:
				resolveCalls.Add(1)

				writeJSON(t, w, map[string]any{"sha": "base-sha"})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits":
				listCalls.Add(1)

				testastic.Equal(t, branch, r.URL.Query().Get("sha"))
				testastic.Equal(t, "100", r.URL.Query().Get("per_page"))

				switch r.URL.Query().Get("page") {
				case "":
					w.Header().Set(
						"Link",
						fmt.Sprintf(
							`<%s/repos/o/r/commits?sha=%s&per_page=100&page=2>; rel="next"`,
							server.URL,
							url.QueryEscape(branch),
						),
					)
					writeJSON(t, w, []map[string]any{
						{"sha": "head-1", "commit": map[string]any{"message": "feat: add API"}},
						{"sha": "head-2", "commit": map[string]any{"message": "fix: patch API"}},
					})
				case "2":
					writeJSON(t, w, []map[string]any{
						{"sha": "base-sha", "commit": map[string]any{"message": "chore: previous release"}},
						{"sha": "older", "commit": map[string]any{"message": "docs: older commit"}},
					})
				default:
					t.Fatalf("unexpected GitHub commits page: %s", r.URL.RawQuery)
				}
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		entries, err := gh.GetCommitsSince(context.Background(), ref, branch)

		testastic.NoError(t, err)
		testastic.Equal(t, int32(1), resolveCalls.Load())
		testastic.Equal(t, int32(2), listCalls.Load())
		testastic.Equal(t, 2, len(entries))
		testastic.Equal(t, "head-1", entries[0].Hash)
		testastic.Equal(t, "feat: add API", entries[0].Message)
		testastic.Equal(t, "head-2", entries[1].Hash)
	})

	t.Run("returns boundary error when ref is not reachable from branch", func(t *testing.T) {
		t.Parallel()

		const (
			branch = "release/main"
			ref    = "v1.0.0"
		)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/"+ref:
				writeJSON(t, w, map[string]any{"sha": "base-sha"})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits":
				testastic.Equal(t, branch, r.URL.Query().Get("sha"))
				writeJSON(t, w, []map[string]any{
					{"sha": "head-1", "commit": map[string]any{"message": "feat: add API"}},
					{"sha": "head-2", "commit": map[string]any{"message": "fix: patch API"}},
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		entries, err := gh.GetCommitsSince(context.Background(), ref, branch)

		testastic.Error(t, err)
		testastic.Equal(t, 0, len(entries))
		testastic.ErrorIs(t, err, provider.ErrCommitBoundaryNotFound)
		testastic.ErrorContains(t, err, ref)
		testastic.ErrorContains(t, err, branch)

		var boundaryErr *provider.CommitBoundaryNotFoundError
		testastic.ErrorAs(t, err, &boundaryErr)
		testastic.Equal(t, ref, boundaryErr.Ref)
		testastic.Equal(t, branch, boundaryErr.Branch)
	})

	t.Run("allows initial release without boundary", func(t *testing.T) {
		t.Parallel()

		const branch = "release/main"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits":
				testastic.Equal(t, branch, r.URL.Query().Get("sha"))
				writeJSON(t, w, []map[string]any{
					{"sha": "head-1", "commit": map[string]any{"message": "feat: add API"}},
					{"sha": "head-2", "commit": map[string]any{"message": "fix: patch API"}},
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		entries, err := gh.GetCommitsSince(context.Background(), "", branch)

		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(entries))
		testastic.Equal(t, "head-1", entries[0].Hash)
		testastic.Equal(t, "head-2", entries[1].Hash)
	})
}

func TestGitLabGetCommitsSince(t *testing.T) {
	t.Parallel()

	t.Run("returns commits from requested branch until boundary across pages", func(t *testing.T) {
		t.Parallel()

		const (
			branch = "release/main"
			ref    = "v1.0.0"
		)

		var listCalls atomic.Int32

		var resolveCalls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case isGitLabCommitResolveRequest(r):
				resolveCalls.Add(1)

				writeJSON(t, w, map[string]any{"id": "base-sha"})
			case isGitLabCommitsListRequest(r):
				listCalls.Add(1)

				testastic.Equal(t, branch, r.URL.Query().Get("ref_name"))
				testastic.Equal(t, "100", r.URL.Query().Get("per_page"))

				switch r.URL.Query().Get("page") {
				case "":
					w.Header().Set("X-Next-Page", "2")
					writeJSON(t, w, []map[string]any{
						{"id": "head-1", "message": "feat: add API"},
						{"id": "head-2", "message": "fix: patch API"},
					})
				case "2":
					writeJSON(t, w, []map[string]any{
						{"id": "base-sha", "message": "chore: previous release"},
						{"id": "older", "message": "docs: older commit"},
					})
				default:
					t.Fatalf("unexpected GitLab commits page: %s", r.URL.RawQuery)
				}
			default:
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client, err := gitlabapi.NewClient(
			"",
			gitlabapi.WithBaseURL(server.URL),
			gitlabapi.WithHTTPClient(server.Client()),
			gitlabapi.WithoutRetries(),
		)
		testastic.NoError(t, err)

		gl := provider.NewGitLab(client, "o/r")

		entries, err := gl.GetCommitsSince(context.Background(), ref, branch)

		testastic.NoError(t, err)
		testastic.Equal(t, int32(1), resolveCalls.Load())
		testastic.Equal(t, int32(2), listCalls.Load())
		testastic.Equal(t, 2, len(entries))
		testastic.Equal(t, "head-1", entries[0].Hash)
		testastic.Equal(t, "feat: add API", entries[0].Message)
		testastic.Equal(t, "head-2", entries[1].Hash)
	})

	t.Run("returns boundary error when ref is not reachable from branch", func(t *testing.T) {
		t.Parallel()

		const (
			branch = "release/main"
			ref    = "v1.0.0"
		)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case isGitLabCommitResolveRequest(r):
				writeJSON(t, w, map[string]any{"id": "base-sha"})
			case isGitLabCommitsListRequest(r):
				testastic.Equal(t, branch, r.URL.Query().Get("ref_name"))
				writeJSON(t, w, []map[string]any{
					{"id": "head-1", "message": "feat: add API"},
					{"id": "head-2", "message": "fix: patch API"},
				})
			default:
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client, err := gitlabapi.NewClient(
			"",
			gitlabapi.WithBaseURL(server.URL),
			gitlabapi.WithHTTPClient(server.Client()),
			gitlabapi.WithoutRetries(),
		)
		testastic.NoError(t, err)

		gl := provider.NewGitLab(client, "o/r")

		entries, err := gl.GetCommitsSince(context.Background(), ref, branch)

		testastic.Error(t, err)
		testastic.Equal(t, 0, len(entries))
		testastic.ErrorIs(t, err, provider.ErrCommitBoundaryNotFound)
		testastic.ErrorContains(t, err, ref)
		testastic.ErrorContains(t, err, branch)

		var boundaryErr *provider.CommitBoundaryNotFoundError
		testastic.ErrorAs(t, err, &boundaryErr)
		testastic.Equal(t, ref, boundaryErr.Ref)
		testastic.Equal(t, branch, boundaryErr.Branch)
	})

	t.Run("allows initial release without boundary", func(t *testing.T) {
		t.Parallel()

		const branch = "release/main"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case isGitLabCommitsListRequest(r):
				testastic.Equal(t, branch, r.URL.Query().Get("ref_name"))
				writeJSON(t, w, []map[string]any{
					{"id": "head-1", "message": "feat: add API"},
					{"id": "head-2", "message": "fix: patch API"},
				})
			default:
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client, err := gitlabapi.NewClient(
			"",
			gitlabapi.WithBaseURL(server.URL),
			gitlabapi.WithHTTPClient(server.Client()),
			gitlabapi.WithoutRetries(),
		)
		testastic.NoError(t, err)

		gl := provider.NewGitLab(client, "o/r")

		entries, err := gl.GetCommitsSince(context.Background(), "", branch)

		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(entries))
		testastic.Equal(t, "head-1", entries[0].Hash)
		testastic.Equal(t, "head-2", entries[1].Hash)
	})
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	testastic.NoError(t, err)

	return parsed
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(value)
	testastic.NoError(t, err)
}

func isGitLabCommitResolveRequest(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/repository/commits/")
}

func isGitLabCommitsListRequest(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/commits"
}

// Compile-time interface checks.
var (
	_ provider.Provider = (*provider.GitHub)(nil)
	_ provider.Provider = (*provider.GitLab)(nil)
)

// Compile-time strategy checks.
var _ commit.BumpType = commit.BumpMajor
