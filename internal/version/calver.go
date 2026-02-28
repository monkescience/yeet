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

	_, err := c.parseParts(cleaned)
	if err != nil {
		return "", err
	}

	return cleaned, nil
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

	currentYearMonth := fmt.Sprintf("%s.%s", parts[0], parts[1])

	micro := 1

	if currentYearMonth == yearMonth {
		micro = parts[2].(int) + 1 //nolint:forcetypeassert // parseParts guarantees int
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

type calverParts [3]any

func (c *CalVer) parseParts(version string) (calverParts, error) {
	segments := strings.SplitN(version, ".", 3) //nolint:mnd // calver has 3 parts: YYYY.MM.MICRO
	if len(segments) != 3 {                     //nolint:mnd // calver has 3 parts
		return calverParts{}, fmt.Errorf("%w: expected YYYY.MM.MICRO format, got %q", ErrInvalidVersion, version)
	}

	micro, err := strconv.Atoi(segments[2])
	if err != nil {
		return calverParts{}, fmt.Errorf("%w: invalid micro version %q: %w", ErrInvalidVersion, segments[2], err)
	}

	return calverParts{segments[0], segments[1], micro}, nil
}
