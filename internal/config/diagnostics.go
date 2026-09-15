package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/monkescience/yeet/internal/version"
)

type ValidationError struct {
	Problem        string
	Line           int
	Column         int
	cause          error
	deferToDecoder bool
}

func (e *ValidationError) Error() string {
	return ErrInvalidConfig.Error() + ": " + e.Problem
}

func (e *ValidationError) Unwrap() error {
	return e.cause
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidConfig
}

func Invalidf(format string, args ...any) error {
	return InvalidWithCausef(ErrInvalidConfig, format, args...)
}

func InvalidWithCausef(cause error, format string, args ...any) error {
	err := &ValidationError{Problem: fmt.Sprintf(format, args...), cause: cause}
	if load, ok := singleLoadError(cause); ok {
		err.Line = load.Mark.Line
		err.Column = load.Mark.Column
	}

	return err
}

func singleLoadError(cause error) (*yaml.LoadError, bool) {
	if loads, ok := errors.AsType[*yaml.LoadErrors](cause); ok {
		if len(loads.Errors) != 1 {
			return nil, false
		}

		return loads.Errors[0], true
	}

	return errors.AsType[*yaml.LoadError](cause)
}

func describeDecodeFailure(cause error) string {
	var loads []*yaml.LoadError

	if multiple, ok := errors.AsType[*yaml.LoadErrors](cause); ok {
		loads = multiple.Errors
	} else if single, ok := errors.AsType[*yaml.LoadError](cause); ok {
		loads = []*yaml.LoadError{single}
	}

	problems := make([]string, 0, len(loads))

	for _, load := range loads {
		message := normalizeDecodeMessage(load.Message)
		if message == "" {
			continue
		}

		if load.Mark.Line > 0 && len(loads) > 1 {
			message = fmt.Sprintf("line %d: %s", load.Mark.Line, message)
		}

		problems = append(problems, message)
	}

	if len(problems) == 0 {
		return "configuration could not be decoded"
	}

	return "configuration could not be decoded: " + strings.Join(problems, "; ")
}

func validationReason(err error) string {
	reason := err.Error()
	for _, prefix := range []string{version.ErrInvalidVersion.Error() + ": ", "error parsing regexp: "} {
		reason = strings.TrimPrefix(reason, prefix)
	}

	return strings.TrimSpace(reason)
}

func normalizeDecodeMessage(message string) string {
	message = strings.TrimSpace(message)

	field, _, found := strings.Cut(message, " not found in type ")
	if !found {
		return message
	}

	return "unknown field " + strconv.Quote(strings.TrimPrefix(field, "field "))
}

func deferredValidationError(problem string) error {
	return &ValidationError{Problem: problem, cause: ErrInvalidConfig, deferToDecoder: true}
}

type FileError struct {
	Path  string
	cause error
}

func (e *FileError) Error() string {
	return fmt.Sprintf("%s: %s", e.cause, e.Path)
}

func (e *FileError) Unwrap() error {
	return e.cause
}

func newFileError(path string, cause error) error {
	return &FileError{Path: path, cause: cause}
}
