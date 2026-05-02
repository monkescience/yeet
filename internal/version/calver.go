package version

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/commit"
)

const DefaultCalVerFormat = "YYYY.0M.MICRO"

const (
	maxMonth = 12
	maxDay   = 31
	maxWeek  = 53
	weekDays = 7

	fullYearMinDigits = 4
	shortYearBase     = 2000
	shortYearPad      = 2
)

// CalVer implements calendar versioning.
// Supported formats use CalVer date tokens and a MICRO incrementing counter.
type CalVer struct {
	Format string
	Prefix string
	Now    func() time.Time
}

func ValidateCalVerFormat(format string) error {
	_, err := compileCalVerFormat(format)

	return err
}

func (c *CalVer) Current(tag string) (string, error) {
	cleaned := strings.TrimPrefix(tag, c.Prefix)

	format, err := c.compileFormat()
	if err != nil {
		return "", err
	}

	parts, err := format.parse(cleaned)
	if err != nil {
		return "", err
	}

	return format.render(parts), nil
}

// Next increments the micro counter if the current calendar period matches.
// Otherwise, it resets the micro counter to 1.
func (c *CalVer) Next(current string, bump commit.BumpType) (string, error) {
	if bump == commit.BumpNone {
		return current, nil
	}

	format, err := c.compileFormat()
	if err != nil {
		return "", err
	}

	now := c.now()
	nowParts := format.partsFromTime(now)

	if current == "" {
		nowParts.Micro = 1

		return format.render(nowParts), nil
	}

	parts, err := format.parse(current)
	if err != nil {
		return "", err
	}

	nowParts.Micro = 1
	if format.sameCalendarPeriod(parts, nowParts) {
		nowParts.Micro = parts.Micro + 1
	}

	return format.render(nowParts), nil
}

func (c *CalVer) Tag(version string) string {
	return c.Prefix + version
}

// InitialVersion returns an empty string since calver starts from the current date.
func (c *CalVer) InitialVersion() string {
	return ""
}

func (c *CalVer) Less(leftVersion, rightVersion, leftRef, rightRef string) bool {
	format, err := c.compileFormat()
	if err != nil {
		return leftRef < rightRef
	}

	leftParts, err := format.parse(leftVersion)
	if err != nil {
		return leftRef < rightRef
	}

	rightParts, err := format.parse(rightVersion)
	if err != nil {
		return leftRef < rightRef
	}

	if compareCalVerParts(format, leftParts, rightParts) != 0 {
		return compareCalVerParts(format, leftParts, rightParts) < 0
	}

	return leftRef < rightRef
}

func (c *CalVer) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}

	return time.Now()
}

func (c *CalVer) compileFormat() (calverFormat, error) {
	return compileCalVerFormat(c.format())
}

func (c *CalVer) format() string {
	if strings.TrimSpace(c.Format) == "" {
		return DefaultCalVerFormat
	}

	return c.Format
}

type calverParts struct {
	Year  int
	Month int
	Week  int
	Day   int
	Micro int
}

type calverToken string

const (
	calverTokenYearFull  calverToken = "YYYY"
	calverTokenYearShort calverToken = "YY"
	calverTokenYearPad   calverToken = "0Y"
	calverTokenMonth     calverToken = "MM"
	calverTokenMonthPad  calverToken = "0M"
	calverTokenWeek      calverToken = "WW"
	calverTokenWeekPad   calverToken = "0W"
	calverTokenDay       calverToken = "DD"
	calverTokenDayPad    calverToken = "0D"
	calverTokenMicro     calverToken = "MICRO"
)

type calverFormat struct {
	raw      string
	parts    []calverFormatPart
	tokens   []calverToken
	hasMonth bool
	hasWeek  bool
	hasDay   bool
}

type calverFormatPart struct {
	token   calverToken
	literal string
}

type calverTokenSpec struct {
	text  string
	token calverToken
}

