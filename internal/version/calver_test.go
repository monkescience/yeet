package version_test

import (
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/version"
)

func fixedTime(year int, month time.Month) func() time.Time {
	return func() time.Time {
		return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	}
}

func fixedDate(year int, month time.Month, day int) func() time.Time {
	return func() time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
}

func TestValidateCalVerFormat(t *testing.T) {
	t.Parallel()

	validFormats := []string{
		"YYYY.0M.MICRO",
		"YY.MM.MICRO",
		"0Y.0W.MICRO",
		"YYYY.0M.0D.MICRO",
	}

	for _, format := range validFormats {
		t.Run("valid "+format, func(t *testing.T) {
			t.Parallel()

			// given: a supported calendar version format

			// when: validating the format
			err := version.ValidateCalVerFormat(format)

			// then: it is accepted
			testastic.NoError(t, err)
		})
	}

	invalidFormats := []string{
		"",
		"YYYY.0M",
		"YYYY.0D.MICRO",
		"YYYY.MICRO.0M",
		"YYYY-0M-MICRO",
		"YYYY_0M_MICRO",
		"YYYY.QQ.MICRO",
		"YYYY.0M.MICRO.MICRO",
		"0M.MICRO",
		"YYYY.MM.0M.MICRO",
		"YYYY.0M.WW.MICRO",
		"0Y.WW.0W.MICRO",
		"YYYY.0M.DD.0D.MICRO",
		"YYYY.YY.0M.MICRO",
		".YYYY.0M.MICRO",
		"   ",
	}

	for _, format := range invalidFormats {
		t.Run("invalid "+format, func(t *testing.T) {
			t.Parallel()

			// given: an unsupported calendar version format

			// when: validating the format
			err := version.ValidateCalVerFormat(format)

			// then: it is rejected
			testastic.Error(t, err)
			testastic.ErrorIs(t, err, version.ErrInvalidVersion)
		})
	}
}

func TestNewCalVer(t *testing.T) {
	t.Parallel()

	// given: a format without a micro token
	// when: constructing a calendar version strategy
	cv, err := version.NewCalVer("YYYY.0M", "v", nil)

	// then: construction rejects the format and returns no strategy
	testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	testastic.True(t, cv == nil)
}

