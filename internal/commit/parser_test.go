package commit_test

import (
	"bytes"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
)

func TestParseLogging(t *testing.T) {
	// given: a non-conventional commit containing private text and debug logging
	var logOutput bytes.Buffer

	previousLogger := slog.Default()

	slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	// when: parsing the commit
	parsed := commit.Parse(t.Context(), "abc1234", "private customer incident details")

	// then: the commit is parsed without copying its text into the log
	testastic.Equal(t, "private customer incident details", parsed.Description)
	testastic.True(t, strings.Contains(logOutput.String(), `"hash":"abc1234"`))
	testastic.False(t, strings.Contains(logOutput.String(), "private customer incident details"))
}

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("simple feat", func(t *testing.T) {
		t.Parallel()

		// given: a simple feature commit message
		raw := "feat: add user authentication"

		// when: parsing the commit
		c := commit.Parse(t.Context(), "abc1234", raw)

		// then: type, description, and hash are extracted
		testastic.Equal(t, "feat", c.Type)
		testastic.Equal(t, "add user authentication", c.Description)
		testastic.Equal(t, "abc1234", c.Hash)
		testastic.False(t, c.Breaking)
		testastic.True(t, c.IsConventional())
	})

	t.Run("feat with scope", func(t *testing.T) {
		t.Parallel()

		// given: a feature commit with scope
		raw := "feat(auth): add OAuth2 support"

		// when: parsing the commit
		c := commit.Parse(t.Context(), "def5678", raw)

		// then: scope is extracted
		testastic.Equal(t, "feat", c.Type)
		testastic.Equal(t, "auth", c.Scope)
		testastic.Equal(t, "add OAuth2 support", c.Description)
	})

	t.Run("breaking change with bang", func(t *testing.T) {
		t.Parallel()

		// given: a commit with breaking change indicator
		raw := "feat(api)!: remove deprecated endpoints"

		// when: parsing the commit
		c := commit.Parse(t.Context(), "ghi9012", raw)

		// then: breaking flag is set
		testastic.Equal(t, "feat", c.Type)
		testastic.Equal(t, "api", c.Scope)
		testastic.True(t, c.Breaking)
	})

	t.Run("breaking change in footer", func(t *testing.T) {
		t.Parallel()

		// given: a commit with BREAKING CHANGE footer
		raw, err := os.ReadFile("testdata/breaking_in_footer/input.txt")
		testastic.NoError(t, err)

		// when: parsing the commit
		c := commit.Parse(t.Context(), "jkl3456", string(raw))

		// then: breaking flag is set and footer is parsed
		testastic.True(t, c.Breaking)
		testastic.Equal(t, "Some body text.", c.Body)
		testastic.Equal(t, 1, len(c.Footers))
		testastic.Equal(t, "BREAKING CHANGE", c.Footers[0].Key)
		testastic.Equal(t, "old auth tokens are no longer valid", c.Footers[0].Value)
	})

	t.Run("breaking change footer without space", func(t *testing.T) {
		t.Parallel()

		// given: a BREAKING CHANGE footer without a space after the separator
		raw := "feat: remove legacy API\n\nBREAKING CHANGE:drop legacy API"

		// when: parsing the commit
		c := commit.Parse(t.Context(), "nos1234", raw)

		// then: the footer is parsed and marks the commit as breaking
		testastic.True(t, c.Breaking)
		testastic.Equal(t, 1, len(c.Footers))
		testastic.Equal(t, "BREAKING CHANGE", c.Footers[0].Key)
		testastic.Equal(t, "drop legacy API", c.Footers[0].Value)
	})

	t.Run("fix commit", func(t *testing.T) {
		t.Parallel()

		// given: a fix commit
		raw := "fix: resolve null pointer in user handler"

		// when: parsing the commit
		c := commit.Parse(t.Context(), "mno7890", raw)

		// then: type is fix
		testastic.Equal(t, "fix", c.Type)
		testastic.Equal(t, "resolve null pointer in user handler", c.Description)
	})

	t.Run("non-conventional commit", func(t *testing.T) {
		t.Parallel()

		// given: a non-conventional commit message
		raw := "Update README with new instructions"

		// when: parsing the commit
		c := commit.Parse(t.Context(), "pqr1234", raw)

		// then: type is empty and it's not conventional
		testastic.Equal(t, "", c.Type)
		testastic.Equal(t, "Update README with new instructions", c.Description)
		testastic.False(t, c.IsConventional())
	})

	t.Run("malformed conventional headers", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			raw  string
		}{
			{name: "uppercase type", raw: "FEAT: add authentication"},
			{name: "empty scope", raw: "fix(): resolve timeout"},
			{name: "missing separator space", raw: "fix:resolve timeout"},
			{name: "extra separator spaces", raw: "fix:  resolve timeout"},
			{name: "empty description", raw: "fix: "},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				// when: parsing a malformed conventional-looking header
				c := commit.Parse(t.Context(), "bad1234", tt.raw)

				// then: it is treated as a non-conventional commit
				testastic.Equal(t, "", c.Type)
				testastic.Equal(t, strings.TrimSpace(tt.raw), c.Description)
				testastic.False(t, c.IsConventional())
			})
		}
	})

	t.Run("commit with multiple footers", func(t *testing.T) {
		t.Parallel()

		// given: a commit with multiple footers
		raw, err := os.ReadFile("testdata/multiple_footers/input.txt")
		testastic.NoError(t, err)

		// when: parsing the commit
		c := commit.Parse(t.Context(), "stu5678", string(raw))

		// then: all footers are parsed
		testastic.Equal(t, "feat", c.Type)
		testastic.Equal(t, "Implement Stripe integration.", c.Body)
		testastic.Equal(t, 2, len(c.Footers))
	})

	t.Run("footer-shaped lines inside body", func(t *testing.T) {
		t.Parallel()

		// given: body prose that resembles footers before the final footer block
		raw := "feat: improve retries\n\nExplain the retry behavior.\nNote: this is body text\nretries: 3\n\nRefs: #123"

		// when: parsing the commit
		c := commit.Parse(t.Context(), "ftr1234", raw)

		// then: only the final blank-line-separated block is parsed as footers
		testastic.Equal(t, "Explain the retry behavior.\nNote: this is body text\nretries: 3", c.Body)
		testastic.Equal(t, 1, len(c.Footers))
		testastic.Equal(t, "Refs", c.Footers[0].Key)
		testastic.Equal(t, "#123", c.Footers[0].Value)
	})

	t.Run("multi-line breaking change footer", func(t *testing.T) {
		t.Parallel()

		// given: a commit with a multi-line BREAKING CHANGE footer
		raw, err := os.ReadFile("testdata/multi_line_breaking/input.txt")
		testastic.NoError(t, err)

		// when: parsing the commit
		c := commit.Parse(t.Context(), "mln1234", string(raw))

		// then: continuation lines are included in the footer value
		testastic.True(t, c.Breaking)
		testastic.Equal(t, 1, len(c.Footers))
		testastic.Equal(t, "BREAKING CHANGE", c.Footers[0].Key)
		testastic.AssertFile(t, "testdata/multi_line_breaking/expected.txt", c.Footers[0].Value)
	})

	t.Run("multi-line footer followed by another footer", func(t *testing.T) {
		t.Parallel()

		// given: a commit with a multi-line footer followed by another footer
		raw, err := os.ReadFile("testdata/multi_line_then_another/input.txt")
		testastic.NoError(t, err)

		// when: parsing the commit
		c := commit.Parse(t.Context(), "mln5678", string(raw))

		// then: continuation stops at the next footer token
		testastic.True(t, c.Breaking)
		testastic.Equal(t, 2, len(c.Footers))
		testastic.Equal(t, "BREAKING CHANGE", c.Footers[0].Key)
		testastic.AssertFile(t, "testdata/multi_line_then_another/expected.txt", c.Footers[0].Value)
		testastic.Equal(t, "Release-As", c.Footers[1].Key)
		testastic.Equal(t, "2.0.0", c.Footers[1].Value)
	})

	t.Run("footer with blank continuation line", func(t *testing.T) {
		t.Parallel()

		// given: a commit with a footer containing a blank line in its value
		raw, err := os.ReadFile("testdata/blank_continuation/input.txt")
		testastic.NoError(t, err)

		// when: parsing the commit
		c := commit.Parse(t.Context(), "mln9012", string(raw))

		// then: blank lines within the footer value are preserved
		testastic.True(t, c.Breaking)
		testastic.Equal(t, 1, len(c.Footers))
		testastic.AssertFile(t, "testdata/blank_continuation/expected.txt", c.Footers[0].Value)
	})
}

