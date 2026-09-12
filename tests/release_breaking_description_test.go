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
		{name: "multiline description", footer: "BREAKING CHANGE: remove\nAPI\n"},
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