func TestCalVerCurrent(t *testing.T) {
	t.Parallel()

	cv := newCalVer(t, "", "v", nil)

	t.Run("parses valid tag", func(t *testing.T) {
		t.Parallel()

		// given: a valid calver tag
		tag := "v2026.02.1"

		// when: parsing current version
		v, err := cv.Current(tag)

		// then: version is extracted
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.1", v)
	})

	t.Run("rejects invalid format", func(t *testing.T) {
		t.Parallel()

		// given: an invalid calver tag
		tag := "v1.2"

		// when: parsing current version
		_, err := cv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects non-numeric micro", func(t *testing.T) {
		t.Parallel()

		// given: a tag with non-numeric micro
		tag := "v2026.02.abc"

		// when: parsing current version
		_, err := cv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
	})

	t.Run("rejects negative micro", func(t *testing.T) {
		t.Parallel()

		// given: a tag with negative micro
		tag := "v2026.02.-1"

		// when: parsing current version
		_, err := cv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("normalizes non-zero-padded month", func(t *testing.T) {
		t.Parallel()

		// given: a valid calver tag with non-zero-padded month
		tag := "v2026.2.1"

		// when: parsing current version
		v, err := cv.Current(tag)

		// then: version is extracted with normalized month
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.1", v)
	})

	t.Run("uses short year and unpadded month format", func(t *testing.T) {
		t.Parallel()

		// given: a calver tag using YY.MM.MICRO
		cv := newCalVer(t, "YY.MM.MICRO", "v", nil)
		tag := "v26.02.7"

		// when: parsing current version
		v, err := cv.Current(tag)

		// then: version is normalized according to the configured format
		testastic.NoError(t, err)
		testastic.Equal(t, "26.2.7", v)
	})

	t.Run("uses day format", func(t *testing.T) {
		t.Parallel()

		// given: a calver tag using YYYY.0M.0D.MICRO
		cv := newCalVer(t, "YYYY.0M.0D.MICRO", "v", nil)
		tag := "v2026.2.3.7"

		// when: parsing current version
		v, err := cv.Current(tag)

		// then: month and day are normalized according to the configured format
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.03.7", v)
	})

	t.Run("rejects non-numeric year", func(t *testing.T) {
		t.Parallel()

		// given: a tag with non-numeric year
		tag := "vfoo.02.1"

		// when: parsing current version
		_, err := cv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects invalid month", func(t *testing.T) {
		t.Parallel()

		// given: a tag with out-of-range month
		tag := "v2026.13.1"

		// when: parsing current version
		_, err := cv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects month zero", func(t *testing.T) {
		t.Parallel()

		// given: a tag with month zero
		tag := "v2026.00.1"

		// when: parsing current version
		_, err := cv.Current(tag)

		// then: error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})
}

func TestCalVerInitialVersion(t *testing.T) {
	t.Parallel()

	// given: a calver strategy
	cv := newCalVer(t, "", "v", nil)

	// when: requesting the initial version
	initial := cv.InitialVersion()

	// then: empty string since calver starts from current date
	testastic.Equal(t, "", initial)
}

func TestCalVerNext(t *testing.T) {
	t.Parallel()

	t.Run("first release of the month", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy set to Feb 2026
		cv := newCalVer(t, "", "v", fixedTime(2026, time.February))

		// when: calculating next from an empty version
		next, err := cv.Next("", commit.BumpMinor)

		// then: first release of the month
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.1", next)
	})

	t.Run("increment within same month", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy with existing version in same month
		cv := newCalVer(t, "", "v", fixedTime(2026, time.February))

		// when: calculating next from existing version in same month
		next, err := cv.Next("2026.02.3", commit.BumpPatch)

		// then: micro increments
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.4", next)
	})

	t.Run("new month resets micro", func(t *testing.T) {
		t.Parallel()

		// given: current version from January, now it's February
		cv := newCalVer(t, "", "v", fixedTime(2026, time.February))

		// when: calculating next
		next, err := cv.Next("2026.01.5", commit.BumpPatch)

		// then: micro resets to 1
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.1", next)
	})

	t.Run("rejects current version from a future calendar period", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			format  string
			current string
			now     func() time.Time
		}{
			{
				name:    "default month format",
				current: "2026.09.5",
				now:     fixedDate(2026, time.August, 10),
			},
			{
				name:    "short year format",
				format:  "YY.MM.MICRO",
				current: "26.9.5",
				now:     fixedDate(2026, time.August, 10),
			},
			{
				name:    "day format",
				format:  "YYYY.0M.0D.MICRO",
				current: "2026.08.11.5",
				now:     fixedDate(2026, time.August, 10),
			},
			{
				name:    "week format",
				format:  "YYYY.0W.MICRO",
				current: "2026.06.5",
				now:     fixedDate(2026, time.February, 4),
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()

				// given: the current version belongs to a calendar period after the fixed clock
				cv := newCalVer(t, testCase.format, "v", testCase.now)

				// when: calculating the next version
				_, err := cv.Next(testCase.current, commit.BumpPatch)

				// then: the future version is rejected before a lower version is produced
				testastic.Error(t, err)
				testastic.ErrorIs(t, err, version.ErrInvalidVersion)
				testastic.ErrorContains(t, err, "future calendar period")
			})
		}
	})

	t.Run("calendar token order does not affect period comparison", func(t *testing.T) {
		t.Parallel()

		type comparisonCase struct {
			name   string
			future string
			past   string
		}

		testCases := []struct {
			name        string
			format      string
			wantNext    string
			comparisons []comparisonCase
		}{
			{
				name:     "year",
				format:   "YYYY.MICRO",
				wantNext: "2026.1",
				comparisons: []comparisonCase{
					{name: "year", future: "2027.5", past: "2025.7"},
				},
			},
			{
				name:     "year month",
				format:   "YYYY.0M.MICRO",
				wantNext: "2026.08.1",
				comparisons: []comparisonCase{
					{name: "year", future: "2027.01.5", past: "2025.12.7"},
					{name: "month", future: "2026.09.5", past: "2026.07.7"},
				},
			},
			{
				name:     "month year",
				format:   "0M.YYYY.MICRO",
				wantNext: "08.2026.1",
				comparisons: []comparisonCase{
					{name: "year", future: "01.2027.5", past: "12.2025.7"},
					{name: "month", future: "09.2026.5", past: "07.2026.7"},
				},
			},
			{
				name:     "year week",
				format:   "YYYY.0W.MICRO",
				wantNext: "2026.32.1",
				comparisons: []comparisonCase{
					{name: "year", future: "2027.01.5", past: "2025.53.7"},
					{name: "week", future: "2026.33.5", past: "2026.31.7"},
				},
			},
			{
				name:     "week year",
				format:   "0W.YYYY.MICRO",
				wantNext: "32.2026.1",
				comparisons: []comparisonCase{
					{name: "year", future: "01.2027.5", past: "53.2025.7"},
					{name: "week", future: "33.2026.5", past: "31.2026.7"},
				},
			},
			{
				name:     "year month day",
				format:   "YYYY.0M.0D.MICRO",
				wantNext: "2026.08.10.1",
				comparisons: []comparisonCase{
					{name: "year", future: "2027.01.01.5", past: "2025.12.31.7"},
					{name: "month", future: "2026.09.01.5", past: "2026.07.31.7"},
					{name: "day", future: "2026.08.11.5", past: "2026.08.09.7"},
				},
			},
			{
				name:     "year day month",
				format:   "YYYY.0D.0M.MICRO",
				wantNext: "2026.10.08.1",
				comparisons: []comparisonCase{
					{name: "year", future: "2027.01.01.5", past: "2025.31.12.7"},
					{name: "month", future: "2026.01.09.5", past: "2026.31.07.7"},
					{name: "day", future: "2026.11.08.5", past: "2026.09.08.7"},
				},
			},
			{
				name:     "month year day",
				format:   "0M.YYYY.0D.MICRO",
				wantNext: "08.2026.10.1",
				comparisons: []comparisonCase{
					{name: "year", future: "01.2027.01.5", past: "12.2025.31.7"},
					{name: "month", future: "09.2026.01.5", past: "07.2026.31.7"},
					{name: "day", future: "08.2026.11.5", past: "08.2026.09.7"},
				},
			},
			{
				name:     "month day year",
				format:   "0M.0D.YYYY.MICRO",
				wantNext: "08.10.2026.1",
				comparisons: []comparisonCase{
					{name: "year", future: "01.01.2027.5", past: "12.31.2025.7"},
					{name: "month", future: "09.01.2026.5", past: "07.31.2026.7"},
					{name: "day", future: "08.11.2026.5", past: "08.09.2026.7"},
				},
			},
			{
				name:     "day year month",
				format:   "0D.YYYY.0M.MICRO",
				wantNext: "10.2026.08.1",
				comparisons: []comparisonCase{
					{name: "year", future: "01.2027.01.5", past: "31.2025.12.7"},
					{name: "month", future: "01.2026.09.5", past: "31.2026.07.7"},
					{name: "day", future: "11.2026.08.5", past: "09.2026.08.7"},
				},
			},
			{
				name:     "day month year",
				format:   "0D.0M.YYYY.MICRO",
				wantNext: "10.08.2026.1",
				comparisons: []comparisonCase{
					{name: "year", future: "01.01.2027.5", past: "31.12.2025.7"},
					{name: "month", future: "01.09.2026.5", past: "31.07.2026.7"},
					{name: "day", future: "11.08.2026.5", past: "09.08.2026.7"},
				},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()

				cv := newCalVer(t, testCase.format, "v", fixedDate(2026, time.August, 10))

				for _, comparison := range testCase.comparisons {
					t.Run(comparison.name, func(t *testing.T) {
						t.Parallel()

						// given: future and past values in one chronological component
						// when: calculating the next version from each value
						_, futureErr := cv.Next(comparison.future, commit.BumpPatch)
						next, pastErr := cv.Next(comparison.past, commit.BumpPatch)

						// then: chronological order wins independently of display order
						testastic.ErrorIs(t, futureErr, version.ErrInvalidVersion)
						testastic.NoError(t, pastErr)
						testastic.Equal(t, testCase.wantNext, next)
					})
				}
			})
		}
	})

	t.Run("no bump returns same", func(t *testing.T) {
		t.Parallel()

		// given: current version
		cv := newCalVer(t, "", "v", fixedTime(2026, time.February))

		// when: applying no bump
		next, err := cv.Next("2026.02.1", commit.BumpNone)

		// then: version unchanged
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.1", next)
	})

	t.Run("increment within same month with non-zero-padded input", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy with existing version using non-zero-padded month
		cv := newCalVer(t, "", "v", fixedTime(2026, time.February))

		// when: calculating next from "2026.2.3" (non-zero-padded)
		next, err := cv.Next("2026.2.3", commit.BumpPatch)

		// then: micro increments (month padding should not cause a reset)
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.4", next)
	})

	t.Run("new year resets", func(t *testing.T) {
		t.Parallel()

		// given: current version from last year
		cv := newCalVer(t, "", "v", fixedTime(2027, time.January))

		// when: calculating next
		next, err := cv.Next("2026.12.7", commit.BumpMinor)

		// then: new year, new month, micro resets
		testastic.NoError(t, err)
		testastic.Equal(t, "2027.01.1", next)
	})

	t.Run("uses configured short year format", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy with YY.MM.MICRO
		cv := newCalVer(t, "YY.MM.MICRO", "v", fixedTime(2026, time.February))

		// when: calculating next from existing version in same month
		next, err := cv.Next("26.2.3", commit.BumpPatch)

		// then: micro increments and the configured format is preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "26.2.4", next)
	})

	t.Run("resets micro when configured day changes", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy with YYYY.0M.0D.MICRO
		cv := newCalVer(t, "YYYY.0M.0D.MICRO", "v", fixedDate(2026, time.February, 3))

		// when: calculating next from the previous day
		next, err := cv.Next("2026.02.02.7", commit.BumpPatch)

		// then: micro resets for the new day
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.02.03.1", next)
	})

	t.Run("week format increments within same week", func(t *testing.T) {
		t.Parallel()

		// given: a week-based calver strategy fixed mid-week
		cv := newCalVer(t, "YYYY.0W.MICRO", "v", fixedDate(2026, time.February, 4))

		// when: calculating next within the same week
		next, err := cv.Next("2026.05.2", commit.BumpPatch)

		// then: micro increments
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.05.3", next)
	})

	t.Run("week format resets on new week", func(t *testing.T) {
		t.Parallel()

		// given: a week-based calver strategy moved into the next week
		cv := newCalVer(t, "YYYY.0W.MICRO", "v", fixedDate(2026, time.February, 8))

		// when: calculating next after the week rolls over
		next, err := cv.Next("2026.05.4", commit.BumpPatch)

		// then: micro resets
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.06.1", next)
	})

	t.Run("week format resets on new year", func(t *testing.T) {
		t.Parallel()

		// given: a week-based calver strategy on the first day of a new year
		cv := newCalVer(t, "YYYY.0W.MICRO", "v", fixedDate(2027, time.January, 1))

		// when: calculating next from the previous year's final week
		next, err := cv.Next("2026.53.4", commit.BumpPatch)

		// then: micro resets and year rolls over
		testastic.NoError(t, err)
		testastic.Equal(t, "2027.01.1", next)
	})

	t.Run("year-pad format renders padded year", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy using the 0Y year-pad token
		cv := newCalVer(t, "0Y.0M.MICRO", "v", fixedTime(2026, time.February))

		// when: rendering the first version of the period
		next, err := cv.Next("", commit.BumpMinor)

		// then: year is padded to two digits
		testastic.NoError(t, err)
		testastic.Equal(t, "26.02.1", next)
	})

	t.Run("year-pad format pads single-digit year", func(t *testing.T) {
		t.Parallel()

		// given: a 0Y format computed for an early-millennium year
		cv := newCalVer(t, "0Y.0M.MICRO", "v", fixedTime(2007, time.March))

		// when: rendering the first version of the period
		next, err := cv.Next("", commit.BumpMinor)

		// then: year is zero-padded to two digits
		testastic.NoError(t, err)
		testastic.Equal(t, "07.03.1", next)
	})

	t.Run("non-padded day format renders unpadded day", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy using non-padded day token
		cv := newCalVer(t, "YYYY.MM.DD.MICRO", "v", fixedDate(2026, time.February, 3))

		// when: rendering the first version of the period
		next, err := cv.Next("", commit.BumpMinor)

		// then: month and day are not zero-padded
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.2.3.1", next)
	})

	t.Run("rejects unparseable current version", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy and an unparseable current value
		cv := newCalVer(t, "", "v", fixedTime(2026, time.February))

		// when: calculating next from a malformed current
		_, err := cv.Next("not a calver", commit.BumpPatch)

		// then: the parse error surfaces
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})
}

