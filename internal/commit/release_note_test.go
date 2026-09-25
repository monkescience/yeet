package commit_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
)

func TestParseWithReleaseNote(t *testing.T) {
	t.Parallel()

	t.Run("keeps parsed fields identical to the message without the fence", func(t *testing.T) {
		t.Parallel()

		// given: a breaking commit with a fence containing footer-like lines and a Release-As footer
		withFence := "feat(config)!: rename targets\n\nBody text.\n\n" +
			"```release-note\nRefs: #999\nBREAKING CHANGE: fake\nRelease-As: 9.9.9\n```\n\n" +
			"BREAKING CHANGE: targets are units now\nRelease-As: 2.0.0\nRefs: #1"
		withoutFence := "feat(config)!: rename targets\n\nBody text.\n\n\n\n" +
			"BREAKING CHANGE: targets are units now\nRelease-As: 2.0.0\nRefs: #1"

		// when: extracting the note and parsing the remaining message
		parsed, err := commit.ParseWithReleaseNote(t.Context(), "abc1234", withFence)
		expected := commit.Parse(t.Context(), "abc1234", withoutFence)
		expected.Note = "Refs: #999\nBREAKING CHANGE: fake\nRelease-As: 9.9.9"

		// then: the fence content is the note and parsing matches the message without the fence
		testastic.NoError(t, err)
		testastic.Equal(t, "Refs: #999\nBREAKING CHANGE: fake\nRelease-As: 9.9.9", parsed.Note)
		testastic.DeepEqual(t, expected, parsed)
		testastic.Equal(t, "config", parsed.Scope)
		testastic.True(t, parsed.Breaking)
		testastic.DeepEqual(t, []commit.Footer{
			{Key: "BREAKING CHANGE", Value: "targets are units now"},
			{Key: "Release-As", Value: "2.0.0"},
			{Key: "Refs", Value: "#1"},
		}, parsed.Footers)
	})

	t.Run("does not make a fence-only breaking footer breaking", func(t *testing.T) {
		t.Parallel()

		// given: a feature whose only breaking footer is inside the fence
		message := "feat: add flag\n\n```release-note\nBREAKING CHANGE: not really\n```"

		// when: extracting the note and parsing the remaining message
		parsed, err := commit.ParseWithReleaseNote(t.Context(), "abc1234", message)

		// then: the commit is not breaking and has no footers
		testastic.NoError(t, err)
		testastic.False(t, parsed.Breaking)
		testastic.Equal(t, 0, len(parsed.Footers))
	})

	t.Run("returns a message without a fence unchanged", func(t *testing.T) {
		t.Parallel()

		// given: a message with a non release-note code block and CRLF line endings
		message := "fix: patch\r\n\r\n```go\r\nfmt.Println()\r\n```\r\n"

		// when: extracting the note
		parsed, err := commit.ParseWithReleaseNote(t.Context(), "abc1234", message)

		// then: there is no note and the message is untouched
		testastic.NoError(t, err)
		testastic.Equal(t, "", parsed.Note)
		testastic.DeepEqual(t, commit.Parse(t.Context(), "abc1234", message), parsed)
	})

	t.Run("isolates footers inside an unclosed fence", func(t *testing.T) {
		t.Parallel()

		// given: an unclosed fence followed by a breaking footer
		message := "chore: tidy\n\n```release-note\nNote.\n\nBREAKING CHANGE: gone"

		// when: extracting the note and parsing the remaining message
		parsed, err := commit.ParseWithReleaseNote(t.Context(), "abc1234", message)

		// then: the invalid note is reported without promoting its contents to footers
		testastic.ErrorIs(t, err, commit.ErrReleaseNoteUnclosed)
		testastic.False(t, parsed.Breaking)
		testastic.Equal(t, 0, len(parsed.Footers))
	})

	t.Run("strips control characters before checking headings", func(t *testing.T) {
		t.Parallel()

		// given: a heading hidden behind a control character
		message := "feat: add flag\n\n```release-note\n\x01## v9.9.9\n```"

		// when: extracting the note
		_, err := commit.ParseWithReleaseNote(t.Context(), "abc1234", message)

		// then: the heading is rejected
		testastic.ErrorIs(t, err, commit.ErrReleaseNoteHeading)
	})

	t.Run("concatenates multiple fences in order", func(t *testing.T) {
		t.Parallel()

		// given: two release-note fences in one commit
		message := "feat: add flag\n\n```release-note\nFirst note.\n```\n\nBody.\n\n~~~release-note\nSecond note.\n~~~"

		// when: extracting the note
		parsed, err := commit.ParseWithReleaseNote(t.Context(), "abc1234", message)

		// then: the notes are joined in order and the body remains
		testastic.NoError(t, err)
		testastic.Equal(t, "First note.\n\nSecond note.", parsed.Note)
		testastic.Equal(t, "Body.", parsed.Body)
	})

	t.Run("ignores fences with other info strings", func(t *testing.T) {
		t.Parallel()

		// given: fences whose info string is not exactly release-note
		message := "feat: add flag\n\n```release-notes\nalias\n```\n\n```release-note yaml\nextra\n```"

		// when: extracting the note
		parsed, err := commit.ParseWithReleaseNote(t.Context(), "abc1234", message)

		// then: nothing is extracted
		testastic.NoError(t, err)
		testastic.Equal(t, "", parsed.Note)
		testastic.DeepEqual(t, commit.Parse(t.Context(), "abc1234", message), parsed)
	})

	t.Run("keeps nested code blocks and deep headings", func(t *testing.T) {
		t.Parallel()

		// given: a four-backtick outer fence and a tilde outer fence with inner backtick blocks
		message := "feat: add flag\n\n" +
			"````release-note\n#### Before\n\n```yaml\ntargets: []\n```\n````\n\n" +
			"~~~release-note\n```sh\nyeet release\n```\n~~~"

		// when: extracting the note
		parsed, err := commit.ParseWithReleaseNote(t.Context(), "abc1234", message)

		// then: the inner blocks are preserved verbatim
		testastic.NoError(t, err)
		testastic.Equal(t, "#### Before\n\n```yaml\ntargets: []\n```\n\n```sh\nyeet release\n```", parsed.Note)
	})

	for _, scenario := range []struct {
		name    string
		message string
		want    error
	}{
		{
			name:    "fails on a fence without closing fence",
			message: "feat: add flag\n\n```release-note\nNote.\n\nBREAKING CHANGE: gone",
			want:    commit.ErrReleaseNoteUnclosed,
		},
		{
			name:    "fails on an empty fence",
			message: "feat: add flag\n\n```release-note\n\n  \n```",
			want:    commit.ErrReleaseNoteEmpty,
		},
		{
			name:    "fails on a level 3 heading",
			message: "feat: add flag\n\n```release-note\n### Migration\n\nDo it.\n```",
			want:    commit.ErrReleaseNoteHeading,
		},
		{
			name:    "fails on a level 1 heading",
			message: "feat: add flag\n\n```release-note\n# Migration\n```",
			want:    commit.ErrReleaseNoteHeading,
		},
		{
			name:    "fails on a leaked inner fence",
			message: "feat: add flag\n\n```release-note\nRename:\n\n```yaml\nunits: []\n```\n```",
			want:    commit.ErrReleaseNoteUnclosedCodeBlock,
		},
		{
			name:    "fails on an unclosed HTML block",
			message: "feat: add flag\n\n```release-note\nRun:\n\n<pre>\nyeet release\n```",
			want:    commit.ErrReleaseNoteUnclosedHTML,
		},
		{
			name:    "fails on an unclosed HTML block in a blockquote",
			message: "feat: add flag\n\n```release-note\n> <pre>\n> yeet release\n```",
			want:    commit.ErrReleaseNoteUnclosedHTML,
		},
		{
			name:    "fails on an unclosed HTML block in a list",
			message: "feat: add flag\n\n```release-note\n- <pre>\n  yeet release\n```",
			want:    commit.ErrReleaseNoteUnclosedHTML,
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a commit message with a malformed release-note fence

			// when: extracting the note
			parsed, err := commit.ParseWithReleaseNote(t.Context(), "abc1234", scenario.message)

			// then: the matching problem is reported and no note is returned
			testastic.ErrorIs(t, err, scenario.want)
			testastic.Equal(t, "", parsed.Note)
		})
	}
}

