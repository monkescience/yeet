package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	githubapi "github.com/google/go-github/v89/github"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/provider"
	gitlabapi "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	testReleaseLabelPending = "autorelease: pending"
	testReleaseLabelTagged  = "autorelease: tagged"
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
		testastic.Equal(t, "unable to parse remote URL: https://***@", err.Error())
	})

	t.Run("unknown remote error redacts username-only token", func(t *testing.T) {
		t.Parallel()

		// given: an unparseable remote URL with a token in the username position
		url := "https://ghp-secret-token@"

		// when: parsing the remote
		_, err := provider.ParseRemote(url)

		// then: the error hides the token
		testastic.ErrorIs(t, err, provider.ErrUnknownRemote)
		testastic.Equal(t, "unable to parse remote URL: https://***@", err.Error())
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

func TestGitHubCreateRelease(t *testing.T) {
	t.Parallel()

	// given: a GitHub provider backed by a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/v1.2.3":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/commits/main":
			writeJSON(t, w, map[string]any{"sha": "6865616473686131323300000000000000000000"})
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
			testastic.Equal(t, "6865616473686131323300000000000000000000", request.Object)
			testastic.Equal(t, "commit", request.Type)

			writeJSON(t, w, map[string]any{
				"tag":     request.Tag,
				"sha":     "7461676f626a6563747368610000000000000000",
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
			testastic.Equal(t, "7461676f626a6563747368610000000000000000", request.SHA)

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

func TestGitHubCreateReleaseReusesExistingTag(t *testing.T) {
	t.Parallel()

	// given: a GitHub repository where the target tag already exists
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/tags/v1.2.3":
			writeJSON(t, w, map[string]any{
				"ref": "refs/tags/v1.2.3",
				"object": map[string]any{
					"sha":  "6578697374696e67746167736861000000000000",
					"type": "tag",
				},
			})
		case isGitHubCreateReleaseRequest(r):
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

	// when: creating a release for the existing tag
	release, err := gh.CreateRelease(context.Background(), provider.ReleaseOptions{
		TagName: "v1.2.3",
		Ref:     "main",
		Name:    "v1.2.3",
		Body:    "release notes",
	})

	// then: the existing tag is reused without another tag creation request
	testastic.NoError(t, err)
	testastic.Equal(t, "v1.2.3", release.TagName)
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

func TestGitHubEnsureLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yeet     bool
		phase    provider.ReleasePRPhase
		expected []string
	}{
		{
			name:     "creates managed and lifecycle labels when not found",
			yeet:     true,
			phase:    provider.ReleasePRPhasePending,
			expected: []string{provider.ReleaseLabelYeet, testReleaseLabelPending, testReleaseLabelTagged},
		},
		{
			name:     "does not create managed label when disabled",
			yeet:     false,
			phase:    provider.ReleasePRPhasePending,
			expected: []string{testReleaseLabelPending, testReleaseLabelTagged},
		},
		{
			name:     "tagged transition recreates only the tagged label",
			yeet:     true,
			phase:    provider.ReleasePRPhaseTagged,
			expected: []string{testReleaseLabelTagged},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a GitHub API where the labels do not exist
			var created []string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/labels/"):
					w.WriteHeader(http.StatusNotFound)
					writeJSON(t, w, map[string]any{"message": "Not Found"})
				case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/labels":
					var request struct {
						Name string `json:"name"`
					}
					decodeJSONRequest(t, r, &request)
					created = append(created, request.Name)

					w.WriteHeader(http.StatusCreated)
					writeJSON(t, w, map[string]any{"name": request.Name})
				case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/42/labels":
					writeJSON(t, w, []map[string]any{{"name": testReleaseLabelPending}})
				case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/repos/o/r/issues/42/labels/"):
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.String())
				}
			}))
			defer server.Close()

			client := newGitHubTestClient(t, server)

			gh := provider.NewGitHub(client, "o", "r")
			labels := defaultReleasePRLabels()
			labels.Yeet = test.yeet

			// when: the requested phase is applied
			err := gh.SetReleasePRLabels(context.Background(), 42, labels, test.phase)

			// then: only definitions owned by that phase are created
			testastic.NoError(t, err)
			testastic.SliceEqual(t, test.expected, created)
		})
	}
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
		_, err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
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
		_, err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
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
		_, err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
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
		_, err := gh.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            provider.MergeMethodAuto,
		})

		// then: merge is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
	})
}

