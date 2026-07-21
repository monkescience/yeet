package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	githubapi "github.com/google/go-github/v89/github"
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

	t.Run("unknown remote error redacts user and password", func(t *testing.T) {
		t.Parallel()

		// given: an unparseable remote URL with user:password credentials
		url := "https://ci:secret-token@"

		// when: parsing the remote
		_, err := provider.ParseRemote(url)

		// then: the error names the URL with the entire userinfo redacted
		testastic.ErrorIs(t, err, provider.ErrUnknownRemote)
		testastic.False(t, strings.Contains(err.Error(), "secret-token"))
		testastic.False(t, strings.Contains(err.Error(), "ci:"))
		testastic.True(t, strings.Contains(err.Error(), "https://***@"))
	})

	t.Run("unknown remote error redacts username-only token", func(t *testing.T) {
		t.Parallel()

		// given: an unparseable remote URL with a token in the username position
		url := "https://ghp-secret-token@"

		// when: parsing the remote
		_, err := provider.ParseRemote(url)

		// then: the error hides the token
		testastic.ErrorIs(t, err, provider.ErrUnknownRemote)
		testastic.False(t, strings.Contains(err.Error(), "ghp-secret-token"))
		testastic.True(t, strings.Contains(err.Error(), "https://***@"))
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

	t.Run("azure devops cloud https", func(t *testing.T) {
		t.Parallel()

		// given: a cloud Azure DevOps HTTPS remote
		url := "https://dev.azure.com/contoso/platform/_git/yeet"

		// when: parsing the remote
		info, err := provider.ParseRemote(url)

		// then: org, project, and repo are extracted under the cloud host
		testastic.NoError(t, err)
		testastic.Equal(t, "dev.azure.com", info.Host)
		testastic.Equal(t, "contoso", info.Organization)
		testastic.Equal(t, "platform", info.Project)
		testastic.Equal(t, "yeet", info.Repo)
	})

	t.Run("azure devops legacy visualstudio https", func(t *testing.T) {
		t.Parallel()

		// given: a legacy Azure DevOps remote where the org is the host subdomain
		url := "https://contoso.visualstudio.com/platform/_git/yeet"

		// when: parsing the remote
		info, err := provider.ParseRemote(url)

		// then: the host is normalized to dev.azure.com so the API base URL
		// resolves to dev.azure.com/{org}, not the broken {org}.visualstudio.com/{org}
		testastic.NoError(t, err)
		testastic.Equal(t, "dev.azure.com", info.Host)
		testastic.Equal(t, "contoso", info.Organization)
		testastic.Equal(t, "platform", info.Project)
		testastic.Equal(t, "yeet", info.Repo)
	})

	t.Run("azure devops ssh", func(t *testing.T) {
		t.Parallel()

		// given: an Azure DevOps SSH remote
		url := "git@ssh.dev.azure.com:v3/contoso/platform/yeet"

		// when: parsing the remote
		info, err := provider.ParseRemote(url)

		// then: org, project, and repo are extracted under the cloud host
		testastic.NoError(t, err)
		testastic.Equal(t, "dev.azure.com", info.Host)
		testastic.Equal(t, "contoso", info.Organization)
		testastic.Equal(t, "platform", info.Project)
		testastic.Equal(t, "yeet", info.Repo)
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

func TestDetectType(t *testing.T) {
	t.Parallel()

	t.Run("detects github hosts", func(t *testing.T) {
		t.Parallel()

		providerType, err := provider.DetectType("github.com")

		testastic.NoError(t, err)
		testastic.Equal(t, "github", providerType)
	})

	t.Run("detects gitlab hosts", func(t *testing.T) {
		t.Parallel()

		providerType, err := provider.DetectType("gitlab.com")

		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab", providerType)
	})

	t.Run("fails on github custom hosts", func(t *testing.T) {
		t.Parallel()

		_, err := provider.DetectType("github.company.com")

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
	})

	t.Run("fails on gitlab custom hosts", func(t *testing.T) {
		t.Parallel()

		_, err := provider.DetectType("gitlab.company.com")

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
	})

	t.Run("fails on unsupported hosts", func(t *testing.T) {
		t.Parallel()

		_, err := provider.DetectType("code.company.com")

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
	})
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

		client := newGitHubTestClient(t, server)

		gh := provider.NewGitHub(client, "o", "r")

		// when: resolving the latest version ref
		ref, err := gh.GetLatestVersionRef(context.Background())

		// then: the latest published release tag is preferred
		testastic.NoError(t, err)
		testastic.Equal(t, "v1.2.4", ref)
	})

	t.Run("latest release lookup does not fall back to tags", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub repository without a published release
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/repos/o/r/releases/latest" {
				t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
			}

			http.NotFound(w, r)
		}))
		defer server.Close()

		gh := provider.NewGitHub(newGitHubTestClient(t, server), "o", "r")

		// when: resolving only the latest provider release
		_, err := gh.GetLatestReleaseRef(context.Background())

		// then: the missing-release sentinel is returned without listing tags
		testastic.ErrorIs(t, err, provider.ErrNoRelease)
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

		client := newGitHubTestClient(t, server)

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

		client := newGitHubTestClient(t, server)

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

		client := newGitHubTestClient(t, server)

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

	// given: a merged pending release pull request returned by GitHub search
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/search/issues":
			testastic.Equal(t, `repo:o/r is:pr is:merged base:main label:"autorelease: pending"`, r.URL.Query().Get("q"))
			writeJSON(t, w, map[string]any{
				"total_count": 1,
				"items": []map[string]any{{
					"number": 42,
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/42":
			writeJSON(t, w, map[string]any{
				"number":    42,
				"title":     "chore: release 1.2.3",
				"body":      "<!-- yeet-release-tag: v1.2.3 -->",
				"html_url":  "https://example.com/pr/42",
				"merged_at": "2026-03-01T00:00:00Z",
				"head": map[string]any{
					"ref":  "yeet/release-main",
					"repo": map[string]any{"full_name": "o/r"},
				},
				"merge_commit_sha": "merge-sha",
			})
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newGitHubTestClient(t, server)

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

	t.Run("latest release lookup does not fall back to tags", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab repository without a published release
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v4/projects/o%2Fr/releases" {
				t.Fatalf("unexpected GitLab request: %s %s", r.Method, r.URL.String())
			}

			writeJSON(t, w, []map[string]any{})
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

		// when: resolving only the latest provider release
		_, err = gl.GetLatestReleaseRef(context.Background())

		// then: the missing-release sentinel is returned without listing tags
		testastic.ErrorIs(t, err, provider.ErrNoRelease)
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
					"commit": map[string]any{"id": "abc123"},
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

func TestGitHubCreateRelease(t *testing.T) {
	t.Parallel()

	// given: a GitHub provider backed by a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/v1.2.3":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/main":
			writeJSON(t, w, map[string]any{"sha": "head-sha-123"})
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			writeJSON(t, w, map[string]any{
				"login": "yeet-tester",
				"name":  "Yeet Tester",
				"email": "tester@yeet.dev",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/tags":
			var request struct {
				Tag     string `json:"tag"`
				Message string `json:"message"`
				Object  string `json:"object"`
				Type    string `json:"type"`
			}

			err := json.NewDecoder(r.Body).Decode(&request)
			testastic.NoError(t, err)
			testastic.Equal(t, "v1.2.3", request.Tag)
			testastic.Equal(t, "release notes", request.Message)
			testastic.Equal(t, "head-sha-123", request.Object)
			testastic.Equal(t, "commit", request.Type)

			writeJSON(t, w, map[string]any{
				"tag":     request.Tag,
				"sha":     "tag-object-sha",
				"message": request.Message,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
			var request struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			}

			err := json.NewDecoder(r.Body).Decode(&request)
			testastic.NoError(t, err)
			testastic.Equal(t, "refs/tags/v1.2.3", request.Ref)
			testastic.Equal(t, "tag-object-sha", request.SHA)

			writeJSON(t, w, map[string]any{
				"ref": request.Ref,
				"object": map[string]any{
					"sha":  request.SHA,
					"type": "tag",
				},
			})
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

	client := newGitHubTestClient(t, server)

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

func newGitHubTestClient(t *testing.T, server *httptest.Server) *githubapi.Client {
	t.Helper()

	baseURL := server.URL + "/"

	client, err := githubapi.NewClient(
		githubapi.WithHTTPClient(server.Client()),
		githubapi.WithURLs(&baseURL, &baseURL),
	)
	testastic.NoError(t, err)

	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(value)
	testastic.NoError(t, err)
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

		client := newGitHubTestClient(t, server)

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

		client := newGitHubTestClient(t, server)

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

		client := newGitHubTestClient(t, server)

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
	client, err := githubapi.NewClient()
	testastic.NoError(t, err)

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

	client := newGitHubTestClient(t, server)

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

	client := newGitHubTestClient(t, server)

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
						"head": map[string]any{
							"ref":  "yeet/release-main",
							"repo": map[string]any{"full_name": "o/r"},
						},
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

		client := newGitHubTestClient(t, server)

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

		client := newGitHubTestClient(t, server)

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

		client := newGitHubTestClient(t, server)

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

		client := newGitHubTestClient(t, server)

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
					"head": map[string]any{
						"sha":  "abc123",
						"ref":  "yeet/release-main",
						"repo": map[string]any{"full_name": "o/r"},
					},
					"base": map[string]any{"ref": "main"},
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

		client := newGitHubTestClient(t, server)

		gh := provider.NewGitHub(client, "o", "r")

		// when: merging with auto method
		err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            provider.MergeMethodAuto,
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
					"head": map[string]any{
						"sha":  "abc123",
						"ref":  "yeet/release-main",
						"repo": map[string]any{"full_name": "o/r"},
					},
					"base": map[string]any{"ref": "main"},
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

		client := newGitHubTestClient(t, server)

		gh := provider.NewGitHub(client, "o", "r")

		// when: merging with squash method (which is disabled)
		err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            provider.MergeMethodSquash,
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
					"head": map[string]any{
						"sha":  "abc123",
						"ref":  "yeet/release-main",
						"repo": map[string]any{"full_name": "o/r"},
					},
					"base": map[string]any{"ref": "main"},
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

		client := newGitHubTestClient(t, server)

		gh := provider.NewGitHub(client, "o", "r")

		// when: merging with auto method
		err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            provider.MergeMethodAuto,
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
					"head": map[string]any{
						"sha":  "abc123",
						"ref":  "yeet/release-main",
						"repo": map[string]any{"full_name": "o/r"},
					},
					"base": map[string]any{"ref": "main"},
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

		client := newGitHubTestClient(t, server)

		gh := provider.NewGitHub(client, "o", "r")

		// when: merging with auto method
		err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            provider.MergeMethodAuto,
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

	client := newGitHubTestClient(t, server)

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

func TestGitLabCommitPullRequestBodyPaginatesPastFirstPage(t *testing.T) {
	t.Parallel()

	// given: the matching merge-commit MR is only present on the second page
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() ==
			"/api/v4/projects/o%2Fr/repository/commits/abc123/merge_requests":
			switch r.URL.Query().Get("page") {
			case "":
				w.Header().Set("X-Next-Page", "2")
				writeJSON(t, w, []map[string]any{
					{
						"iid":              1,
						"description":      "wrong body",
						"merge_commit_sha": "def456",
					},
				})
			case "2":
				writeJSON(t, w, []map[string]any{
					{
						"iid":              2,
						"description":      "override body",
						"merge_commit_sha": "abc123",
					},
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

	// when: finding an MR body for the commit
	body, found, err := gl.CommitPullRequestBody(context.Background(), "abc123")

	// then: the MR on the second page is found
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
					"iid":               10,
					"title":             "chore: release v2.0.0",
					"description":       "mr body",
					"web_url":           "https://gitlab.com/o/r/-/merge_requests/10",
					"source_branch":     "yeet/release-main",
					"source_project_id": 10,
					"target_project_id": 10,
					"state":             "opened",
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
						"iid":               5,
						"title":             "chore: release v1.0.0",
						"description":       "merged mr",
						"web_url":           "https://gitlab.com/o/r/-/merge_requests/5",
						"source_branch":     "yeet/release-main",
						"source_project_id": 10,
						"target_project_id": 10,
						"state":             "merged",
						"merge_commit_sha":  "abc123",
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

	t.Run("selects the most recently merged release MR", func(t *testing.T) {
		t.Parallel()

		// given: two merged release MRs where the most recently updated one was
		// merged earlier than the other
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests":
				writeJSON(t, w, []map[string]any{
					{
						"iid":               7,
						"title":             "chore: release v1.0.0",
						"description":       "stale mr",
						"web_url":           "https://gitlab.com/o/r/-/merge_requests/7",
						"source_branch":     "yeet/release-main",
						"source_project_id": 10,
						"target_project_id": 10,
						"state":             "merged",
						"merge_commit_sha":  "stale-sha",
						"merged_at":         "2024-01-01T00:00:00Z",
						"updated_at":        "2024-06-01T00:00:00Z",
					},
					{
						"iid":               8,
						"title":             "chore: release v1.1.0",
						"description":       "fresh mr",
						"web_url":           "https://gitlab.com/o/r/-/merge_requests/8",
						"source_branch":     "yeet/release-main",
						"source_project_id": 10,
						"target_project_id": 10,
						"state":             "merged",
						"merge_commit_sha":  "fresh-sha",
						"merged_at":         "2024-05-01T00:00:00Z",
						"updated_at":        "2024-02-01T00:00:00Z",
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

		// then: the most recently merged MR is returned, not the most recently updated
		testastic.NoError(t, err)
		testastic.Equal(t, 8, pr.Number)
		testastic.Equal(t, "fresh-sha", pr.MergeCommitSHA)
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
					"source_branch":         "yeet/release-main",
					"target_branch":         "main",
					"source_project_id":     10,
					"target_project_id":     10,
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
			BypassMergeChecks: false,
			Method:            provider.MergeMethodAuto,
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
					"source_branch":         "yeet/release-main",
					"target_branch":         "main",
					"source_project_id":     10,
					"target_project_id":     10,
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
			BypassMergeChecks: false,
			Method:            provider.MergeMethodSquash,
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

	client := newGitHubTestClient(t, server)

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

func TestAzureDevOpsLatestReleaseRef(t *testing.T) {
	t.Parallel()

	// given: an Azure DevOps provider, which represents releases as tags
	az := provider.NewAzureDevOps(nil, "https://dev.azure.com", "", "org", "org", "proj", "repo")

	// when: resolving only a provider release object
	_, err := az.GetLatestReleaseRef(context.Background())

	// then: no release is reported so the history source can use its tag snapshot
	testastic.ErrorIs(t, err, provider.ErrNoRelease)
}

func TestMaxPRBodyLength(t *testing.T) {
	t.Parallel()

	t.Run("azure devops enforces its 4000 character limit", func(t *testing.T) {
		t.Parallel()

		// given: an Azure DevOps provider
		az := provider.NewAzureDevOps(http.DefaultClient, "https://dev.azure.com", "pat", "org", "org", "proj", "repo")

		// when: reading its max PR body length
		limit := az.MaxPRBodyLength()

		// then: the Azure DevOps hard limit is reported
		testastic.Equal(t, 4000, limit)
	})

	t.Run("github reports no enforced limit", func(t *testing.T) {
		t.Parallel()

		// given: a GitHub provider
		client, err := githubapi.NewClient()
		testastic.NoError(t, err)

		gh := provider.NewGitHub(client, "o", "r")

		// when: reading its max PR body length
		limit := gh.MaxPRBodyLength()

		// then: no limit is enforced
		testastic.Equal(t, 0, limit)
	})

	t.Run("gitlab reports no enforced limit", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab provider
		client, err := gitlabapi.NewClient("")
		testastic.NoError(t, err)

		gl := provider.NewGitLab(client, "o/r")

		// when: reading its max PR body length
		limit := gl.MaxPRBodyLength()

		// then: no limit is enforced
		testastic.Equal(t, 0, limit)
	})
}

var (
	_ provider.Provider = (*provider.GitHub)(nil)
	_ provider.Provider = (*provider.GitLab)(nil)
)

var _ commit.BumpType = commit.BumpMajor