func TestCalVerCurrent_AdditionalFormats(t *testing.T) {
	t.Parallel()

	t.Run("rejects trailing data", func(t *testing.T) {
		t.Parallel()

		// given: a tag with characters past the format
		cv := newCalVer(t, "", "v", nil)

		// when: parsing
		_, err := cv.Current("v2026.02.1+extra")

		// then: the trailing data is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects empty segment", func(t *testing.T) {
		t.Parallel()

		// given: a tag with consecutive separators
		cv := newCalVer(t, "", "v", nil)

		// when: parsing
		_, err := cv.Current("v2026..1")

		// then: the empty segment is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects missing literal separator", func(t *testing.T) {
		t.Parallel()

		// given: a tag missing the trailing separator before MICRO
		cv := newCalVer(t, "", "v", nil)

		// when: parsing
		_, err := cv.Current("v2026021")

		// then: parsing fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects invalid date in day format", func(t *testing.T) {
		t.Parallel()

		// given: a calver tag with February 30
		cv := newCalVer(t, "YYYY.0M.0D.MICRO", "v", nil)

		// when: parsing
		_, err := cv.Current("v2026.02.30.1")

		// then: validateParts rejects the impossible date
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
		testastic.Equal(t, "invalid version: invalid date in \"2026.02.30.1\"", err.Error())
	})

	t.Run("rejects out-of-range day", func(t *testing.T) {
		t.Parallel()

		// given: a calver tag with day 32
		cv := newCalVer(t, "YYYY.0M.0D.MICRO", "v", nil)

		// when: parsing
		_, err := cv.Current("v2026.01.32.1")

		// then: parsing rejects the day
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects out-of-range week", func(t *testing.T) {
		t.Parallel()

		// given: a calver tag with week 54
		cv := newCalVer(t, "YYYY.0W.MICRO", "v", nil)

		// when: parsing
		_, err := cv.Current("v2026.54.1")

		// then: parsing rejects the week
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects week zero", func(t *testing.T) {
		t.Parallel()

		// given: a calver tag with week 0
		cv := newCalVer(t, "YYYY.0W.MICRO", "v", nil)

		// when: parsing
		_, err := cv.Current("v2026.00.1")

		// then: parsing rejects the week
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects short year missing required digits", func(t *testing.T) {
		t.Parallel()

		// given: YYYY format with three-digit year
		cv := newCalVer(t, "YYYY.0M.MICRO", "v", nil)

		// when: parsing
		_, err := cv.Current("v202.02.1")

		// then: year token enforces four digits
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects negative short year", func(t *testing.T) {
		t.Parallel()

		// given: YY format with a negative year segment
		cv := newCalVer(t, "YY.MM.MICRO", "v", nil)

		// when: parsing
		_, err := cv.Current("v-1.02.1")

		// then: parsing rejects the negative year
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("rejects non-numeric segment", func(t *testing.T) {
		t.Parallel()

		// given: a tag where the month segment is not numeric
		cv := newCalVer(t, "", "v", nil)

		// when: parsing
		_, err := cv.Current("v2026.foo.1")

		// then: parsing fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
	})

	t.Run("uses non-padded day format", func(t *testing.T) {
		t.Parallel()

		// given: YYYY.MM.DD.MICRO with valid input
		cv := newCalVer(t, "YYYY.MM.DD.MICRO", "v", nil)

		// when: parsing
		v, err := cv.Current("v2026.2.3.7")

		// then: the version round-trips through render
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.2.3.7", v)
	})

	t.Run("uses week-pad format", func(t *testing.T) {
		t.Parallel()

		// given: 0Y.0W.MICRO with valid input
		cv := newCalVer(t, "0Y.0W.MICRO", "v", nil)

		// when: parsing
		v, err := cv.Current("v26.05.2")

		// then: the version round-trips through render
		testastic.NoError(t, err)
		testastic.Equal(t, "26.05.2", v)
	})

	t.Run("uses non-padded week format", func(t *testing.T) {
		t.Parallel()

		// given: YYYY.WW.MICRO with valid input
		cv := newCalVer(t, "YYYY.WW.MICRO", "v", nil)

		// when: parsing
		v, err := cv.Current("v2026.5.2")

		// then: the version round-trips through render
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.5.2", v)
	})

	t.Run("now defaults to time.Now when unset", func(t *testing.T) {
		t.Parallel()

		// given: a calver strategy without an injected clock
		cv := newCalVer(t, "", "v", nil)

		// when: calculating next from empty current with a bump
		next, err := cv.Next("", commit.BumpMinor)

		// then: the call succeeds (the resulting year matches the real clock)
		testastic.NoError(t, err)
		testastic.True(t, next != "")
	})
}

func newCalVer(t *testing.T, format, prefix string, now func() time.Time) *version.CalVer {
	t.Helper()

	cv, err := version.NewCalVer(format, prefix, now)
	testastic.NoError(t, err)

	return cv
}

func TestCalVerPrereleaseNextRelease(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		format  string
		now     func() time.Time
		current string
		want    string
	}{
		{
			name:    "first release starts the period at micro 1",
			now:     fixedTime(2026, time.November),
			current: "",
			want:    "2026.11.1-beta.1",
		},
		{
			name:    "stable release in the period takes the next micro",
			now:     fixedTime(2026, time.November),
			current: "2026.11.1",
			want:    "2026.11.2-beta.1",
		},
		{
			name:    "stable release from an earlier period resets micro",
			now:     fixedTime(2026, time.November),
			current: "2026.10.4",
			want:    "2026.11.1-beta.1",
		},
		{
			name:    "prerelease in the period increments the counter",
			now:     fixedTime(2026, time.November),
			current: "2026.11.2-beta.1",
			want:    "2026.11.2-beta.2",
		},
		{
			name:    "first prerelease of the period keeps micro 1",
			now:     fixedTime(2026, time.November),
			current: "2026.11.1-beta.1",
			want:    "2026.11.1-beta.2",
		},
		{
			name:    "counter compares numerically",
			now:     fixedTime(2026, time.November),
			current: "2026.11.2-beta.9",
			want:    "2026.11.2-beta.10",
		},
		{
			name:    "prerelease from an earlier period starts a new base",
			now:     fixedTime(2026, time.December),
			current: "2026.11.2-beta.3",
			want:    "2026.12.1-beta.1",
		},
		{
			name:    "prerelease from an earlier year starts a new base",
			now:     fixedTime(2027, time.January),
			current: "2026.12.1-beta.3",
			want:    "2027.01.1-beta.1",
		},
		{
			name:    "prerelease without a counter starts the counter",
			now:     fixedTime(2026, time.November),
			current: "2026.11.2-beta",
			want:    "2026.11.2-beta.1",
		},
		{
			name:    "unpadded format increments the counter",
			format:  "YY.MM.MICRO",
			now:     fixedTime(2026, time.February),
			current: "26.2.4-beta.1",
			want:    "26.2.4-beta.2",
		},
		{
			name:    "unpadded format normalizes a padded tag",
			format:  "YY.MM.MICRO",
			now:     fixedTime(2026, time.February),
			current: "26.02.3",
			want:    "26.2.4-beta.1",
		},
		{
			name:    "day format increments the counter on the same day",
			format:  "YYYY.0M.0D.MICRO",
			now:     fixedDate(2026, time.February, 3),
			current: "2026.02.03.1-beta.2",
			want:    "2026.02.03.1-beta.3",
		},
		{
			name:    "day format starts a new base on the next day",
			format:  "YYYY.0M.0D.MICRO",
			now:     fixedDate(2026, time.February, 4),
			current: "2026.02.03.1-beta.2",
			want:    "2026.02.04.1-beta.1",
		},
		{
			name:    "week format starts a new base in the next week",
			format:  "YYYY.0W.MICRO",
			now:     fixedDate(2026, time.February, 8),
			current: "2026.05.2-beta.4",
			want:    "2026.06.1-beta.1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a calver strategy with a fixed clock and the latest version on the channel
			cv := newCalVer(t, testCase.format, "v", testCase.now)

			// when: the next beta release is resolved
			next, bump, shouldRelease, err := cv.NextRelease(testCase.current, commit.BumpPatch, "", "beta")

			// then: the version is the next stable base with the beta counter
			testastic.NoError(t, err)
			testastic.Equal(t, testCase.want, next)
			testastic.Equal(t, commit.BumpPatch, bump)
			testastic.Equal(t, true, shouldRelease)
		})
	}

	t.Run("reports nothing due without a bump", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel and no releasable commits
		cv := newCalVer(t, "", "v", fixedTime(2026, time.November))

		// when: the next beta release is resolved
		next, bump, shouldRelease, err := cv.NextRelease("2026.11.2-beta.1", commit.BumpNone, "", "beta")

		// then: no release is due
		testastic.NoError(t, err)
		testastic.Equal(t, "", next)
		testastic.Equal(t, commit.BumpNone, bump)
		testastic.Equal(t, false, shouldRelease)
	})

	t.Run("rejects a prerelease from a future calendar period", func(t *testing.T) {
		t.Parallel()

		// given: the latest beta belongs to a period after the fixed clock
		cv := newCalVer(t, "", "v", fixedTime(2026, time.November))

		// when: the next beta release is resolved
		_, _, _, err := cv.NextRelease("2026.12.1-beta.1", commit.BumpPatch, "", "beta")

		// then: the future version is rejected
		testastic.ErrorIs(t, err, version.ErrInvalidVersion)
		testastic.ErrorContains(t, err, "future calendar period")
	})

	t.Run("rejects an invalid prerelease counter", func(t *testing.T) {
		t.Parallel()

		// given: the latest beta has a counter that is not a number
		cv := newCalVer(t, "", "v", fixedTime(2026, time.November))

		// when: the next beta release is resolved
		_, _, _, err := cv.NextRelease("2026.11.2-beta.x", commit.BumpPatch, "", "beta")

		// then: the counter is reported
		testastic.ErrorContains(t, err, `invalid prerelease counter "x"`)
	})
}

