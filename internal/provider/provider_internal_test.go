package provider

import (
	"strings"
	"testing"

	"github.com/monkescience/testastic"
)

func TestIsFullCommitSHA(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name  string
		ref   string
		valid bool
	}{
		{name: "accepts a lowercase commit SHA", ref: strings.Repeat("a1b2", 10), valid: true},
		{name: "accepts an uppercase commit SHA", ref: strings.Repeat("A1B2", 10), valid: true},
		{name: "rejects a branch name", ref: "main", valid: false},
		{name: "rejects an abbreviated SHA", ref: strings.Repeat("a1b2", 10)[:7], valid: false},
		{name: "rejects an over-long SHA", ref: strings.Repeat("a1b2", 10) + "0", valid: false},
		{name: "rejects non-hex characters", ref: strings.Repeat("a1b2", 9) + "zzzz", valid: false},
		{name: "rejects a blank ref", ref: strings.Repeat(" ", 40), valid: false},
		{name: "rejects an empty ref", ref: "", valid: false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a ref a caller passed as a release target
			ref := scenario.ref

			// when: the ref is checked against the commit SHA rule
			valid := isFullCommitSHA(ref)

			// then: only a full-length hexadecimal SHA is accepted
			testastic.Equal(t, scenario.valid, valid)
		})
	}
}
