package version

// MarkerToken names the addressable components of a parsed version string.
// Marker-based version-file updaters use these to look up the rendered value
// for a given file marker (e.g. x-yeet-month).
type MarkerToken string

const (
	MarkerTokenYear  MarkerToken = "year"
	MarkerTokenMonth MarkerToken = "month"
	MarkerTokenWeek  MarkerToken = "week"
	MarkerTokenDay   MarkerToken = "day"
	MarkerTokenMicro MarkerToken = "micro"
)

// CalVerScheme is a compiled CalVer format ready to extract per-token values
// from a version string. Construct via NewCalVerScheme; treat as opaque.
type CalVerScheme struct {
	format calverFormat
}

// NewCalVerScheme compiles the format once and returns a reusable scheme.
// The empty string compiles to DefaultCalVerFormat.
func NewCalVerScheme(format string) (*CalVerScheme, error) {
	if format == "" {
		format = DefaultCalVerFormat
	}

	compiled, err := compileCalVerFormat(format)
	if err != nil {
		return nil, err
	}

	return &CalVerScheme{format: compiled}, nil
}

// Format returns the raw format string the scheme was compiled from.
func (s *CalVerScheme) Format() string {
	return s.format.raw
}

// HasMonth reports whether the format addresses a month component.
func (s *CalVerScheme) HasMonth() bool {
	return s.format.hasMonth
}

// HasWeek reports whether the format addresses a week component.
func (s *CalVerScheme) HasWeek() bool {
	return s.format.hasWeek
}

// HasDay reports whether the format addresses a day component.
func (s *CalVerScheme) HasDay() bool {
	return s.format.hasDay
}

// MarkerValues parses a CalVer-shaped version string and returns rendered
// strings for each addressable token, keyed by canonical token name.
// Widths match the format's tokens (e.g. 0M zero-pads to 2). Tokens not
// present in the format are absent from the map.
func (s *CalVerScheme) MarkerValues(version string) (map[MarkerToken]string, error) {
	parts, err := s.format.parse(version)
	if err != nil {
		return nil, err
	}

	values := map[MarkerToken]string{
		MarkerTokenYear:  s.renderFor(MarkerTokenYear, parts),
		MarkerTokenMicro: renderCalVerToken(calverTokenMicro, parts),
	}

	if s.format.hasMonth {
		values[MarkerTokenMonth] = s.renderFor(MarkerTokenMonth, parts)
	}

	if s.format.hasWeek {
		values[MarkerTokenWeek] = s.renderFor(MarkerTokenWeek, parts)
	}

	if s.format.hasDay {
		values[MarkerTokenDay] = s.renderFor(MarkerTokenDay, parts)
	}

	return values, nil
}

// renderFor finds the format's token for a marker kind and renders it. Falls
// back to a width-less render when no matching token is present (only happens
// for the year, which is always present, so the loop always finds a match).
func (s *CalVerScheme) renderFor(kind MarkerToken, parts calverParts) string {
	for _, token := range s.format.tokens {
		if markerKindFor(token) == kind {
			return renderCalVerToken(token, parts)
		}
	}

	return ""
}

func markerKindFor(token calverToken) MarkerToken {
	switch token {
	case calverTokenYearFull, calverTokenYearShort, calverTokenYearPad:
		return MarkerTokenYear
	case calverTokenMonth, calverTokenMonthPad:
		return MarkerTokenMonth
	case calverTokenWeek, calverTokenWeekPad:
		return MarkerTokenWeek
	case calverTokenDay, calverTokenDayPad:
		return MarkerTokenDay
	case calverTokenMicro:
		return MarkerTokenMicro
	default:
		return ""
	}
}
