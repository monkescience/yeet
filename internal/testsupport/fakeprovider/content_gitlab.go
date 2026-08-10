package fakeprovider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/provider"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// NewGitLabContentProvider returns a real GitLab adapter wired to a stateful
// content fake over content.
func NewGitLabContentProvider(t *testing.T, content *RepoContent) forge.Provider {
	t.Helper()

	server := httptest.NewServer(newGitLabContentHandler(t, content))
	t.Cleanup(server.Close)

	client, err := gitlab.NewClient(
		"",
		gitlab.WithBaseURL(server.URL),
		gitlab.WithHTTPClient(server.Client()),
		gitlab.WithoutRetries(),
	)
	testastic.NoError(t, err)

	return provider.NewGitLab(client, ContentProject)
}

func newGitLabContentHandler(t *testing.T, content *RepoContent) http.Handler {
	t.Helper()

	prefix := "/api/v4/projects/" + url.PathEscape(ContentProject)
	filesPrefix := prefix + "/repository/files/"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()

		switch {
		case r.Method == http.MethodGet &&
			strings.HasPrefix(path, filesPrefix) && strings.HasSuffix(path, "/raw"):
			gitLabContentFile(
				t, w, content,
				r.URL.Query().Get("ref"),
				strings.TrimSuffix(strings.TrimPrefix(path, filesPrefix), "/raw"),
			)
		case r.Method == http.MethodPost && path == prefix+"/repository/commits":
			gitLabContentCommit(t, w, r, content)
		default:
			t.Errorf("fakeprovider/gitlab content: unexpected request %s %s", r.Method, r.URL.String())
			http.Error(w, "unhandled", http.StatusNotImplemented)
		}
	})
}

func gitLabContentFile(t *testing.T, w http.ResponseWriter, content *RepoContent, ref, escapedPath string) {
	t.Helper()

	path, err := url.PathUnescape(escapedPath)
	if err != nil {
		t.Errorf("fakeprovider/gitlab content: undecodable file path %q: %v", escapedPath, err)
		http.Error(w, "invalid path", http.StatusBadRequest)

		return
	}

	blob, exists := content.read(ref, path)
	if !exists {
		writeGitLabContentError(w, http.StatusNotFound, "404 File Not Found")

		return
	}

	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(blob))
}

func gitLabContentCommit(t *testing.T, w http.ResponseWriter, r *http.Request, content *RepoContent) {
	t.Helper()

	var request struct {
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

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Errorf("fakeprovider/gitlab content: decode %s %s: %v", r.Method, r.URL.Path, err)
		writeGitLabContentError(w, http.StatusBadRequest, "invalid commit request")

		return
	}

	changes := make([]contentChange, 0, len(request.Actions))
	for _, action := range request.Actions {
		changes = append(changes, contentChange{
			path:    action.FilePath,
			content: action.Content,
			exists:  action.Action == string(gitlab.FileUpdate),
		})
	}

	if mismatch, found := content.mismatchedChange(request.StartBranch, changes); found {
		writeGitLabContentError(
			w,
			http.StatusBadRequest,
			"A file with this name "+gitLabContentExistence(mismatch.exists)+": "+mismatch.path,
		)

		return
	}

	content.commit(request.Branch, request.StartBranch, request.CommitMessage, changes)
	writeJSON(w, map[string]any{"id": "6e6577636f6d6d69747368610000000000000000"})
}

func gitLabContentExistence(wanted bool) string {
	if wanted {
		return "doesn't exist"
	}

	return "already exists"
}

func writeGitLabContentError(w http.ResponseWriter, status int, message string) {
	writeContentJSON(w, status, map[string]any{"message": message})
}
