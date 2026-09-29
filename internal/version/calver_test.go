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
		_, err := cv.Current("v2026.02.1-extra")

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
