package changelog

import (
	"errors"
	"regexp/syntax"
	"testing"

	"github.com/monkescience/testastic"
)

func TestPatternCompileReason(t *testing.T) {
	t.Parallel()

	t.Run("names the syntax problem", func(t *testing.T) {
		t.Parallel()

		// given: a pattern the regexp parser rejects
		_, err := syntax.Parse("[invalid", syntax.Perl)
		testastic.Error(t, err)

		// when: describing why it failed
		reason := patternCompileReason(err)

		// then: the parser's own classification reaches the caller
		testastic.Equal(t, "missing closing ]", reason)
	})

	t.Run("falls back for unclassified errors", func(t *testing.T) {
		t.Parallel()

		// given: an error the regexp parser did not produce
		err := errors.New("some other failure")

		// when: describing why it failed
		reason := patternCompileReason(err)

		// then: a stable classification is reported instead
		testastic.Equal(t, "pattern could not be compiled", reason)
	})
}