func TestGitLabFindMergedReleasePR(t *testing.T) {
	t.Parallel()

	t.Run("uses source tip for fast-forward merged MR", func(t *testing.T) {
		t.Parallel()

		// given: GitLab returns a fast-forward merged MR without merge or squash commit SHAs
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests":
				writeJSON(t, w, []map[string]any{
					{
						"iid":               6,
						"title":             "chore: release v1.0.0",
						"description":       "fast-forward merged mr",
						"web_url":           "https://gitlab.com/o/r/-/merge_requests/6",
						"source_branch":     "yeet/release-main",
						"source_project_id": 10,
						"target_project_id": 10,
						"state":             "merged",
						"sha":               "736f757263657469707368610000000000000000",
						"squash_on_merge":   false,
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

		// when: finding the fast-forward merged release MR
		pr, err := gl.FindMergedReleasePR(context.Background(), "main", testReleaseLabelPending)

		// then: the source tip identifies the commit now on the target branch
		testastic.NoError(t, err)
		testastic.Equal(t, 6, pr.Number)
		testastic.Equal(t, "736f757263657469707368610000000000000000", pr.MergeCommitSHA)
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
						"merge_commit_sha":  "7374616c65736861000000000000000000000000",
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
						"merge_commit_sha":  "6672657368736861000000000000000000000000",
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
		pr, err := gl.FindMergedReleasePR(context.Background(), "main", testReleaseLabelPending)

		// then: the most recently merged MR is returned, not the most recently updated
		testastic.NoError(t, err)
		testastic.Equal(t, 8, pr.Number)
		testastic.Equal(t, "6672657368736861000000000000000000000000", pr.MergeCommitSHA)
	})
}

func TestGitLabEnsureLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yeet     bool
		phase    provider.ReleasePRPhase
		expected []string
	}{
		{
			name:     "creates managed and lifecycle labels when not found",
			yeet:     true,
			phase:    provider.ReleasePRPhasePending,
			expected: []string{provider.ReleaseLabelYeet, testReleaseLabelPending, testReleaseLabelTagged},
		},
		{
			name:     "does not create managed label when disabled",
			yeet:     false,
			phase:    provider.ReleasePRPhasePending,
			expected: []string{testReleaseLabelPending, testReleaseLabelTagged},
		},
		{
			name:     "tagged transition recreates only the tagged label",
			yeet:     true,
			phase:    provider.ReleasePRPhaseTagged,
			expected: []string{testReleaseLabelTagged},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a GitLab API where the labels do not exist
			var created []string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && strings.Contains(r.URL.EscapedPath(), "/labels/"):
					w.WriteHeader(http.StatusNotFound)
					writeJSON(t, w, map[string]any{"message": "404 Not Found"})
				case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/labels":
					var request struct {
						Name string `json:"name"`
					}
					decodeJSONRequest(t, r, &request)
					created = append(created, request.Name)

					w.WriteHeader(http.StatusCreated)
					writeJSON(t, w, map[string]any{"name": request.Name})
				case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/42":
					writeJSON(t, w, map[string]any{"iid": 42})
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
			labels := defaultReleasePRLabels()
			labels.Yeet = test.yeet

			// when: the requested phase is applied
			err = gl.SetReleasePRLabels(context.Background(), 42, labels, test.phase)

			// then: only definitions owned by that phase are created
			testastic.NoError(t, err)
			testastic.SliceEqual(t, test.expected, created)
		})
	}
}