var calverTokenSpecs = []calverTokenSpec{
	{text: string(calverTokenMicro), token: calverTokenMicro},
	{text: string(calverTokenYearFull), token: calverTokenYearFull},
	{text: string(calverTokenYearShort), token: calverTokenYearShort},
	{text: string(calverTokenYearPad), token: calverTokenYearPad},
	{text: string(calverTokenMonth), token: calverTokenMonth},
	{text: string(calverTokenMonthPad), token: calverTokenMonthPad},
	{text: string(calverTokenWeek), token: calverTokenWeek},
	{text: string(calverTokenWeekPad), token: calverTokenWeekPad},
	{text: string(calverTokenDay), token: calverTokenDay},
	{text: string(calverTokenDayPad), token: calverTokenDayPad},
}

func compileCalVerFormat(rawFormat string) (calverFormat, error) {
	format := strings.TrimSpace(rawFormat)
	if format == "" {
		return calverFormat{}, fmt.Errorf("%w: calver format must not be empty", ErrInvalidVersion)
	}

	compiled := calverFormat{raw: format}
	seen := make(map[calverToken]bool)

	for idx := 0; idx < len(format); {
		if spec, ok := matchCalVerToken(format[idx:]); ok {
			if seen[spec.token] {
				return calverFormat{}, fmt.Errorf(
					"%w: duplicate %s token in calver format %q",
					ErrInvalidVersion,
					spec.text,
					format,
				)
			}

			seen[spec.token] = true
			compiled.parts = append(compiled.parts, calverFormatPart{token: spec.token})
			compiled.tokens = append(compiled.tokens, spec.token)
			idx += len(spec.text)

			continue
		}

		literalStart := idx
		for idx < len(format) {
			if _, ok := matchCalVerToken(format[idx:]); ok {
				break
			}

			if format[idx] != '.' {
				return calverFormat{}, fmt.Errorf(
					"%w: calver format only supports dots as separators: %q",
					ErrInvalidVersion,
					format,
				)
			}

			idx++
		}

		compiled.parts = append(compiled.parts, calverFormatPart{literal: format[literalStart:idx]})
	}

	err := validateCompiledCalVerFormat(format, &compiled, seen)
	if err != nil {
		return calverFormat{}, err
	}

	return compiled, nil
}

func validateCompiledCalVerFormat(format string, compiled *calverFormat, seen map[calverToken]bool) error {
	if len(compiled.parts) == 0 || compiled.parts[0].literal != "" || compiled.parts[len(compiled.parts)-1].literal != "" {
		return fmt.Errorf("%w: calver format must start and end with a token: %q", ErrInvalidVersion, format)
	}

	if !seen[calverTokenYearFull] && !seen[calverTokenYearShort] && !seen[calverTokenYearPad] {
		return fmt.Errorf("%w: calver format must include a year token: %q", ErrInvalidVersion, format)
	}

	if countSeen(seen, calverTokenYearFull, calverTokenYearShort, calverTokenYearPad) > 1 {
		return fmt.Errorf("%w: calver format must include only one year token: %q", ErrInvalidVersion, format)
	}

	if !seen[calverTokenMicro] {
		return fmt.Errorf("%w: calver format must include MICRO: %q", ErrInvalidVersion, format)
	}

	if compiled.tokens[len(compiled.tokens)-1] != calverTokenMicro {
		return fmt.Errorf("%w: calver format must end with MICRO: %q", ErrInvalidVersion, format)
	}

	compiled.hasMonth = seen[calverTokenMonth] || seen[calverTokenMonthPad]
	compiled.hasWeek = seen[calverTokenWeek] || seen[calverTokenWeekPad]
	compiled.hasDay = seen[calverTokenDay] || seen[calverTokenDayPad]

	if countSeen(seen, calverTokenMonth, calverTokenMonthPad) > 1 {
		return fmt.Errorf("%w: calver format must include only one month token: %q", ErrInvalidVersion, format)
	}

	if countSeen(seen, calverTokenWeek, calverTokenWeekPad) > 1 {
		return fmt.Errorf("%w: calver format must include only one week token: %q", ErrInvalidVersion, format)
	}

	if countSeen(seen, calverTokenDay, calverTokenDayPad) > 1 {
		return fmt.Errorf("%w: calver format must include only one day token: %q", ErrInvalidVersion, format)
	}

	if compiled.hasWeek && (compiled.hasMonth || compiled.hasDay) {
		return fmt.Errorf("%w: week tokens cannot be combined with month or day tokens: %q", ErrInvalidVersion, format)
	}

	if compiled.hasDay && !compiled.hasMonth {
		return fmt.Errorf("%w: day tokens require a month token: %q", ErrInvalidVersion, format)
	}

	for idx := 1; idx < len(compiled.parts); idx++ {
		if compiled.parts[idx-1].literal == "" && compiled.parts[idx].literal == "" {
			return fmt.Errorf("%w: calver format tokens must be separated by literals: %q", ErrInvalidVersion, format)
		}
	}

	return nil
}

