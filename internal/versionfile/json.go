package versionfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const jsonStringQuoteCount = 2

var (
	ErrInvalidJSON          = errors.New("invalid json")
	ErrInvalidJSONPointer   = errors.New("invalid json pointer")
	ErrJSONPointerNotFound  = errors.New("json pointer not found")
	ErrJSONPointerNonString = errors.New("json pointer does not resolve to a string")
)

// ApplyJSONPointer updates the string value addressed by an RFC 6901 JSON Pointer.
func ApplyJSONPointer(content, nextVersion, pointer string) (string, bool, error) {
	path, err := parseJSONPointer(pointer)
	if err != nil {
		return content, false, err
	}

	data := []byte(content)
	if !json.Valid(data) {
		return content, false, ErrInvalidJSON
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	span, err := locateJSONPointerString(decoder, data, path)
	if err != nil {
		return content, false, err
	}

	replacement, err := json.Marshal(nextVersion)
	if err != nil {
		return content, false, fmt.Errorf("marshal json version: %w", err)
	}

	result := string(data[:span.start]) + string(replacement) + string(data[span.end:])

	return result, result != content, nil
}

type byteSpan struct {
	start int
	end   int
}

func parseJSONPointer(pointer string) ([]string, error) {
	if pointer == "" || pointer[0] != '/' {
		return nil, fmt.Errorf("%w: must start with /", ErrInvalidJSONPointer)
	}

	parts := strings.Split(pointer[1:], "/")
	for i, part := range parts {
		unescaped, err := unescapeJSONPointerPart(part)
		if err != nil {
			return nil, err
		}

		parts[i] = unescaped
	}

	return parts, nil
}

func unescapeJSONPointerPart(part string) (string, error) {
	var builder strings.Builder

	for i := 0; i < len(part); i++ {
		if part[i] != '~' {
			builder.WriteByte(part[i])

			continue
		}

		if i+1 >= len(part) {
			return "", fmt.Errorf("%w: invalid escape", ErrInvalidJSONPointer)
		}

		switch part[i+1] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", fmt.Errorf("%w: invalid escape", ErrInvalidJSONPointer)
		}

		i++
	}

	return builder.String(), nil
}

func locateJSONPointerString(decoder *json.Decoder, data []byte, path []string) (byteSpan, error) {
	token, err := decoder.Token()
	if err != nil {
		return byteSpan{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	if len(path) == 0 {
		_, ok := token.(string)
		if !ok {
			return byteSpan{}, ErrJSONPointerNonString
		}

		end := int(decoder.InputOffset())

		start, err := stringTokenStart(data, end)
		if err != nil {
			return byteSpan{}, err
		}

		return byteSpan{start: start, end: end}, nil
	}

	delim, ok := token.(json.Delim)
	if !ok {
		return byteSpan{}, ErrJSONPointerNotFound
	}

	switch delim {
	case '{':
		return locateObjectJSONPointerString(decoder, data, path)
	case '[':
		return locateArrayJSONPointerString(decoder, data, path)
	default:
		return byteSpan{}, ErrJSONPointerNotFound
	}
}

func locateObjectJSONPointerString(decoder *json.Decoder, data []byte, path []string) (byteSpan, error) {
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return byteSpan{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
		}

		key, ok := keyToken.(string)
		if !ok {
			return byteSpan{}, ErrInvalidJSON
		}

		if key == path[0] {
			return locateJSONPointerString(decoder, data, path[1:])
		}

		if err := skipJSONValue(decoder); err != nil {
			return byteSpan{}, err
		}
	}

	_, err := decoder.Token()
	if err != nil {
		return byteSpan{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	return byteSpan{}, ErrJSONPointerNotFound
}

func locateArrayJSONPointerString(decoder *json.Decoder, data []byte, path []string) (byteSpan, error) {
	index, err := strconv.Atoi(path[0])
	if err != nil || index < 0 {
		return byteSpan{}, ErrJSONPointerNotFound
	}

	for current := 0; decoder.More(); current++ {
		if current == index {
			return locateJSONPointerString(decoder, data, path[1:])
		}

		if err := skipJSONValue(decoder); err != nil {
			return byteSpan{}, err
		}
	}

	_, err = decoder.Token()
	if err != nil {
		return byteSpan{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	return byteSpan{}, ErrJSONPointerNotFound
}

func skipJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		for decoder.More() {
			_, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
			}

			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return nil
	}

	_, err = decoder.Token()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	return nil
}

func stringTokenStart(data []byte, end int) (int, error) {
	if end < jsonStringQuoteCount || end > len(data) || data[end-1] != '"' {
		return 0, ErrInvalidJSON
	}

	// Decoder.InputOffset points after the closing quote. An even backslash run identifies an unescaped opener.
	for i := end - jsonStringQuoteCount; i >= 0; i-- {
		if data[i] != '"' || hasOddBackslashesBefore(data, i) {
			continue
		}

		return i, nil
	}

	return 0, ErrInvalidJSON
}

func hasOddBackslashesBefore(data []byte, index int) bool {
	count := 0
	for i := index - 1; i >= 0 && data[i] == '\\'; i-- {
		count++
	}

	return count%2 == 1
}
