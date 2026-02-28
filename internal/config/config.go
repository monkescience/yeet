// Package config handles parsing and validation of .yeet.toml configuration files.
package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const DefaultFile = ".yeet.toml"

type VersioningStrategy = string

const (
	// VersioningSemver uses semantic versioning (MAJOR.MINOR.PATCH).
	VersioningSemver VersioningStrategy = "semver"
	// VersioningCalVer uses calendar versioning (e.g., YYYY.0M.MICRO).
	VersioningCalVer VersioningStrategy = "calver"
)

type ProviderType = string

const (
	ProviderGitHub ProviderType = "github"
	ProviderGitLab ProviderType = "gitlab"
)

type Config struct {
	Versioning VersioningStrategy `toml:"versioning"`
	Branch     string             `toml:"branch"`
	Provider   ProviderType       `toml:"provider"`
	TagPrefix  string             `toml:"tag_prefix"`
	Changelog  ChangelogConfig    `toml:"changelog"`
	CalVer     CalVerConfig       `toml:"calver"`
}

type ChangelogConfig struct {
	File     string            `toml:"file"`
	Include  []string          `toml:"include"`
	Sections map[string]string `toml:"sections"`
}

type CalVerConfig struct {
	Format string `toml:"format"`
}

var ErrInvalidConfig = errors.New("invalid config")

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from user config, not user input
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	cfg := Default()

	err := toml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	err = cfg.Validate()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func Default() *Config {
	return &Config{
		Versioning: VersioningSemver,
		Branch:     "main",
		TagPrefix:  "v",
		Changelog: ChangelogConfig{
			File:    "CHANGELOG.md",
			Include: []string{"feat", "fix", "perf", "revert"},
			Sections: map[string]string{
				"feat":     "Features",
				"fix":      "Bug Fixes",
				"perf":     "Performance Improvements",
				"revert":   "Reverts",
				"docs":     "Documentation",
				"style":    "Styles",
				"refactor": "Code Refactoring",
				"test":     "Tests",
				"build":    "Build System",
				"ci":       "Continuous Integration",
				"chore":    "Miscellaneous Chores",
				"breaking": "Breaking Changes",
			},
		},
		CalVer: CalVerConfig{
			Format: "YYYY.0M.MICRO",
		},
	}
}

func (c *Config) Validate() error {
	if c.Versioning != VersioningSemver && c.Versioning != VersioningCalVer {
		return fmt.Errorf("%w: versioning must be %q or %q, got %q",
			ErrInvalidConfig, VersioningSemver, VersioningCalVer, c.Versioning)
	}

	if c.Branch == "" {
		return fmt.Errorf("%w: branch must not be empty", ErrInvalidConfig)
	}

	if c.Provider != "" && c.Provider != ProviderGitHub && c.Provider != ProviderGitLab {
		return fmt.Errorf("%w: provider must be %q or %q, got %q",
			ErrInvalidConfig, ProviderGitHub, ProviderGitLab, c.Provider)
	}

	if c.Changelog.File == "" {
		return fmt.Errorf("%w: changelog.file must not be empty", ErrInvalidConfig)
	}

	if len(c.Changelog.Include) == 0 {
		return fmt.Errorf("%w: changelog.include must not be empty", ErrInvalidConfig)
	}

	return nil
}