func TestCalVerNextAfterPrerelease(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		now     func() time.Time
		current string
		want    string
	}{
		{
			name:    "stable release in the same period takes the prerelease base",
			now:     fixedTime(2026, time.November),
			current: "2026.11.2-beta.3",
			want:    "2026.11.2",
		},
		{
			name:    "stable release in a later period resets micro",
			now:     fixedTime(2026, time.December),
			current: "2026.11.2-beta.3",
			want:    "2026.12.1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: the latest version is a beta
			cv := newCalVer(t, "", "v", testCase.now)

			// when: the next stable version is calculated from it
			next, err := cv.Next(testCase.current, commit.BumpPatch)

			// then: the beta base is not skipped
			testastic.NoError(t, err)
			testastic.Equal(t, testCase.want, next)
		})
	}
}

func TestCalVerCurrentPrerelease(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		name, format, tag, want string
	}{
		{name: "keeps the prerelease suffix", tag: "v2026.11.2-beta.1", want: "2026.11.2-beta.1"},
		{name: "normalizes the base", tag: "v2026.2.1-rc.1", want: "2026.02.1-rc.1"},
		{name: "accepts a dotted suffix", tag: "v2026.11.2-beta.1.hotfix", want: "2026.11.2-beta.1.hotfix"},
		{name: "uses the configured format", format: "YY.MM.MICRO", tag: "v26.02.1-beta.3", want: "26.2.1-beta.3"},
	}

	for _, testCase := range validCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a prerelease tag
			cv := newCalVer(t, testCase.format, "v", nil)

			// when: parsing the tag
			current, err := cv.Current(testCase.tag)

			// then: the base is rendered in the format and the suffix is kept
			testastic.NoError(t, err)
			testastic.Equal(t, testCase.want, current)
		})
	}

	for _, tag := range []string{
		"v2026.11.2-",
		"v2026.11.2-beta..1",
		"v2026.11.2-beta.01",
		"v2026.11.2-beta.1+build",
		"v2026.11.2-beta!",
		"v2026.13.2-beta.1",
	} {
		t.Run("rejects "+tag, func(t *testing.T) {
			t.Parallel()

			// given: a tag with an invalid base or prerelease suffix
			cv := newCalVer(t, "", "v", nil)

			// when: parsing the tag
			_, err := cv.Current(tag)

			// then: the tag is rejected
			testastic.ErrorIs(t, err, version.ErrInvalidVersion)
		})
	}
}

