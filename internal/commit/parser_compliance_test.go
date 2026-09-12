package commit_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
)

func TestParseUnicodeType(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		message string
		kind    string
	}{
		{message: "sécurité!: remove API", kind: "sécurité"},
		{message: "SÉCURITÉ!: remove API", kind: "sécurité"},
		{message: "se\u0301curite\u0301!: remove API", kind: "se\u0301curite\u0301"},
		{message: "安全!: remove API", kind: "安全"},
	} {
		t.Run(scenario.message, func(t *testing.T) {
			t.Parallel()

			// given: a custom noun type with Unicode letters or combining marks
			// when: parsing a breaking commit with that type
			parsed := commit.Parse(t.Context(), "unicode123", scenario.message)

			// then: the normalized type and major release signal survive
			testastic.Equal(t, scenario.kind, parsed.Type)
			testastic.True(t, parsed.IsConventional())
			testastic.True(t, parsed.Breaking)
			testastic.Equal(t, commit.BumpMajor, commit.DetermineBump([]commit.Commit{parsed}, nil))
		})
	}
}

func TestParseWordFooterBoundaries(t *testing.T) {
	t.Parallel()

	for _, token := range []string{"Reviewed_by", "Référence", "Re\u0301fe\u0301rence", "参考"} {
		for _, separator := range []string{": ", " #"} {
			t.Run(token+separator, func(t *testing.T) {
				t.Parallel()

				// given: a breaking description followed by a word token with either footer separator
				raw := "fix: update API\n\nBREAKING CHANGE: remove API\n" + token + separator + "123\nRefs: #456"

				// when: parsing the footer values
				parsed := commit.Parse(t.Context(), "footer123", raw)

				// then: the metadata terminates the breaking description and remains a separate footer
				value := "123"
				if separator == " #" {
					value = "#123"
				}

				testastic.SliceEqual(t, []commit.Footer{
					{Key: "BREAKING CHANGE", Value: "remove API"},
					{Key: token, Value: value},
					{Key: "Refs", Value: "#456"},
				}, parsed.Footers)
			})
		}
	}
}

func TestParseInvalidHeaderWhitespace(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"  fix: resolve timeout",
		"\tfix!: remove API",
		"\u00a0fix: resolve timeout",
		"fix(   ): resolve timeout",
		"fix(\t)!: remove API",
		"fix(\u00a0): resolve timeout",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			// given: a header with leading whitespace or a whitespace-only scope
			// when: parsing the malformed commit
			parsed := commit.Parse(t.Context(), "invalid123", raw)

			// then: the message cannot trigger a release even when it contains a bang
			testastic.False(t, parsed.IsConventional())
			testastic.False(t, parsed.Breaking)
			testastic.Equal(t, commit.BumpNone, commit.DetermineBump([]commit.Commit{parsed}, nil))
		})
	}
}
