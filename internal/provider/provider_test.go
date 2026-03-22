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
			}

			err := json.NewDecoder(r.Body).Decode(&request)
			testastic.NoError(t, err)
			testastic.Equal(t, "v1.2.3", request.TagName)
			testastic.Equal(t, "main", request.TargetCommitish)
			testastic.Equal(t, "v1.2.3", request.Name)
			testastic.Equal(t, "release notes", request.Body)

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
		TagName: "v1.2.3",
		Ref:     "main",
		Name:    "v1.2.3",
		Body:    "release notes",
	})

	// then: target_commitish is forwarded to GitHub
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

// Compile-time interface checks.
var (
	_ provider.Provider = (*provider.GitHub)(nil)
	_ provider.Provider = (*provider.GitLab)(nil)
)

// Compile-time strategy checks.
var _ commit.BumpType = commit.BumpMajor
