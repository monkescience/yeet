package version

type MarkerToken string

const (
	MarkerTokenYear  MarkerToken = "year"
	MarkerTokenMonth MarkerToken = "month"
	MarkerTokenWeek  MarkerToken = "week"
	MarkerTokenDay   MarkerToken = "day"
	MarkerTokenMicro MarkerToken = "micro"
)

type CalVerScheme struct {
	format calverFormat
}

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

func (s *CalVerScheme) Format() string {
	return s.format.raw
}

func (s *CalVerScheme) HasMonth() bool {
	return s.format.hasMonth
}

func (s *CalVerScheme) HasWeek() bool {
	return s.format.hasWeek
}

func (s *CalVerScheme) HasDay() bool {
	return s.format.hasDay
}

func (s *CalVerScheme) MarkerValues(version string) (map[MarkerToken]string, error) {
	parts, err := s.format.parse(version)
	if err != nil {
		return nil, err
	}

	values := map[MarkerToken]string{
		MarkerTokenYear:  s.renderConfiguredToken(MarkerTokenYear, parts),
		MarkerTokenMicro: renderCalVerToken(calverTokenMicro, parts),
	}

	if s.format.hasMonth {
		values[MarkerTokenMonth] = s.renderConfiguredToken(MarkerTokenMonth, parts)
	}

	if s.format.hasWeek {
		values[MarkerTokenWeek] = s.renderConfiguredToken(MarkerTokenWeek, parts)
	}

	if s.format.hasDay {
		values[MarkerTokenDay] = s.renderConfiguredToken(MarkerTokenDay, parts)
	}

	return values, nil
}

func (s *CalVerScheme) renderConfiguredToken(kind MarkerToken, parts calverParts) string {
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
