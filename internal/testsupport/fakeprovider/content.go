package fakeprovider

import (
	"encoding/json/v2"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"
)

func writeContentJSON(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_, _ = w.Write(body)
}

// Payload keys and change types repeated across the content fakes.
const (
	contentKeyPath        = "path"
	contentKeyTree        = "tree"
	contentChangeTypeEdit = "edit"
	contentChangeTypeAdd  = "add"
)

// Coordinates the content fakes are reachable at, so a caller can point a real
// adapter at one without restating them.
const (
	ContentOwner        = "o"
	ContentRepo         = "r"
	ContentProject      = "o/r"
	ContentAzureOrg     = "contoso-org"
	ContentAzureProject = "contoso-project"
	ContentAzureRepo    = "contoso-repo"
)

// RepoContent is branch-and-blob state shared by the three content fakes, so a
// test can seed a base branch, run any forge adapter against it, and read back
// the result in one vocabulary.
//
// It models force-update-from-base, which is what all three adapters do: the
// updated branch is the base branch plus the commit's changes, never whatever
// the branch held before.
type RepoContent struct {
	mu       sync.Mutex
	branches map[string]map[string]string
	tips     map[string]string
	commits  []ContentCommit
	reads    []ContentRead
	nextTip  int
}

// ContentCommit is one accepted write, with paths in the order the forge
// received them.
type ContentCommit struct {
	Branch  string
	Base    string
	Message string
	Paths   []string
}

// ContentRead is one blob read.
type ContentRead struct {
	Branch string
	Path   string
}

type contentChange struct {
	path    string
	content string
	exists  bool
}

// NewRepoContent returns content whose base branch exists and is empty.
func NewRepoContent(baseBranch string) *RepoContent {
	content := &RepoContent{
		branches: map[string]map[string]string{},
		tips:     map[string]string{},
	}

	content.mu.Lock()
	defer content.mu.Unlock()

	content.createBranch(baseBranch)

	return content
}

// Seed puts a blob on a branch before any adapter runs.
func (c *RepoContent) Seed(branch, path, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	files, exists := c.branches[branch]
	if !exists {
		files = c.createBranch(branch)
	}

	files[path] = content
}

// File returns a blob without recording a read.
func (c *RepoContent) File(branch, path string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	content, exists := c.branches[branch][path]

	return content, exists
}

// Commits returns the accepted writes in order.
func (c *RepoContent) Commits() []ContentCommit {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.commits)
}

// Reads returns the blob reads in order.
func (c *RepoContent) Reads() []ContentRead {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.reads)
}

func (c *RepoContent) paths(branch string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Sorted(maps.Keys(c.branches[branch]))
}

func (c *RepoContent) createBranch(branch string) map[string]string {
	files := map[string]string{}
	c.branches[branch] = files
	c.tips[branch] = c.newTip()

	return files
}

func (c *RepoContent) newTip() string {
	c.nextTip++

	return fmt.Sprintf("tip-%d", c.nextTip)
}

func (c *RepoContent) read(branch, path string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.reads = append(c.reads, ContentRead{Branch: branch, Path: path})

	content, exists := c.branches[branch][path]

	return content, exists
}

func (c *RepoContent) tip(branch string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tip, exists := c.tips[branch]

	return tip, exists
}

func (c *RepoContent) branchAtTip(sha string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for branch, tip := range c.tips {
		if tip == sha {
			return branch, true
		}
	}

	return "", false
}

// mismatchedChange reports the first change whose exists flag contradicts the
// base branch. GitLab picks create versus update from it and Azure DevOps add
// versus edit, and both APIs reject the wrong one.
func (c *RepoContent) mismatchedChange(base string, changes []contentChange) (contentChange, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, change := range changes {
		if _, present := c.branches[base][change.path]; present != change.exists {
			return change, true
		}
	}

	return contentChange{}, false
}

func (c *RepoContent) commit(branch, base, message string, changes []contentChange) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	files := maps.Clone(c.branches[base])
	if files == nil {
		files = map[string]string{}
	}

	paths := make([]string, 0, len(changes))

	for _, change := range changes {
		files[change.path] = change.content
		paths = append(paths, change.path)
	}

	c.branches[branch] = files

	tip := c.newTip()
	c.tips[branch] = tip

	c.commits = append(c.commits, ContentCommit{
		Branch:  branch,
		Base:    base,
		Message: message,
		Paths:   paths,
	})

	return tip
}