func TestParseFoldsLargeFooterBlockLinearly(t *testing.T) {
	// given: a footer followed by many continuation lines, as a squash-merged pull request body can produce
	const (
		continuationLines  = 50_000
		continuationLine   = "\ncontinuation line xyz"
		allocBudgetPerByte = 32
	)

	var builder strings.Builder

	builder.WriteString("fix: a\n\nRefs: #1")

	for range continuationLines {
		builder.WriteString(continuationLine)
	}

	raw := builder.String()

	// when: parsing the commit while measuring how much the parse allocates
	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)

	c := commit.Parse(t.Context(), "big1234", raw)

	runtime.ReadMemStats(&after)

	// then: the folded footer value keeps every continuation line verbatim
	testastic.Equal(t, 1, len(c.Footers))
	testastic.Equal(t, "Refs", c.Footers[0].Key)
	testastic.Equal(t, "#1"+strings.Repeat(continuationLine, continuationLines), c.Footers[0].Value)
	testastic.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(allocBudgetPerByte*len(raw)))
}

func TestDetermineBump(t *testing.T) {
	t.Parallel()

	mapping := commit.BumpMapping{
		"feat": commit.BumpMinor,
		"fix":  commit.BumpPatch,
		"perf": commit.BumpPatch,
	}

	t.Run("no commits", func(t *testing.T) {
		t.Parallel()

		// given: an empty commit list
		commits := []commit.Commit{}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: no bump is needed
		testastic.Equal(t, commit.BumpNone, bump)
	})

	t.Run("only fix commits", func(t *testing.T) {
		t.Parallel()

		// given: only fix commits
		commits := []commit.Commit{
			{Type: "fix", Description: "fix bug 1"},
			{Type: "fix", Description: "fix bug 2"},
		}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: patch bump
		testastic.Equal(t, commit.BumpPatch, bump)
	})

	t.Run("feat and fix commits", func(t *testing.T) {
		t.Parallel()

		// given: a mix of feat and fix commits
		commits := []commit.Commit{
			{Type: "fix", Description: "fix bug"},
			{Type: "feat", Description: "new feature"},
		}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: minor bump (feat > fix)
		testastic.Equal(t, commit.BumpMinor, bump)
	})

	t.Run("breaking change overrides all", func(t *testing.T) {
		t.Parallel()

		// given: commits with a breaking change
		commits := []commit.Commit{
			{Type: "fix", Description: "fix bug"},
			{Type: "feat", Description: "new feature", Breaking: true},
		}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: major bump
		testastic.Equal(t, commit.BumpMajor, bump)
	})

	t.Run("non-releasable commits only", func(t *testing.T) {
		t.Parallel()

		// given: only chore/docs commits
		commits := []commit.Commit{
			{Type: "chore", Description: "update deps"},
			{Type: "docs", Description: "update readme"},
		}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: no bump
		testastic.Equal(t, commit.BumpNone, bump)
	})

	t.Run("perf triggers patch", func(t *testing.T) {
		t.Parallel()

		// given: a perf commit
		commits := []commit.Commit{
			{Type: "perf", Description: "optimize query"},
		}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: patch bump
		testastic.Equal(t, commit.BumpPatch, bump)
	})

	t.Run("custom mapping docs as patch", func(t *testing.T) {
		t.Parallel()

		// given: a docs commit with a custom mapping
		mapping := commit.BumpMapping{"docs": commit.BumpPatch}
		commits := []commit.Commit{
			{Type: "docs", Description: "update readme"},
		}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: patch bump
		testastic.Equal(t, commit.BumpPatch, bump)
	})

	t.Run("custom mapping type not in mapping returns none", func(t *testing.T) {
		t.Parallel()

		// given: a feat commit with a mapping that does not include feat
		mapping := commit.BumpMapping{"fix": commit.BumpPatch}
		commits := []commit.Commit{
			{Type: "feat", Description: "new feature"},
		}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: no bump
		testastic.Equal(t, commit.BumpNone, bump)
	})

	t.Run("breaking overrides custom mapping", func(t *testing.T) {
		t.Parallel()

		// given: a breaking commit with a mapping that only has patch types
		mapping := commit.BumpMapping{"fix": commit.BumpPatch}
		commits := []commit.Commit{
			{Type: "fix", Description: "fix bug", Breaking: true},
		}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: major bump regardless of mapping
		testastic.Equal(t, commit.BumpMajor, bump)
	})

	t.Run("empty mapping only breaking triggers bump", func(t *testing.T) {
		t.Parallel()

		// given: feat and fix commits with an empty mapping
		mapping := commit.BumpMapping{}
		commits := []commit.Commit{
			{Type: "feat", Description: "new feature"},
			{Type: "fix", Description: "fix bug"},
		}

		// when: determining bump
		bump := commit.DetermineBump(commits, mapping)

		// then: no bump
		testastic.Equal(t, commit.BumpNone, bump)
	})
}