func matchCalVerToken(s string) (calverTokenSpec, bool) {
	for _, spec := range calverTokenSpecs {
		if strings.HasPrefix(s, spec.text) {
			return spec, true
		}
	}

	return calverTokenSpec{}, false
}

func countSeen(seen map[calverToken]bool, tokens ...calverToken) int {
	count := 0

	for _, token := range tokens {
		if seen[token] {
			count++
		}
	}

	return count
}

func (f calverFormat) parse(version string) (calverParts, error) {
	version = strings.TrimSpace(version)
	parts := calverParts{}
	position := 0

	for idx, formatPart := range f.parts {
		if formatPart.literal != "" {
			if !strings.HasPrefix(version[position:], formatPart.literal) {
				return calverParts{}, fmt.Errorf("%w: expected literal %q in %q", ErrInvalidVersion, formatPart.literal, version)
			}

			position += len(formatPart.literal)

			continue
		}

		valueEnd := len(version)
		if nextLiteral := nextCalVerLiteral(f.parts[idx+1:]); nextLiteral != "" {
			nextIdx := strings.Index(version[position:], nextLiteral)
			if nextIdx < 0 {
				return calverParts{}, fmt.Errorf("%w: expected literal %q in %q", ErrInvalidVersion, nextLiteral, version)
			}

			valueEnd = position + nextIdx
		}

		value := version[position:valueEnd]
		if value == "" {
			return calverParts{}, fmt.Errorf("%w: empty %s segment in %q", ErrInvalidVersion, formatPart.token, version)
		}

		err := parts.set(formatPart.token, value, version)
		if err != nil {
			return calverParts{}, err
		}

		position = valueEnd
	}

	if position != len(version) {
		return calverParts{}, fmt.Errorf("%w: unexpected trailing data in %q", ErrInvalidVersion, version)
	}

	err := f.validateParts(parts, version)
	if err != nil {
		return calverParts{}, err
	}

	return parts, nil
}

func nextCalVerLiteral(parts []calverFormatPart) string {
	for _, part := range parts {
		if part.literal != "" {
			return part.literal
		}
	}

	return ""
}

func (p *calverParts) set(token calverToken, rawValue, version string) error {
	value, err := strconv.Atoi(rawValue)
	if err != nil {
		return fmt.Errorf("%w: invalid %s segment %q in %q", ErrInvalidVersion, token, rawValue, version)
	}

	switch token {
	case calverTokenYearFull:
		if value < 1 || len(rawValue) < fullYearMinDigits {
			return fmt.Errorf("%w: invalid year %q in %q", ErrInvalidVersion, rawValue, version)
		}

		p.Year = value
	case calverTokenYearShort, calverTokenYearPad:
		if value < 0 {
			return fmt.Errorf("%w: invalid year %q in %q", ErrInvalidVersion, rawValue, version)
		}

		p.Year = shortYearBase + value
	case calverTokenMonth, calverTokenMonthPad:
		if value < 1 || value > maxMonth {
			return fmt.Errorf("%w: invalid month %q in %q", ErrInvalidVersion, rawValue, version)
		}

		p.Month = value
	case calverTokenWeek, calverTokenWeekPad:
		if value < 1 || value > maxWeek {
			return fmt.Errorf("%w: invalid week %q in %q", ErrInvalidVersion, rawValue, version)
		}

		p.Week = value
	case calverTokenDay, calverTokenDayPad:
		if value < 1 || value > maxDay {
			return fmt.Errorf("%w: invalid day %q in %q", ErrInvalidVersion, rawValue, version)
		}

		p.Day = value
	case calverTokenMicro:
		if value < 0 {
			return fmt.Errorf("%w: invalid micro version %q in %q", ErrInvalidVersion, rawValue, version)
		}

		p.Micro = value
	}

	return nil
}

