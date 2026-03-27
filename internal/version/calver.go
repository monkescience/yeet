package version

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/commit"
)

// CalVer implements calendar versioning.
// Supported format: YYYY.0M.MICRO where MICRO is an incrementing counter.
type CalVer struct {
	Format string
	Prefix string
	Now    func() time.Time
}

func (c *CalVer) Current(tag string) (string, error) {
	cleaned := strings.TrimPrefix(tag, c.Prefix)

	parts, err := c.parseParts(cleaned)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s.%s.%d", parts.Year, parts.Month, parts.Micro), nil
}

// Next increments the micro counter if the current year/month matches.
// Otherwise, it resets the micro counter to 1.
func (c *CalVer) Next(current string, bump commit.BumpType) (string, error) {
	if bump == commit.BumpNone {
		return current, nil
	}

	now := c.now()
	yearMonth := fmt.Sprintf("%d.%02d", now.Year(), now.Month())

	if current == "" {
		return yearMonth + ".1", nil
	}

	parts, err := c.parseParts(current)
	if err != nil {
		return "", err
	}

	currentYearMonth := fmt.Sprintf("%s.%s", parts.Year, parts.Month)

	micro := 1

	if currentYearMonth == yearMonth {
		micro = parts.Micro + 1
	}

	return fmt.Sprintf("%s.%d", yearMonth, micro), nil
}

func (c *CalVer) Tag(version string) string {
	return c.Prefix + version
}

// InitialVersion returns an empty string since calver starts from the current date.
func (c *CalVer) InitialVersion() string {
	return ""
}

func (c *CalVer) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}

	return time.Now()
}

type calverParts struct {
	Year  string
	Month string
	Micro int
}

func (c *CalVer) parseParts(version string) (calverParts, error) {
	segments := strings.SplitN(version, ".", 3) //nolint:mnd // calver has 3 parts: YYYY.MM.MICRO
	if len(segments) != 3 {                     //nolint:mnd // calver has 3 parts
		return calverParts{}, fmt.Errorf("%w: expected YYYY.MM.MICRO format, got %q", ErrInvalidVersion, version)
	}

	year, err := strconv.Atoi(segments[0])
	if err != nil || year < 1 {
		return calverParts{}, fmt.Errorf("%w: invalid year %q in %q", ErrInvalidVersion, segments[0], version)
	}

	month, err := strconv.Atoi(segments[1])
	if err != nil || month < 1 || month > 12 {
		return calverParts{}, fmt.Errorf("%w: invalid month %q in %q", ErrInvalidVersion, segments[1], version)
	}

	micro, err := strconv.Atoi(segments[2])
	if err != nil || micro < 0 {
		return calverParts{}, fmt.Errorf("%w: invalid micro version %q in %q", ErrInvalidVersion, segments[2], version)
	}

	return calverParts{Year: segments[0], Month: fmt.Sprintf("%02d", month), Micro: micro}, nil
}