func TestReleaseNoteLines(t *testing.T) {
	t.Parallel()

	t.Run("marks the lines of a closed fence", func(t *testing.T) {
		t.Parallel()

		// given: text with a fenced block between plain lines
		text := "feat: a\n\n```release-note\nfix: b\n```\nafter"

		// when: computing the fenced lines
		inside := commit.ReleaseNoteLines(text)

		// then: only the fence lines are marked
		testastic.DeepEqual(t, []bool{false, false, true, true, true, false}, inside)
	})

	t.Run("marks an unclosed release-note fence", func(t *testing.T) {
		t.Parallel()

		// given: text whose fence never closes
		text := "feat: a\n```release-note\n\nfix: b"

		// when: computing the fenced lines
		inside := commit.ReleaseNoteLines(text)

		// then: the whole unclosed note is isolated
		testastic.DeepEqual(t, []bool{false, true, true, true}, inside)
	})
}

func TestUnclosedBlockOffsets(t *testing.T) {
	t.Parallel()

	t.Run("reports the first unclosed top-level block", func(t *testing.T) {
		t.Parallel()

		// given: an unclosed HTML block and an unclosed fence after closed ones
		text := "<pre>closed</pre>\n\n```\ncode\n```\n\n<script>\n\nrest\n\n```\nopen"

		// when: finding unclosed block openings
		openings := commit.UnclosedBlockOffsets(text)

		// then: only the unclosed HTML block opening is reported, since it swallows the later fence
		testastic.DeepEqual(t, []int{33}, openings)
	})

	t.Run("reports unclosed HTML blocks nested in containers", func(t *testing.T) {
		t.Parallel()

		// given: unclosed HTML blocks inside a blockquote and a list item
		text := "> <pre>\n> quoted\n\n- <script>\n  listed"

		// when: finding unclosed block openings
		openings := commit.UnclosedBlockOffsets(text)

		// then: each opening tag is reported at its own offset
		testastic.DeepEqual(t, []int{2, 20}, openings)
	})
}
