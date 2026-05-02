package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	githubapi "github.com/google/go-github/v85/github"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go/v2"
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

	t.Run("gitlab subgroup ssh url", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab SSH URL remote with a subgroup path
		url := "ssh://git@gitlab.company.com/group/subgroup/service.git"

		// when: parsing the remote
		info, err := provider.ParseRemote(url)

		// then: the host and full project path are preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab.company.com", info.Host)
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

		providerType, err := provider.DetectProviderType("github.com")

		testastic.NoError(t, err)
		testastic.Equal(t, "github", providerType)
	})

	t.Run("detects gitlab hosts", func(t *testing.T) {
		t.Parallel()

		providerType, err := provider.DetectProviderType("gitlab.com")

		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab", providerType)
	})

	t.Run("fails on github custom hosts", func(t *testing.T) {
		t.Parallel()

		_, err := provider.DetectProviderType("github.company.com")

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
	})

	t.Run("fails on gitlab custom hosts", func(t *testing.T) {
		t.Parallel()

		_, err := provider.DetectProviderType("gitlab.company.com")

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
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

func TestGitHubVersionLookup(t *testing.T) {
	t.Parallel()

	t.Run("returns latest version ref from latest release", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub repository with a published release
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/latest":
				writeJSON(t, w, map[string]any{
					"tag_name": "v1.2.4",
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: resolving the latest version ref
		ref, err := gh.GetLatestVersionRef(context.Background())

		// then: the latest published release tag is preferred
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.4", ref)
	})

	t.Run("falls back to tags when no release exists", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub repository with tags but no published release
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/latest":
				http.NotFound(w, r)
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/tags":
				testastic.Equal(t, "100", r.URL.Query().Get("per_page"))
				writeJSON(t, w, []map[string]any{{
					"name":   "v1.2.3",
					"commit": map[string]any{"sha": "abc123"},
				}})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: resolving the latest version ref
		ref, err := gh.GetLatestVersionRef(context.Background())

		// then: the latest tag is returned when no release exists
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", ref)
	})

	t.Run("returns release by exact tag", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub repository with a release for the requested tag
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/v1.2.3":
				writeJSON(t, w, map[string]any{
					"tag_name": "v1.2.3",
					"name":     "v1.2.3",
					"body":     "release notes",
					"html_url": "https://example.com/releases/v1.2.3",
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: looking up the release by tag
		release, err := gh.GetReleaseByTag(context.Background(), "v1.2.3")

		// then: the exact release is returned
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, "release notes", release.Body)
	})

	t.Run("reports whether exact tag exists", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub repository with one existing tag
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/v1.2.3":
				writeJSON(t, w, map[string]any{
					"ref": "refs/tags/v1.2.3",
					"object": map[string]any{
						"sha":  "abc123",
						"type": "commit",
					},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/v9.9.9":
				http.NotFound(w, r)
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: checking existing and missing tags
		exists, err := gh.TagExists(context.Background(), "v1.2.3")
		testastic.NoError(t, err)
		testastic.True(t, exists)

		missing, err := gh.TagExists(context.Background(), "v9.9.9")

		// then: the exact tag existence is reported without treating missing tags as errors
		testastic.NoError(t, err)
		testastic.False(t, missing)
	})
}

func TestGitHubFindMergedReleasePRIncludesMergeCommitSHA(t *testing.T) {
	t.Parallel()

	// given: a merged pending release pull request on GitHub
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls":
			writeJSON(t, w, []map[string]any{{
				"number":    42,
				"title":     "chore: release 1.2.3",
				"body":      "<!-- yeet-release-tag: v1.2.3 -->",
				"html_url":  "https://example.com/pr/42",
				"merged_at": "2026-03-01T00:00:00Z",
				"labels": []map[string]any{{
					"name": provider.ReleaseLabelPending,
				}},
				"head": map[string]any{
					"ref": "yeet/release-main",
				},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
			writeJSON(t, w, map[string]any{
				"number":           42,
				"merge_commit_sha": "merge-sha",
			})
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := githubapi.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")

	gh := provider.NewGitHub(client, "o", "r")

	// when: finding the merged pending release PR
	pullRequest, err := gh.FindMergedReleasePR(context.Background(), "main")

	// then: the merged commit SHA is populated for stale-release finalization
	testastic.NoError(t, err)
	testastic.Equal(t, 42, pullRequest.Number)
	testastic.Equal(t, "merge-sha", pullRequest.MergeCommitSHA)
}

func TestGitLabVersionLookup(t *testing.T) {
	t.Parallel()

	t.Run("returns latest version ref from latest release", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab repository with a published release
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/releases":
				testastic.Equal(t, "1", r.URL.Query().Get("per_page"))
				writeJSON(t, w, []map[string]any{{
					"tag_name": "v1.2.4",
				}})
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

		// when: resolving the latest version ref
		ref, err := gl.GetLatestVersionRef(context.Background())

		// then: the latest published release tag is preferred
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.4", ref)
	})

	t.Run("falls back to tags when no release exists", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab repository with tags but no published release
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/releases":
				writeJSON(t, w, []map[string]any{})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/tags":
				testastic.Equal(t, "100", r.URL.Query().Get("per_page"))
				writeJSON(t, w, []map[string]any{{
					"name":   "v1.2.3",
					"target": "abc123",
				}})
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

		// when: resolving the latest version ref
		ref, err := gl.GetLatestVersionRef(context.Background())

		// then: the latest tag is returned when no release exists
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", ref)
	})

	t.Run("returns release by exact tag", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab repository with a release for the requested tag
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/o%2Fr/releases/"):
				writeJSON(t, w, map[string]any{
					"tag_name":    "v1.2.3",
					"name":        "v1.2.3",
					"description": "release notes",
					"_links": map[string]any{
						"self": "https://example.com/releases/v1.2.3",
					},
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

		// when: looking up the release by tag
		release, err := gl.GetReleaseByTag(context.Background(), "v1.2.3")

		// then: the exact release is returned
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.3", release.TagName)
		testastic.Equal(t, "release notes", release.Body)
	})

	t.Run("reports whether exact tag exists", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab repository with one existing tag
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.EscapedPath(), "/repository/tags/v1%2E2%2E3"):
				writeJSON(t, w, map[string]any{
					"name":   "v1.2.3",
					"target": "abc123",
				})
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.EscapedPath(), "/repository/tags/v9%2E9%2E9"):
				http.NotFound(w, r)
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

		// when: checking existing and missing tags
		exists, err := gl.TagExists(context.Background(), "v1.2.3")
		testastic.NoError(t, err)
		testastic.True(t, exists)

		missing, err := gl.TagExists(context.Background(), "v9.9.9")

		// then: the exact tag existence is reported without treating missing tags as errors
		testastic.NoError(t, err)
		testastic.False(t, missing)
	})
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
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/head-1":
				writeJSON(t, w, map[string]any{
					"sha":   "head-1",
					"files": []map[string]any{{"filename": "services/api/main.go"}},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/head-2":
				writeJSON(t, w, map[string]any{
					"sha":   "head-2",
					"files": []map[string]any{{"filename": "services/api/http.go"}},
				})
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

		entries, err := gh.GetCommitsSince(context.Background(), ref, branch, true)

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
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/head-1":
				writeJSON(t, w, map[string]any{
					"sha":   "head-1",
					"files": []map[string]any{{"filename": "services/api/main.go"}},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/head-2":
				writeJSON(t, w, map[string]any{
					"sha":   "head-2",
					"files": []map[string]any{{"filename": "services/api/http.go"}},
				})
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

		entries, err := gh.GetCommitsSince(context.Background(), ref, branch, true)

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
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/head-1":
				writeJSON(t, w, map[string]any{
					"sha":   "head-1",
					"files": []map[string]any{{"filename": "services/api/main.go"}},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/head-2":
				writeJSON(t, w, map[string]any{
					"sha":   "head-2",
					"files": []map[string]any{{"filename": "services/api/http.go"}},
				})
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

		entries, err := gh.GetCommitsSince(context.Background(), "", branch, true)

		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(entries))
		testastic.Equal(t, "head-1", entries[0].Hash)
		testastic.Equal(t, "head-2", entries[1].Hash)
	})

	t.Run("collects changed files across commit detail pages", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub commit whose changed files span multiple commit detail pages
		const branch = "release/main"

		var server *httptest.Server

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/head-1":
				testastic.Equal(t, "100", r.URL.Query().Get("per_page"))

				switch r.URL.Query().Get("page") {
				case "":
					w.Header().Set(
						"Link",
						fmt.Sprintf(
							`<%s/repos/o/r/commits/head-1?per_page=100&page=2>; rel="next"`,
							server.URL,
						),
					)

					writeJSON(t, w, map[string]any{
						"sha": "head-1",
						"files": []map[string]any{
							{"filename": "services/api/main.go"},
							{"previous_filename": "services/api/legacy.go"},
						},
					})
				case "2":
					writeJSON(t, w, map[string]any{
						"sha": "head-1",
						"files": []map[string]any{
							{"filename": "services/api/http.go"},
							{"filename": "services/api/main.go"},
						},
					})
				default:
					t.Fatalf("unexpected GitHub commit detail page: %s", r.URL.RawQuery)
				}
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits":
				testastic.Equal(t, branch, r.URL.Query().Get("sha"))

				writeJSON(t, w, []map[string]any{{
					"sha":    "head-1",
					"commit": map[string]any{"message": "feat: add API"},
				}})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: listing commits for the branch
		entries, err := gh.GetCommitsSince(context.Background(), "", branch, true)

		// then: all changed paths from every commit detail page are collected once
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(entries))
		testastic.SliceEqual(
			t,
			[]string{"services/api/main.go", "services/api/legacy.go", "services/api/http.go"},
			entries[0].Paths,
		)
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
			case isGitLabCommitDiffRequest(r, "head-1"):
				writeJSON(t, w, []map[string]any{{"new_path": "services/api/main.go", "old_path": "services/api/main.go"}})
			case isGitLabCommitDiffRequest(r, "head-2"):
				writeJSON(t, w, []map[string]any{{"new_path": "services/api/http.go", "old_path": "services/api/http.go"}})
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

		entries, err := gl.GetCommitsSince(context.Background(), ref, branch, true)

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
			case isGitLabCommitDiffRequest(r, "head-1"):
				writeJSON(t, w, []map[string]any{{"new_path": "services/api/main.go", "old_path": "services/api/main.go"}})
			case isGitLabCommitDiffRequest(r, "head-2"):
				writeJSON(t, w, []map[string]any{{"new_path": "services/api/http.go", "old_path": "services/api/http.go"}})
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

		entries, err := gl.GetCommitsSince(context.Background(), ref, branch, true)

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
			case isGitLabCommitDiffRequest(r, "head-1"):
				writeJSON(t, w, []map[string]any{{"new_path": "services/api/main.go", "old_path": "services/api/main.go"}})
			case isGitLabCommitDiffRequest(r, "head-2"):
				writeJSON(t, w, []map[string]any{{"new_path": "services/api/http.go", "old_path": "services/api/http.go"}})
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

		entries, err := gl.GetCommitsSince(context.Background(), "", branch, true)

		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(entries))
		testastic.Equal(t, "head-1", entries[0].Hash)
		testastic.Equal(t, "head-2", entries[1].Hash)
	})

	t.Run("collects changed files across commit diff pages", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab commit whose diff spans multiple pages
		const branch = "release/main"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case isGitLabCommitDiffRequest(r, "head-1"):
				testastic.Equal(t, "100", r.URL.Query().Get("per_page"))

				switch r.URL.Query().Get("page") {
				case "":
					w.Header().Set("X-Next-Page", "2")
					writeJSON(t, w, []map[string]any{
						{"new_path": "services/api/main.go", "old_path": "services/api/main.go"},
						{"new_path": "services/api/legacy.go", "old_path": "services/api/legacy_old.go"},
					})
				case "2":
					writeJSON(t, w, []map[string]any{
						{"new_path": "services/api/http.go", "old_path": "services/api/http.go"},
						{"new_path": "services/api/main.go", "old_path": "services/api/main.go"},
					})
				default:
					t.Fatalf("unexpected GitLab commit diff page: %s", r.URL.RawQuery)
				}
			case isGitLabCommitsListRequest(r):
				testastic.Equal(t, branch, r.URL.Query().Get("ref_name"))

				writeJSON(t, w, []map[string]any{{
					"id":      "head-1",
					"message": "feat: add API",
				}})
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

		// when: listing commits for the branch
		entries, err := gl.GetCommitsSince(context.Background(), "", branch, true)

		// then: all changed paths from every diff page are collected once
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(entries))
		testastic.SliceEqual(
			t,
			[]string{
				"services/api/main.go",
				"services/api/legacy.go",
				"services/api/legacy_old.go",
				"services/api/http.go",
			},
			entries[0].Paths,
		)
	})
}

