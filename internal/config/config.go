// Package config handles parsing and validation of .yeet.yaml configuration files.
package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// Conventional commit types referenced in defaults. Defined as constants so
// the same literal isn't repeated across bump-type and changelog defaults.
const (
	commitTypeFeat   = "feat"
	commitTypeFix    = "fix"
	commitTypePerf   = "perf"
	commitTypeRevert = "revert"
)

type VersioningStrategy string

const (
	// VersioningSemver uses semantic versioning (MAJOR.MINOR.PATCH).
	VersioningSemver VersioningStrategy = "semver"
	// VersioningCalVer uses calendar versioning (e.g., YYYY.0M.MICRO).
	VersioningCalVer VersioningStrategy = "calver"
)

type ProviderType string

const (
	ProviderAuto        ProviderType = "auto"
	ProviderGitHub      ProviderType = "github"
	ProviderGitLab      ProviderType = "gitlab"
	ProviderAzureDevOps ProviderType = "azuredevops"
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
	VersionFiles               []VersionFile      `yaml:"version_files,omitempty"`
	Release                    ReleaseConfig      `yaml:"release"`
	Changelog                  ChangelogConfig    `yaml:"changelog"`
	CalVer                     CalVerConfig       `yaml:"calver"`
	Targets                    map[string]Target  `yaml:"targets"`
	ActiveChannel              string             `yaml:"-"`
}

type VersionFileFormat string

const (
	VersionFileFormatMarkers VersionFileFormat = "markers"
	VersionFileFormatJSON    VersionFileFormat = "json"
)

type VersionFile struct {
	Path        string            `yaml:"path"`
	Format      VersionFileFormat `yaml:"format,omitempty"`
	JSONPointer string            `yaml:"json_pointer,omitempty"`
}

func (v *VersionFile) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var path string

		err := value.Decode(&path)
		if err != nil {
			return fmt.Errorf("decode version file path: %w", err)
		}

		*v = VersionFile{Path: path, Format: VersionFileFormatMarkers}

		return nil
	}

	type versionFile VersionFile

	var decoded versionFile

	err := value.Decode(&decoded)
	if err != nil {
		return fmt.Errorf("decode version file: %w", err)
	}

	*v = VersionFile(decoded)
	if v.Format == "" {
		v.Format = VersionFileFormatMarkers
	}

	return nil
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
	VersionFiles               []VersionFile      `yaml:"version_files,omitempty"`
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
	VersionFiles               []VersionFile
	Changelog                  ChangelogConfig
	CalVer                     CalVerConfig
	ExcludePaths               []string
	Includes                   []string
}

// RepositoryConfig holds the git remote name plus exactly one provider
// sub-section. The sub-section that may be set is determined by the
// top-level Provider field: setting a sub-section that does not match
// Provider is a validation error.
type RepositoryConfig struct {
	Remote      string                       `yaml:"remote"`
	GitHub      *GitHubRepositoryConfig      `yaml:"github,omitempty"`
	GitLab      *GitLabRepositoryConfig      `yaml:"gitlab,omitempty"`
	AzureDevOps *AzureDevOpsRepositoryConfig `yaml:"azuredevops,omitempty"`
}

type GitHubRepositoryConfig struct {
	Host    string `yaml:"host,omitempty"`
	Owner   string `yaml:"owner,omitempty"`
	Repo    string `yaml:"repo,omitempty"`
	Project string `yaml:"project,omitempty"`
}

type GitLabRepositoryConfig struct {
	Host    string `yaml:"host,omitempty"`
	Project string `yaml:"project,omitempty"`
}

