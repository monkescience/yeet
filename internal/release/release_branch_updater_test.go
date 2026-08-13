//nolint:testpackage // This test validates unexported release branch update behavior.
package release

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/internal/versionfile"
)

const updateFilesCommitSubject = "commit subject"

var errUnexpectedHistoryScan = errors.New("release branch update must not scan commit history")

// baseFileSource rejects history scans because an empty range would falsely report success.
type baseFileSource struct {
	files releaseFileProvider
}

func (s baseFileSource) GetFile(ctx context.Context, branch, path string) (string, error) {
	return s.files.GetFile(ctx, branch, path)
}

func (baseFileSource) ListTags(context.Context) ([]string, error) {
	return nil, errUnexpectedHistoryScan
}

func (baseFileSource) GetCommitsSinceRefs(
	context.Context,
	[]string,
	string,
	bool,
	[]forge.TagRef,
) (history.CommitHistory, error) {
	return history.CommitHistory{}, errUnexpectedHistoryScan
}

type updateFilesForge struct {
	name        string
	newProvider func(t *testing.T, content *fakeprovider.RepoContent) forge.Provider
}

func updateFilesForges() []updateFilesForge {
	return []updateFilesForge{
		{name: "github", newProvider: fakeprovider.NewGitHubContentProvider},
		{name: "gitlab", newProvider: fakeprovider.NewGitLabContentProvider},
		{name: "azuredevops", newProvider: fakeprovider.NewAzureContentProvider},
	}
}

func newUpdateFilesUpdater(t *testing.T, cfg *config.Config, forge forge.Provider) *releaseBranchUpdater {
	t.Helper()

	return newUpdateFilesUpdaterWithSource(t, cfg, forge, baseFileSource{files: forge})
}

