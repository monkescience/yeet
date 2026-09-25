//nolint:testpackage // This test validates unexported release behavior.
package release

import (
	"context"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
)

func releaseNoteStub(commits ...history.CommitEntry) *providerStub {
	stub := newProviderStub()
	stub.latestRelease = &forge.Release{TagName: "v1.2.3"}
	stub.tagList = []string{"v1.2.3"}
	stub.commits = commits

	return stub
}

func TestReleaseNotes(t *testing.T) {
	t.Parallel()

	t.Run("override entries carry their own fences and ignore outside fences", func(t *testing.T) {
		t.Parallel()

		// given: an override whose entries have fences, one with a header-like line, and a malformed outside fence
		stub := releaseNoteStub(history.CommitEntry{
			Hash: "abcdef1234567890",
			Message: "fix: squashed\n\n```release-note\n### Outside heading\n```\n\n" +
				"BEGIN_COMMIT_OVERRIDE\nfeat(auth): add refresh\n\n```release-note\nRefresh tokens.\n\n" +
				"fix: not an entry\n```\n\nfix(api): return 401\n\n~~~release-note\nExpect 401.\n~~~\nEND_COMMIT_OVERRIDE",
		})
		r := newTestReleaser(t, config.Default(), stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: each override entry keeps its own note and the outside fence is ignored
		testastic.NoError(t, err)
		testastic.AssertFile(t,
			"testdata/release_notes/override_entries_carry_their_own_fences/changelog.expected.md",
			changelog.Render(result.Plans[0].Entry))
	})

	t.Run("ignores override markers inside a release-note fence", func(t *testing.T) {
		t.Parallel()

		// given: a feature whose note documents the override start marker
		stub := releaseNoteStub(history.CommitEntry{
			Hash:    "abcdef1234567890",
			Message: "feat: document overrides\n\n```release-note\nStart a block with BEGIN_COMMIT_OVERRIDE.\n```",
		})
		r := newTestReleaser(t, config.Default(), stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: the commit is parsed normally and keeps its note
		testastic.NoError(t, err)
		testastic.Equal(t, "Features", result.Plans[0].Entry.Sections[0].Heading)
		testastic.SliceEqual(t, []string{
			"- document overrides (abcdef1)",
			"  > Start a block with BEGIN_COMMIT_OVERRIDE.",
		}, result.Plans[0].Entry.Sections[0].Lines)
	})

	t.Run("ignores override markers inside an unclosed note", func(t *testing.T) {
		t.Parallel()

		// given: a feature with override-like text inside an unclosed note
		stub := releaseNoteStub(history.CommitEntry{
			Hash: "abcdef1234567890",
			Message: "feat: merge PR\n\n```release-note\nstray\n\n" +
				"BEGIN_COMMIT_OVERRIDE\nfeat: add x\nEND_COMMIT_OVERRIDE\n",
		})
		r := newTestReleaser(t, config.Default(), stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: only the real commit controls the bump and changelog
		testastic.NoError(t, err)
		testastic.Equal(t, "1.3.0", result.Plans[0].NextVersion)
		testastic.SliceEqual(t, []string{"- merge PR (abcdef1)"}, result.Plans[0].Entry.Sections[0].Lines)
	})

	t.Run("ignores fences on excluded commits", func(t *testing.T) {
		t.Parallel()

		// given: excluded commits with a malformed and a valid fence next to an included fix
		stub := releaseNoteStub(
			history.CommitEntry{Hash: "1111111111", Message: "chore: tidy\n\n```release-note\nunclosed"},
			history.CommitEntry{Hash: "2222222222", Message: "docs: guide\n\n```release-note\nRead the guide.\n```"},
			history.CommitEntry{Hash: "3333333333", Message: "fix: patch"},
		)
		r := newTestReleaser(t, config.Default(), stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: the release succeeds without a notes section
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Plans[0].Entry.Sections))
		testastic.Equal(t, "Bug Fixes", result.Plans[0].Entry.Sections[0].Heading)
	})
}

func TestReleaseNoteWarnings(t *testing.T) {
	t.Run("ignores breaking footer text inside an unclosed note", func(t *testing.T) {
		// given: a feature whose unclosed note contains a breaking-footer example
		logs := captureWarnings(t)
		stub := releaseNoteStub(history.CommitEntry{
			Hash:    "abcdef1234567890",
			Message: "feat: tidy\n\n```release-note\nNote.\n\nBREAKING CHANGE: gone",
		})
		r := newTestReleaser(t, config.Default(), stub)

		// when: calculating a release
		result, err := r.Release(context.Background(), true)

		// then: the note is skipped with a warning and the feature keeps its minor bump
		testastic.NoError(t, err)
		testastic.Contains(t, logs.String(), "skipped invalid release note")
		testastic.Equal(t, "1.3.0", result.Plans[0].NextVersion)
		testastic.SliceEqual(t, []string{"- tidy (abcdef1)"}, result.Plans[0].Entry.Sections[0].Lines)
	})

	for _, scenario := range []struct {
		name    string
		message string
		problem error
		hint    string
	}{
		{
			name:    "missing closing fence",
			message: "feat: add\n\n```release-note\nNote.",
			problem: commit.ErrReleaseNoteUnclosed,
			hint:    "close the release-note fence",
		},
		{
			name:    "empty fence",
			message: "feat: add\n\n```release-note\n```",
			problem: commit.ErrReleaseNoteEmpty,
			hint:    "write the note inside the release-note fence or remove the fence",
		},
		{
			name:    "section heading",
			message: "feat: add\n\n```release-note\n## Upgrade\n```",
			problem: commit.ErrReleaseNoteHeading,
			hint:    "use level 4 or deeper headings inside release notes",
		},
		{
			name:    "leaked inner fence",
			message: "feat: add\n\n```release-note\n```sh\nyeet\n```\n```",
			problem: commit.ErrReleaseNoteUnclosedCodeBlock,
			hint:    "use a longer backtick or a tilde outer fence around nested code blocks",
		},
		{
			name:    "unclosed HTML block",
			message: "feat: add\n\n```release-note\n<pre>\nyeet\n```",
			problem: commit.ErrReleaseNoteUnclosedHTML,
			hint:    "close the HTML block or escape its opening tag",
		},
		{
			name: "unclosed override entry fence",
			message: "chore: squash\n\nBEGIN_COMMIT_OVERRIDE\nfeat: add\n\n" +
				"```release-note\n\nBREAKING CHANGE: example only\nEND_COMMIT_OVERRIDE",
			problem: commit.ErrReleaseNoteUnclosed,
			hint:    "close the release-note fence",
		},
		{
			name:    "override entry fence",
			message: "chore: squash\n\nBEGIN_COMMIT_OVERRIDE\nfeat: add\n\n```release-note\n\n```\nEND_COMMIT_OVERRIDE",
			problem: commit.ErrReleaseNoteEmpty,
			hint:    "write the note inside the release-note fence or remove the fence",
		},
	} {
		t.Run("skips a note with "+scenario.name, func(t *testing.T) {
			// given: an included commit with a malformed release-note fence
			logs := captureWarnings(t)
			stub := releaseNoteStub(history.CommitEntry{Hash: "abcdef1234567890", Message: scenario.message})
			r := newTestReleaser(t, config.Default(), stub)

			// when: calculating a release
			result, err := r.Release(context.Background(), true)

			// then: the release keeps the commit without a note and warns with the commit, the problem and a hint
			testastic.NoError(t, err)
			testastic.SliceEqual(t, []string{"Features"}, sectionHeadingsOf(result.Plans[0].Entry.Sections))
			testastic.Contains(t, logs.String(), "skipped invalid release note")
			testastic.Contains(t, logs.String(), "commit=abcdef1234567890")
			testastic.Contains(t, logs.String(), scenario.problem.Error())
			testastic.Contains(t, logs.String(), scenario.hint)
		})
	}
}

func sectionHeadingsOf(sections []changelog.Section) []string {
	headings := make([]string, 0, len(sections))
	for _, section := range sections {
		headings = append(headings, section.Heading)
	}

	return headings
}

func TestCachedHiddenOverrideWarning(t *testing.T) {
	// given: a cached excluded commit whose first invalid note precedes an unclosed note hiding an override
	logs := captureWarnings(t)
	analyzer := &releaseAnalyzer{parseCache: make(map[string][]parsedCommit)}
	entries := []history.CommitEntry{{
		Hash: "abcdef1234567890",
		Message: "chore: squash\n\n```release-note\n```\n\n```release-note\nstray\n\n" +
			"BEGIN_COMMIT_OVERRIDE\nfeat: add x\nEND_COMMIT_OVERRIDE",
	}}
	target := config.ResolvedTarget{ID: "first", Changelog: config.Default().Changelog}
	_, err := analyzer.parseCommits(t.Context(), entries, target)
	testastic.NoError(t, err)
	logs.Reset()

	target.ID = "second"

	// when: another target parses the same cached commit
	commits, err := analyzer.parseCommits(t.Context(), entries, target)

	// then: its warning names the target and gives the recovery step while the excluded commit stays unchanged
	testastic.NoError(t, err)
	testastic.Equal(t, "chore", commits[0].Type)
	testastic.Contains(t, logs.String(), "ignored commit override inside unclosed release note")
	testastic.Contains(t, logs.String(), "target=second")
	testastic.Contains(t, logs.String(), "close the release-note fence before BEGIN_COMMIT_OVERRIDE")
}