func TestFilterByTypes(t *testing.T) {
	t.Parallel()

	t.Run("filters matching types", func(t *testing.T) {
		t.Parallel()

		// given: a list of commits
		commits := []commit.Commit{
			{Type: "feat", Description: "new feature"},
			{Type: "fix", Description: "fix bug"},
			{Type: "chore", Description: "update deps"},
			{Type: "perf", Description: "optimize"},
		}

		// when: filtering by feat and fix
		filtered := commit.FilterByTypes(commits, []string{"feat", "fix"})

		// then: only feat and fix commits are returned
		testastic.Equal(t, 2, len(filtered))
		testastic.Equal(t, "feat", filtered[0].Type)
		testastic.Equal(t, "fix", filtered[1].Type)
	})

	t.Run("includes breaking changes from non-included types", func(t *testing.T) {
		t.Parallel()

		// given: a breaking chore commit
		commits := []commit.Commit{
			{Type: "feat", Description: "new feature"},
			{Type: "chore", Description: "restructure", Breaking: true},
		}

		// when: filtering by feat only
		filtered := commit.FilterByTypes(commits, []string{"feat"})

		// then: breaking chore is also included
		testastic.Equal(t, 2, len(filtered))
	})
}

func FuzzParse(f *testing.F) {
	seeds := []string{
		"feat: add user authentication",
		"feat(auth)!: drop token v1\n\nbody text\n\nBREAKING CHANGE: tokens are invalid",
		"fix(api): return 401 for expired sessions\n\nRefs: JIRA-123\nCloses #45",
		"not a conventional message",
		"",
		"feat(scope with spaces): description",
		"FEAT: uppercase type",
		"feat:no space after colon",
		"chore(deps): bump\n\nSigned-off-by: bot <bot@example.com>\n continuation line",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rawMessage string) {
		c := commit.Parse(t.Context(), "abc1234", rawMessage)

		testastic.Equal(t, "abc1234", c.Hash)
		testastic.Equal(t, strings.ToLower(c.Type), c.Type)
		testastic.Equal(t, c.Type != "", c.IsConventional())
	})
}
