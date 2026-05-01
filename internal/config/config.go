// Package config handles parsing and validation of .yeet.yaml configuration files.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/version"
	"go.yaml.in/yaml/v4"
)

const DefaultFile = ".yeet.yaml"

const DefaultSchemaURL = "https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json"

const SchemaDirective = "# yaml-language-server: $schema=" + DefaultSchemaURL

const githubProjectSegments = 2

type VersioningStrategy string

const (
	// VersioningSemver uses semantic versioning (MAJOR.MINOR.PATCH).
	VersioningSemver VersioningStrategy = "semver"
	// VersioningCalVer uses calendar versioning (e.g., YYYY.0M.MICRO).
	VersioningCalVer VersioningStrategy = "calver"
)

type ProviderType string

const (
	ProviderAuto   ProviderType = "auto"
	ProviderGitHub ProviderType = "github"
	ProviderGitLab ProviderType = "gitlab"
)

type AutoMergeMethod string

const (
	AutoMergeMethodAuto   AutoMergeMethod = "auto"
	AutoMergeMethodSquash AutoMergeMethod = "squash"
	AutoMergeMethodRebase AutoMergeMethod = "rebase"
	AutoMergeMethodMerge  AutoMergeMethod = "merge"
)

type Config struct {
	Versioning                 VersioningStrategy `yaml:"versioning"`
	Branch                     string             `yaml:"branch"`
	Provider                   ProviderType       `yaml:"provider"`
	PreMajorBreakingBumpsMinor bool               `yaml:"pre_major_breaking_bumps_minor"`
	PreMajorFeaturesBumpPatch  bool               `yaml:"pre_major_features_bump_patch"`
	BumpTypes                  BumpTypesConfig    `yaml:"bump_types"`
	Repository                 RepositoryConfig   `yaml:"repository"`
	VersionFiles               []string           `yaml:"version_files,omitempty"`
	Release                    ReleaseConfig      `yaml:"release"`
	Changelog                  ChangelogConfig    `yaml:"changelog"`
	CalVer                     CalVerConfig       `yaml:"calver"`
	Targets                    map[string]Target  `yaml:"targets"`
	ActiveChannel              string             `yaml:"-"`
}

type TargetType string

const (
	TargetTypePath    TargetType = "path"
	TargetTypeDerived TargetType = "derived"
)

type Target struct {
	Type                       TargetType         `yaml:"type"`
	Path                       string             `yaml:"path,omitempty"`
	TagPrefix                  string             `yaml:"tag_prefix,omitempty"`
	Versioning                 VersioningStrategy `yaml:"versioning,omitempty"`
	PreMajorBreakingBumpsMinor *bool              `yaml:"pre_major_breaking_bumps_minor,omitempty"`
	PreMajorFeaturesBumpPatch  *bool              `yaml:"pre_major_features_bump_patch,omitempty"`
	VersionFiles               []string           `yaml:"version_files,omitempty"`
	Changelog                  ChangelogConfig    `yaml:"changelog,omitempty"`
	CalVer                     CalVerConfig       `yaml:"calver,omitempty"`
	ExcludePaths               []string           `yaml:"exclude_paths,omitempty"`
	Includes                   []string           `yaml:"includes,omitempty"`
}

type ResolvedTarget struct {
	ID                         string
	Type                       TargetType
	Path                       string
	TagPrefix                  string
	Versioning                 VersioningStrategy
	PreMajorBreakingBumpsMinor bool
	PreMajorFeaturesBumpPatch  bool
	VersionFiles               []string
	Changelog                  ChangelogConfig
	CalVer                     CalVerConfig
	ExcludePaths               []string
	Includes                   []string
}

type RepositoryConfig struct {
	Remote  string `yaml:"remote"`
	Host    string `yaml:"host"`
	Owner   string `yaml:"owner"`
	Repo    string `yaml:"repo"`
	Project string `yaml:"project"`
}

type ReleaseConfig struct {
	SubjectIncludeBranch bool                            `yaml:"subject_include_branch"`
	AutoMerge            bool                            `yaml:"auto_merge"`
	AutoMergeForce       bool                            `yaml:"auto_merge_force"`
	AutoMergeMethod      AutoMergeMethod                 `yaml:"auto_merge_method"`
	PRBodyHeader         string                          `yaml:"pr_body_header"`
	PRBodyFooter         string                          `yaml:"pr_body_footer"`
	Channels             map[string]ReleaseChannelConfig `yaml:"channels,omitempty"`
}

