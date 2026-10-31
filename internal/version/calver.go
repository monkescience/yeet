package version

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/commit"
)

var _ Strategy = (*CalVer)(nil)

const DefaultCalVerFormat = "YYYY.0M.MICRO"

const (
	maxMonth = 12
	maxDay   = 31
	maxWeek  = 53

	fullYearMinDigits = 4
	shortYearBase     = 2000
	shortYearPad      = 2

	optionalMicroParts = 2
)

type CalVer struct {
	format calverFormat
	prefix string
	clock  func() time.Time
}

func NewCalVer(format, prefix string, now func() time.Time) (*CalVer, error) {
	if strings.TrimSpace(format) == "" {
		format = DefaultCalVerFormat
	}

	compiled, err := compileCalVerFormat(format)
	if err != nil {
		return nil, err
	}

	return &CalVer{format: compiled, prefix: prefix, clock: now}, nil
}

func (c *CalVer) Tag(version string) string {
	return c.prefix + version
}

func (c *CalVer) SupportsReleaseAs() bool {
	return false
}

func (c *CalVer) SupportsPrerelease() bool {
	return false
}

func (c *CalVer) NormalizeReleaseAs(value string) (string, error) {
	return "", releaseAsError(value, "", "calver targets do not support Release-As",
		fmt.Errorf("%w: calver targets do not support %q", ErrInvalidReleaseAs, value))
}

func (c *CalVer) NextRelease(
	current string,
	bump commit.BumpType,
	_, _ string,
) (string, commit.BumpType, bool, error) {
	if bump == commit.BumpNone {
		return "", bump, false, nil
	}

	next, err := c.Next(current, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next version: %w", err)
	}

	return next, bump, true, nil
}

func (c *CalVer) PrereleaseAllowed(_, identifier string) bool {
	return identifier == ""
}

func ValidateCalVerFormat(format string) error {
	_, err := compileCalVerFormat(format)
	if err != nil {
		return err
	}

	return nil
}

func (c *CalVer) Current(tag string) (string, error) {
	parts, err := c.parseRef(tag)
	if err != nil {
		return "", err
	}

	return c.format.render(parts), nil
}

func (c *CalVer) OrderedRefs(refs []string, _ string) []string {
	return orderedRefs(refs, c.parseRef, func(calverParts) bool { return true },
		func(left, right calverParts) int { return compareCalVerParts(c.format, left, right) })
}

func (c *CalVer) Next(current string, bump commit.BumpType) (string, error) {
	if bump == commit.BumpNone {
		return current, nil
	}

	now := c.now()
	nowParts := c.format.partsFromTime(now)
	nowParts.Micro = c.format.firstMicro()

	if current == "" {
		return c.format.render(nowParts), nil
	}

	parts, err := c.format.parse(current)
	if err != nil {
		return "", err
	}

	if compareCalVerPeriods(c.format, parts, nowParts) > 0 {
		return "", fmt.Errorf("%w: current calver %q is from a future calendar period", ErrInvalidVersion, current)
	}

	if c.format.sameCalendarPeriod(parts, nowParts) {
		nowParts.Micro = parts.Micro + 1
	}

	return c.format.render(nowParts), nil
}

func (c *CalVer) InitialVersion() string {
	return ""
}

func (c *CalVer) parseRef(ref string) (calverParts, error) {
	return c.format.parse(strings.TrimPrefix(ref, c.prefix))
}

func (c *CalVer) now() time.Time {
	if c.clock != nil {
		return c.clock()
	}

	return time.Now()
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
	calverTokenMonth     calverToken = "M"
	calverTokenMonthPad  calverToken = "0M"
	calverTokenWeek      calverToken = "W"
	calverTokenWeekPad   calverToken = "0W"
	calverTokenDay       calverToken = "D"
	calverTokenDayPad    calverToken = "0D"
	calverTokenMicro     calverToken = "MICRO"
)

type calverFormat struct {
	raw           string
	parts         []calverFormatPart
	tokens        []calverToken
	hasMonth      bool
	hasWeek       bool
	hasDay        bool
	optionalMicro bool
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
	{text: "MM", token: calverTokenMonth},
	{text: string(calverTokenMonth), token: calverTokenMonth},
	{text: string(calverTokenMonthPad), token: calverTokenMonthPad},
	{text: "WW", token: calverTokenWeek},
	{text: string(calverTokenWeek), token: calverTokenWeek},
	{text: string(calverTokenWeekPad), token: calverTokenWeekPad},
	{text: "DD", token: calverTokenDay},
	{text: string(calverTokenDay), token: calverTokenDay},
	{text: string(calverTokenDayPad), token: calverTokenDayPad},
}

