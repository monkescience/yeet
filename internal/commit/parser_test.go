package commit_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
)

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("simple feat", func(t *testing.T) {
		t.Parallel()

		// given: a simple feature commit message
		raw := "feat: add user authentication"

		// when: parsing the commit
		c := commit.Parse("abc1234", raw)

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
		c := commit.Parse("def5678", raw)

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
		c := commit.Parse("ghi9012", raw)

		// then: breaking flag is set
		testastic.Equal(t, "feat", c.Type)
		testastic.Equal(t, "api", c.Scope)
		testastic.True(t, c.Breaking)
	})

	t.Run("breaking change in footer", func(t *testing.T) {
		t.Parallel()

		// given: a commit with BREAKING CHANGE footer
		raw := "feat: new auth flow\n\nSome body text.\n\nBREAKING CHANGE: old auth tokens are no longer valid"

		// when: parsing the commit
		c := commit.Parse("jkl3456", raw)

		// then: breaking flag is set and footer is parsed
		testastic.True(t, c.Breaking)
		testastic.Equal(t, "Some body text.", c.Body)
		testastic.Equal(t, 1, len(c.Footers))
		testastic.Equal(t, "BREAKING CHANGE", c.Footers[0].Key)
		testastic.Equal(t, "old auth tokens are no longer valid", c.Footers[0].Value)
	})

	t.Run("fix commit", func(t *testing.T) {
		t.Parallel()

		// given: a fix commit
		raw := "fix: resolve null pointer in user handler"

		// when: parsing the commit
		c := commit.Parse("mno7890", raw)

		// then: type is fix
		testastic.Equal(t, "fix", c.Type)
		testastic.Equal(t, "resolve null pointer in user handler", c.Description)
	})

	t.Run("non-conventional commit", func(t *testing.T) {
		t.Parallel()

		// given: a non-conventional commit message
		raw := "Update README with new instructions"

		// when: parsing the commit
		c := commit.Parse("pqr1234", raw)

		// then: type is empty and it's not conventional
		testastic.Equal(t, "", c.Type)
		testastic.Equal(t, "Update README with new instructions", c.Description)
		testastic.False(t, c.IsConventional())
	})

	t.Run("commit with multiple footers", func(t *testing.T) {
		t.Parallel()

		// given: a commit with multiple footers
		raw := "feat: add payment processing\n\nImplement Stripe integration.\n\nRefs: TICKET-123\nReviewed-by: Alice"

		// when: parsing the commit
		c := commit.Parse("stu5678", raw)

		// then: all footers are parsed
		testastic.Equal(t, "feat", c.Type)
		testastic.Equal(t, "Implement Stripe integration.", c.Body)
		testastic.Equal(t, 2, len(c.Footers))
	})

	t.Run("multi-line breaking change footer", func(t *testing.T) {
		t.Parallel()

		// given: a commit with a multi-line BREAKING CHANGE footer
		raw := "feat!: redesign auth\n\n" +
			"BREAKING CHANGE: The session token format changed\n" +
			"from JWT to opaque tokens. Migrate before upgrading."

		// when: parsing the commit
		c := commit.Parse("mln1234", raw)

		// then: continuation lines are included in the footer value
		wantValue := "The session token format changed\n" +
			"from JWT to opaque tokens. Migrate before upgrading."

		testastic.True(t, c.Breaking)
		testastic.Equal(t, 1, len(c.Footers))
		testastic.Equal(t, "BREAKING CHANGE", c.Footers[0].Key)
		testastic.Equal(t, wantValue, c.Footers[0].Value)
	})

	t.Run("multi-line footer followed by another footer", func(t *testing.T) {
		t.Parallel()

		// given: a commit with a multi-line footer followed by another footer
		raw := "feat!: redesign auth\n\n" +
			"BREAKING CHANGE: The session token format changed\n" +
			"from JWT to opaque tokens.\n" +
			"Release-As: 2.0.0"

		// when: parsing the commit
		c := commit.Parse("mln5678", raw)

		// then: continuation stops at the next footer token
		wantValue := "The session token format changed\n" +
			"from JWT to opaque tokens."

		testastic.True(t, c.Breaking)
		testastic.Equal(t, 2, len(c.Footers))
		testastic.Equal(t, "BREAKING CHANGE", c.Footers[0].Key)
		testastic.Equal(t, wantValue, c.Footers[0].Value)
		testastic.Equal(t, "Release-As", c.Footers[1].Key)
		testastic.Equal(t, "2.0.0", c.Footers[1].Value)
	})

	t.Run("footer with blank continuation line", func(t *testing.T) {
		t.Parallel()

		// given: a commit with a footer containing a blank line in its value
		raw := "feat!: big change\n\nBREAKING CHANGE: First paragraph.\n\nSecond paragraph after blank line."

		// when: parsing the commit
		c := commit.Parse("mln9012", raw)

		// then: blank lines within the footer value are preserved
		testastic.True(t, c.Breaking)
		testastic.Equal(t, 1, len(c.Footers))
		testastic.Equal(t, "First paragraph.\n\nSecond paragraph after blank line.", c.Footers[0].Value)
	})
}

func TestDetermineBump(t *testing.T) {
	t.Parallel()

	t.Run("no commits", func(t *testing.T) {
		t.Parallel()

		// given: an empty commit list
		commits := []commit.Commit{}

		// when: determining bump
		bump := commit.DetermineBump(commits)

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
		bump := commit.DetermineBump(commits)

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
		bump := commit.DetermineBump(commits)

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
		bump := commit.DetermineBump(commits)

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
		bump := commit.DetermineBump(commits)

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
		bump := commit.DetermineBump(commits)

		// then: patch bump
		testastic.Equal(t, commit.BumpPatch, bump)
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