type ReleaseChannelConfig struct {
	Branch        string `yaml:"branch"`
	Prerelease    string `yaml:"prerelease"`
	ChangelogFile string `yaml:"changelog_file,omitempty"`
}

type ChangelogConfig struct {
	File       string            `yaml:"file"`
	Include    []string          `yaml:"include"`
	Sections   map[string]string `yaml:"sections"`
	References ReferencesConfig  `yaml:"references"`
}

// ReferencesConfig controls how issue/ticket references are linked in changelogs.
type ReferencesConfig struct {
	Patterns []ReferencePattern `yaml:"patterns,omitempty"`
	Footers  map[string]string  `yaml:"footers,omitempty"`
}

// ReferencePattern matches issue references inline in commit descriptions.
type ReferencePattern struct {
	Pattern string `yaml:"pattern"`
	URL     string `yaml:"url"`
}

type BumpTypesConfig struct {
	Minor []string `yaml:"minor"`
	Patch []string `yaml:"patch"`
}

// ToBumpMapping converts the config into a commit.BumpMapping.
func (b BumpTypesConfig) ToBumpMapping() commit.BumpMapping {
	m := make(commit.BumpMapping, len(b.Minor)+len(b.Patch))

	for _, t := range b.Minor {
		m[t] = commit.BumpMinor
	}

	for _, t := range b.Patch {
		m[t] = commit.BumpPatch
	}

	return m
}

type CalVerConfig struct {
	Format string `yaml:"format"`
}

var ErrInvalidConfig = errors.New("invalid config")

var ErrEmptyRepoPath = errors.New("must not be empty")

var ErrPathMustBeRepoRelative = errors.New("must be repo-relative")

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

func defaultBumpTypes() BumpTypesConfig {
	return BumpTypesConfig{
		Minor: []string{"feat"},
		Patch: []string{"fix", "perf"},
	}
}

func defaultChangelogInclude() []string {
	return []string{"feat", "fix", "perf", "revert"}
}

func defaultChangelogSections() map[string]string {
	return map[string]string{
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
	}
}