func TestGitHubCreateRelease(t *testing.T) {
	t.Parallel()

	// given: a GitHub provider backed by a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isGitHubCreateReleaseRequest(r):
			var request struct {
				TagName         string `json:"tag_name"`
				TargetCommitish string `json:"target_commitish"`
				Name            string `json:"name"`
				Body            string `json:"body"`
				Prerelease      bool   `json:"prerelease"`
			}

			err := json.NewDecoder(r.Body).Decode(&request)
			testastic.NoError(t, err)
			testastic.Equal(t, "v1.2.3", request.TagName)
			testastic.Equal(t, "main", request.TargetCommitish)
			testastic.Equal(t, "v1.2.3", request.Name)
			testastic.Equal(t, "release notes", request.Body)
			testastic.True(t, request.Prerelease)

			writeJSON(t, w, map[string]any{
				"tag_name":         request.TagName,
				"target_commitish": request.TargetCommitish,
				"name":             request.Name,
				"body":             request.Body,
				"html_url":         "https://example.com/releases/v1.2.3",
			})
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := githubapi.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")

	gh := provider.NewGitHub(client, "o", "r")

	// when: creating a release with an explicit ref
	release, err := gh.CreateRelease(context.Background(), provider.ReleaseOptions{
		TagName:    "v1.2.3",
		Ref:        "main",
		Name:       "v1.2.3",
		Body:       "release notes",
		Prerelease: true,
	})

	// then: target_commitish and prerelease flag are forwarded to GitHub
	testastic.NoError(t, err)
	testastic.Equal(t, "v1.2.3", release.TagName)
	testastic.Equal(t, "release notes", release.Body)
	testastic.Equal(t, "https://example.com/releases/v1.2.3", release.URL)
}

