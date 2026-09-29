package versionfile

import (
	"errors"
	"fmt"
	"slices"
)

type Format string

const (
	FormatMarkers Format = "markers"
	FormatJSON    Format = "json"
)

var ErrInvalidLocation = errors.New("invalid version file location")

type Location struct {
	format Format
	path   []string
}

func NewLocation(format Format, pointer string) (Location, error) {
	switch format {
	case "", FormatMarkers:
		if pointer != "" {
			return Location{}, fmt.Errorf("%w: json pointer requires json format", ErrInvalidLocation)
		}

		return Location{format: FormatMarkers}, nil
	case FormatJSON:
		path, err := parseJSONPointer(pointer)
		if err != nil {
			return Location{}, err
		}

		return Location{format: FormatJSON, path: path}, nil
	default:
		return Location{}, fmt.Errorf("%w: unknown format %q", ErrInvalidLocation, format)
	}
}

func (l Location) Format() Format {
	return l.format
}

func (l Location) Apply(content, nextVersion string, scheme Scheme) (string, bool, error) {
	if l.format == FormatJSON {
		updated, changed, err := applyJSONPointer(content, nextVersion, l.path)
		if err != nil {
			return content, false, fmt.Errorf("apply json pointer: %w", err)
		}

		return updated, changed, nil
	}

	if l.format != FormatMarkers {
		return content, false, ErrInvalidLocation
	}

	updated, changed, err := applyGenericMarkers(content, nextVersion, scheme)
	if err != nil {
		return content, false, fmt.Errorf("apply markers: %w", err)
	}

	return updated, changed, nil
}

func (l Location) Overlaps(other Location) bool {
	if l.format == FormatJSON && other.format == FormatJSON {
		return slices.Equal(l.path, other.path)
	}

	return true
}