func TestGitLabMergeReleasePRMethods(t *testing.T) {
	t.Parallel()

	t.Run("auto method prefers squash when the project permits it", func(t *testing.T) {
		t.Parallel()

		var accepted struct {
			Squash *bool `json:"squash"`
		}

		// given: a GitLab project that allows squashing but does not force it
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
				err := json.NewDecoder(r.Body).Decode(&accepted)
				testastic.NoError(t, err)

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
		_, err = gl.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Method: provider.MergeMethodAuto,
		})

		// then: squash is requested
		testastic.NoError(t, err)
		testastic.Equal(t, true, accepted.Squash != nil && *accepted.Squash)
	})

	t.Run("auto method does not squash when the project forbids it", func(t *testing.T) {
		t.Parallel()

		var accepted struct {
			Squash *bool `json:"squash"`
		}

		// given: a GitLab project that forbids squashing
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
					"merge_method":  "rebase_merge",
					"squash_option": "never",
				})
			case r.Method == http.MethodPut && strings.Contains(r.URL.EscapedPath(), "/merge"):
				err := json.NewDecoder(r.Body).Decode(&accepted)
				testastic.NoError(t, err)

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
		_, err = gl.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Method: provider.MergeMethodAuto,
		})

		// then: the project's own merge method is left untouched
		testastic.NoError(t, err)
		testastic.Nil(t, accepted.Squash)
	})

	t.Run("auto method returns source tip for fast-forward merge", func(t *testing.T) {
		t.Parallel()

		// given: a GitLab project using fast-forward merges without squashing
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				writeJSON(t, w, map[string]any{
					"iid":                   1,
					"state":                 "opened",
					"draft":                 false,
					"has_conflicts":         false,
					"detailed_merge_status": "mergeable",
					"sha":                   "736f757263657469707368610000000000000000",
					"source_branch":         "yeet/release-main",
					"target_branch":         "main",
					"source_project_id":     10,
					"target_project_id":     10,
				})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSON(t, w, map[string]any{
					"merge_method":  "ff",
					"squash_option": "never",
				})
			case r.Method == http.MethodPut && strings.Contains(r.URL.EscapedPath(), "/merge"):
				writeJSON(t, w, map[string]any{
					"iid":             1,
					"state":           "merged",
					"sha":             "736f757263657469707368610000000000000000",
					"squash_on_merge": false,
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

		// when: merging with the project's fast-forward method
		mergeSHA, err := gl.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Method: provider.MergeMethodAuto,
		})

		// then: the source tip is returned as the final commit on the target branch
		testastic.NoError(t, err)
		testastic.Equal(t, "736f757263657469707368610000000000000000", mergeSHA)
	})

	t.Run("auto method waits for the asynchronous accept to finalize", func(t *testing.T) {
		t.Parallel()

		// given: GitLab accepts the MR asynchronously and reports it merged shortly after
		var accepted atomic.Bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				state := "opened"
				if accepted.Load() {
					state = "merged"
				}

				writeJSON(t, w, map[string]any{
					"iid":                   1,
					"state":                 state,
					"draft":                 false,
					"has_conflicts":         false,
					"detailed_merge_status": "mergeable",
					"sha":                   "736f757263657469707368610000000000000000",
					"source_branch":         "yeet/release-main",
					"target_branch":         "main",
					"source_project_id":     10,
					"target_project_id":     10,
				})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSON(t, w, map[string]any{
					"merge_method":  "ff",
					"squash_option": "never",
				})
			case r.Method == http.MethodPut && strings.Contains(r.URL.EscapedPath(), "/merge"):
				accepted.Store(true)

				writeJSON(t, w, map[string]any{
					"iid":   1,
					"state": "opened",
					"sha":   "736f757263657469707368610000000000000000",
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

		gl := provider.NewGitLab(client, "o/r", provider.WithMergePolling(time.Millisecond, 5*time.Second))

		// when: merging with the project's asynchronous fast-forward flow
		mergeSHA, err := gl.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Method: provider.MergeMethodAuto,
		})

		// then: the source tip is returned once GitLab reports the MR merged
		testastic.NoError(t, err)
		testastic.Equal(t, "736f757263657469707368610000000000000000", mergeSHA)
	})

	t.Run("auto method reports an accept that never finalizes", func(t *testing.T) {
		t.Parallel()

		// given: GitLab accepts the MR but never reports it merged
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr/merge_requests/1":
				writeJSON(t, w, map[string]any{
					"iid":                   1,
					"state":                 "opened",
					"draft":                 false,
					"has_conflicts":         false,
					"detailed_merge_status": "mergeable",
					"sha":                   "736f757263657469707368610000000000000000",
					"source_branch":         "yeet/release-main",
					"target_branch":         "main",
					"source_project_id":     10,
					"target_project_id":     10,
				})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/o%2Fr":
				writeJSON(t, w, map[string]any{
					"merge_method":  "ff",
					"squash_option": "never",
				})
			case r.Method == http.MethodPut && strings.Contains(r.URL.EscapedPath(), "/merge"):
				writeJSON(t, w, map[string]any{
					"iid":   1,
					"state": "opened",
					"sha":   "736f757263657469707368610000000000000000",
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

		gl := provider.NewGitLab(client, "o/r", provider.WithMergePolling(time.Millisecond, 50*time.Millisecond))

		// when: merging with the project's asynchronous fast-forward flow
		_, err = gl.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			Method: provider.MergeMethodAuto,
		})

		// then: the unfinalized merge is reported instead of an empty commit
		testastic.ErrorIs(t, err, provider.ErrMergeNotFinalized)
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
		_, err = gl.MergeReleasePR(context.Background(), 1, provider.MergeReleasePROptions{
			BypassMergeChecks: false,
			Method:            provider.MergeMethodSquash,
		})

		// then: merge is blocked
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
	})
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
