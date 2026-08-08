package fakeprovider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/v89/github"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/provider"
)

// NewGitHubContentProvider returns a real GitHub adapter wired to a stateful
// content fake over content.
func NewGitHubContentProvider(t *testing.T, content *RepoContent) provider.Provider {
	t.Helper()

	server := httptest.NewServer(newGitHubContentHandler(t, content))
	t.Cleanup(server.Close)

	baseURL := server.URL + "/"

	client, err := github.NewClient(
		github.WithHTTPClient(server.Client()),
		github.WithURLs(&baseURL, &baseURL),
	)
	testastic.NoError(t, err)

	return provider.NewGitHub(client, ContentOwner, ContentRepo)
}

// gitHubContentState holds the tree and commit objects a push travels through,
// because GitHub commits a branch in four requests rather than one.
type gitHubContentState struct {
	mu          sync.Mutex
	branchBySHA map[string]string
	trees       map[string]gitHubPendingTree
	commits     map[string]gitHubPendingTree
	next        int
}

type gitHubPendingTree struct {
	base    string
	message string
	changes []contentChange
}

func newGitHubContentHandler(t *testing.T, content *RepoContent) http.Handler {
	t.Helper()

	state := &gitHubContentState{
		branchBySHA: map[string]string{},
		trees:       map[string]gitHubPendingTree{},
		commits:     map[string]gitHubPendingTree{},
	}

	prefix := "/repos/" + ContentOwner + "/" + ContentRepo

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, prefix+"/contents/"):
			gitHubContentFile(w, content, r.URL.Query().Get("ref"), strings.TrimPrefix(path, prefix+"/contents/"))
		case r.Method == http.MethodGet && strings.HasPrefix(path, prefix+"/git/ref/heads/"):
			gitHubContentRef(w, content, state, strings.TrimPrefix(path, prefix+"/git/ref/heads/"))
		case r.Method == http.MethodGet && strings.HasPrefix(path, prefix+"/git/commits/"):
			gitHubContentCommitObject(w, state, strings.TrimPrefix(path, prefix+"/git/commits/"))
		case r.Method == http.MethodPost && path == prefix+"/git/trees":
			gitHubContentCreateTree(t, w, r, state)
		case r.Method == http.MethodPost && path == prefix+"/git/commits":
			gitHubContentCreateCommit(t, w, r, state)
		case r.Method == http.MethodPost && path == prefix+"/git/refs":
			gitHubContentCreateRef(t, w, r, content, state)
		case r.Method == http.MethodPatch && strings.HasPrefix(path, prefix+"/git/refs/heads/"):
			gitHubContentUpdateRef(t, w, r, content, state, strings.TrimPrefix(path, prefix+"/git/refs/heads/"))
		default:
			t.Errorf("fakeprovider/github content: unexpected request %s %s", r.Method, r.URL.String())
			http.Error(w, "unhandled", http.StatusNotImplemented)
		}
	})
}

func gitHubContentFile(w http.ResponseWriter, content *RepoContent, ref, path string) {
	blob, exists := content.read(ref, path)
	if !exists {
		writeGitHubContentNotFound(w)

		return
	}

	writeJSON(w, map[string]any{
		"type":         "file",
		contentKeyPath: path,
		"encoding":     "base64",
		"content":      base64.StdEncoding.EncodeToString([]byte(blob)),
	})
}

func gitHubContentRef(w http.ResponseWriter, content *RepoContent, state *gitHubContentState, branch string) {
	tip, exists := content.tip(branch)
	if !exists {
		writeGitHubContentNotFound(w)

		return
	}

	state.mu.Lock()
	state.branchBySHA[tip] = branch
	state.mu.Unlock()

	writeGitHubContentRef(w, branch, tip)
}

