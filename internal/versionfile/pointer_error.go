package versionfile

import "fmt"

type JSONPointerReason uint8

const (
	JSONPointerMissingSlash JSONPointerReason = iota + 1
	JSONPointerInvalidEscape
)

type JSONPointerError struct {
	Reason JSONPointerReason
}

func (e *JSONPointerError) Error() string {
	if e.Reason == JSONPointerMissingSlash {
		return fmt.Sprintf("%s: must start with /", ErrInvalidJSONPointer)
	}

	return fmt.Sprintf("%s: invalid escape", ErrInvalidJSONPointer)
}

func (e *JSONPointerError) Unwrap() error {
	return ErrInvalidJSONPointer
}