func TestCalVerReleaseAs(t *testing.T) {
	t.Parallel()

	november := fixedTime(2026, time.November)

	t.Run("normalizes the footer to the format", func(t *testing.T) {
		t.Parallel()

		// given: a footer with an unpadded month
		cv := newCalVer(t, "", "v", november)

		// when: the footer is normalized
		normalized, err := cv.NormalizeReleaseAs("2026.1.5")

		// then: it is rendered in the configured format
		testastic.NoError(t, err)
		testastic.Equal(t, "2026.01.5", normalized)
	})

	validCases := []struct {
		name, current, releaseAs, channel, want string
		bump, wantBump                          commit.BumpType
	}{
		{
			name: "sets the next stable version", current: "2026.11.1", releaseAs: "2026.11.5",
			bump: commit.BumpNone, want: "2026.11.5", wantBump: commit.BumpPatch,
		},
		{
			name: "sets the first version", releaseAs: "2026.11.3",
			bump: commit.BumpPatch, want: "2026.11.3", wantBump: commit.BumpPatch,
		},
		{
			name: "keeps the commit bump", current: "2026.10.4", releaseAs: "2026.11.2",
			bump: commit.BumpMinor, want: "2026.11.2", wantBump: commit.BumpMinor,
		},
		{
			name: "adds the channel suffix", current: "2026.11.2-beta.1", releaseAs: "2026.11.5", channel: "beta",
			bump: commit.BumpPatch, want: "2026.11.5-beta.1", wantBump: commit.BumpPatch,
		},
	}

	for _, testCase := range validCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a calver strategy with a fixed clock and a Release-As footer
			cv := newCalVer(t, "", "v", november)

			// when: the next release is resolved
			next, bump, shouldRelease, err := cv.NextRelease(
				testCase.current, testCase.bump, testCase.releaseAs, testCase.channel)

			// then: the footer sets the version
			testastic.NoError(t, err)
			testastic.Equal(t, testCase.want, next)
			testastic.Equal(t, testCase.wantBump, bump)
			testastic.Equal(t, true, shouldRelease)
		})
	}

	invalidCases := []struct {
		name, current, releaseAs, channel, problem string
		reason                                     version.ReleaseAsReason
	}{
		{
			name: "rejects a version in another format", current: "2026.11.1", releaseAs: "2.0.0",
			problem: "requested version does not match the calver format YYYY.0M.MICRO",
			reason:  version.ReleaseAsReasonFormat,
		},
		{
			name: "rejects a prerelease", current: "2026.11.1", releaseAs: "2026.11.5-beta.1",
			problem: "requested version must be stable",
		},
		{
			name: "rejects a future period", current: "2026.11.1", releaseAs: "2026.12.1",
			problem: "requested version is from a future calendar period",
			reason:  version.ReleaseAsReasonFuturePeriod,
		},
		{
			name: "rejects the current version", current: "2026.11.1", releaseAs: "2026.11.1",
			problem: "requested version is not newer than the current version",
		},
		{
			name: "rejects the base of the current prerelease", current: "2026.11.2-beta.1", releaseAs: "2026.11.2",
			channel: "beta", problem: "requested version is not newer than the current version",
		},
	}

	for _, testCase := range invalidCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a calver strategy with a fixed clock and an invalid Release-As footer
			cv := newCalVer(t, "", "v", november)

			// when: the next release is resolved
			_, _, _, err := cv.NextRelease(testCase.current, commit.BumpPatch, testCase.releaseAs, testCase.channel)

			// then: the footer is rejected with the problem
			testastic.ErrorIs(t, err, version.ErrInvalidReleaseAs)

			var releaseAs *version.ReleaseAsError

			testastic.ErrorAs(t, err, &releaseAs)

			if releaseAs == nil {
				return
			}

			testastic.Equal(t, testCase.problem, releaseAs.Problem)
			testastic.Equal(t, testCase.reason, releaseAs.Reason)
		})
	}
}