type AzureDevOpsRepositoryConfig struct {
	Host         string `yaml:"host,omitempty"`
	Organization string `yaml:"organization,omitempty"`
	Project      string `yaml:"project,omitempty"`
	Repo         string `yaml:"repo,omitempty"`
	Collection   string `yaml:"collection,omitempty"`
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

var (
	ErrInvalidConfig          = errors.New("invalid config")
	ErrEmptyRepoPath          = errors.New("must not be empty")
	ErrPathMustBeRepoRelative = errors.New("must be repo-relative")

	errJSONPointerMustStartWithSlash = errors.New("must start with /")
)

var errJSONPointerInvalidEscape = errors.New("contains invalid escape")

func Load(ctx context.Context, path string) (*Config, error) {
	slog.DebugContext(ctx, "config: loading file", slog.String("path", path))

	data, err := os.ReadFile(path) //nolint:gosec // path is from user config, not user input
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	cfg, err := Parse(ctx, data)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "config: loaded file",
		slog.String("path", path),
		slog.Int("bytes", len(data)),
		slog.Int("targets", len(cfg.Targets)),
	)

	return cfg, nil
}

func Parse(ctx context.Context, data []byte) (*Config, error) {
	cfg := Default()

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	err := decoder.Decode(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: parse config: %w", ErrInvalidConfig, err)
	}

	err = validateRepositorySubsection(&cfg.Repository, cfg.Provider)
	if err != nil {
		return nil, err
	}

	err = cfg.Validate(ctx)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// validateRepositorySubsection enforces the single-active-sub-section
// invariant: at most one sub-section may be set, and if one is set it must
// match the top-level provider. Called from Parse, before structural and
// per-provider validation runs.
func validateRepositorySubsection(repository *RepositoryConfig, provider ProviderType) error {
	set := []ProviderType{}
	if repository.GitHub != nil {
		set = append(set, ProviderGitHub)
	}

	if repository.GitLab != nil {
		set = append(set, ProviderGitLab)
	}

	if repository.AzureDevOps != nil {
		set = append(set, ProviderAzureDevOps)
	}

	if len(set) > 1 {
		return fmt.Errorf(
			"%w: only one of repository.github, repository.gitlab, repository.azuredevops may be set",
			ErrInvalidConfig,
		)
	}

	if provider == ProviderAuto && len(set) == 1 {
		return fmt.Errorf(
			"%w: repository.%s set but provider is auto; set an explicit provider",
			ErrInvalidConfig,
			set[0],
		)
	}

	if len(set) == 1 && set[0] != provider {
		return fmt.Errorf(
			"%w: repository.%s set but provider is %s",
			ErrInvalidConfig,
			set[0],
			provider,
		)
	}

	return nil
}

func defaultBumpTypes() BumpTypesConfig {
	return BumpTypesConfig{
		Minor: []string{commitTypeFeat},
		Patch: []string{commitTypeFix, commitTypePerf},
	}
}

func defaultChangelogInclude() []string {
	return []string{commitTypeFeat, commitTypeFix, commitTypePerf, commitTypeRevert}
}

func defaultChangelogSections() map[string]string {
	return map[string]string{
		commitTypeFeat:   "Features",
		commitTypeFix:    "Bug Fixes",
		commitTypePerf:   "Performance Improvements",
		commitTypeRevert: "Reverts",
		"docs":           "Documentation",
		"style":          "Styles",
		"refactor":       "Code Refactoring",
		"test":           "Tests",
		"build":          "Build System",
		"ci":             "Continuous Integration",
		"chore":          "Miscellaneous Chores",
		"breaking":       "Breaking Changes",
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
			PRBodyFooter: "_Auto-generated preview — edit `CHANGELOG.md` to customize release notes._\n\n" +
				"_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._",
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

//nolint:funlen // Top-level validation deliberately enumerates every field check.
func (c *Config) Validate(ctx context.Context) error {
	if c.Versioning != VersioningSemver && c.Versioning != VersioningCalVer {
		return fmt.Errorf("%w: versioning must be %q or %q, got %q",
			ErrInvalidConfig, VersioningSemver, VersioningCalVer, c.Versioning)
	}

	if c.Branch == "" {
		return fmt.Errorf("%w: branch must not be empty", ErrInvalidConfig)
	}

	if c.Provider != ProviderAuto &&
		c.Provider != ProviderGitHub &&
		c.Provider != ProviderGitLab &&
		c.Provider != ProviderAzureDevOps {
		return fmt.Errorf("%w: provider must be %q, %q, %q, or %q, got %q",
			ErrInvalidConfig, ProviderAuto, ProviderGitHub, ProviderGitLab, ProviderAzureDevOps, c.Provider)
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

	for _, versionFile := range c.VersionFiles {
		err = validateVersionFile("version_files", versionFile)
		if err != nil {
			return err
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

	_, err = c.ResolvedTargets(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (c *Config) ResolvedTargets(ctx context.Context) (map[string]ResolvedTarget, error) {
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

	for _, t := range resolved {
		slog.DebugContext(ctx, "config: resolved target",
			slog.String("id", t.ID),
			slog.String("type", string(t.Type)),
			slog.String("path", t.Path),
			slog.String("versioning", string(t.Versioning)),
		)
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

	for _, versionFile := range resolved.VersionFiles {
		err := validateVersionFile("targets."+targetID+".version_files", versionFile)
		if err != nil {
			return ResolvedTarget{}, err
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

	if targetType == TargetTypeDerived && len(resolved.Includes) == 0 {
		return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.includes must not be empty", ErrInvalidConfig, targetID)
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
		for _, versionFile := range target.VersionFiles {
			normalizedVersionFilePath := strings.TrimSpace(versionFile.Path)

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

func validateVersionFile(configPath string, versionFile VersionFile) error {
	if strings.TrimSpace(versionFile.Path) == "" {
		return fmt.Errorf("%w: %s must not contain empty paths", ErrInvalidConfig, configPath)
	}

	switch versionFile.Format {
	case "", VersionFileFormatMarkers:
		if strings.TrimSpace(versionFile.JSONPointer) != "" {
			return fmt.Errorf(
				"%w: %s json_pointer requires format %q",
				ErrInvalidConfig,
				configPath,
				VersionFileFormatJSON,
			)
		}
	case VersionFileFormatJSON:
		if strings.TrimSpace(versionFile.JSONPointer) == "" {
			return fmt.Errorf(
				"%w: %s json_pointer is required for format %q",
				ErrInvalidConfig,
				configPath,
				VersionFileFormatJSON,
			)
		}

		err := validateJSONPointerSyntax(versionFile.JSONPointer)
		if err != nil {
			return fmt.Errorf("%w: %s json_pointer: %w", ErrInvalidConfig, configPath, err)
		}
	default:
		return fmt.Errorf(
			"%w: %s format must be %q or %q, got %q",
			ErrInvalidConfig,
			configPath,
			VersionFileFormatMarkers,
			VersionFileFormatJSON,
			versionFile.Format,
		)
	}

	return nil
}

func validateJSONPointerSyntax(pointer string) error {
	if pointer == "" || pointer[0] != '/' {
		return errJSONPointerMustStartWithSlash
	}

	for i := 0; i < len(pointer); i++ {
		if pointer[i] != '~' {
			continue
		}

		if i+1 >= len(pointer) || (pointer[i+1] != '0' && pointer[i+1] != '1') {
			return errJSONPointerInvalidEscape
		}

		i++
	}

	return nil
}

func resolveBool(override *bool, defaultValue bool) bool {
	if override != nil {
		return *override
	}

	return defaultValue
}

func resolveVersionFiles(overridePaths, defaultPaths []VersionFile) []VersionFile {
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
	if strings.TrimSpace(repository.Remote) == "" {
		return fmt.Errorf("%w: repository.remote must not be empty", ErrInvalidConfig)
	}

	switch provider {
	case ProviderGitHub:
		return validateGitHubRepositoryConfig(repository.GitHub)
	case ProviderGitLab:
		return validateGitLabRepositoryConfig(repository.GitLab)
	case ProviderAzureDevOps:
		return validateAzureDevOpsRepositoryConfig(repository.AzureDevOps)
	case ProviderAuto:
		return nil
	default:
		return nil
	}
}

func validateGitHubRepositoryConfig(github *GitHubRepositoryConfig) error {
	if github == nil {
		return nil
	}

	host := strings.TrimSpace(github.Host)
	owner := strings.TrimSpace(github.Owner)
	repo := strings.TrimSpace(github.Repo)
	project := normalizeRepositoryProjectPath(github.Project)

	if github.Host != "" && host == "" {
		return fmt.Errorf("%w: repository.github.host must not be blank", ErrInvalidConfig)
	}

	if github.Owner != "" && owner == "" {
		return fmt.Errorf("%w: repository.github.owner must not be blank", ErrInvalidConfig)
	}

	if github.Repo != "" && repo == "" {
		return fmt.Errorf("%w: repository.github.repo must not be blank", ErrInvalidConfig)
	}

	if github.Project != "" && project == "" {
		return fmt.Errorf("%w: repository.github.project must not be blank", ErrInvalidConfig)
	}

	if (owner == "") != (repo == "") {
		return fmt.Errorf(
			"%w: repository.github.owner and repository.github.repo must be set together",
			ErrInvalidConfig,
		)
	}

	if project != "" && owner != "" && repo != "" && project != owner+"/"+repo {
		return fmt.Errorf(
			"%w: repository.github.project must match repository.github.owner/repo",
			ErrInvalidConfig,
		)
	}

	if strings.Contains(owner, "/") {
		return fmt.Errorf("%w: repository.github.owner must not contain '/'", ErrInvalidConfig)
	}

	if project != "" {
		projectOwner, _, ok := splitGitHubProjectPath(project)
		if !ok || strings.Contains(projectOwner, "/") {
			return fmt.Errorf(
				"%w: repository.github.project must be in owner/repo form",
				ErrInvalidConfig,
			)
		}
	}

	return nil
}

func validateGitLabRepositoryConfig(gitlab *GitLabRepositoryConfig) error {
	if gitlab == nil {
		return nil
	}

	host := strings.TrimSpace(gitlab.Host)
	project := normalizeRepositoryProjectPath(gitlab.Project)

	if gitlab.Host != "" && host == "" {
		return fmt.Errorf("%w: repository.gitlab.host must not be blank", ErrInvalidConfig)
	}

	if gitlab.Project != "" && project == "" {
		return fmt.Errorf("%w: repository.gitlab.project must not be blank", ErrInvalidConfig)
	}

	return nil
}

func validateAzureDevOpsRepositoryConfig(azure *AzureDevOpsRepositoryConfig) error {
	if azure == nil {
		return fmt.Errorf("%w: repository.azuredevops is required when provider is azuredevops", ErrInvalidConfig)
	}

	host := strings.TrimSpace(azure.Host)
	organization := strings.TrimSpace(azure.Organization)
	project := normalizeRepositoryProjectPath(azure.Project)
	repo := strings.TrimSpace(azure.Repo)
	collection := strings.TrimSpace(azure.Collection)

	if azure.Host != "" && host == "" {
		return fmt.Errorf("%w: repository.azuredevops.host must not be blank", ErrInvalidConfig)
	}

	if azure.Organization != "" && organization == "" {
		return fmt.Errorf("%w: repository.azuredevops.organization must not be blank", ErrInvalidConfig)
	}

	if azure.Project != "" && project == "" {
		return fmt.Errorf("%w: repository.azuredevops.project must not be blank", ErrInvalidConfig)
	}

	if azure.Repo != "" && repo == "" {
		return fmt.Errorf("%w: repository.azuredevops.repo must not be blank", ErrInvalidConfig)
	}

	if azure.Collection != "" && collection == "" {
		return fmt.Errorf("%w: repository.azuredevops.collection must not be blank", ErrInvalidConfig)
	}

	if organization == "" {
		return fmt.Errorf("%w: repository.azuredevops.organization is required", ErrInvalidConfig)
	}

	if project == "" {
		return fmt.Errorf("%w: repository.azuredevops.project is required", ErrInvalidConfig)
	}

	if repo == "" {
		return fmt.Errorf("%w: repository.azuredevops.repo is required", ErrInvalidConfig)
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
