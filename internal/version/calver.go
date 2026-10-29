package version

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/commit"
)

var _ Strategy = (*CalVer)(nil)

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

type CalVer struct {
	format calverFormat
	prefix string
	label  string
	clock  func() time.Time
}

// NewCalVer returns a calver strategy. A non-empty label is added to every stable version, such as 2026.11.1-beta.
func NewCalVer(format, prefix, label string, now func() time.Time) (*CalVer, error) {
	if strings.TrimSpace(format) == "" {
		format = DefaultCalVerFormat
	}

	compiled, err := compileCalVerFormat(format)
	if err != nil {
		return nil, err
	}

	if label != "" {
		err = ValidateCalVerLabel(label)
		if err != nil {
			return nil, err
		}
	}

	return &CalVer{format: compiled, prefix: prefix, label: label, clock: now}, nil
}

// ValidateCalVerLabel checks that label is one semver prerelease identifier with at least one letter.
// A label has no dot, so it cannot be mistaken for a channel prerelease such as beta.1.
func ValidateCalVerLabel(label string) error {
	hasLetter := false

	for _, ch := range label {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z':
			hasLetter = true
		case ch >= '0' && ch <= '9', ch == '-':
		default:
			return fmt.Errorf("%w: calver label %q must contain only letters, digits, and hyphens",
				ErrInvalidVersion, label)
		}
	}

	if !hasLetter {
		return fmt.Errorf("%w: calver label %q must contain a letter", ErrInvalidVersion, label)
	}

	return nil
}

func (c *CalVer) Tag(version string) string {
	return c.prefix + version
}

func (c *CalVer) SupportsReleaseAs() bool {
	return true
}

func (c *CalVer) SupportsPrerelease() bool {
	return true
}

func (c *CalVer) NormalizeReleaseAs(value string) (string, error) {
	parts, err := c.parseReleaseAs(value, "")
	if err != nil {
		return "", err
	}

	return c.format.render(parts), nil
}

func (c *CalVer) NextRelease(
	current string,
	bump commit.BumpType,
	releaseAs, prereleaseIdentifier string,
) (string, commit.BumpType, bool, error) {
	if releaseAs != "" {
		next, err := c.applyReleaseAs(current, releaseAs)
		if err != nil {
			return "", commit.BumpNone, false, err
		}

		if prereleaseIdentifier != "" {
			next.Prerelease = prereleaseIdentifier + ".1"
		} else {
			next.Label = c.label
		}

		// Calver has no bump levels, so a forced release counts as at least a patch.
		if bump == commit.BumpNone {
			bump = commit.BumpPatch
		}

		return c.format.render(next), bump, true, nil
	}

	if bump == commit.BumpNone {
		return "", bump, false, nil
	}

	if prereleaseIdentifier != "" {
		next, err := c.nextPrerelease(current, prereleaseIdentifier)
		if err != nil {
			return "", commit.BumpNone, false, fmt.Errorf("calculate next prerelease version: %w", err)
		}

		return next, bump, true, nil
	}

	next, err := c.Next(current, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next version: %w", err)
	}

	return next, bump, true, nil
}

