package config_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	// given: nothing

	// when: creating a default config
	cfg := config.Default()

	// then: sensible defaults are set
	testastic.Equal(t, config.VersioningSemver, cfg.Versioning)
	testastic.Equal(t, "main", cfg.Branch)
	testastic.Equal(t, "v", cfg.TagPrefix)
	testastic.False(t, cfg.Release.SubjectIncludeBranch)
	testastic.Equal(t, 0, len(cfg.VersionFiles))
	testastic.Equal(t, "CHANGELOG.md", cfg.Changelog.File)
	testastic.Equal(t, 4, len(cfg.Changelog.Include))
	testastic.Equal(t, "YYYY.0M.MICRO", cfg.CalVer.Format)
}

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("valid minimal config", func(t *testing.T) {
		t.Parallel()

		// given: a minimal TOML config
		data := []byte(`
versioning = "semver"
branch = "main"
`)

		// when: parsing the config
		cfg, err := config.Parse(data)

		// then: it succeeds with defaults filled in
		testastic.NoError(t, err)
		testastic.Equal(t, config.VersioningSemver, cfg.Versioning)
		testastic.Equal(t, "main", cfg.Branch)
		testastic.Equal(t, "v", cfg.TagPrefix)
	})

	t.Run("valid full config", func(t *testing.T) {
		t.Parallel()

		// given: a full TOML config
		data := []byte(`
versioning = "calver"
branch = "develop"
provider = "gitlab"
tag_prefix = "release-"
version_files = ["VERSION", "cmd/yeet/version.txt"]

[changelog]
file = "CHANGES.md"
include = ["feat", "fix"]

[changelog.sections]
feat = "New Features"
fix = "Bug Fixes"

[calver]
format = "YYYY.0M.MICRO"

[release]
subject_include_branch = true
`)

		// when: parsing the config
		cfg, err := config.Parse(data)

		// then: all values are set correctly
		testastic.NoError(t, err)
		testastic.Equal(t, config.VersioningCalVer, cfg.Versioning)
		testastic.Equal(t, "develop", cfg.Branch)
		testastic.Equal(t, config.ProviderGitLab, cfg.Provider)
		testastic.Equal(t, "release-", cfg.TagPrefix)
		testastic.True(t, cfg.Release.SubjectIncludeBranch)
		testastic.Equal(t, 2, len(cfg.VersionFiles))
		testastic.Equal(t, "VERSION", cfg.VersionFiles[0])
		testastic.Equal(t, "CHANGES.md", cfg.Changelog.File)
		testastic.Equal(t, 2, len(cfg.Changelog.Include))
		testastic.Equal(t, "New Features", cfg.Changelog.Sections["feat"])
	})

	t.Run("invalid versioning", func(t *testing.T) {
		t.Parallel()

		// given: config with invalid versioning strategy
		data := []byte(`
versioning = "invalid"
branch = "main"
`)

		// when: parsing the config
		_, err := config.Parse(data)

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("invalid provider", func(t *testing.T) {
		t.Parallel()

		// given: config with invalid provider
		data := []byte(`
versioning = "semver"
branch = "main"
provider = "bitbucket"
`)

		// when: parsing the config
		_, err := config.Parse(data)

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("empty branch", func(t *testing.T) {
		t.Parallel()

		// given: config with empty branch
		data := []byte(`
versioning = "semver"
branch = ""
`)

		// when: parsing the config
		_, err := config.Parse(data)

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("invalid toml", func(t *testing.T) {
		t.Parallel()

		// given: invalid TOML syntax
		data := []byte(`this is not valid toml [[[`)

		// when: parsing the config
		_, err := config.Parse(data)

		// then: parsing fails
		testastic.Error(t, err)
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid config passes", func(t *testing.T) {
		t.Parallel()

		// given: a valid default config
		cfg := config.Default()

		// when: validating
		err := cfg.Validate()

		// then: no error
		testastic.NoError(t, err)
	})

	t.Run("empty changelog include fails", func(t *testing.T) {
		t.Parallel()

		// given: config with empty changelog include
		cfg := config.Default()
		cfg.Changelog.Include = nil

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("empty changelog file fails", func(t *testing.T) {
		t.Parallel()

		// given: config with empty changelog file
		cfg := config.Default()
		cfg.Changelog.File = ""

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("empty version file path fails", func(t *testing.T) {
		t.Parallel()

		// given: config with an empty version file path
		cfg := config.Default()
		cfg.VersionFiles = []string{"  "}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})
}