func Default() *Config {
	return &Config{
		Versioning:                 VersioningSemver,
		Branch:                     "main",
		Provider:                   ProviderAuto,
		PreMajorBreakingBumpsMinor: true,
		PreMajorFeaturesBumpPatch:  true,
		BumpTypes:                  defaultBumpTypes(),
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
			File:     "CHANGELOG.md",
			Include:  defaultChangelogInclude(),
			Sections: defaultChangelogSections(),
		},
		CalVer: CalVerConfig{
			Format: version.DefaultCalVerFormat,
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

	if c.Provider != ProviderAuto && c.Provider != ProviderGitHub && c.Provider != ProviderGitLab {
		return fmt.Errorf("%w: provider must be %q, %q, or %q, got %q",
			ErrInvalidConfig, ProviderAuto, ProviderGitHub, ProviderGitLab, c.Provider)
	}

	err := validateBumpTypes(c.BumpTypes)
	if err != nil {
		return err
	}

	err = validateRepositoryConfig(c.Provider, c.Repository)
	if err != nil {
		return err
	}

	if c.Changelog.File == "" {
		return fmt.Errorf("%w: changelog.file must not be empty", ErrInvalidConfig)
	}

	if len(c.Changelog.Include) == 0 {
		return fmt.Errorf("%w: changelog.include must not be empty", ErrInvalidConfig)
	}

	err = validateCalVerConfig("calver.format", c.CalVer)
	if err != nil {
		return err
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

	err = validateReleaseChannelBranches(c.Branch, c.Release.Channels)
	if err != nil {
		return err
	}

	_, err = c.ResolvedTargets()
	if err != nil {
		return err
	}

	return nil
}

func (c *Config) ResolvedTargets() (map[string]ResolvedTarget, error) {
	if len(c.Targets) == 0 {
		return nil, fmt.Errorf("%w: targets must not be empty", ErrInvalidConfig)
	}

	resolved := make(map[string]ResolvedTarget, len(c.Targets))

	for id, target := range c.Targets {
		resolvedTarget, err := c.resolveTarget(id, target)
		if err != nil {
			return nil, err
		}

		if _, exists := resolved[resolvedTarget.ID]; exists {
			return nil, fmt.Errorf("%w: target IDs must be unique and non-empty", ErrInvalidConfig)
		}

		resolved[resolvedTarget.ID] = resolvedTarget
	}

	err := validateResolvedTargets(resolved)
	if err != nil {
		return nil, err
	}

	return resolved, nil
}

//nolint:funlen,gocognit // Target resolution intentionally centralizes validation and defaulting.
func (c *Config) resolveTarget(id string, target Target) (ResolvedTarget, error) {
	targetID := strings.TrimSpace(id)
	if targetID == "" {
		return ResolvedTarget{}, fmt.Errorf("%w: target IDs must be unique and non-empty", ErrInvalidConfig)
	}

	targetType := TargetType(strings.TrimSpace(string(target.Type)))
	if targetType != TargetTypePath && targetType != TargetTypeDerived {
		return ResolvedTarget{}, fmt.Errorf(
			"%w: targets.%s.type must be %q or %q, got %q",
			ErrInvalidConfig,
			targetID,
			TargetTypePath,
			TargetTypeDerived,
			target.Type,
		)
	}

	resolved := ResolvedTarget{
		ID:                         targetID,
		Type:                       targetType,
		TagPrefix:                  strings.TrimSpace(target.TagPrefix),
		Versioning:                 firstVersioning(target.Versioning, c.Versioning),
		PreMajorBreakingBumpsMinor: resolveBool(target.PreMajorBreakingBumpsMinor, c.PreMajorBreakingBumpsMinor),
		PreMajorFeaturesBumpPatch:  resolveBool(target.PreMajorFeaturesBumpPatch, c.PreMajorFeaturesBumpPatch),
		VersionFiles:               resolveVersionFiles(target.VersionFiles, c.VersionFiles),
		Changelog:                  mergeChangelogConfig(c.Changelog, target.Changelog),
		CalVer:                     mergeCalVerConfig(c.CalVer, target.CalVer),
		ExcludePaths:               make([]string, 0, len(target.ExcludePaths)),
		Includes:                   normalizeTargetIDs(target.Includes),
	}

	preMajorErr := validatePreMajorCalVer(targetID, resolved.Versioning, target)
	if preMajorErr != nil {
		return ResolvedTarget{}, preMajorErr
	}

	if resolved.Versioning != VersioningSemver && resolved.Versioning != VersioningCalVer {
		return ResolvedTarget{}, fmt.Errorf(
			"%w: targets.%s.versioning must be %q or %q, got %q",
			ErrInvalidConfig,
			targetID,
			VersioningSemver,
			VersioningCalVer,
			resolved.Versioning,
		)
	}

	err := validateCalVerConfig("targets."+targetID+".calver.format", resolved.CalVer)
	if err != nil {
		return ResolvedTarget{}, err
	}

	if resolved.TagPrefix == "" {
		return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.tag_prefix must not be empty", ErrInvalidConfig, targetID)
	}

	if resolved.Changelog.File == "" {
		return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.changelog.file must not be empty", ErrInvalidConfig, targetID)
	}

	if len(resolved.Changelog.Include) == 0 {
		return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.changelog.include must not be empty", ErrInvalidConfig, targetID)
	}

	for _, path := range resolved.VersionFiles {
		if strings.TrimSpace(path) == "" {
			return ResolvedTarget{}, fmt.Errorf(
				"%w: targets.%s.version_files must not contain empty paths",
				ErrInvalidConfig,
				targetID,
			)
		}
	}

	if targetType == TargetTypePath || strings.TrimSpace(target.Path) != "" {
		normalizedPath, err := normalizeRepoPath(target.Path)
		if err != nil {
			return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.path %w", ErrInvalidConfig, targetID, err)
		}

		resolved.Path = normalizedPath
	}

	for _, excludePath := range target.ExcludePaths {
		normalizedExcludePath, err := normalizeRepoPath(excludePath)
		if err != nil {
			return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.exclude_paths contains %w", ErrInvalidConfig, targetID, err)
		}

		resolved.ExcludePaths = append(resolved.ExcludePaths, normalizedExcludePath)
	}

	if resolved.Path != "." {
		for _, excludePath := range resolved.ExcludePaths {
			if !RepoPathContains(resolved.Path, excludePath) {
				return ResolvedTarget{}, fmt.Errorf(
					"%w: targets.%s.exclude_paths entry %q must be inside %q",
					ErrInvalidConfig,
					targetID,
					excludePath,
					resolved.Path,
				)
			}
		}
	}

	if targetType == TargetTypePath {
		if resolved.Path == "" {
			return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.path must not be empty", ErrInvalidConfig, targetID)
		}

		if len(resolved.Includes) > 0 {
			return ResolvedTarget{}, fmt.Errorf(
				"%w: targets.%s.includes is only valid for derived targets",
				ErrInvalidConfig,
				targetID,
			)
		}
	}

	if targetType == TargetTypeDerived {
		if len(resolved.Includes) == 0 {
			return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.includes must not be empty", ErrInvalidConfig, targetID)
		}
	}

	return resolved, nil
}

func normalizeTargetIDs(ids []string) []string {
	normalizedIDs := make([]string, 0, len(ids))

	for _, id := range ids {
		normalizedIDs = append(normalizedIDs, strings.TrimSpace(id))
	}

	return normalizedIDs
}

//nolint:funlen // Cross-target validation is easier to review in one place.
func validateResolvedTargets(targets map[string]ResolvedTarget) error {
	if len(targets) == 0 {
		return fmt.Errorf("%w: targets must not be empty", ErrInvalidConfig)
	}

	tagPrefixes := make(map[string]string, len(targets))

	for id, target := range targets {
		if otherID, exists := tagPrefixes[target.TagPrefix]; exists {
			return fmt.Errorf(
				"%w: targets.%s.tag_prefix %q duplicates targets.%s.tag_prefix",
				ErrInvalidConfig,
				id,
				target.TagPrefix,
				otherID,
			)
		}

		tagPrefixes[target.TagPrefix] = id
	}

	for id, target := range targets {
		if target.Type != TargetTypeDerived {
			continue
		}

		for _, includeID := range target.Includes {
			normalizedIncludeID := strings.TrimSpace(includeID)

			includedTarget, exists := targets[normalizedIncludeID]
			if !exists {
				return fmt.Errorf(
					"%w: targets.%s.includes entry %q does not refer to a defined target",
					ErrInvalidConfig,
					id,
					normalizedIncludeID,
				)
			}

			if includedTarget.Type != TargetTypePath {
				return fmt.Errorf(
					"%w: targets.%s.includes entry %q must refer to a path target in v1",
					ErrInvalidConfig,
					id,
					normalizedIncludeID,
				)
			}
		}
	}

	directTargets := make([]ResolvedTarget, 0, len(targets))
	for _, target := range targets {
		if target.Path == "" {
			continue
		}

		directTargets = append(directTargets, target)
	}

	for leftIdx := range directTargets {
		leftTarget := directTargets[leftIdx]

		for rightIdx := leftIdx + 1; rightIdx < len(directTargets); rightIdx++ {
			rightTarget := directTargets[rightIdx]

			if !directTargetsOverlap(leftTarget, rightTarget) {
				continue
			}

			return fmt.Errorf(
				"%w: direct path ownership overlaps between targets.%s and targets.%s",
				ErrInvalidConfig,
				leftTarget.ID,
				rightTarget.ID,
			)
		}
	}

	err := validateResolvedTargetVersionFileOwnership(targets)
	if err != nil {
		return err
	}

	return nil
}

func validateResolvedTargetVersionFileOwnership(targets map[string]ResolvedTarget) error {
	targetIDs := make([]string, 0, len(targets))
	for id := range targets {
		targetIDs = append(targetIDs, id)
	}

	slices.Sort(targetIDs)

	versionFileOwners := make(map[string]string)

	for _, id := range targetIDs {
		target := targets[id]
		for _, versionFilePath := range target.VersionFiles {
			normalizedVersionFilePath := strings.TrimSpace(versionFilePath)

			otherID, exists := versionFileOwners[normalizedVersionFilePath]
			if exists && otherID != id {
				return fmt.Errorf(
					"%w: targets.%s.version_files entry %q duplicates targets.%s.version_files entry",
					ErrInvalidConfig,
					id,
					normalizedVersionFilePath,
					otherID,
				)
			}

			versionFileOwners[normalizedVersionFilePath] = id
		}
	}

	return nil
}

func directTargetsOverlap(leftTarget, rightTarget ResolvedTarget) bool {
	if leftTarget.Path == "" || rightTarget.Path == "" {
		return false
	}

	samplePath := overlappingSamplePath(leftTarget.Path, rightTarget.Path)
	if samplePath == "" {
		return false
	}

	return targetOwnsPath(leftTarget, samplePath) && targetOwnsPath(rightTarget, samplePath)
}

func overlappingSamplePath(leftPath, rightPath string) string {
	if RepoPathContains(leftPath, rightPath) {
		return rightPath
	}

	if RepoPathContains(rightPath, leftPath) {
		return leftPath
	}

	return ""
}

func targetOwnsPath(target ResolvedTarget, candidate string) bool {
	if !RepoPathContains(target.Path, candidate) {
		return false
	}

	for _, excludePath := range target.ExcludePaths {
		if RepoPathContains(excludePath, candidate) {
			return false
		}
	}

	return true
}

// RepoPathContains reports whether candidatePath is inside basePath using
// repo-relative forward-slash semantics. A basePath of "." contains everything.
func RepoPathContains(basePath, candidatePath string) bool {
	if basePath == "." {
		return true
	}

	if candidatePath == basePath {
		return true
	}

	return strings.HasPrefix(candidatePath, basePath+"/")
}

func normalizeRepoPath(rawPath string) (string, error) {
	trimmedPath := strings.TrimSpace(rawPath)
	if trimmedPath == "" {
		return "", ErrEmptyRepoPath
	}

	if isRepoPathAbsolute(trimmedPath) {
		return "", ErrPathMustBeRepoRelative
	}

	normalizedPath := filepath.ToSlash(trimmedPath)
	if path.IsAbs(normalizedPath) {
		return "", ErrPathMustBeRepoRelative
	}

	normalizedPath = path.Clean(normalizedPath)
	if normalizedPath == "." {
		return ".", nil
	}

	if normalizedPath == ".." || strings.HasPrefix(normalizedPath, "../") {
		return "", ErrPathMustBeRepoRelative
	}

	return normalizedPath, nil
}

func isRepoPathAbsolute(rawPath string) bool {
	const windowsDrivePrefixLength = 3

	if filepath.IsAbs(rawPath) {
		return true
	}

	normalizedPath := filepath.ToSlash(rawPath)
	if len(normalizedPath) < windowsDrivePrefixLength {
		return false
	}

	if normalizedPath[1] != ':' || normalizedPath[2] != '/' {
		return false
	}

	return (normalizedPath[0] >= 'A' && normalizedPath[0] <= 'Z') ||
		(normalizedPath[0] >= 'a' && normalizedPath[0] <= 'z')
}

func firstVersioning(values ...VersioningStrategy) VersioningStrategy {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return VersioningSemver
}

func validatePreMajorCalVer(targetID string, versioning VersioningStrategy, target Target) error {
	if versioning != VersioningCalVer {
		return nil
	}

	if target.PreMajorBreakingBumpsMinor != nil {
		return fmt.Errorf(
			"%w: targets.%s.pre_major_breaking_bumps_minor has no effect with calver versioning",
			ErrInvalidConfig,
			targetID,
		)
	}

	if target.PreMajorFeaturesBumpPatch != nil {
		return fmt.Errorf(
			"%w: targets.%s.pre_major_features_bump_patch has no effect with calver versioning",
			ErrInvalidConfig,
			targetID,
		)
	}

	return nil
}

func validateCalVerConfig(path string, calver CalVerConfig) error {
	err := version.ValidateCalVerFormat(calver.Format)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalidConfig, path, err)
	}

	return nil
}