func TestGitLabCreateRelease(t *testing.T) {
	t.Parallel()

	// given: a GitLab provider backed by a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isGitLabCreateReleaseRequest(r):
			var request struct {
				TagName     string `json:"tag_name"`
				Ref         string `json:"ref"`
				Name        string `json:"name"`
				Description string `json:"description"`
			}

			err := json.NewDecoder(r.Body).Decode(&request)
			testastic.NoError(t, err)
			testastic.Equal(t, "v1.2.3", request.TagName)
			testastic.Equal(t, "main", request.Ref)
			testastic.Equal(t, "v1.2.3", request.Name)
			testastic.Equal(t, "release notes", request.Description)

			writeJSON(t, w, map[string]any{
				"tag_name":    request.TagName,
				"name":        request.Name,
				"description": request.Description,
				"_links": map[string]any{
					"self": "https://example.com/releases/v1.2.3",
				},
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

	// when: creating a release with an explicit ref
	release, err := gl.CreateRelease(context.Background(), provider.ReleaseOptions{
		TagName: "v1.2.3",
		Ref:     "main",
		Name:    "v1.2.3",
		Body:    "release notes",
	})

	// then: ref is forwarded to GitLab
	testastic.NoError(t, err)
	testastic.Equal(t, "v1.2.3", release.TagName)
	testastic.Equal(t, "release notes", release.Body)
	testastic.Equal(t, "https://example.com/releases/v1.2.3", release.URL)
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

func isGitLabCommitDiffRequest(r *http.Request, commitID string) bool {
	return r.Method == http.MethodGet &&
		r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/commits/"+commitID+"/diff"
}

func isGitHubCreateReleaseRequest(r *http.Request) bool {
	return r.Method == http.MethodPost &&
		r.URL.Path == "/repos/o/r/releases"
}

func isGitLabCreateReleaseRequest(r *http.Request) bool {
	return r.Method == http.MethodPost &&
		r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/releases"
}

func TestGitHubCreateBranch(t *testing.T) {
	t.Parallel()

	t.Run("succeeds when branch does not exist", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub repository where the branch does not yet exist
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
				writeJSON(t, w, map[string]any{
					"ref":    "refs/heads/main",
					"object": map[string]any{"sha": "abc123"},
				})
			case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
				w.WriteHeader(http.StatusCreated)
				writeJSON(t, w, map[string]any{
					"ref":    "refs/heads/release-main",
					"object": map[string]any{"sha": "abc123"},
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: creating the branch
		err := gh.CreateBranch(context.Background(), "release-main", "main")

		// then: no error is returned
		testastic.NoError(t, err)
	})

	t.Run("succeeds when branch already exists", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub repository where the branch already exists (API returns 422)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
				writeJSON(t, w, map[string]any{
					"ref":    "refs/heads/main",
					"object": map[string]any{"sha": "abc123"},
				})
			case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
				w.WriteHeader(http.StatusUnprocessableEntity)
				writeJSON(t, w, map[string]any{
					"message": "Reference already exists",
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: creating a branch that already exists
		err := gh.CreateBranch(context.Background(), "release-main", "main")

		// then: no error is returned (idempotent)
		testastic.NoError(t, err)
	})

	t.Run("returns error on unexpected failure", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub repository where the API returns an unexpected error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
				writeJSON(t, w, map[string]any{
					"ref":    "refs/heads/main",
					"object": map[string]any{"sha": "abc123"},
				})
			case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(t, w, map[string]any{
					"message": "Internal Server Error",
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: creating a branch and the API fails
		err := gh.CreateBranch(context.Background(), "release-main", "main")

		// then: the error is propagated
		testastic.Error(t, err)
	})
}

func TestGitLabCreateBranch(t *testing.T) {
	t.Parallel()

	t.Run("succeeds when branch does not exist", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab project where the branch does not yet exist
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches":
				w.WriteHeader(http.StatusCreated)
				writeJSON(t, w, map[string]any{
					"name": "release-main",
					"commit": map[string]any{
						"id": "abc123",
					},
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

		// when: creating the branch
		err = gl.CreateBranch(context.Background(), "release-main", "main")

		// then: no error is returned
		testastic.NoError(t, err)
	})

	t.Run("succeeds when branch already exists", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab project where the branch already exists (API returns 400)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches":
				w.WriteHeader(http.StatusBadRequest)
				writeJSON(t, w, map[string]any{
					"message": "Branch already exists",
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

		// when: creating a branch that already exists
		err = gl.CreateBranch(context.Background(), "release-main", "main")

		// then: no error is returned (idempotent)
		testastic.NoError(t, err)
	})

	t.Run("returns error on unexpected failure", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab project where the API returns an unexpected error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/repository/branches":
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(t, w, map[string]any{
					"message": "Internal Server Error",
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

		// when: creating a branch and the API fails
		err = gl.CreateBranch(context.Background(), "release-main", "main")

		// then: the error is propagated
		testastic.Error(t, err)
	})
}

func TestGitHubPathPrefix(t *testing.T) {
	t.Parallel()

	// given: a GitHub provider
	client := githubapi.NewClient(nil)
	gh := provider.NewGitHub(client, "o", "r")

	// when: requesting the path prefix
	prefix := gh.PathPrefix()

	// then: GitHub has no path prefix
	testastic.Equal(t, "", prefix)
}

func TestGitLabPathPrefix(t *testing.T) {
	t.Parallel()

	// given: a GitLab provider
	client, err := gitlabapi.NewClient("", gitlabapi.WithoutRetries())
	testastic.NoError(t, err)

	gl := provider.NewGitLab(client, "o/r")

	// when: requesting the path prefix
	prefix := gl.PathPrefix()

	// then: GitLab uses /-  path prefix
	testastic.Equal(t, "/-", prefix)
}

func TestGitHubCreateReleasePR(t *testing.T) {
	t.Parallel()

	// given: a GitHub API that creates a pull request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/pulls":
			w.WriteHeader(http.StatusCreated)
			writeJSON(t, w, map[string]any{
				"number":   42,
				"title":    "chore: release v1.0.0",
				"body":     "release body",
				"html_url": "https://github.com/o/r/pull/42",
			})
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := githubapi.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")

	gh := provider.NewGitHub(client, "o", "r")

	// when: creating a release PR
	pr, err := gh.CreateReleasePR(context.Background(), provider.ReleasePROptions{
		Title:         "chore: release v1.0.0",
		Body:          "release body",
		ReleaseBranch: "yeet/release-main",
		BaseBranch:    "main",
	})

	// then: the PR is returned with correct fields
	testastic.NoError(t, err)
	testastic.Equal(t, 42, pr.Number)
	testastic.Equal(t, "chore: release v1.0.0", pr.Title)
	testastic.Equal(t, "yeet/release-main", pr.Branch)
}

func TestGitHubUpdateReleasePR(t *testing.T) {
	t.Parallel()

	// given: a GitHub API that updates a pull request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/o/r/pulls/42":
			writeJSON(t, w, map[string]any{
				"number": 42,
				"title":  "chore: release v1.1.0",
				"body":   "updated body",
			})
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := githubapi.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")

	gh := provider.NewGitHub(client, "o", "r")

	// when: updating the release PR
	err := gh.UpdateReleasePR(context.Background(), 42, provider.ReleasePROptions{
		Title: "chore: release v1.1.0",
		Body:  "updated body",
	})

	// then: no error
	testastic.NoError(t, err)
}

func TestGitHubFindOpenPendingReleasePRs(t *testing.T) {
	t.Parallel()

	t.Run("finds pending release PRs", func(t *testing.T) {
		t.Parallel()

		// given: GitHub returns open PRs with one matching release branch and pending label
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls":
				writeJSON(t, w, []map[string]any{
					{
						"number":   10,
						"title":    "chore: release v2.0.0",
						"body":     "pr body",
						"html_url": "https://github.com/o/r/pull/10",
						"head":     map[string]any{"ref": "yeet/release-main"},
						"labels": []map[string]any{
							{"name": provider.ReleaseLabelPending},
						},
					},
					{
						"number":   11,
						"title":    "other pr",
						"body":     "",
						"html_url": "https://github.com/o/r/pull/11",
						"head":     map[string]any{"ref": "feature/something"},
						"labels":   []map[string]any{},
					},
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: finding open pending release PRs
		prs, err := gh.FindOpenPendingReleasePRs(context.Background(), "main")

		// then: only the release PR with pending label is returned
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(prs))
		testastic.Equal(t, 10, prs[0].Number)
		testastic.Equal(t, "yeet/release-main", prs[0].Branch)
	})
}

func TestGitHubGetFile(t *testing.T) {
	t.Parallel()

	t.Run("returns file content", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub API that returns file content
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/contents/VERSION.txt":
				writeJSON(t, w, map[string]any{
					"type":     "file",
					"encoding": "base64",
					"content":  "MS4yLjM=", // base64 of "1.2.3"
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: getting a file
		content, err := gh.GetFile(context.Background(), "main", "VERSION.txt")

		// then: decoded content is returned
		testastic.NoError(t, err)
		testastic.Equal(t, "1.2.3", content)
	})

	t.Run("returns ErrFileNotFound for missing file", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub API that returns 404
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			writeJSON(t, w, map[string]any{"message": "Not Found"})
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: getting a missing file
		_, err := gh.GetFile(context.Background(), "main", "MISSING.txt")

		// then: ErrFileNotFound is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrFileNotFound)
	})
}

func TestGitHubEnsureLabel(t *testing.T) {
	t.Parallel()

	t.Run("creates label when not found", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub API where the label does not exist
		var created atomic.Bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/labels/"):
				w.WriteHeader(http.StatusNotFound)
				writeJSON(t, w, map[string]any{"message": "Not Found"})
			case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/labels":
				created.Store(true)
				w.WriteHeader(http.StatusCreated)
				writeJSON(t, w, map[string]any{"name": provider.ReleaseLabelPending})
			case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/1/labels":
				writeJSON(t, w, []map[string]any{{"name": provider.ReleaseLabelPending}})
			case r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNotFound)
				writeJSON(t, w, map[string]any{"message": "Not Found"})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: marking a PR as pending (triggers ensureReleaseLabels)
		err := gh.MarkReleasePRPending(context.Background(), 1)

		// then: labels are created and no error is returned
		testastic.NoError(t, err)
		testastic.True(t, created.Load())
	})
}

func TestGitHubResolveGitHubMergeMethod(t *testing.T) {
	t.Parallel()

	t.Run("auto selects squash when enabled", func(t *testing.T) {
		t.Parallel()

		// given: a repository that allows squash merge
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/1":
				writeJSON(t, w, map[string]any{
					"number":          1,
					"state":           "open",
					"merged":          false,
					"draft":           false,
					"mergeable_state": "clean",
					"head":            map[string]any{"sha": "abc123", "ref": "yeet/release-main"},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
				writeJSON(t, w, map[string]any{
					"allow_squash_merge": true,
					"allow_rebase_merge": false,
					"allow_merge_commit": false,
				})
			case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/1/merge":
				writeJSON(t, w, map[string]any{"merged": true, "sha": "def456"})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: merging with auto method
		err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Force:  false,
			Method: provider.MergeMethodAuto,
		})

		// then: no error
		testastic.NoError(t, err)
	})

	t.Run("rejects disabled merge method", func(t *testing.T) {
		t.Parallel()

		// given: a repository that only allows merge commits
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/1":
				writeJSON(t, w, map[string]any{
					"number":          1,
					"state":           "open",
					"merged":          false,
					"draft":           false,
					"mergeable_state": "clean",
					"head":            map[string]any{"sha": "abc123", "ref": "yeet/release-main"},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
				writeJSON(t, w, map[string]any{
					"allow_squash_merge": false,
					"allow_rebase_merge": false,
					"allow_merge_commit": true,
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: merging with squash method (which is disabled)
		err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Force:  false,
			Method: provider.MergeMethodSquash,
		})

		// then: merge is blocked because squash is disabled
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
	})

	t.Run("auto falls back to rebase when squash disabled", func(t *testing.T) {
		t.Parallel()

		// given: a repository that allows only rebase
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/1":
				writeJSON(t, w, map[string]any{
					"number":          1,
					"state":           "open",
					"merged":          false,
					"draft":           false,
					"mergeable_state": "clean",
					"head":            map[string]any{"sha": "abc123", "ref": "yeet/release-main"},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
				writeJSON(t, w, map[string]any{
					"allow_squash_merge": false,
					"allow_rebase_merge": true,
					"allow_merge_commit": false,
				})
			case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/1/merge":
				writeJSON(t, w, map[string]any{"merged": true, "sha": "def456"})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: merging with auto method
		err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Force:  false,
			Method: provider.MergeMethodAuto,
		})

		// then: no error - auto selects rebase
		testastic.NoError(t, err)
	})

	t.Run("auto fails when no merge methods enabled", func(t *testing.T) {
		t.Parallel()

		// given: a repository with all merge methods disabled
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/1":
				writeJSON(t, w, map[string]any{
					"number":          1,
					"state":           "open",
					"merged":          false,
					"draft":           false,
					"mergeable_state": "clean",
					"head":            map[string]any{"sha": "abc123", "ref": "yeet/release-main"},
				})
			case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r":
				writeJSON(t, w, map[string]any{
					"allow_squash_merge": false,
					"allow_rebase_merge": false,
					"allow_merge_commit": false,
				})
			default:
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		client := githubapi.NewClient(server.Client())
		client.BaseURL = mustParseURL(t, server.URL+"/")

		gh := provider.NewGitHub(client, "o", "r")

		// when: merging with auto method
		err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Force:  false,
			Method: provider.MergeMethodAuto,
		})

		// then: merge is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
	})
}

func TestGitHubCommitPullRequestBody(t *testing.T) {
	t.Parallel()

	// given: GitHub returns PRs associated with a commit, but only one is the merge commit
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/abc123/pulls":
			writeJSON(t, w, []map[string]any{
				{
					"number":           1,
					"body":             "wrong body",
					"merge_commit_sha": "def456",
				},
				{
					"number":           2,
					"body":             "override body",
					"merge_commit_sha": "abc123",
				},
			})
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := githubapi.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")

	gh := provider.NewGitHub(client, "o", "r")

	// when: finding a PR body for the commit
	body, found, err := gh.CommitPullRequestBody(context.Background(), "abc123")

	// then: only the exact merge-commit PR body is returned
	testastic.NoError(t, err)
	testastic.True(t, found)
	testastic.Equal(t, "override body", body)
}

func TestGitLabCreateReleasePR(t *testing.T) {
	t.Parallel()

	// given: a GitLab API that creates a merge request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests":
			w.WriteHeader(http.StatusCreated)
			writeJSON(t, w, map[string]any{
				"iid":         42,
				"title":       "chore: release v1.0.0",
				"description": "release body",
				"web_url":     "https://gitlab.com/o/r/-/merge_requests/42",
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

	// when: creating a release MR
	pr, err := gl.CreateReleasePR(context.Background(), provider.ReleasePROptions{
		Title:         "chore: release v1.0.0",
		Body:          "release body",
		ReleaseBranch: "yeet/release-main",
		BaseBranch:    "main",
	})

	// then: the MR is returned with correct fields
	testastic.NoError(t, err)
	testastic.Equal(t, 42, pr.Number)
	testastic.Equal(t, "chore: release v1.0.0", pr.Title)
	testastic.Equal(t, "yeet/release-main", pr.Branch)
}

func TestGitLabUpdateReleasePR(t *testing.T) {
	t.Parallel()

	// given: a GitLab API that updates a merge request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
			writeJSON(t, w, map[string]any{
				"iid":         42,
				"title":       "chore: release v1.1.0",
				"description": "updated body",
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

	// when: updating the release MR
	err = gl.UpdateReleasePR(context.Background(), 42, provider.ReleasePROptions{
		Title: "chore: release v1.1.0",
		Body:  "updated body",
	})

	// then: no error
	testastic.NoError(t, err)
}

func TestGitLabCommitPullRequestBody(t *testing.T) {
	t.Parallel()

	// given: GitLab returns MRs associated with a commit, but only one is the merge commit
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() ==
			"/api/v4/projects/o%2Fr/repository/commits/abc123/merge_requests":
			writeJSON(t, w, []map[string]any{
				{
					"iid":              1,
					"description":      "wrong body",
					"merge_commit_sha": "def456",
				},
				{
					"iid":              2,
					"description":      "override body",
					"merge_commit_sha": "abc123",
				},
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

	// when: finding an MR body for the commit
	body, found, err := gl.CommitPullRequestBody(context.Background(), "abc123")

	// then: only the exact merge-commit MR body is returned
	testastic.NoError(t, err)
	testastic.True(t, found)
	testastic.Equal(t, "override body", body)
}

func TestGitLabFindOpenPendingReleasePRs(t *testing.T) {
	t.Parallel()

	// given: GitLab returns open MRs with one matching release branch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests":
			writeJSON(t, w, []map[string]any{
				{
					"iid":           10,
					"title":         "chore: release v2.0.0",
					"description":   "mr body",
					"web_url":       "https://gitlab.com/o/r/-/merge_requests/10",
					"source_branch": "yeet/release-main",
					"state":         "opened",
				},
				{
					"iid":           11,
					"title":         "feature mr",
					"description":   "",
					"web_url":       "https://gitlab.com/o/r/-/merge_requests/11",
					"source_branch": "feature/something",
					"state":         "opened",
				},
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

	// when: finding open pending release MRs
	prs, err := gl.FindOpenPendingReleasePRs(context.Background(), "main")

	// then: only the release MR is returned
	testastic.NoError(t, err)
	testastic.Equal(t, 1, len(prs))
	testastic.Equal(t, 10, prs[0].Number)
	testastic.Equal(t, "yeet/release-main", prs[0].Branch)
}

func TestGitLabFindMergedReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("finds merged release MR", func(t *testing.T) {
		t.Parallel()

		// given: GitLab returns merged MRs with one matching release branch
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests":
				writeJSON(t, w, []map[string]any{
					{
						"iid":              5,
						"title":            "chore: release v1.0.0",
						"description":      "merged mr",
						"web_url":          "https://gitlab.com/o/r/-/merge_requests/5",
						"source_branch":    "yeet/release-main",
						"state":            "merged",
						"merge_commit_sha": "abc123",
					},
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

		// when: finding merged release MR
		pr, err := gl.FindMergedReleasePR(context.Background(), "main")

		// then: the merged MR is returned with merge commit SHA
		testastic.NoError(t, err)
		testastic.Equal(t, 5, pr.Number)
		testastic.Equal(t, "abc123", pr.MergeCommitSHA)
	})

	t.Run("returns ErrNoPR when none found", func(t *testing.T) {
		t.Parallel()

		// given: GitLab returns no matching MRs
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests":
				writeJSON(t, w, []map[string]any{})
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

		// when: finding merged release MR
		_, err = gl.FindMergedReleasePR(context.Background(), "main")

		// then: ErrNoPR is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrNoPR)
	})
}

func TestGitLabEnsureLabel(t *testing.T) {
	t.Parallel()

	t.Run("creates label when not found", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab API where the label does not exist
		var created atomic.Bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.Contains(r.URL.EscapedPath(), "/labels/"):
				w.WriteHeader(http.StatusNotFound)
				writeJSON(t, w, map[string]any{"message": "404 Not Found"})
			case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/labels":
				created.Store(true)
				w.WriteHeader(http.StatusCreated)
				writeJSON(t, w, map[string]any{"name": provider.ReleaseLabelPending})
			case r.Method == http.MethodPut && strings.Contains(r.URL.EscapedPath(), "/merge_requests/"):
				writeJSON(t, w, map[string]any{"iid": 1})
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

		// when: marking an MR as pending (triggers ensureReleaseLabels)
		err = gl.MarkReleasePRPending(context.Background(), 1)

		// then: labels are created and no error is returned
		testastic.NoError(t, err)
		testastic.True(t, created.Load())
	})
}

func TestGitLabMergeReleasePRMethods(t *testing.T) {
	t.Parallel()

	t.Run("auto method succeeds", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab API with an open MR and project settings
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				writeJSON(t, w, map[string]any{
					"iid":                   1,
					"state":                 "opened",
					"draft":                 false,
					"has_conflicts":         false,
					"detailed_merge_status": "mergeable",
					"sha":                   "abc123",
				})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSON(t, w, map[string]any{
					"merge_method":  "merge",
					"squash_option": "default_off",
				})
			case r.Method == http.MethodPut && strings.Contains(r.URL.EscapedPath(), "/merge"):
				writeJSON(t, w, map[string]any{
					"iid":              1,
					"state":            "merged",
					"merge_commit_sha": "def456",
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

		// when: merging with auto method
		err = gl.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Force:  false,
			Method: provider.MergeMethodAuto,
		})

		// then: no error
		testastic.NoError(t, err)
	})

	t.Run("squash blocked by project settings", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab project with squash disabled
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				writeJSON(t, w, map[string]any{
					"iid":                   1,
					"state":                 "opened",
					"draft":                 false,
					"has_conflicts":         false,
					"detailed_merge_status": "mergeable",
					"sha":                   "abc123",
				})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSON(t, w, map[string]any{
					"merge_method":  "merge",
					"squash_option": "never",
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

		// when: merging with squash method
		err = gl.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Force:  false,
			Method: provider.MergeMethodSquash,
		})

		// then: merge is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
	})
}

func TestGitHubListTagsPaginationLimit(t *testing.T) {
	t.Parallel()

	// given: a GitHub API that always returns a next page
	var calls atomic.Int32

	var server *httptest.Server

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1))
		w.Header().Set(
			"Link",
			fmt.Sprintf(`<%s/repos/o/r/tags?per_page=100&page=%d>; rel="next"`, server.URL, n+1),
		)
		writeJSON(t, w, []map[string]any{{
			"name":   fmt.Sprintf("v0.0.%d", n),
			"commit": map[string]any{"sha": fmt.Sprintf("sha-%d", n)},
		}})
	}))
	defer server.Close()

	client := githubapi.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")

	gh := provider.NewGitHub(client, "o", "r")

	// when: listing tags from an effectively infinite repository
	_, err := gh.ListTags(context.Background())

	// then: pagination limit is enforced
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, provider.ErrPaginationLimitExceeded)
}

func TestGitLabListTagsPaginationLimit(t *testing.T) {
	t.Parallel()

	// given: a GitLab API that always returns a next page
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1))
		w.Header().Set("X-Next-Page", strconv.Itoa(n+1))
		writeJSON(t, w, []map[string]any{{
			"name": fmt.Sprintf("v0.0.%d", n),
			"commit": map[string]any{
				"id":         fmt.Sprintf("sha-%d", n),
				"message":    "tag commit",
				"created_at": "2026-01-01T00:00:00Z",
			},
		}})
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

	// when: listing tags from an effectively infinite repository
	_, err = gl.ListTags(context.Background())

	// then: pagination limit is enforced
	testastic.Error(t, err)
	testastic.ErrorIs(t, err, provider.ErrPaginationLimitExceeded)
}

// Compile-time interface checks.
var (
	_ provider.Provider = (*provider.GitHub)(nil)
	_ provider.Provider = (*provider.GitLab)(nil)
)

// Compile-time strategy checks.
var _ commit.BumpType = commit.BumpMajor
