package integration_test

import "testing"

func TestReleaseBreakingDescriptionTrailingWhitespace(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name   string
		footer string
	}{
		{name: "trailing newline", footer: "BREAKING CHANGE: remove API\n"},
		{name: "trailing whitespace", footer: "BREAKING-CHANGE: remove API \t\n\n"},
		{name: "CRLF newline", footer: "BREAKING CHANGE: remove API\r\n"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a breaking description with trailing whitespace in the commit message
			message := "fix: update API\n\n" + scenario.footer

			// when: creating its release through the CLI
			// then: the rendered breaking description has no whitespace before the hash separator
			assertConventionalRelease(t, message, "2.0.0",
				"testdata/release/breaking_description_whitespace/pull_request.expected.md")
		})
	}
}

func TestReleaseBreakingDescriptionLineBreaks(t *testing.T) {
	t.Parallel()

	// given: a breaking footer whose description spans two lines
	message := "fix: update API\n\nBREAKING CHANGE: remove\nAPI\n"

	// when: creating its release through the CLI
	// then: the note keeps the footer line break
	assertConventionalRelease(t, message, "2.0.0",
		"testdata/release/breaking_description_multiline/pull_request.expected.md")
}