func resolveBool(override *bool, defaultValue bool) bool {
	if override != nil {
		return *override
	}

	return defaultValue
}

func resolveVersionFiles(overridePaths, defaultPaths []string) []string {
	if len(overridePaths) > 0 {
		return slices.Clone(overridePaths)
	}

	return slices.Clone(defaultPaths)
}

func mergeChangelogConfig(defaultConfig, overrideConfig ChangelogConfig) ChangelogConfig {
	merged := defaultConfig

	if overrideConfig.File != "" {
		merged.File = overrideConfig.File
	}

	if len(overrideConfig.Include) > 0 {
		merged.Include = slices.Clone(overrideConfig.Include)
	}

	if len(overrideConfig.Sections) > 0 {
		merged.Sections = make(map[string]string, len(defaultConfig.Sections)+len(overrideConfig.Sections))
		maps.Copy(merged.Sections, defaultConfig.Sections)
		maps.Copy(merged.Sections, overrideConfig.Sections)
	}

	merged.References = mergeReferencesConfig(defaultConfig.References, overrideConfig.References)

	return merged
}

func mergeReferencesConfig(defaultConfig, overrideConfig ReferencesConfig) ReferencesConfig {
	merged := defaultConfig

	if len(overrideConfig.Patterns) > 0 {
		merged.Patterns = slices.Clone(overrideConfig.Patterns)
	}

	if len(overrideConfig.Footers) > 0 {
		merged.Footers = make(map[string]string, len(defaultConfig.Footers)+len(overrideConfig.Footers))
		maps.Copy(merged.Footers, defaultConfig.Footers)
		maps.Copy(merged.Footers, overrideConfig.Footers)
	}

	return merged
}