func compileCalVerFormat(rawFormat string) (calverFormat, error) {
	format := strings.TrimSpace(rawFormat)
	if format == "" {
		return calverFormat{}, fmt.Errorf("%w: calver format must not be empty", ErrInvalidVersion)
	}

	compiled := calverFormat{raw: format}

	pattern, optionalMicro, err := stripOptionalMicro(format)
	if err != nil {
		return calverFormat{}, err
	}

	compiled.optionalMicro = optionalMicro
	seen := make(map[calverToken]string)

	for idx := 0; idx < len(pattern); {
		if spec, ok := matchCalVerToken(pattern[idx:]); ok {
			if first, ok := seen[spec.token]; ok {
				return calverFormat{}, duplicateCalVerTokenError(first, spec.text, format)
			}

			seen[spec.token] = spec.text
			compiled.parts = append(compiled.parts, calverFormatPart{token: spec.token})
			compiled.tokens = append(compiled.tokens, spec.token)
			idx += len(spec.text)

			continue
		}

		literalStart := idx
		for idx < len(pattern) {
			if _, ok := matchCalVerToken(pattern[idx:]); ok {
				break
			}

			if pattern[idx] != '.' {
				return calverFormat{}, fmt.Errorf(
					"%w: calver format only supports dots as separators: %q",
					ErrInvalidVersion,
					format,
				)
			}

			idx++
		}

		compiled.parts = append(compiled.parts, calverFormatPart{literal: pattern[literalStart:idx]})
	}

	err = validateCompiledCalVerFormat(format, &compiled, seen)
	if err != nil {
		return calverFormat{}, err
	}

	return compiled, nil
}

func stripOptionalMicro(format string) (string, bool, error) {
	if !strings.ContainsAny(format, "[]") {
		return format, false, nil
	}

	base, optional, opened := strings.Cut(format, "[")
	optional, closed := strings.CutSuffix(optional, "]")
	separator, hasMicro := strings.CutSuffix(optional, string(calverTokenMicro))

	if !opened || !closed || !hasMicro || separator == "" || strings.Trim(separator, ".") != "" ||
		strings.HasSuffix(base, ".") || strings.Contains(base, "]") {
		return "", false, fmt.Errorf(
			"%w: calver format brackets must wrap a separator and MICRO at the end, as in [.MICRO]: %q",
			ErrInvalidVersion,
			format,
		)
	}

	return base + optional, true, nil
}

func duplicateCalVerTokenError(first, second, format string) error {
	if first != second {
		return fmt.Errorf(
			"%w: calver format uses %s and %s, which are the same token: %q",
			ErrInvalidVersion,
			first,
			second,
			format,
		)
	}

	return fmt.Errorf("%w: duplicate %s token in calver format %q", ErrInvalidVersion, second, format)
}