func newUpdateFilesUpdaterWithSource(
	t *testing.T,
	cfg *config.Config,
	forge forge.Provider,
	source releaseSource,
) *releaseBranchUpdater {
	t.Helper()

	if len(cfg.Targets) == 0 {
		cfg.Targets = map[string]config.Target{
			"default": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
	}

	core, err := newReleaseCore(t.Context(), cfg, forge)
	testastic.NoError(t, err)

	return newReleaseBranchUpdater(core, source, forge)
}

func assertUpdateFilesCommit(
	t *testing.T,
	content *fakeprovider.RepoContent,
	branch, base string,
	paths ...string,
) {
	t.Helper()

	commits := content.Commits()

	testastic.Equal(t, 1, len(commits))

	if len(commits) != 1 {
		return
	}

	testastic.Equal(t, branch, commits[0].Branch)
	testastic.Equal(t, base, commits[0].Base)
	testastic.Equal(t, updateFilesCommitSubject, commits[0].Message)
	testastic.Equal(t, strings.Join(paths, "\n"), strings.Join(commits[0].Paths, "\n"))
	testastic.True(t, slices.IsSorted(commits[0].Paths))
}

func TestUpdateReleaseBranchFiles(t *testing.T) {
	t.Parallel()

	for _, adapter := range updateFilesForges() {
		t.Run(adapter.name, func(t *testing.T) {
			t.Parallel()

			t.Run("creates missing changelog with top-level header", func(t *testing.T) {
				t.Parallel()

				// given: a base branch without a changelog file
				cfg := config.Default()
				content := fakeprovider.NewRepoContent(cfg.Branch)
				updater := newUpdateFilesUpdater(t, cfg, adapter.newProvider(t, content))

				branch := "yeet/release-v0.1.0"
				plans := []TargetPlan{{
					ID:          "default",
					NextVersion: "0.1.0",
					NextTag:     "v0.1.0",
					Entry: changelog.ParseEntry(readTestFile(
						t,
						"testdata/update_release_branch_files/"+
							"creates_missing_changelog_with_top_level_header/changelog.input.md",
					)),
				}}

				// when: updating release branch files
				err := updater.updateFiles(t.Context(), branch, plans, updateFilesCommitSubject)

				// then: the forge accepted the create and wrote the release-please style header
				testastic.NoError(t, err)

				updated, exists := content.File(branch, cfg.Changelog.File)
				testastic.True(t, exists)
				testastic.AssertFile(
					t,
					"testdata/update_release_branch_files/creates_missing_changelog_with_top_level_header/"+
						"changelog.expected.md",
					strings.TrimSpace(updated),
				)
				assertUpdateFilesCommit(t, content, branch, cfg.Branch, cfg.Changelog.File)
			})

			t.Run("updates configured version files", func(t *testing.T) {
				t.Parallel()

				// given: one configured version file containing yeet markers
				cfg := config.Default()
				cfg.VersionFiles = []config.VersionFile{{Path: "VERSION.txt"}}

				content := fakeprovider.NewRepoContent(cfg.Branch)
				content.Seed(cfg.Branch, "VERSION.txt", "version=1.2.3 # x-yeet-version")

				updater := newUpdateFilesUpdater(t, cfg, adapter.newProvider(t, content))

				branch := "yeet/release-v1.2.4"
				plans := []TargetPlan{{
					ID:          "default",
					NextVersion: "1.2.4",
					NextTag:     "v1.2.4",
					Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
				}}

				// when: updating release branch files
				err := updater.updateFiles(t.Context(), branch, plans, updateFilesCommitSubject)

				// then: both files land in one commit with the version rewritten
				testastic.NoError(t, err)
				assertUpdateFilesCommit(
					t, content, branch, cfg.Branch,
					cfg.Changelog.File, "VERSION.txt",
				)

				updated, exists := content.File(branch, "VERSION.txt")
				testastic.True(t, exists)
				testastic.Equal(t, "version=1.2.4 # x-yeet-version", updated)
			})

			t.Run("rejects a changelog that collides with another target's version file", func(t *testing.T) {
				t.Parallel()

				// given: one target's version file sharing a path with another target's changelog
				cfg := config.Default()
				cfg.Targets = map[string]config.Target{
					"api": {
						Type:         config.TargetTypePath,
						Path:         "services/api",
						TagPrefix:    "api-v",
						Changelog:    config.ChangelogConfig{File: "api/CHANGELOG.md"},
						VersionFiles: []config.VersionFile{{Path: "shared.md"}},
					},
					"web": {
						Type:      config.TargetTypePath,
						Path:      "apps/web",
						TagPrefix: "web-v",
						Changelog: config.ChangelogConfig{File: "shared.md"},
					},
				}

				content := fakeprovider.NewRepoContent(cfg.Branch)
				content.Seed(cfg.Branch, "shared.md", "version=1.2.3 # x-yeet-version")
				content.Seed(cfg.Branch, "api/CHANGELOG.md", "# Changelog\n")

				updater := newUpdateFilesUpdater(t, cfg, adapter.newProvider(t, content))

				plans := []TargetPlan{
					{
						ID:          "api",
						NextVersion: "1.2.4",
						NextTag:     "api-v1.2.4",
						Entry:       changelog.ParseEntry("## api-v1.2.4 (2026-03-01)\n"),
					},
					{
						ID:          "web",
						NextVersion: "2.3.4",
						NextTag:     "web-v2.3.4",
						Entry:       changelog.ParseEntry("## web-v2.3.4 (2026-03-01)\n"),
					},
				}

				// when: updating release branch files
				err := updater.updateFiles(t.Context(), "yeet/release-main", plans, updateFilesCommitSubject)

				// then: the collision is reported instead of prepending markdown into the version file
				testastic.ErrorIs(t, err, errConflictingFileUpdate)
				testastic.Equal(t, 0, len(content.Commits()))
			})

			t.Run("reads base files only from the local release source", func(t *testing.T) {
				t.Parallel()

				// given: separate local and provider sources holding the same two base files
				cfg := config.Default()
				cfg.VersionFiles = []config.VersionFile{{Path: "VERSION.txt"}}
				cfg.Targets = map[string]config.Target{
					"default": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
				}

				local := fakeprovider.NewRepoContent(cfg.Branch)
				remote := fakeprovider.NewRepoContent(cfg.Branch)

				for _, content := range []*fakeprovider.RepoContent{local, remote} {
					content.Seed(cfg.Branch, cfg.Changelog.File, "# Changelog\n")
					content.Seed(cfg.Branch, "VERSION.txt", "version=1.2.3 # x-yeet-version")
				}

				updater := newUpdateFilesUpdaterWithSource(
					t,
					cfg,
					adapter.newProvider(t, remote),
					baseFileSource{files: adapter.newProvider(t, local)},
				)

				branch := "yeet/release-v1.2.4"
				plans := []TargetPlan{{
					ID:          "default",
					NextVersion: "1.2.4",
					NextTag:     "v1.2.4",
					Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
				}}

				// when: release branch files are prepared and written
				err := updater.updateFiles(t.Context(), branch, plans, updateFilesCommitSubject)

				// then: local blobs are read and the provider only receives one batched write
				testastic.NoError(t, err)
				testastic.Equal(t, 2, len(local.Reads()))
				testastic.Equal(t, 0, len(remote.Reads()))
				testastic.Equal(t, 0, len(local.Commits()))
				assertUpdateFilesCommit(
					t, remote, branch, cfg.Branch,
					cfg.Changelog.File, "VERSION.txt",
				)
			})

			t.Run("updates configured json version file", func(t *testing.T) {
				t.Parallel()

				// given: one configured JSON version file using an explicit pointer
				cfg := config.Default()
				cfg.VersionFiles = []config.VersionFile{{
					Path:        "package.json",
					Format:      config.VersionFileFormatJSON,
					JSONPointer: "/version",
				}}

				content := fakeprovider.NewRepoContent(cfg.Branch)
				content.Seed(cfg.Branch, "package.json", strings.Join([]string{
					`{`,
					`  "name": "app",`,
					`  "version": "1.2.3"`,
					`}`,
				}, "\n"))

				updater := newUpdateFilesUpdater(t, cfg, adapter.newProvider(t, content))

				branch := "yeet/release-v1.2.4"
				plans := []TargetPlan{{
					ID:          "default",
					NextVersion: "1.2.4",
					NextTag:     "v1.2.4",
					Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
				}}

				// when: updating release branch files
				err := updater.updateFiles(t.Context(), branch, plans, updateFilesCommitSubject)

				// then: changelog and JSON version file are updated
				testastic.NoError(t, err)
				assertUpdateFilesCommit(
					t, content, branch, cfg.Branch,
					cfg.Changelog.File, "package.json",
				)

				updated, exists := content.File(branch, "package.json")
				testastic.True(t, exists)
				testastic.Equal(t, strings.Join([]string{
					`{`,
					`  "name": "app",`,
					`  "version": "1.2.4"`,
					`}`,
				}, "\n"), updated)
			})

			t.Run("updates configured calver json version file", func(t *testing.T) {
				t.Parallel()

				// given: a calver release with one configured JSON version file
				cfg := config.Default()
				cfg.Versioning = config.VersioningCalVer
				cfg.VersionFiles = []config.VersionFile{{
					Path:        "package.json",
					Format:      config.VersionFileFormatJSON,
					JSONPointer: "/version",
				}}

				content := fakeprovider.NewRepoContent(cfg.Branch)
				content.Seed(cfg.Branch, "package.json", `{"name":"app","version":"2026.02.7"}`)

				updater := newUpdateFilesUpdater(t, cfg, adapter.newProvider(t, content))

				branch := "yeet/release-v2026.03.1"
				plans := []TargetPlan{{
					ID:          "default",
					NextVersion: "2026.03.1",
					NextTag:     "v2026.03.1",
					Entry:       changelog.ParseEntry("## v2026.03.1 (2026-03-01)\n"),
				}}

				// when: updating release branch files
				err := updater.updateFiles(t.Context(), branch, plans, updateFilesCommitSubject)

				// then: changelog and JSON version file are updated with the calver string
				testastic.NoError(t, err)
				assertUpdateFilesCommit(
					t, content, branch, cfg.Branch,
					cfg.Changelog.File, "package.json",
				)

				updated, exists := content.File(branch, "package.json")
				testastic.True(t, exists)
				testastic.Equal(t, `{"name":"app","version":"2026.03.1"}`, updated)
			})

			t.Run("fails when configured version file has no yeet markers", func(t *testing.T) {
				t.Parallel()

				// given: one configured version file without markers
				cfg := config.Default()
				cfg.VersionFiles = []config.VersionFile{{Path: "VERSION.txt"}}

				content := fakeprovider.NewRepoContent(cfg.Branch)
				content.Seed(cfg.Branch, "VERSION.txt", "version=1.2.3")

				updater := newUpdateFilesUpdater(t, cfg, adapter.newProvider(t, content))

				plans := []TargetPlan{{
					ID:          "default",
					NextVersion: "1.2.4",
					NextTag:     "v1.2.4",
					Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
				}}

				// when: updating release branch files
				err := updater.updateFiles(t.Context(), "yeet/release-v1.2.4", plans, updateFilesCommitSubject)

				// then: missing markers abort the release and nothing is written
				testastic.ErrorIs(t, err, versionfile.ErrNoMarkersFound)
				testastic.Equal(t, 0, len(content.Commits()))
			})

			t.Run("prepends changelog entry and normalizes headerless history", func(t *testing.T) {
				t.Parallel()

				// given: an existing changelog without a top header and a new release entry
				cfg := config.Default()

				content := fakeprovider.NewRepoContent(cfg.Branch)
				content.Seed(cfg.Branch, cfg.Changelog.File, strings.TrimSpace(readTestFile(
					t,
					"testdata/update_release_branch_files/"+
						"prepends_changelog_entry_and_normalizes_headerless_history/"+
						"existing_changelog.input.md",
				)))

				updater := newUpdateFilesUpdater(t, cfg, adapter.newProvider(t, content))

				branch := "yeet/release-v0.1.1"
				plans := []TargetPlan{{
					ID:          "default",
					NextVersion: "0.1.1",
					NextTag:     "v0.1.1",
					Entry: changelog.ParseEntry(readTestFile(
						t,
						"testdata/update_release_branch_files/"+
							"prepends_changelog_entry_and_normalizes_headerless_history/"+
							"changelog.input.md",
					)),
				}}

				// when: updating release branch files
				err := updater.updateFiles(t.Context(), branch, plans, updateFilesCommitSubject)

				// then: the new entry is prepended and the changelog gains a top-level header
				testastic.NoError(t, err)
				assertUpdateFilesCommit(t, content, branch, cfg.Branch, cfg.Changelog.File)

				updated, exists := content.File(branch, cfg.Changelog.File)
				testastic.True(t, exists)
				testastic.AssertFile(t, "testdata/update_release_branch_files_prepends_header.expected.md", updated)
			})

			t.Run("merges multiple target entries into a shared changelog file", func(t *testing.T) {
				t.Parallel()

				// given: two path targets that both write to the default shared changelog file
				cfg := config.Default()
				cfg.Targets = map[string]config.Target{
					"api": {
						Type:      config.TargetTypePath,
						Path:      "services/api",
						TagPrefix: "api-v",
					},
					"web": {
						Type:      config.TargetTypePath,
						Path:      "apps/web",
						TagPrefix: "web-v",
					},
				}

				content := fakeprovider.NewRepoContent(cfg.Branch)
				updater := newUpdateFilesUpdater(t, cfg, adapter.newProvider(t, content))

				branch := "yeet/release-wave"
				plans := []TargetPlan{
					{
						ID: "api",
						Entry: changelog.ParseEntry(readTestFile(
							t,
							"testdata/update_release_branch_files/"+
								"merges_multiple_target_entries_into_a_shared_changelog_file/"+
								"api_changelog.input.md",
						)),
					},
					{
						ID: "web",
						Entry: changelog.ParseEntry(readTestFile(
							t,
							"testdata/update_release_branch_files/"+
								"merges_multiple_target_entries_into_a_shared_changelog_file/"+
								"web_changelog.input.md",
						)),
					},
				}

				// when: updating release branch files
				err := updater.updateFiles(t.Context(), branch, plans, updateFilesCommitSubject)

				// then: the shared changelog carries both entries in one file
				testastic.NoError(t, err)
				assertUpdateFilesCommit(t, content, branch, cfg.Branch, cfg.Changelog.File)

				updated, exists := content.File(branch, cfg.Changelog.File)
				testastic.True(t, exists)
				testastic.AssertFile(t, "testdata/update_release_branch_files_shared_changelog.expected.md", updated)
			})

			t.Run("fails when configured version file is missing", func(t *testing.T) {
				t.Parallel()

				// given: a configured version file absent from the base branch
				cfg := config.Default()
				cfg.VersionFiles = []config.VersionFile{{Path: "VERSION.txt"}}

				content := fakeprovider.NewRepoContent(cfg.Branch)
				updater := newUpdateFilesUpdater(t, cfg, adapter.newProvider(t, content))

				plans := []TargetPlan{{
					ID:          "default",
					NextVersion: "1.2.4",
					NextTag:     "v1.2.4",
					Entry:       changelog.ParseEntry("## v1.2.4 (2026-03-01)\n"),
				}}

				// when: updating release branch files
				err := updater.updateFiles(t.Context(), "yeet/release-v1.2.4", plans, updateFilesCommitSubject)

				// then: the missing file is reported and nothing is written
				testastic.ErrorIs(t, err, forge.ErrFileNotFound)
				testastic.Equal(t, 0, len(content.Commits()))
			})
		})
	}
}
