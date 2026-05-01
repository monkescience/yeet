//nolint:testpackage // This test validates unexported release version planning helpers.
package release

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestParsePrereleaseCounter(t *testing.T) {
	t.Parallel()

	t.Run("single digit counter", func(t *testing.T) {
		t.Parallel()

		// given: a single-digit numeric counter
		// when: parsing
		counter, err := parsePrereleaseCounter("1")

		// then: the counter is returned
		testastic.NoError(t, err)
		testastic.Equal(t, int64(1), counter)
	})

	t.Run("multi digit counter", func(t *testing.T) {
		t.Parallel()

		// given: a multi-digit numeric counter
		// when: parsing
		counter, err := parsePrereleaseCounter("123")

		// then: the counter is returned
		testastic.NoError(t, err)
		testastic.Equal(t, int64(123), counter)
	})

	t.Run("empty input is rejected", func(t *testing.T) {
		t.Parallel()

		// given: an empty counter string
		// when: parsing
		_, err := parsePrereleaseCounter("")

		// then: error indicates an invalid release-as
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("non numeric input is rejected", func(t *testing.T) {
		t.Parallel()

		// given: a counter with a trailing non-digit
		// when: parsing
		_, err := parsePrereleaseCounter("1a")

		// then: error indicates an invalid release-as
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("zero is rejected", func(t *testing.T) {
		t.Parallel()

		// given: a counter equal to zero
		// when: parsing
		_, err := parsePrereleaseCounter("0")

		// then: error indicates an invalid release-as because counters must be >= 1
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrInvalidReleaseAs)
	})

	t.Run("leading zero is accepted as numeric", func(t *testing.T) {
		t.Parallel()

		// given: a counter with a leading zero
		// when: parsing
		counter, err := parsePrereleaseCounter("01")

		// then: the numeric value is returned
		testastic.NoError(t, err)
		testastic.Equal(t, int64(1), counter)
	})
}