func validateCompiledCalVerFormat(format string, compiled *calverFormat, seen map[calverToken]string) error {
	if len(compiled.parts) == 0 || compiled.parts[0].literal != "" || compiled.parts[len(compiled.parts)-1].literal != "" {
		return fmt.Errorf("%w: calver format must start and end with a token: %q", ErrInvalidVersion, format)
	}

	if countSeen(seen, calverTokenYearFull, calverTokenYearShort, calverTokenYearPad) == 0 {
		return fmt.Errorf("%w: calver format must include a year token: %q", ErrInvalidVersion, format)
	}

	if countSeen(seen, calverTokenYearFull, calverTokenYearShort, calverTokenYearPad) > 1 {
		return fmt.Errorf("%w: calver format must include only one year token: %q", ErrInvalidVersion, format)
	}

	if countSeen(seen, calverTokenMicro) == 0 {
		return fmt.Errorf("%w: calver format must include MICRO: %q", ErrInvalidVersion, format)
	}

	if compiled.tokens[len(compiled.tokens)-1] != calverTokenMicro {
		return fmt.Errorf("%w: calver format must end with MICRO: %q", ErrInvalidVersion, format)
	}

	compiled.hasMonth = countSeen(seen, calverTokenMonth, calverTokenMonthPad) > 0
	compiled.hasWeek = countSeen(seen, calverTokenWeek, calverTokenWeekPad) > 0
	compiled.hasDay = countSeen(seen, calverTokenDay, calverTokenDayPad) > 0

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

	if compiled.optionalMicro && !compiled.hasMonth && !compiled.hasWeek {
		return fmt.Errorf("%w: optional MICRO requires a month or week token: %q", ErrInvalidVersion, format)
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

func countSeen(seen map[calverToken]string, tokens ...calverToken) int {
	count := 0

	for _, token := range tokens {
		if _, ok := seen[token]; ok {
			count++
		}
	}

	return count
}

func (f calverFormat) parse(version string) (calverParts, error) {
	version = strings.TrimSpace(version)
	parts := calverParts{}
	position := 0

	optionalLiteral := f.optionalMicroLiteralIndex()

	for idx, formatPart := range f.parts {
		if idx == optionalLiteral && position == len(version) {
			break
		}

		if formatPart.literal != "" {
			if !strings.HasPrefix(version[position:], formatPart.literal) {
				return calverParts{}, fmt.Errorf("%w: expected literal %q in %q", ErrInvalidVersion, formatPart.literal, version)
			}

			position += len(formatPart.literal)

			continue
		}

		valueEnd, err := f.tokenValueEnd(version, position, idx)
		if err != nil {
			return calverParts{}, err
		}

		value := version[position:valueEnd]
		if value == "" {
			return calverParts{}, fmt.Errorf("%w: empty %s segment in %q", ErrInvalidVersion, formatPart.token, version)
		}

		err = parts.set(formatPart.token, value, version)
		if err != nil {
			return calverParts{}, err
		}

		if f.optionalMicro && formatPart.token == calverTokenMicro && parts.Micro == 0 {
			return calverParts{}, fmt.Errorf(
				"%w: micro version 0 must be omitted in %q for calver format %q",
				ErrInvalidVersion,
				version,
				f.raw,
			)
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

func (f calverFormat) tokenValueEnd(version string, position, idx int) (int, error) {
	nextLiteral := nextCalVerLiteral(f.parts[idx+1:])
	if nextLiteral == "" {
		return len(version), nil
	}

	if nextIdx := strings.Index(version[position:], nextLiteral); nextIdx >= 0 {
		return position + nextIdx, nil
	}

	if idx+1 == f.optionalMicroLiteralIndex() {
		return len(version), nil
	}

	return 0, fmt.Errorf("%w: expected literal %q in %q", ErrInvalidVersion, nextLiteral, version)
}

func (f calverFormat) optionalMicroLiteralIndex() int {
	if !f.optionalMicro {
		return -1
	}

	return len(f.parts) - optionalMicroParts
}

func (f calverFormat) firstMicro() int {
	if f.optionalMicro {
		return 0
	}

	return 1
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
		if value < 0 || len(rawValue) >= fullYearMinDigits {
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
	if f.hasWeek && parts.Week > isoWeeksInYear(parts.Year) {
		return fmt.Errorf("%w: ISO year %d has no week %d in %q", ErrInvalidVersion, parts.Year, parts.Week, version)
	}

	if f.hasMonth && f.hasDay {
		date := time.Date(parts.Year, time.Month(parts.Month), parts.Day, 0, 0, 0, 0, time.UTC)
		if date.Year() != parts.Year || int(date.Month()) != parts.Month || date.Day() != parts.Day {
			return fmt.Errorf("%w: invalid date in %q", ErrInvalidVersion, version)
		}
	}

	return nil
}

func isoWeeksInYear(year int) int {
	_, week := time.Date(year, time.December, 28, 0, 0, 0, 0, time.UTC).ISOWeek()

	return week
}

func (f calverFormat) partsFromTime(t time.Time) calverParts {
	if f.hasWeek {
		year, week := t.ISOWeek()

		return calverParts{Year: year, Week: week}
	}

	return calverParts{
		Year:  t.Year(),
		Month: int(t.Month()),
		Day:   t.Day(),
	}
}

func (f calverFormat) render(parts calverParts) string {
	var builder strings.Builder

	formatParts := f.parts
	if f.optionalMicro && parts.Micro == 0 {
		formatParts = formatParts[:f.optionalMicroLiteralIndex()]
	}

	for _, part := range formatParts {
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

func compareCalVerPeriods(format calverFormat, left, right calverParts) int {
	if left.Year != right.Year {
		return cmp.Compare(left.Year, right.Year)
	}

	if format.hasMonth && left.Month != right.Month {
		return cmp.Compare(left.Month, right.Month)
	}

	if format.hasWeek && left.Week != right.Week {
		return cmp.Compare(left.Week, right.Week)
	}

	if format.hasDay && left.Day != right.Day {
		return cmp.Compare(left.Day, right.Day)
	}

	return 0
}

func compareCalVerParts(format calverFormat, left, right calverParts) int {
	if period := compareCalVerPeriods(format, left, right); period != 0 {
		return period
	}

	return cmp.Compare(left.Micro, right.Micro)
}
