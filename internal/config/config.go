// Package config handles parsing and validation of .yeet.yaml configuration files.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v4"
)

const DefaultFile = ".yeet.yaml"

const DefaultSchemaURL = "https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json"

const SchemaDirective = "# yaml-language-server: $schema=" + DefaultSchemaURL

const githubProjectSegments = 2

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

type AutoMergeMethod = string

const (
	AutoMergeMethodAuto   AutoMergeMethod = "auto"
	AutoMergeMethodSquash AutoMergeMethod = "squash"
	AutoMergeMethodRebase AutoMergeMethod = "rebase"
	AutoMergeMethodMerge  AutoMergeMethod = "merge"
)

type Config struct {
	Versioning   VersioningStrategy `yaml:"versioning"`
	Branch       string             `yaml:"branch"`
	Provider     ProviderType       `yaml:"provider"`
	TagPrefix    string             `yaml:"tag_prefix"`
	Repository   RepositoryConfig   `yaml:"repository"`
	VersionFiles []string           `yaml:"version_files,omitempty"`
	Release      ReleaseConfig      `yaml:"release"`
	Changelog    ChangelogConfig    `yaml:"changelog"`
	CalVer       CalVerConfig       `yaml:"calver"`
}

type RepositoryConfig struct {
	Remote  string `yaml:"remote"`
	Host    string `yaml:"host"`
	Owner   string `yaml:"owner"`
	Repo    string `yaml:"repo"`
	Project string `yaml:"project"`
}

type ReleaseConfig struct {
	SubjectIncludeBranch bool            `yaml:"subject_include_branch"`
	AutoMerge            bool            `yaml:"auto_merge"`
	AutoMergeForce       bool            `yaml:"auto_merge_force"`
	AutoMergeMethod      AutoMergeMethod `yaml:"auto_merge_method"`
	PRBodyHeader         string          `yaml:"pr_body_header"`
	PRBodyFooter         string          `yaml:"pr_body_footer"`
}

type ChangelogConfig struct {
	File     string            `yaml:"file"`
	Include  []string          `yaml:"include"`
	Sections map[string]string `yaml:"sections"`
}

type CalVerConfig struct {
	Format string `yaml:"format"`
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

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	err := decoder.Decode(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: parse config: %w", ErrInvalidConfig, err)
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
		Repository: RepositoryConfig{
			Remote: "origin",
		},
		Release: ReleaseConfig{
			SubjectIncludeBranch: false,
			AutoMerge:            false,
			AutoMergeForce:       false,
			AutoMergeMethod:      AutoMergeMethodAuto,
			PRBodyHeader:         "## ٩(^ᴗ^)۶ release created",
			PRBodyFooter:         "_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
		},
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

	err := validateRepositoryConfig(c.Provider, c.Repository)
	if err != nil {
		return err
	}

	if c.Changelog.File == "" {
		return fmt.Errorf("%w: changelog.file must not be empty", ErrInvalidConfig)
	}

	if len(c.Changelog.Include) == 0 {
		return fmt.Errorf("%w: changelog.include must not be empty", ErrInvalidConfig)
	}

	for _, path := range c.VersionFiles {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%w: version_files must not contain empty paths", ErrInvalidConfig)
		}
	}

	err = validateReleaseConfig(c.Release)
	if err != nil {
		return err
	}

	return nil
}

func validateRepositoryConfig(provider ProviderType, repository RepositoryConfig) error {
	remote := strings.TrimSpace(repository.Remote)
	host := strings.TrimSpace(repository.Host)
	owner := strings.TrimSpace(repository.Owner)
	repo := strings.TrimSpace(repository.Repo)
	project := normalizeRepositoryProjectPath(repository.Project)

	if remote == "" {
		return fmt.Errorf("%w: repository.remote must not be empty", ErrInvalidConfig)
	}

	if repository.Host != "" && host == "" {
		return fmt.Errorf("%w: repository.host must not be blank", ErrInvalidConfig)
	}

	if repository.Owner != "" && owner == "" {
		return fmt.Errorf("%w: repository.owner must not be blank", ErrInvalidConfig)
	}

	if repository.Repo != "" && repo == "" {
		return fmt.Errorf("%w: repository.repo must not be blank", ErrInvalidConfig)
	}

	if repository.Project != "" && project == "" {
		return fmt.Errorf("%w: repository.project must not be blank", ErrInvalidConfig)
	}

	hasOwnerRepoMismatch := (owner == "") != (repo == "")
	if hasOwnerRepoMismatch {
		return fmt.Errorf("%w: repository.owner and repository.repo must be set together", ErrInvalidConfig)
	}

	if project != "" && owner != "" && repo != "" && project != owner+"/"+repo {
		return fmt.Errorf("%w: repository.project must match repository.owner/repo", ErrInvalidConfig)
	}

	if provider == ProviderGitHub {
		if strings.Contains(owner, "/") {
			return fmt.Errorf("%w: repository.owner must not contain '/' for github", ErrInvalidConfig)
		}

		if project != "" {
			projectOwner, _, ok := splitGitHubProjectPath(project)
			if !ok || strings.Contains(projectOwner, "/") {
				return fmt.Errorf("%w: repository.project must be in owner/repo form for github", ErrInvalidConfig)
			}
		}
	}

	return nil
}

func normalizeRepositoryProjectPath(project string) string {
	return strings.Trim(strings.TrimSpace(project), "/")
}

func splitGitHubProjectPath(project string) (string, string, bool) {
	parts := strings.Split(project, "/")
	if len(parts) != githubProjectSegments {
		return "", "", false
	}

	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSpace(parts[1])

	if owner == "" || repo == "" {
		return "", "", false
	}

	return owner, repo, true
}

func validateReleaseConfig(release ReleaseConfig) error {
	if release.AutoMergeMethod != AutoMergeMethodAuto &&
		release.AutoMergeMethod != AutoMergeMethodSquash &&
		release.AutoMergeMethod != AutoMergeMethodRebase &&
		release.AutoMergeMethod != AutoMergeMethodMerge {
		return fmt.Errorf(
			"%w: release.auto_merge_method must be %q, %q, %q, or %q, got %q",
			ErrInvalidConfig,
			AutoMergeMethodAuto,
			AutoMergeMethodSquash,
			AutoMergeMethodRebase,
			AutoMergeMethodMerge,
			release.AutoMergeMethod,
		)
	}

	return nil
}
