package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/monkescience/yeet/internal/commit"
	"go.yaml.in/yaml/v4"
)

const DefaultFile = ".yeet.yaml"

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
	VersioningSemver VersioningStrategy = "semver"
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
	Labels             ReleaseLabelsConfig             `yaml:"labels"`
	PRTitle            string                          `yaml:"pr_title"`
	PRTitleGroup       string                          `yaml:"pr_title_group"`
	CommitSubject      string                          `yaml:"commit_subject"`
	CommitSubjectGroup string                          `yaml:"commit_subject_group"`
	AutoMerge          bool                            `yaml:"auto_merge"`
	AutoMergeForce     bool                            `yaml:"auto_merge_force"`
	AutoMergeMethod    AutoMergeMethod                 `yaml:"auto_merge_method"`
	PRBodyHeader       string                          `yaml:"pr_body_header"`
	PRBodyFooter       string                          `yaml:"pr_body_footer"`
	PRBodyMaxLength    int                             `yaml:"pr_body_max_length"`
	Reviewers          []string                        `yaml:"reviewers,omitempty"`
	Channels           map[string]ReleaseChannelConfig `yaml:"channels,omitempty"`
}

type ReleaseLabelsConfig struct {
	Pending string   `yaml:"pending"`
	Tagged  string   `yaml:"tagged"`
	Yeet    bool     `yaml:"yeet"`
	Extra   []string `yaml:"extra,omitempty"`
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

type ReferencesConfig struct {
	Patterns []ReferencePattern `yaml:"patterns,omitempty"`
	Footers  map[string]string  `yaml:"footers,omitempty"`
}

type ReferencePattern struct {
	Pattern string `yaml:"pattern"`
	URL     string `yaml:"url"`
}

type BumpTypesConfig struct {
	Minor []string `yaml:"minor"`
	Patch []string `yaml:"patch"`
}

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
	errEmptyRepoPath          = errors.New("must not be empty")
	errPathMustBeRepoRelative = errors.New("must be repo-relative")

	errJSONPointerMustStartWithSlash = errors.New("must start with /")
)

var errJSONPointerInvalidEscape = errors.New("contains invalid escape")

func load(ctx context.Context, path string) (*Config, error) {
	slog.DebugContext(ctx, "config: loading file", slog.String("path", path))

	data, err := os.ReadFile(path) //nolint:gosec // path is from user config, not user input
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	cfg, err := parse(data)
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

func parse(data []byte) (*Config, error) {
	var instance any

	err := yaml.Unmarshal(data, &instance)
	if err != nil {
		return nil, fmt.Errorf("%w: parse config: %v", ErrInvalidConfig, err)
	}

	if err := validateAgainstSchema(instance); err != nil {
		return nil, err
	}

	cfg := Default()

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	err = decoder.Decode(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: parse config: %v", ErrInvalidConfig, err)
	}

	if err := validateRepositorySubsection(&cfg.Repository, cfg.Provider); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
