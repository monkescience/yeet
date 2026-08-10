package fakeprovider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/provider"
)

// NewAzureContentProvider returns a real Azure DevOps adapter wired to a
// stateful content fake over content.
func NewAzureContentProvider(t *testing.T, content *RepoContent) forge.Provider {
	t.Helper()

	server := httptest.NewServer(newAzureContentHandler(t, content))
	t.Cleanup(server.Close)

	return provider.NewAzureDevOps(
		server.Client(),
		server.URL,
		"contoso-pat",
		ContentAzureOrg,
		ContentAzureOrg,
		ContentAzureProject,
		ContentAzureRepo,
	)
}

func newAzureContentHandler(t *testing.T, content *RepoContent) http.Handler {
	t.Helper()

	rootAPI := "/" + ContentAzureOrg + "/_apis"
	repoAPI := "/" + ContentAzureOrg + "/" + ContentAzureProject +
		"/_apis/git/repositories/" + ContentAzureRepo

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodOptions && path == rootAPI:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(azureResourceLocations)
		case r.Method == http.MethodGet && strings.EqualFold(path, rootAPI+"/ResourceAreas"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(azureResourceAreasEmpty)
		case r.Method == http.MethodGet && path == repoAPI+"/items":
			azureContentFile(
				w, content,
				r.URL.Query().Get("versionDescriptor.version"),
				strings.TrimPrefix(r.URL.Query().Get("path"), "/"),
			)
		case r.Method == http.MethodGet && path == repoAPI+"/refs":
			azureContentRefs(w, content, r.URL.Query().Get("filter"))
		case r.Method == http.MethodPost && path == repoAPI+"/refs":
			azureContentUpdateRefs(t, w, r)
		case r.Method == http.MethodPost && path == repoAPI+"/pushes":
			azureContentPush(t, w, r, content)
		default:
			t.Errorf("fakeprovider/azure content: unexpected request %s %s", r.Method, r.URL.String())
			http.Error(w, "unhandled", http.StatusNotImplemented)
		}
	})
}

func azureContentFile(w http.ResponseWriter, content *RepoContent, branch, path string) {
	blob, exists := content.read(branch, path)
	if !exists {
		http.Error(w, "not found", http.StatusNotFound)

		return
	}

	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(blob))
}

func azureContentRefs(w http.ResponseWriter, content *RepoContent, filter string) {
	branch := strings.TrimPrefix(filter, "heads/")

	tip, exists := content.tip(branch)
	if !exists {
		writeContentJSON(w, http.StatusOK, map[string]any{azureKeyCount: 0, azureKeyValue: []any{}})

		return
	}

	writeContentJSON(w, http.StatusOK, map[string]any{
		azureKeyCount: 1,
		azureKeyValue: []map[string]any{{
			gitlabKeyName:    "refs/heads/" + branch,
			azureKeyObjectID: tip,
		}},
	})
}

// azureContentUpdateRefs answers the branch reset without moving any blob,
// because Azure DevOps commits the content in the push that follows.
func azureContentUpdateRefs(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	var request []struct {
		Name        string `json:"name"`
		OldObjectID string `json:"oldObjectId"`
		NewObjectID string `json:"newObjectId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Errorf("fakeprovider/azure content: decode %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "invalid ref update", http.StatusBadRequest)

		return
	}

	results := make([]map[string]any, 0, len(request))
	for _, update := range request {
		results = append(results, map[string]any{
			gitlabKeyName:  update.Name,
			"oldObjectId":  update.OldObjectID,
			"newObjectId":  update.NewObjectID,
			"success":      true,
			"updateStatus": "succeeded",
		})
	}

	writeContentJSON(w, http.StatusOK, map[string]any{azureKeyCount: len(results), azureKeyValue: results})
}

type azurePushRequest struct {
	RefUpdates []struct {
		Name        string `json:"name"`
		OldObjectID string `json:"oldObjectId"`
	} `json:"refUpdates"`
	Commits []struct {
		Comment string `json:"comment"`
		Changes []struct {
			ChangeType string `json:"changeType"`
			Item       struct {
				Path string `json:"path"`
			} `json:"item"`
			NewContent struct {
				Content string `json:"content"`
			} `json:"newContent"`
		} `json:"changes"`
	} `json:"commits"`
}

func azureContentPush(t *testing.T, w http.ResponseWriter, r *http.Request, content *RepoContent) {
	t.Helper()

	var push azurePushRequest

	if err := json.NewDecoder(r.Body).Decode(&push); err != nil {
		t.Errorf("fakeprovider/azure content: decode %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "invalid push", http.StatusBadRequest)

		return
	}

	if len(push.RefUpdates) != 1 || len(push.Commits) != 1 {
		http.Error(w, "a push carries one ref update and one commit", http.StatusBadRequest)

		return
	}

	branch := strings.TrimPrefix(push.RefUpdates[0].Name, "refs/heads/")

	base, found := content.branchAtTip(push.RefUpdates[0].OldObjectID)
	if !found {
		http.Error(w, "push is not based on a known commit", http.StatusConflict)

		return
	}

	changes := make([]contentChange, 0, len(push.Commits[0].Changes))
	for _, change := range push.Commits[0].Changes {
		changes = append(changes, contentChange{
			path:    strings.TrimPrefix(change.Item.Path, "/"),
			content: change.NewContent.Content,
			exists:  change.ChangeType == contentChangeTypeEdit,
		})
	}

	if mismatch, conflicting := content.mismatchedChange(base, changes); conflicting {
		http.Error(
			w,
			"change type "+azureContentChangeType(mismatch.exists)+" is invalid for "+mismatch.path,
			http.StatusConflict,
		)

		return
	}

	content.commit(branch, base, push.Commits[0].Comment, changes)
	writeContentJSON(w, http.StatusOK, map[string]any{"pushId": 1})
}

func azureContentChangeType(exists bool) string {
	if exists {
		return contentChangeTypeEdit
	}

	return contentChangeTypeAdd
}
