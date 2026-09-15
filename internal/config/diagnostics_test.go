package config_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestInvalidf(t *testing.T) {
	t.Parallel()

	// given: a controlled validation problem
	err := config.Invalidf("targets.%s.tag_prefix must not be empty", "api")

	// when: callers inspect the typed error
	var validationErr *config.ValidationError

	matched := errors.As(err, &validationErr)

	// then: the problem and invalid-config identity are retained
	testastic.True(t, matched)
	testastic.Equal(t, "targets.api.tag_prefix must not be empty", validationErr.Problem)
	testastic.Equal(t, "invalid config: targets.api.tag_prefix must not be empty", err.Error())
	testastic.ErrorIs(t, err, config.ErrInvalidConfig)
}

func TestInvalidWithCausef(t *testing.T) {
	t.Parallel()

	// given: a parser cause and controlled problem
	cause := errors.New("parser failure")
	err := config.InvalidWithCausef(cause, "parse config")

	// when: callers inspect the error chain
	var validationErr *config.ValidationError

	matched := errors.As(err, &validationErr)

	// then: both the structured problem and original cause are available
	testastic.True(t, matched)
	testastic.Equal(t, "parse config", validationErr.Problem)
	testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	testastic.ErrorIs(t, err, cause)
}

func TestInitializeExistingFileReturnsFileError(t *testing.T) {
	t.Parallel()

	// given: an existing configuration file
	path := filepath.Join(t.TempDir(), config.DefaultFile)
	err := os.WriteFile(path, []byte("targets: {}\n"), 0o600)
	testastic.NoError(t, err)

	// when: initialization targets the existing file
	err = config.Initialize(context.Background(), path)

	var fileErr *config.FileError

	matched := errors.As(err, &fileErr)

	// then: callers can read the path while preserving the exists identity
	testastic.True(t, matched)
	testastic.Equal(t, path, fileErr.Path)
	testastic.ErrorIs(t, err, config.ErrExists)
	testastic.Equal(t, "config file already exists: "+path, err.Error())
}