func (c *CalVer) PrereleaseAllowed(version, identifier string) bool {
	parts, err := c.format.parse(version)
	if err != nil {
		return false
	}

	return prereleaseAllowed(parts.Prerelease, identifier)
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

func (c *CalVer) OrderedRefs(refs []string, prereleaseIdentifier string) []string {
	return orderedRefs(refs, c.parseRef,
		func(parts calverParts) bool { return prereleaseAllowed(parts.Prerelease, prereleaseIdentifier) },
		func(left, right calverParts) int { return compareCalVerParts(c.format, left, right) })
}

func (c *CalVer) Next(current string, bump commit.BumpType) (string, error) {
	if bump == commit.BumpNone {
		return current, nil
	}

	next, err := c.nextStable(current)
	if err != nil {
		return "", err
	}

	next.Label = c.label

	return c.format.render(next), nil
}

func (c *CalVer) InitialVersion() string {
	return ""
}

// nextStable returns the stable version that follows current at the current time, without a label.
// A prerelease does not release its base version, so in the same period that base is next.
// A labeled version is a release, so the next micro follows it.
func (c *CalVer) nextStable(current string) (calverParts, error) {
	nowParts := c.format.partsFromTime(c.now())
	nowParts.Micro = 1

	if current == "" {
		return nowParts, nil
	}

	parts, err := c.format.parse(current)
	if err != nil {
		return calverParts{}, err
	}

	if compareCalVerPeriods(c.format, parts, nowParts) > 0 {
		return calverParts{}, fmt.Errorf(
			"%w: current calver %q is from a future calendar period", ErrInvalidVersion, current)
	}

	if !c.format.sameCalendarPeriod(parts, nowParts) {
		return nowParts, nil
	}

	nowParts.Micro = parts.Micro
	if parts.Prerelease == "" {
		nowParts.Micro++
	}

	return nowParts, nil
}

// nextPrerelease returns the next prerelease of the stable version that follows current.
// The counter continues while that stable version stays the same, and restarts at 1 when it changes.
func (c *CalVer) nextPrerelease(current, prereleaseIdentifier string) (string, error) {
	next, err := c.nextStable(current)
	if err != nil {
		return "", err
	}

	next.Prerelease = prereleaseIdentifier + ".1"

	if current == "" {
		return c.format.render(next), nil
	}

	parts, err := c.format.parse(current)
	if err != nil {
		return "", err
	}

	if parts.Prerelease == "" || parts.Micro != next.Micro || !c.format.sameCalendarPeriod(parts, next) {
		return c.format.render(next), nil
	}

	counter, ok, err := nextPrereleaseCounter(parts.Prerelease, prereleaseIdentifier)
	if err != nil {
		return "", err
	}

	if ok {
		next.Prerelease = fmt.Sprintf("%s.%d", prereleaseIdentifier, counter)
	}

	return c.format.render(next), nil
}

func (c *CalVer) parseReleaseAs(releaseAs, current string) (calverParts, error) {
	parts, err := c.format.parse(releaseAs)
	if err != nil {
		formatErr := releaseAsError(releaseAs, current,
			"requested version does not match the calver format "+c.format.raw,
			fmt.Errorf("%w: %q does not match calver format %q: %v", ErrInvalidReleaseAs, releaseAs, c.format.raw, err))
		formatErr.Reason = ReleaseAsReasonFormat

		return calverParts{}, formatErr
	}

	return parts, nil
}

// applyReleaseAs checks that releaseAs is a stable version after current and not in a future period.
// A later period would stop the next releases until that period starts.
func (c *CalVer) applyReleaseAs(current, releaseAs string) (calverParts, error) {
	target, err := c.parseReleaseAs(releaseAs, current)
	if err != nil {
		return calverParts{}, err
	}

	if target.Prerelease != "" {
		return calverParts{}, releaseAsError(releaseAs, current, "requested version must be stable",
			fmt.Errorf("%w: %q must be a stable version", ErrInvalidReleaseAs, releaseAs))
	}

	if target.Label != "" {
		return calverParts{}, releaseAsError(releaseAs, current, "requested version must not include a label",
			fmt.Errorf("%w: %q must not include a label, calver.label adds it", ErrInvalidReleaseAs, releaseAs))
	}

	if compareCalVerPeriods(c.format, target, c.format.partsFromTime(c.now())) > 0 {
		futureErr := releaseAsError(releaseAs, current, "requested version is from a future calendar period",
			fmt.Errorf("%w: %q is from a future calendar period", ErrInvalidReleaseAs, releaseAs))
		futureErr.Reason = ReleaseAsReasonFuturePeriod

		return calverParts{}, futureErr
	}

	if current == "" {
		return target, nil
	}

	currentParts, err := c.format.parse(current)
	if err != nil {
		return calverParts{}, releaseAsError(releaseAs, current, "current version is not valid calver",
			fmt.Errorf("%w: parse current version %q: %v", ErrInvalidReleaseAs, current, err))
	}

	// The requested version must be after the base of the current prerelease or labeled release.
	currentParts.Prerelease = ""
	currentParts.Label = ""

	if compareCalVerParts(c.format, target, currentParts) <= 0 {
		return calverParts{}, releaseAsError(c.format.render(target), c.format.render(currentParts),
			"requested version is not newer than the current version",
			fmt.Errorf("%w: %s must be greater than current version %s", ErrInvalidReleaseAs,
				c.format.render(target), c.format.render(currentParts)))
	}

	return target, nil
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
	Year       int
	Month      int
	Week       int
	Day        int
	Micro      int
	Prerelease string
	// Label is a suffix without a dot, such as beta. Unlike a prerelease, a labeled version is a release.
	Label string
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

// parse reads a version in the format, with an optional semver prerelease suffix such as -beta.1.
// A suffix that is a valid label, such as -beta, is a label and not a prerelease.
func (f calverFormat) parse(version string) (calverParts, error) {
	version = strings.TrimSpace(version)

	// The format allows only dots as separators, so the first hyphen starts the prerelease.
	core, prerelease, hasPrerelease := strings.Cut(version, "-")

	parts, err := f.parseCore(core)
	if err != nil {
		return calverParts{}, err
	}

	if hasPrerelease {
		parsed, semverErr := semver.StrictNewVersion("0.0.0-" + prerelease)
		if semverErr != nil || parsed.Metadata() != "" {
			return calverParts{}, fmt.Errorf("%w: invalid prerelease %q in %q", ErrInvalidVersion, prerelease, version)
		}

		if ValidateCalVerLabel(prerelease) == nil {
			parts.Label = prerelease
		} else {
			parts.Prerelease = prerelease
		}
	}

	return parts, nil
}

func (f calverFormat) parseCore(version string) (calverParts, error) {
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
		Week:  ordinalWeek(t),
		Day:   t.Day(),
	}
}

func ordinalWeek(t time.Time) int {
	return ((t.YearDay() - 1) / weekDays) + 1
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

	if suffix := parts.suffix(); suffix != "" {
		builder.WriteString("-" + suffix)
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

	if micro := cmp.Compare(left.Micro, right.Micro); micro != 0 {
		return micro
	}

	leftSuffix, rightSuffix := left.suffix(), right.suffix()
	if leftSuffix == rightSuffix {
		return 0
	}

	// A version without a suffix sorts after its prereleases and labeled versions.
	return semver.New(0, 0, 0, leftSuffix, "").Compare(semver.New(0, 0, 0, rightSuffix, ""))
}

// suffix returns the prerelease or the label. A version has at most one of them.
func (p *calverParts) suffix() string {
	if p.Prerelease != "" {
		return p.Prerelease
	}

	return p.Label
}