func gitHubContentCommitObject(w http.ResponseWriter, state *gitHubContentState, sha string) {
	state.mu.Lock()
	branch := state.branchBySHA[sha]
	state.mu.Unlock()

	writeJSON(w, map[string]any{
		githubKeySHA:   sha,
		contentKeyTree: map[string]any{githubKeySHA: gitHubContentTreeSHA(branch)},
	})
}

func gitHubContentCreateTree(t *testing.T, w http.ResponseWriter, r *http.Request, state *gitHubContentState) {
	t.Helper()

	var request struct {
		BaseTree string `json:"base_tree"`
		Tree     []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"tree"`
	}

	if !decodeGitHubContentRequest(t, w, r, &request) {
		return
	}

	changes := make([]contentChange, 0, len(request.Tree))
	for _, entry := range request.Tree {
		changes = append(changes, contentChange{path: entry.Path, content: entry.Content})
	}

	state.mu.Lock()
	state.next++
	sha := fmt.Sprintf("tree-%d", state.next)
	state.trees[sha] = gitHubPendingTree{
		base:    strings.TrimPrefix(request.BaseTree, gitHubContentTreePrefix),
		changes: changes,
	}
	state.mu.Unlock()

	writeJSON(w, map[string]any{githubKeySHA: sha})
}

func gitHubContentCreateCommit(t *testing.T, w http.ResponseWriter, r *http.Request, state *gitHubContentState) {
	t.Helper()

	var request struct {
		Message string `json:"message"`
		Tree    string `json:"tree"`
	}

	if !decodeGitHubContentRequest(t, w, r, &request) {
		return
	}

	state.mu.Lock()
	pending := state.trees[request.Tree]
	pending.message = request.Message
	state.next++
	sha := fmt.Sprintf("commit-%d", state.next)
	state.commits[sha] = pending
	state.mu.Unlock()

	writeJSON(w, map[string]any{githubKeySHA: sha})
}

func gitHubContentCreateRef(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	content *RepoContent,
	state *gitHubContentState,
) {
	t.Helper()

	var request struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}

	if !decodeGitHubContentRequest(t, w, r, &request) {
		return
	}

	branch := strings.TrimPrefix(request.Ref, "refs/heads/")
	gitHubContentApply(w, content, state, branch, request.SHA)
}

func gitHubContentUpdateRef(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	content *RepoContent,
	state *gitHubContentState,
	branch string,
) {
	t.Helper()

	var request struct {
		SHA string `json:"sha"`
	}

	if !decodeGitHubContentRequest(t, w, r, &request) {
		return
	}

	gitHubContentApply(w, content, state, branch, request.SHA)
}

func gitHubContentApply(
	w http.ResponseWriter,
	content *RepoContent,
	state *gitHubContentState,
	branch, sha string,
) {
	state.mu.Lock()
	pending := state.commits[sha]
	state.mu.Unlock()

	tip := content.commit(branch, pending.base, pending.message, pending.changes)

	state.mu.Lock()
	state.branchBySHA[tip] = branch
	state.mu.Unlock()

	writeGitHubContentRef(w, branch, tip)
}

const gitHubContentTreePrefix = "tree:"

func gitHubContentTreeSHA(branch string) string {
	return gitHubContentTreePrefix + branch
}

func writeGitHubContentRef(w http.ResponseWriter, branch, sha string) {
	writeJSON(w, map[string]any{
		githubKeyRef:    "refs/heads/" + branch,
		githubKeyObject: map[string]any{githubKeySHA: sha, githubKeyType: githubKeyCommit},
	})
}

func writeGitHubContentNotFound(w http.ResponseWriter) {
	writeContentJSON(w, http.StatusNotFound, map[string]any{githubKeyMessage: "Not Found"})
}

func decodeGitHubContentRequest(t *testing.T, w http.ResponseWriter, r *http.Request, value any) bool {
	t.Helper()

	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		t.Errorf("fakeprovider/github content: decode %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "invalid request", http.StatusBadRequest)

		return false
	}

	return true
}