func (f calverFormat) validateParts(parts calverParts, version string) error {
	if f.hasMonth && f.hasDay {
		date := time.Date(parts.Year, time.Month(parts.Month), parts.Day, 0, 0, 0, 0, time.UTC)
		if date.Year() != parts.Year || int(date.Month()) != parts.Month || date.Day() != parts.Day {
			return fmt.Errorf("%w: invalid date in %q", ErrInvalidVersion, version)
		}
	}

	return nil
}

func (f calverFormat) partsFromTime(t time.Time) calverParts {
	return calverParts{
		Year:  t.Year(),
		Month: int(t.Month()),
		Week:  ((t.YearDay() - 1) / weekDays) + 1,
		Day:   t.Day(),
	}
}

func (f calverFormat) render(parts calverParts) string {
	var builder strings.Builder

	for _, part := range f.parts {
		if part.literal != "" {
			builder.WriteString(part.literal)

			continue
		}

		builder.WriteString(renderCalVerToken(part.token, parts))
	}

	return builder.String()
}

func renderCalVerToken(token calverToken, parts calverParts) string {
	switch token {
	case calverTokenYearFull:
		return fmt.Sprintf("%04d", parts.Year)
	case calverTokenYearShort:
		return strconv.Itoa(parts.Year - shortYearBase)
	case calverTokenYearPad:
		year := parts.Year - shortYearBase
		if year < 100 { //nolint:mnd // 0Y pads two-digit years only
			return fmt.Sprintf("%0*d", shortYearPad, year)
		}

		return strconv.Itoa(year)
	case calverTokenMonth:
		return strconv.Itoa(parts.Month)
	case calverTokenMonthPad:
		return fmt.Sprintf("%02d", parts.Month)
	case calverTokenWeek:
		return strconv.Itoa(parts.Week)
	case calverTokenWeekPad:
		return fmt.Sprintf("%02d", parts.Week)
	case calverTokenDay:
		return strconv.Itoa(parts.Day)
	case calverTokenDayPad:
		return fmt.Sprintf("%02d", parts.Day)
	case calverTokenMicro:
		return strconv.Itoa(parts.Micro)
	default:
		return ""
	}
}

func (f calverFormat) sameCalendarPeriod(left, right calverParts) bool {
	if left.Year != right.Year {
		return false
	}

	if f.hasMonth && left.Month != right.Month {
		return false
	}

	if f.hasWeek && left.Week != right.Week {
		return false
	}

	if f.hasDay && left.Day != right.Day {
		return false
	}

	return true
}

func compareCalVerParts(format calverFormat, left, right calverParts) int {
	for _, token := range format.tokens {
		leftValue := calVerTokenValue(token, left)
		rightValue := calVerTokenValue(token, right)

		if leftValue < rightValue {
			return -1
		}

		if leftValue > rightValue {
			return 1
		}
	}

	return 0
}

func calVerTokenValue(token calverToken, parts calverParts) int {
	switch token {
	case calverTokenYearFull, calverTokenYearShort, calverTokenYearPad:
		return parts.Year
	case calverTokenMonth, calverTokenMonthPad:
		return parts.Month
	case calverTokenWeek, calverTokenWeekPad:
		return parts.Week
	case calverTokenDay, calverTokenDayPad:
		return parts.Day
	case calverTokenMicro:
		return parts.Micro
	default:
		return 0
	}
}
