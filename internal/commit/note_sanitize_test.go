package commit_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
)

func TestSanitizeNoteText(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		note string
		want string
	}{
		{
			name: "prose comments",
			note: "<!-- hidden\n# Heading\n-->",
			want: "&lt;!-- hidden\n# Heading\n--&gt;",
		},
		{
			name: "inline code",
			note: "Use `<!-- literal -->` and <!-- escape -->.",
			want: "Use `<!-- literal -->` and &lt;!-- escape --&gt;.",
		},
		{
			name: "nested fenced code",
			note: "> ```html\n> <!-- literal -->\n> ```\n\n<!-- escape -->",
			want: "> ```html\n> <!-- literal -->\n> ```\n\n&lt;!-- escape --&gt;",
		},
		{
			name: "indented code",
			note: "    <!-- literal -->\n\n<!-- escape -->",
			want: "    <!-- literal -->\n\n&lt;!-- escape --&gt;",
		},
		{
			name: "controls in markers",
			note: "<!\x00-- escape --\x1b>\r\n",
			want: "&lt;!-- escape --&gt;\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: Markdown containing literal or structural comment markers
			note := tc.note

			// when: sanitizing it repeatedly
			sanitized := commit.SanitizeNoteText(note)
			repeated := commit.SanitizeNoteText(sanitized)

			// then: only structural markers are escaped and sanitation is idempotent
			testastic.Equal(t, tc.want, sanitized)
			testastic.Equal(t, sanitized, repeated)
		})
	}
}