func mergeCalVerConfig(defaultConfig, overrideConfig CalVerConfig) CalVerConfig {
	merged := defaultConfig

	if overrideConfig.Format != "" {
		merged.Format = overrideConfig.Format
	}

	return merged
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

func validateBumpTypes(bt BumpTypesConfig) error {
	seen := make(map[string]string, len(bt.Minor)+len(bt.Patch))

	for _, t := range bt.Minor {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("%w: bump_types.minor must not contain empty strings", ErrInvalidConfig)
		}

		seen[t] = "minor"
	}

	for _, t := range bt.Patch {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("%w: bump_types.patch must not contain empty strings", ErrInvalidConfig)
		}

		if level, exists := seen[t]; exists {
			return fmt.Errorf("%w: bump_types: type %q appears in both %s and patch", ErrInvalidConfig, t, level)
		}
	}

	return nil
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

	err := validateReleaseChannels(release.Channels)
	if err != nil {
		return err
	}

	return nil
}

func validateReleaseChannelBranches(stableBranch string, channels map[string]ReleaseChannelConfig) error {
	stableBranch = strings.TrimSpace(stableBranch)
	for name, channel := range channels {
		branch := strings.TrimSpace(channel.Branch)
		if branch == "" || stableBranch == "" || branch != stableBranch {
			continue
		}

		return fmt.Errorf(
			"%w: release.channels.%s.branch %q duplicates stable branch",
			ErrInvalidConfig,
			strings.TrimSpace(name),
			branch,
		)
	}

	return nil
}

func validateReleaseChannels(channels map[string]ReleaseChannelConfig) error {
	seenBranches := make(map[string]string, len(channels))
	seenPrereleaseIDs := make(map[string]string, len(channels))

	for name, channel := range channels {
		channelName := strings.TrimSpace(name)
		if channelName == "" {
			return fmt.Errorf("%w: release.channels keys must not be empty", ErrInvalidConfig)
		}

		if strings.EqualFold(channelName, "stable") {
			return fmt.Errorf("%w: release.channels.%s must not use reserved name stable", ErrInvalidConfig, channelName)
		}

		branch := strings.TrimSpace(channel.Branch)
		if branch == "" {
			return fmt.Errorf("%w: release.channels.%s.branch must not be empty", ErrInvalidConfig, channelName)
		}

		if otherChannel, exists := seenBranches[branch]; exists {
			return fmt.Errorf(
				"%w: release.channels.%s.branch %q duplicates release.channels.%s.branch",
				ErrInvalidConfig,
				channelName,
				branch,
				otherChannel,
			)
		}

		seenBranches[branch] = channelName

		prerelease := strings.TrimSpace(channel.Prerelease)
		if prerelease == "" {
			return fmt.Errorf("%w: release.channels.%s.prerelease must not be empty", ErrInvalidConfig, channelName)
		}

		err := validatePrereleaseIdentifier(prerelease)
		if err != nil {
			return fmt.Errorf("%w: release.channels.%s.prerelease: %w", ErrInvalidConfig, channelName, err)
		}

		if otherChannel, exists := seenPrereleaseIDs[prerelease]; exists {
			return fmt.Errorf(
				"%w: release.channels.%s.prerelease %q duplicates release.channels.%s.prerelease",
				ErrInvalidConfig,
				channelName,
				prerelease,
				otherChannel,
			)
		}

		seenPrereleaseIDs[prerelease] = channelName

		if channel.ChangelogFile != "" && strings.TrimSpace(channel.ChangelogFile) == "" {
			return fmt.Errorf("%w: release.channels.%s.changelog_file must not be blank", ErrInvalidConfig, channelName)
		}
	}

	return nil
}

func validatePrereleaseIdentifier(identifier string) error {
	_, err := semver.StrictNewVersion("1.0.0-" + identifier)
	if err != nil {
		return fmt.Errorf("invalid semver prerelease identifier %q: %w", identifier, err)
	}

	return nil
}
