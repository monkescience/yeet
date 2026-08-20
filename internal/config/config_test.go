package config_test

import (
	"testing"
	"time"

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
	testastic.Equal(t, "Local", cfg.Timezone)
	testastic.Equal(t, config.ProviderAuto, cfg.Provider)
	testastic.Equal(t, "origin", cfg.Repository.Remote)
	testastic.Equal(t, 30*time.Second, cfg.Network.RequestTimeout)
	testastic.Equal(t, 4, cfg.Network.Retry.MaxAttempts)
	testastic.Equal(t, time.Second, cfg.Network.Retry.MinBackoff)
	testastic.Equal(t, 10*time.Second, cfg.Network.Retry.MaxBackoff)
	testastic.Equal(t, "yeet/release-{{ .Branch }}", cfg.Release.BranchTemplate)
	testastic.Equal(t, "", cfg.Release.NameTemplate)
	testastic.Equal(t, 250*time.Millisecond, cfg.Release.MergePolling.InitialInterval)
	testastic.Equal(t, 5*time.Second, cfg.Release.MergePolling.MaxInterval)
	testastic.Equal(t, 2*time.Minute, cfg.Release.MergePolling.Timeout)
	testastic.False(t, cfg.Release.AutoMerge)
	testastic.False(t, cfg.Release.AutoMergeForce)
	testastic.Equal(t, config.AutoMergeMethodAuto, cfg.Release.AutoMergeMethod)
	testastic.Equal(t, 0, len(cfg.Release.Channels))
	testastic.Equal(t, 0, cfg.Release.PRBodyMaxLength)
	testastic.Equal(t, "autorelease: pending", cfg.Release.Labels.Pending)
	testastic.Equal(t, "autorelease: tagged", cfg.Release.Labels.Tagged)
	testastic.True(t, cfg.Release.Labels.Yeet)
	testastic.Equal(t, 0, len(cfg.Release.Labels.Extra))
	testastic.Equal(t, "", cfg.Release.PRTitle)
	testastic.Equal(t, "", cfg.Release.PRTitleGroup)
	testastic.Equal(t, "", cfg.Release.CommitSubject)
	testastic.Equal(t, "", cfg.Release.CommitSubjectGroup)
	testastic.Equal(t, "## ٩(^ᴗ^)۶ release created", cfg.Release.PRBodyHeader)
	testastic.AssertFile(t, "testdata/default/pr_body_footer.expected.md", cfg.Release.PRBodyFooter)
	testastic.Equal(t, 0, len(cfg.VersionFiles))
	testastic.Equal(t, "CHANGELOG.md", cfg.Changelog.File)
	testastic.Equal(t, 4, len(cfg.Changelog.Include))
	testastic.Equal(t, "⚠ BREAKING CHANGES", cfg.Changelog.Sections["breaking"])
	testastic.Equal(t, "YYYY.0M.MICRO", cfg.CalVer.Format)
	testastic.True(t, cfg.PreMajorBreakingBumpsMinor)
	testastic.True(t, cfg.PreMajorFeaturesBumpPatch)
	testastic.SliceEqual(t, []string{"feat"}, cfg.BumpTypes.Minor)
	testastic.SliceEqual(t, []string{"fix", "perf"}, cfg.BumpTypes.Patch)
}

func TestTimezoneValidation(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Local", "UTC", "Europe/Berlin", "America/Los_Angeles"} {
		t.Run("accepts "+name, func(t *testing.T) {
			t.Parallel()

			cfg := config.Default()
			cfg.Timezone = name

			location, err := cfg.TimeLocation()

			testastic.NoError(t, err)
			testastic.Equal(t, name, location.String())
		})
	}

	tests := []struct {
		name     string
		timezone string
		message  string
	}{
		{
			name:     "blank",
			timezone: " ",
			message:  "invalid config: timezone must not be blank",
		},
		{
			name:     "surrounding whitespace",
			timezone: " UTC ",
			message:  "invalid config: timezone must not contain surrounding whitespace",
		},
		{
			name:     "unknown location",
			timezone: "Mars/Olympus_Mons",
			message:  "invalid config: timezone \"Mars/Olympus_Mons\" is not a valid IANA location",
		},
	}

	for _, tt := range tests {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := config.Default()
			cfg.Timezone = tt.timezone

			err := cfg.Validate()

			testastic.ErrorIs(t, err, config.ErrInvalidConfig)
			testastic.Equal(t, tt.message, err.Error())
		})
	}
}

func TestRepositoryURLValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		apiURL  string
		webURL  string
		message string
	}{
		{
			name:    "http API URL",
			apiURL:  "http://github.com/api/v3",
			message: "invalid config: repository.github.api_url must be an absolute HTTPS URL",
		},
		{
			name:    "relative API URL",
			apiURL:  "/api/v3",
			message: "invalid config: repository.github.api_url must be an absolute HTTPS URL",
		},
		{
			name:    "API URL with credentials",
			apiURL:  "https://user@example.com/api/v3",
			message: "invalid config: repository.github.api_url must not contain credentials",
		},
		{
			name:    "API URL with query",
			apiURL:  "https://example.com/api/v3?tenant=acme",
			message: "invalid config: repository.github.api_url must not contain a query",
		},
		{
			name:    "web URL with fragment",
			webURL:  "https://example.com/git#repos",
			message: "invalid config: repository.github.web_url must not contain a fragment",
		},
		{
			name:    "padded web URL",
			webURL:  " https://example.com/git ",
			message: "invalid config: repository.github.web_url must not contain surrounding whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := config.Default()
			cfg.Provider = config.ProviderGitHub
			cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
				Host:   "github.com",
				APIURL: tt.apiURL,
				WebURL: tt.webURL,
				Owner:  "platform",
				Repo:   "yeet",
			}

			err := cfg.Validate()

			testastic.ErrorIs(t, err, config.ErrInvalidConfig)
			testastic.Equal(t, tt.message, err.Error())
		})
	}

	t.Run("accepts HTTPS paths and ports for every provider", func(t *testing.T) {
		t.Parallel()

		configs := []*config.Config{config.Default(), config.Default(), config.Default()}

		configs[0].Provider = config.ProviderGitHub
		configs[0].Repository.GitHub = &config.GitHubRepositoryConfig{
			Host: "github.example.com", APIURL: "https://github.example.com:8443/api/v3",
			WebURL: "https://github.example.com:8443/code", Owner: "platform", Repo: "yeet",
		}

		configs[1].Provider = config.ProviderGitLab
		configs[1].Repository.GitLab = &config.GitLabRepositoryConfig{
			Host: "gitlab.example.com", APIURL: "https://gitlab.example.com:8443/root/api/v4",
			WebURL: "https://gitlab.example.com:8443/root", Project: "platform/yeet",
		}

		configs[2].Provider = config.ProviderAzureDevOps
		configs[2].Repository.AzureDevOps = &config.AzureDevOpsRepositoryConfig{
			Host: "devops.example.com", APIURL: "https://devops.example.com:8443/tfs",
			WebURL: "https://devops.example.com:8443/tfs", Organization: "platform",
			Project: "tools", Repo: "yeet",
		}

		for _, cfg := range configs {
			cfg.Targets = map[string]config.Target{
				"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
			}

			testastic.NoError(t, cfg.Validate())
		}
	})
}

func TestRepositoryFilePathValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*config.Config)
		message   string
	}{
		{
			name: "top-level version file escapes repository",
			configure: func(cfg *config.Config) {
				cfg.VersionFiles = []config.VersionFile{{Path: "../VERSION"}}
			},
			message: "invalid config: version_files entry \"../VERSION\" must be repo-relative",
		},
		{
			name: "target version file escapes repository",
			configure: func(cfg *config.Config) {
				target := cfg.Targets["app"]
				target.VersionFiles = []config.VersionFile{{Path: "../VERSION"}}
				cfg.Targets["app"] = target
			},
			message: "invalid config: targets.app.version_files entry \"../VERSION\" must be repo-relative",
		},
		{
			name: "top-level changelog escapes repository",
			configure: func(cfg *config.Config) {
				cfg.Changelog.File = "../CHANGELOG.md"
			},
			message: "invalid config: changelog.file must be repo-relative",
		},
		{
			name: "target changelog escapes repository",
			configure: func(cfg *config.Config) {
				target := cfg.Targets["app"]
				target.Changelog.File = "../CHANGELOG.md"
				cfg.Targets["app"] = target
			},
			message: "invalid config: targets.app.changelog.file must be repo-relative",
		},
		{
			name: "channel changelog escapes repository",
			configure: func(cfg *config.Config) {
				cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
					"beta": {
						Branch: "beta", Prerelease: "beta", ChangelogFile: "../CHANGELOG.beta.md",
					},
				}
			},
			message: "invalid config: release.channels.beta.changelog_file must be repo-relative",
		},
		{
			name: "top-level version file uses a Windows drive path",
			configure: func(cfg *config.Config) {
				cfg.VersionFiles = []config.VersionFile{{Path: `C:\repo\VERSION`}}
			},
			message: `invalid config: version_files entry "C:\\repo\\VERSION" must be repo-relative`,
		},
		{
			name: "target version file uses backslash traversal",
			configure: func(cfg *config.Config) {
				target := cfg.Targets["app"]
				target.VersionFiles = []config.VersionFile{{Path: `..\VERSION`}}
				cfg.Targets["app"] = target
			},
			message: `invalid config: targets.app.version_files entry "..\\VERSION" must be repo-relative`,
		},
		{
			name: "top-level changelog uses a UNC path",
			configure: func(cfg *config.Config) {
				cfg.Changelog.File = `\\server\share\CHANGELOG.md`
			},
			message: "invalid config: changelog.file must be repo-relative",
		},
		{
			name: "target changelog uses a Windows drive path",
			configure: func(cfg *config.Config) {
				target := cfg.Targets["app"]
				target.Changelog.File = `D:\repo\CHANGELOG.md`
				cfg.Targets["app"] = target
			},
			message: "invalid config: targets.app.changelog.file must be repo-relative",
		},
		{
			name: "channel changelog uses backslash traversal",
			configure: func(cfg *config.Config) {
				cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
					"beta": {Branch: "beta", Prerelease: "beta", ChangelogFile: `..\CHANGELOG.beta.md`},
				}
			},
			message: "invalid config: release.channels.beta.changelog_file must be repo-relative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given: a config with a repository file path outside the allowed boundary
			cfg := config.Default()
			cfg.Targets = map[string]config.Target{
				"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
			}
			tt.configure(cfg)

			// when: validating the config
			err := cfg.Validate()

			// then: the path is rejected with its field-specific error
			testastic.ErrorIs(t, err, config.ErrInvalidConfig)

			if err == nil {
				return
			}

			testastic.Equal(t, tt.message, err.Error())
		})
	}

	t.Run("repository root is not a file", func(t *testing.T) {
		t.Parallel()

		// given: a config that uses the repository root as its changelog file
		cfg := config.Default()
		cfg.Changelog.File = "./"
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating the config
		err := cfg.Validate()

		// then: the directory is rejected where a file is required
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err == nil {
			return
		}

		testastic.Equal(t, "invalid config: changelog.file must refer to a file", err.Error())
	})
}

func TestWindowsStyleRepositoryPathValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*config.Config)
		message   string
	}{
		{
			name: "target uses a backslash drive path",
			configure: func(cfg *config.Config) {
				target := cfg.Targets["app"]
				target.Path = `C:\repo\app`
				cfg.Targets["app"] = target
			},
			message: "invalid config: targets.app.path must be repo-relative",
		},
		{
			name: "target uses a UNC path",
			configure: func(cfg *config.Config) {
				target := cfg.Targets["app"]
				target.Path = `\\server\share\app`
				cfg.Targets["app"] = target
			},
			message: "invalid config: targets.app.path must be repo-relative",
		},
		{
			name: "exclude uses backslash traversal",
			configure: func(cfg *config.Config) {
				target := cfg.Targets["app"]
				target.ExcludePaths = []string{`..\outside`}
				cfg.Targets["app"] = target
			},
			message: "invalid config: targets.app.exclude_paths contains must be repo-relative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given: a repository path using Windows-only escape syntax
			cfg := config.Default()
			cfg.Targets = map[string]config.Target{
				"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
			}
			tt.configure(cfg)

			// when: validating on any host operating system
			err := cfg.Validate()

			// then: the path is rejected consistently as outside the repository
			testastic.ErrorIs(t, err, config.ErrInvalidConfig)

			if err == nil {
				return
			}

			testastic.Equal(t, tt.message, err.Error())
		})
	}
}

func TestReleaseMergePollingValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*config.ReleaseMergePollingConfig)
		message   string
	}{
		{
			name: "zero initial interval",
			configure: func(polling *config.ReleaseMergePollingConfig) {
				polling.InitialInterval = 0
			},
			message: "invalid config: release.merge_polling.initial_interval must be greater than zero",
		},
		{
			name: "zero maximum interval",
			configure: func(polling *config.ReleaseMergePollingConfig) {
				polling.MaxInterval = 0
			},
			message: "invalid config: release.merge_polling.max_interval must be greater than zero",
		},
		{
			name: "zero timeout",
			configure: func(polling *config.ReleaseMergePollingConfig) {
				polling.Timeout = 0
			},
			message: "invalid config: release.merge_polling.timeout must be greater than zero",
		},
		{
			name: "initial interval exceeds maximum",
			configure: func(polling *config.ReleaseMergePollingConfig) {
				polling.InitialInterval = 6 * time.Second
			},
			message: "invalid config: release.merge_polling.initial_interval must not exceed release.merge_polling.max_interval",
		},
		{
			name: "maximum interval exceeds timeout",
			configure: func(polling *config.ReleaseMergePollingConfig) {
				polling.MaxInterval = 3 * time.Minute
			},
			message: "invalid config: release.merge_polling.max_interval must not exceed release.merge_polling.timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := config.Default()
			tt.configure(&cfg.Release.MergePolling)

			err := cfg.Validate()

			testastic.ErrorIs(t, err, config.ErrInvalidConfig)
			testastic.Equal(t, tt.message, err.Error())
		})
	}
}

func TestNetworkValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*config.NetworkConfig)
		message   string
	}{
		{
			name: "zero request timeout",
			configure: func(network *config.NetworkConfig) {
				network.RequestTimeout = 0
			},
			message: "invalid config: network.request_timeout must be greater than zero",
		},
		{
			name: "zero attempts",
			configure: func(network *config.NetworkConfig) {
				network.Retry.MaxAttempts = 0
			},
			message: "invalid config: network.retry.max_attempts must be at least 1",
		},
		{
			name: "zero minimum backoff",
			configure: func(network *config.NetworkConfig) {
				network.Retry.MinBackoff = 0
			},
			message: "invalid config: network.retry.min_backoff must be greater than zero",
		},
		{
			name: "zero maximum backoff",
			configure: func(network *config.NetworkConfig) {
				network.Retry.MaxBackoff = 0
			},
			message: "invalid config: network.retry.max_backoff must be greater than zero",
		},
		{
			name: "minimum backoff exceeds maximum",
			configure: func(network *config.NetworkConfig) {
				network.Retry.MinBackoff = 11 * time.Second
			},
			message: "invalid config: network.retry.min_backoff must not exceed network.retry.max_backoff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := config.Default()
			tt.configure(&cfg.Network)

			err := cfg.Validate()

			testastic.ErrorIs(t, err, config.ErrInvalidConfig)
			testastic.Equal(t, tt.message, err.Error())
		})
	}
}

func TestReleaseLabelsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		labels config.ReleaseLabelsConfig
	}{
		{
			name: "blank pending label",
			labels: config.ReleaseLabelsConfig{
				Pending: " ",
				Tagged:  "tagged",
			},
		},
		{
			name: "padded tagged label",
			labels: config.ReleaseLabelsConfig{
				Pending: "pending",
				Tagged:  "tagged ",
			},
		},
		{
			name: "matching lifecycle labels",
			labels: config.ReleaseLabelsConfig{
				Pending: "Release",
				Tagged:  "release",
			},
		},
		{
			name: "duplicate extra labels",
			labels: config.ReleaseLabelsConfig{
				Pending: "pending",
				Tagged:  "tagged",
				Extra:   []string{"Automated", "automated"},
			},
		},
		{
			name: "extra label collides with lifecycle label",
			labels: config.ReleaseLabelsConfig{
				Pending: "Pending",
				Tagged:  "tagged",
				Extra:   []string{"pending"},
			},
		},
		{
			name: "pending label collides with managed label",
			labels: config.ReleaseLabelsConfig{
				Pending: "Yeet",
				Tagged:  "tagged",
				Yeet:    true,
			},
		},
		{
			name: "extra label collides with managed label",
			labels: config.ReleaseLabelsConfig{
				Pending: "pending",
				Tagged:  "tagged",
				Yeet:    true,
				Extra:   []string{"YEET"},
			},
		},
		{
			name: "comma in pending label",
			labels: config.ReleaseLabelsConfig{
				Pending: "release, waiting",
				Tagged:  "tagged",
			},
		},
		{
			name: "comma in tagged label",
			labels: config.ReleaseLabelsConfig{
				Pending: "pending",
				Tagged:  "release,done",
			},
		},
		{
			name: "comma in extra label",
			labels: config.ReleaseLabelsConfig{
				Pending: "pending",
				Tagged:  "tagged",
				Extra:   []string{"automated,release"},
			},
		},
		{
			name: "reserved filter value as pending label",
			labels: config.ReleaseLabelsConfig{
				Pending: "Any",
				Tagged:  "tagged",
			},
		},
		{
			name: "reserved filter value as tagged label",
			labels: config.ReleaseLabelsConfig{
				Pending: "pending",
				Tagged:  "none",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given: release labels that violate a provider-neutral rule
			cfg := config.Default()
			cfg.Release.Labels = tt.labels

			// when: validating the config
			err := cfg.Validate()

			// then: validation rejects the labels
			testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		})
	}
}

func TestChangelogSectionHeadingValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		heading string
		message string
	}{
		{
			name:    "line feed",
			heading: "Bug\nFixes",
			message: "must be a single line",
		},
		{
			name:    "carriage return",
			heading: "Bug\rFixes",
			message: "must be a single line",
		},
		{
			name:    "vertical tab",
			heading: "Bug\vFixes",
			message: "must be a single line",
		},
		{
			name:    "form feed",
			heading: "Bug\fFixes",
			message: "must be a single line",
		},
		{
			name:    "next line",
			heading: "Bug\u0085Fixes",
			message: "must be a single line",
		},
		{
			name:    "Unicode line separator",
			heading: "Bug\u2028Fixes",
			message: "must be a single line",
		},
		{
			name:    "Unicode paragraph separator",
			heading: "Bug\u2029Fixes",
			message: "must be a single line",
		},
		{
			name:    "leading whitespace",
			heading: " Bug Fixes",
			message: "must not have leading or trailing whitespace",
		},
		{
			name:    "trailing Unicode whitespace",
			heading: "Bug Fixes\u00a0",
			message: "must not have leading or trailing whitespace",
		},
		{
			name:    "leading Markdown markers",
			heading: "### Bug Fixes",
			message: "must contain heading text without leading or closing Markdown # markers",
		},
		{
			name:    "closing Markdown markers",
			heading: "Bug Fixes ###",
			message: "must contain heading text without leading or closing Markdown # markers",
		},
	}

	for _, tt := range tests {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			t.Parallel()

			// given: a config with a section heading that cannot round-trip through Markdown
			cfg := config.Default()
			cfg.Changelog.Sections["fix"] = tt.heading
			cfg.Targets = map[string]config.Target{
				"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
			}

			// when: validating the config directly
			err := cfg.Validate()

			// then: validation explains the invalid heading shape
			testastic.ErrorIs(t, err, config.ErrInvalidConfig)

			if err == nil {
				return
			}

			testastic.Equal(t, "invalid config: targets.app.changelog.sections.fix "+tt.message, err.Error())
		})
	}

	for _, heading := range []string{"🚀 Features", "C# Integration", "Release###", "#123", "Bug\tFixes"} {
		t.Run("accepts "+heading, func(t *testing.T) {
			t.Parallel()

			// given: a heading whose literal hashes and internal whitespace round-trip through Markdown
			cfg := config.Default()
			cfg.Changelog.Sections["fix"] = heading
			cfg.Targets = map[string]config.Target{
				"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
			}

			// when: validating the config directly
			err := cfg.Validate()

			// then: the heading is accepted
			testastic.NoError(t, err)
		})
	}

	t.Run("rejects duplicate effective headings", func(t *testing.T) {
		t.Parallel()

		// given: a target override that reuses an inherited section heading
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				Changelog: config.ChangelogConfig{
					Sections: map[string]string{"fix": "Features"},
				},
			},
		}

		// when: validating the effective target config
		err := cfg.Validate()

		// then: validation identifies both commit types deterministically
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err == nil {
			return
		}

		testastic.Equal(
			t,
			"invalid config: targets.app.changelog.sections headings must be unique: "+
				"\"Features\" is used by \"feat\" and \"fix\"",
			err.Error(),
		)
	})

	t.Run("rejects a duplicate fallback heading", func(t *testing.T) {
		t.Parallel()

		// given: an unmapped include whose fallback collides with a configured heading
		cfg := config.Default()
		cfg.Changelog.Include = []string{"feat", "features"}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating the effective target config
		err := cfg.Validate()

		// then: validation identifies the configured and fallback owners
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err == nil {
			return
		}

		testastic.Equal(
			t,
			"invalid config: targets.app.changelog.sections headings must be unique: "+
				"\"Features\" is used by \"feat\" and \"features\"",
			err.Error(),
		)
	})

	t.Run("rejects an invalid fallback heading", func(t *testing.T) {
		t.Parallel()

		// given: an unmapped include that would synthesize a multiline heading
		cfg := config.Default()
		cfg.Changelog.Include = []string{"feat", "release\nnotes"}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating the effective target config
		err := cfg.Validate()

		// then: validation identifies the include and invalid fallback shape
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err == nil {
			return
		}

		testastic.Equal(
			t,
			"invalid config: targets.app.changelog.include entry \"release\\nnotes\" "+
				"produces a section heading that must be a single line",
			err.Error(),
		)
	})

	t.Run("rejects breaking in include", func(t *testing.T) {
		t.Parallel()

		// given: an include list that would emit the automatic breaking section twice
		cfg := config.Default()
		cfg.Changelog.Include = []string{"feat", "breaking"}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating the effective target config
		err := cfg.Validate()

		// then: validation explains that breaking changes are already included
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err == nil {
			return
		}

		testastic.Equal(
			t,
			"invalid config: targets.app.changelog.include must not contain \"breaking\" because breaking changes "+
				"are included automatically",
			err.Error(),
		)
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("blank stable branch fails", func(t *testing.T) {
		t.Parallel()

		// given: a config whose stable branch contains only whitespace
		cfg := config.Default()
		cfg.Branch = " \t"
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating the config
		err := cfg.Validate()

		// then: the blank branch is rejected
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err == nil {
			return
		}

		testastic.Equal(t, "invalid config: branch must not be blank", err.Error())
	})

	t.Run("valid config passes", func(t *testing.T) {
		t.Parallel()

		// given: a valid default config with targets
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
			},
		}

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

	t.Run("duplicate changelog include fails", func(t *testing.T) {
		t.Parallel()

		// given: a target inheriting duplicate changelog include entries
		cfg := config.Default()
		cfg.Changelog.Include = []string{"fix", "fix"}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the duplicate before release planning
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: targets.app.changelog.include contains duplicate \"fix\"",
			err.Error(),
		)
	})

	t.Run("target inherits blank changelog section heading fails", func(t *testing.T) {
		t.Parallel()

		// given: a target inheriting a blank top-level changelog section heading
		cfg := config.Default()
		cfg.Changelog.Sections["fix"] = "   "
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the effective heading before release planning
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: targets.app.changelog.sections.fix must not be blank",
			err.Error(),
		)
	})

	t.Run("target blank changelog section heading override fails", func(t *testing.T) {
		t.Parallel()

		// given: a target overriding an inherited changelog section with a blank heading
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				Changelog: config.ChangelogConfig{Sections: map[string]string{"fix": ""}},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the effective heading before release planning
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: targets.app.changelog.sections.fix must not be blank",
			err.Error(),
		)
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

	t.Run("negative pr body max length fails", func(t *testing.T) {
		t.Parallel()

		// given: config with a negative PR body max length
		cfg := config.Default()
		cfg.Release.PRBodyMaxLength = -1

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("invalid reference pattern regex fails", func(t *testing.T) {
		t.Parallel()

		// given: config with a malformed reference pattern regex
		cfg := config.Default()
		cfg.Changelog.References = config.ReferencesConfig{
			Patterns: []config.ReferencePattern{
				{Pattern: `[invalid`, URL: "https://jira.example.com/browse/{value}"},
			},
		}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the malformed regex
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: changelog.references.patterns[0].pattern \"[invalid\" is not a valid "+
				"regular expression: error parsing regexp: missing closing ]: `[invalid`",
			err.Error(),
		)
	})

	t.Run("empty reference pattern fails", func(t *testing.T) {
		t.Parallel()

		// given: config with a blank reference pattern
		cfg := config.Default()
		cfg.Changelog.References = config.ReferencesConfig{
			Patterns: []config.ReferencePattern{
				{Pattern: "  ", URL: "https://jira.example.com/browse/{value}"},
			},
		}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the blank pattern
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: changelog.references.patterns[0].pattern must not be empty", err.Error())
	})

	t.Run("invalid target reference pattern regex fails", func(t *testing.T) {
		t.Parallel()

		// given: a target overriding references with a malformed regex
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				Changelog: config.ChangelogConfig{
					References: config.ReferencesConfig{
						Patterns: []config.ReferencePattern{
							{Pattern: `(unclosed`, URL: "https://example.com/{value}"},
						},
					},
				},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the malformed regex with the target path
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: targets.app.changelog.references.patterns[0].pattern \"(unclosed\" is not "+
				"a valid regular expression: error parsing regexp: missing closing ): `(unclosed`",
			err.Error(),
		)
	})

	t.Run("empty version file path fails", func(t *testing.T) {
		t.Parallel()

		// given: config with an empty version file path
		cfg := config.Default()
		cfg.VersionFiles = []config.VersionFile{{Path: "  "}}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("invalid auto merge method fails", func(t *testing.T) {
		t.Parallel()

		// given: config with unsupported auto merge method
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Release.AutoMergeMethod = "fast-forward"

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err != nil {
			testastic.Equal(
				t,
				"invalid config: release.auto_merge_method must be \"auto\", \"squash\", "+
					"\"rebase\", or \"merge\", got \"fast-forward\"",
				err.Error(),
			)
		}
	})

	t.Run("invalid provider fails", func(t *testing.T) {
		t.Parallel()

		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Provider = "wrongo"

		err := cfg.Validate()

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err != nil {
			testastic.Equal(
				t,
				"invalid config: provider must be \"auto\", \"github\", \"gitlab\", or \"azuredevops\", got \"wrongo\"",
				err.Error(),
			)
		}
	})

	t.Run("repository owner and repo must be set together", func(t *testing.T) {
		t.Parallel()

		// given: github sub-section with only owner set
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Owner: "platform"}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("repository project must match owner and repo when both are set", func(t *testing.T) {
		t.Parallel()

		// given: github sub-section with conflicting explicit coordinates
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
			Owner:   "platform",
			Repo:    "yeet",
			Project: "other/yeet",
		}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.github.project must match repository.github.owner/repo", err.Error())
	})

	t.Run("github project must stay in owner repo form", func(t *testing.T) {
		t.Parallel()

		// given: explicit github provider with a subgroup-style project path
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Project: "group/subgroup/service"}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.github.project must be in owner/repo form", err.Error())
	})

	t.Run("github project in valid owner repo form passes", func(t *testing.T) {
		t.Parallel()

		// given: explicit github provider with a valid two-segment project
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Project: "owner/repo"}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: no error
		testastic.NoError(t, err)
	})

	t.Run("github project with empty owner segment fails", func(t *testing.T) {
		t.Parallel()

		// given: explicit github provider with an empty owner segment
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Project: "/repo"}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.github.project must be in owner/repo form", err.Error())
	})

	t.Run("github project with empty repo segment fails", func(t *testing.T) {
		t.Parallel()

		// given: explicit github provider with an empty repo segment
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Project: "owner/"}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.github.project must be in owner/repo form", err.Error())
	})

	t.Run("empty repository remote fails", func(t *testing.T) {
		t.Parallel()

		// given: repository config with an empty remote name
		cfg := config.Default()
		cfg.Repository.Remote = ""

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("windows drive letter target path fails", func(t *testing.T) {
		t.Parallel()

		// given: a target with an absolute Windows-style path
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "C:/repo/app",
				TagPrefix: "api-v",
			},
		}

		// when: validating the config
		err := cfg.Validate()

		// then: the repo-relative path validation rejects the absolute path
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.api.path must be repo-relative", err.Error())
	})

	t.Run("windows drive letter exclude path fails", func(t *testing.T) {
		t.Parallel()

		// given: a target with an absolute Windows-style exclude path
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"root": {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api", "D:/repo/shared"},
				Includes:     []string{"api"},
			},
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
		}

		// when: validating the config
		err := cfg.Validate()

		// then: the repo-relative path validation rejects the absolute exclude path
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.root.exclude_paths contains must be repo-relative", err.Error())
	})

	t.Run("shared inherited version file across targets succeeds", func(t *testing.T) {
		t.Parallel()

		// given: multiple targets that inherit the same top-level version file path
		cfg := config.Default()
		cfg.VersionFiles = []config.VersionFile{{Path: "VERSION"}}
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
				Changelog: config.ChangelogConfig{File: "services/api/CHANGELOG.md"},
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
				Changelog: config.ChangelogConfig{File: "apps/web/CHANGELOG.md"},
			},
		}

		// when: validating the config
		err := cfg.Validate()

		// then: validation allows targets to share the inherited version file
		testastic.NoError(t, err)
	})

	t.Run("duplicate explicit version file across targets fails", func(t *testing.T) {
		t.Parallel()

		// given: multiple targets that explicitly claim the same version file path
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:         config.TargetTypePath,
				Path:         "services/api",
				TagPrefix:    "api-v",
				VersionFiles: []config.VersionFile{{Path: "VERSION"}},
				Changelog:    config.ChangelogConfig{File: "services/api/CHANGELOG.md"},
			},
			"web": {
				Type:         config.TargetTypePath,
				Path:         "apps/web",
				TagPrefix:    "web-v",
				VersionFiles: []config.VersionFile{{Path: "./VERSION"}},
				Changelog:    config.ChangelogConfig{File: "apps/web/CHANGELOG.md"},
			},
		}

		// when: validating the config
		err := cfg.Validate()

		// then: validation rejects the duplicate explicit ownership before release time
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: targets.web.version_files entry \"VERSION\" duplicates "+
				"targets.api.version_files entry",
			err.Error(),
		)
	})

	t.Run("overlapping bump types fails", func(t *testing.T) {
		t.Parallel()

		// given: config with a type appearing in both minor and patch
		cfg := config.Default()
		cfg.BumpTypes.Minor = []string{"feat"}
		cfg.BumpTypes.Patch = []string{"fix", "feat"}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: bump_types: type \"feat\" appears in both minor and patch", err.Error())
	})

	t.Run("empty string in bump types minor fails", func(t *testing.T) {
		t.Parallel()

		// given: config with an empty string in bump_types.minor
		cfg := config.Default()
		cfg.BumpTypes.Minor = []string{"feat", ""}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: bump_types.minor must not contain empty strings", err.Error())
	})

	t.Run("empty bump types lists are valid", func(t *testing.T) {
		t.Parallel()

		// given: config with empty bump type lists
		cfg := config.Default()
		cfg.BumpTypes.Minor = []string{}
		cfg.BumpTypes.Patch = []string{}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation passes
		testastic.NoError(t, err)
	})

	t.Run("blank repository host fails", func(t *testing.T) {
		t.Parallel()

		// given: github sub-section with host set to whitespace only
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Host: "   ", Owner: "o", Repo: "r"}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the blank host
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.github.host must not be blank", err.Error())
	})

	t.Run("blank repository owner fails", func(t *testing.T) {
		t.Parallel()

		// given: github sub-section with owner set to whitespace only
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Owner: "   "}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the blank owner
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.github.owner must not be blank", err.Error())
	})

	t.Run("blank repository repo fails", func(t *testing.T) {
		t.Parallel()

		// given: github sub-section with repo set to whitespace only
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Repo: "   "}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the blank repo
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.github.repo must not be blank", err.Error())
	})

	t.Run("blank repository project fails", func(t *testing.T) {
		t.Parallel()

		// given: gitlab sub-section with project set to whitespace only
		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.GitLab = &config.GitLabRepositoryConfig{Project: "   "}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the blank project
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.gitlab.project must not be blank", err.Error())
	})

	t.Run("github owner with slash fails", func(t *testing.T) {
		t.Parallel()

		// given: github provider with subgroup-style owner
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Owner: "group/sub", Repo: "service"}

		// when: validating
		err := cfg.Validate()

		// then: github does not allow nested owner paths
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.github.owner must not contain '/'", err.Error())
	})

	t.Run("repository project must match owner and repo when project is owner-repo style", func(t *testing.T) {
		t.Parallel()

		// given: github sub-section where project does not equal owner/repo
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
			Owner:   "platform",
			Repo:    "yeet",
			Project: "platform/other",
		}

		// when: validating
		err := cfg.Validate()

		// then: mismatch is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: repository.github.project must match repository.github.owner/repo", err.Error())
	})

	t.Run("release channel name must not be empty", func(t *testing.T) {
		t.Parallel()

		// given: a channel with a whitespace-only name
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"   ": {Branch: "beta", Prerelease: "beta"},
		}

		// when: validating
		err := cfg.Validate()

		// then: empty channel name is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: release.channels keys must not be empty", err.Error())
	})

	t.Run("release channel name stable is reserved", func(t *testing.T) {
		t.Parallel()

		// given: a channel named "stable" (case-insensitive)
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"Stable": {Branch: "release", Prerelease: "rc"},
		}

		// when: validating
		err := cfg.Validate()

		// then: reserved name is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: release.channels.Stable must not use reserved name stable", err.Error())
	})

	t.Run("release channel branch must not be empty", func(t *testing.T) {
		t.Parallel()

		// given: a channel with an empty branch
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "   ", Prerelease: "beta"},
		}

		// when: validating
		err := cfg.Validate()

		// then: empty branch is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: release.channels.beta.branch must not be empty", err.Error())
	})

	t.Run("release channels with duplicate branches fails", func(t *testing.T) {
		t.Parallel()

		// given: two channels pointing at the same branch
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"alpha": {Branch: "release", Prerelease: "alpha"},
			"beta":  {Branch: "release", Prerelease: "beta"},
		}

		// when: validating
		err := cfg.Validate()

		// then: duplicate branch is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: release.channels.beta.branch \"release\" duplicates "+
				"release.channels.alpha.branch",
			err.Error(),
		)
	})

	t.Run("release channel prerelease must not be empty", func(t *testing.T) {
		t.Parallel()

		// given: a channel with an empty prerelease identifier
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "   "},
		}

		// when: validating
		err := cfg.Validate()

		// then: empty prerelease is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: release.channels.beta.prerelease must not be empty", err.Error())
	})

	t.Run("release channel invalid prerelease identifier fails", func(t *testing.T) {
		t.Parallel()

		// given: a channel with a prerelease identifier semver rejects
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "not valid!"},
		}

		// when: validating
		err := cfg.Validate()

		// then: invalid identifier is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: release.channels.beta.prerelease: invalid semver prerelease identifier "+
				"\"not valid!\": invalid prerelease string",
			err.Error(),
		)
	})

	t.Run("release channels with duplicate prerelease identifiers fails", func(t *testing.T) {
		t.Parallel()

		// given: two channels with the same prerelease identifier
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"alpha": {Branch: "alpha", Prerelease: "rc"},
			"beta":  {Branch: "beta", Prerelease: "rc"},
		}

		// when: validating
		err := cfg.Validate()

		// then: duplicate prerelease identifier is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: release.channels.beta.prerelease \"rc\" duplicates "+
				"release.channels.alpha.prerelease",
			err.Error(),
		)
	})

	t.Run("release channel branch must not duplicate stable branch", func(t *testing.T) {
		t.Parallel()

		// given: a channel branch that collides with the stable branch
		cfg := config.Default()
		cfg.Branch = "main"
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "main", Prerelease: "beta"},
		}

		// when: validating
		err := cfg.Validate()

		// then: stable-branch collision is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: release.channels.beta.branch \"main\" duplicates stable branch", err.Error())
	})

	t.Run("duplicate target tag prefix fails", func(t *testing.T) {
		t.Parallel()

		// given: two targets sharing a tag prefix
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "v",
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "v",
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: shared tag prefix is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.web.tag_prefix \"v\" duplicates targets.api.tag_prefix", err.Error())
	})

	t.Run("derived target with unknown include fails", func(t *testing.T) {
		t.Parallel()

		// given: a derived target referencing a non-existent target
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"root": {
				Type:      config.TargetTypeDerived,
				Path:      ".",
				TagPrefix: "v",
				Includes:  []string{"missing"},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: unknown include is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: targets.root.includes entry \"missing\" does not refer to a defined target",
			err.Error(),
		)
	})

	t.Run("derived target including derived target fails", func(t *testing.T) {
		t.Parallel()

		// given: a derived target whose include is itself derived
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"inner": {
				Type:      config.TargetTypeDerived,
				Path:      "services",
				TagPrefix: "inner-v",
				Includes:  []string{"leaf"},
			},
			"leaf": {
				Type:      config.TargetTypePath,
				Path:      "services/leaf",
				TagPrefix: "leaf-v",
			},
			"outer": {
				Type:      config.TargetTypeDerived,
				Path:      ".",
				TagPrefix: "v",
				Includes:  []string{"inner"},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: derived-of-derived include is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: targets.outer.includes entry \"inner\" must refer to a path target in v1",
			err.Error(),
		)
	})

	t.Run("target with empty type fails", func(t *testing.T) {
		t.Parallel()

		// given: a target without a type
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: missing target type is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.type must be \"path\" or \"derived\", got \"\"", err.Error())
	})

	t.Run("target with empty id fails", func(t *testing.T) {
		t.Parallel()

		// given: a target keyed under whitespace
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"   ": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: empty target ID is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: target IDs must be unique and non-empty", err.Error())
	})

	t.Run("path target with empty path fails", func(t *testing.T) {
		t.Parallel()

		// given: a path target with no path set
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: empty path is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.path must not be empty", err.Error())
	})

	t.Run("path target with includes fails", func(t *testing.T) {
		t.Parallel()

		// given: a path target that also lists includes
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
				Includes:  []string{"web"},
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: includes on a path target are rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.api.includes is only valid for derived targets", err.Error())
	})

	t.Run("derived target with no includes fails", func(t *testing.T) {
		t.Parallel()

		// given: a derived target without includes
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"root": {
				Type:      config.TargetTypeDerived,
				Path:      ".",
				TagPrefix: "v",
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: empty includes is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.root.includes must not be empty", err.Error())
	})

	t.Run("target with empty version_files entry fails", func(t *testing.T) {
		t.Parallel()

		// given: a target whose version_files contains an empty path
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:         config.TargetTypePath,
				Path:         ".",
				TagPrefix:    "v",
				VersionFiles: []config.VersionFile{{Path: "  "}},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: empty version_files entry is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.version_files must not contain empty paths", err.Error())
	})

	t.Run("target with empty changelog file fails", func(t *testing.T) {
		t.Parallel()

		// given: a target overriding changelog file to empty
		cfg := config.Default()
		cfg.Changelog.File = ""
		cfg.Changelog.Include = nil
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: the target that inherited the empty changelog file is named
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.changelog.file must not be empty", err.Error())
	})

	t.Run("target inherits empty top-level changelog include is rejected", func(t *testing.T) {
		t.Parallel()

		// given: a target inheriting an empty top-level changelog include list
		cfg := config.Default()
		cfg.Changelog.Include = nil
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation rejects the empty include
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.changelog.include must not be empty", err.Error())
	})

	t.Run("target with relative parent path fails", func(t *testing.T) {
		t.Parallel()

		// given: a target whose path escapes the repo
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: "../outside", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: parent paths are rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.path must be repo-relative", err.Error())
	})

	t.Run("target with absolute slash path fails", func(t *testing.T) {
		t.Parallel()

		// given: a target with an absolute Unix-style path
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: "/etc/yeet", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: absolute path is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.path must be repo-relative", err.Error())
	})

	t.Run("target with empty path string fails", func(t *testing.T) {
		t.Parallel()

		// given: a target whose path is whitespace only
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: "   ", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: blank path is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.path must not be empty", err.Error())
	})

	t.Run("exclude path outside target path fails", func(t *testing.T) {
		t.Parallel()

		// given: a non-root target with an exclude path that is not inside it
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:         config.TargetTypePath,
				Path:         "services/api",
				TagPrefix:    "api-v",
				ExcludePaths: []string{"services/web"},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: excludes outside the target are rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: targets.api.exclude_paths entry \"services/web\" must be inside "+
				"\"services/api\"",
			err.Error(),
		)
	})

	t.Run("exclude path with parent traversal fails", func(t *testing.T) {
		t.Parallel()

		// given: an exclude path that escapes the repo via ".."
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:         config.TargetTypePath,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"../outside"},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: parent traversal is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.exclude_paths contains must be repo-relative", err.Error())
	})

	t.Run("exclude path empty entry fails", func(t *testing.T) {
		t.Parallel()

		// given: an exclude path entry that normalizes to empty
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:         config.TargetTypePath,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"   "},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: empty exclude entry is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: targets.app.exclude_paths contains must not be empty", err.Error())
	})
}

func TestBumpTypesConfig_ToBumpMapping(t *testing.T) {
	t.Parallel()

	t.Run("default config produces default mapping", func(t *testing.T) {
		t.Parallel()

		// given: default bump types config
		bt := config.BumpTypesConfig{
			Minor: []string{"feat"},
			Patch: []string{"fix", "perf"},
		}

		// when: converting to bump mapping
		mapping := bt.ToBumpMapping()

		// then: matches expected mapping
		testastic.Equal(t, 3, len(mapping))
		testastic.Equal(t, "minor", mapping["feat"])
		testastic.Equal(t, "patch", mapping["fix"])
		testastic.Equal(t, "patch", mapping["perf"])
	})

	t.Run("custom config produces custom mapping", func(t *testing.T) {
		t.Parallel()

		// given: custom bump types config
		bt := config.BumpTypesConfig{
			Minor: []string{"feat", "revert"},
			Patch: []string{"docs"},
		}

		// when: converting to bump mapping
		mapping := bt.ToBumpMapping()

		// then: matches expected mapping
		testastic.Equal(t, 3, len(mapping))
		testastic.Equal(t, "minor", mapping["feat"])
		testastic.Equal(t, "minor", mapping["revert"])
		testastic.Equal(t, "patch", mapping["docs"])
	})

	t.Run("empty config produces empty mapping", func(t *testing.T) {
		t.Parallel()

		// given: empty bump types config
		bt := config.BumpTypesConfig{}

		// when: converting to bump mapping
		mapping := bt.ToBumpMapping()

		// then: empty mapping
		testastic.Equal(t, 0, len(mapping))
	})
}

func TestResolvedTargets(t *testing.T) {
	t.Parallel()

	t.Run("resolves monorepo targets with inherited defaults", func(t *testing.T) {
		t.Parallel()

		// given: a monorepo config with path and derived targets
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"root": {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api"},
				Includes:     []string{"api"},
			},
		}

		// when: resolving targets
		resolvedTargets, err := cfg.ResolvedTargets(t.Context())

		// then: target defaults and ownership data are expanded correctly
		testastic.NoError(t, err)
		testastic.Equal(t, "api-v", resolvedTargets["api"].TagPrefix)
		testastic.Equal(t, config.VersioningSemver, resolvedTargets["api"].Versioning)
		testastic.Equal(t, "CHANGELOG.md", resolvedTargets["root"].Changelog.File)
		testastic.SliceEqual(t, []string{"api"}, resolvedTargets["root"].Includes)
	})

	t.Run("normalizes target ids for lookup and includes", func(t *testing.T) {
		t.Parallel()

		// given: targets whose YAML IDs include surrounding whitespace
		cfg := config.Default()
		spacedAPIID := " api "
		spacedRootID := " root "
		cfg.Targets = map[string]config.Target{
			spacedAPIID: {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			spacedRootID: {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api"},
				Includes:     []string{spacedAPIID},
			},
		}

		// when: resolving targets
		resolvedTargets, err := cfg.ResolvedTargets(t.Context())

		// then: normalized IDs are used consistently
		testastic.NoError(t, err)
		testastic.Equal(t, "api", resolvedTargets["api"].ID)
		testastic.Equal(t, "root", resolvedTargets["root"].ID)
		testastic.SliceEqual(t, []string{"api"}, resolvedTargets["root"].Includes)
	})

	t.Run("rejects duplicate normalized target ids", func(t *testing.T) {
		t.Parallel()

		// given: two target IDs that normalize to the same value
		cfg := config.Default()
		spacedAPIID := " api "
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			spacedAPIID: {
				Type:      config.TargetTypePath,
				Path:      "services/api-alt",
				TagPrefix: "api-alt-v",
			},
		}

		// when: resolving targets
		_, err := cfg.ResolvedTargets(t.Context())

		// then: validation rejects the duplicate logical ID
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: target IDs must be unique and non-empty", err.Error())
	})

	t.Run("rejects overlapping direct ownership", func(t *testing.T) {
		t.Parallel()

		// given: two path targets that directly overlap
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"services": {
				Type:      config.TargetTypePath,
				Path:      "services",
				TagPrefix: "services-v",
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: ambiguous ownership is rejected
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"invalid config: direct path ownership overlaps between targets.api and targets.services",
			err.Error(),
		)
	})
}

func TestPreMajorOptions(t *testing.T) {
	t.Parallel()

	t.Run("target inherits top-level values", func(t *testing.T) {
		t.Parallel()

		// given: top-level options set to false, target does not override
		cfg := config.Default()
		cfg.PreMajorBreakingBumpsMinor = false
		cfg.PreMajorFeaturesBumpPatch = false
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target inherits top-level values
		testastic.NoError(t, err)
		testastic.False(t, resolved["app"].PreMajorBreakingBumpsMinor)
		testastic.False(t, resolved["app"].PreMajorFeaturesBumpPatch)
	})

	t.Run("target overrides top-level true with false", func(t *testing.T) {
		t.Parallel()

		// given: top-level defaults are true, target sets false
		cfg := config.Default()
		breakingFalse := false
		featuresFalse := false
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:                       config.TargetTypePath,
				Path:                       ".",
				TagPrefix:                  "v",
				PreMajorBreakingBumpsMinor: &breakingFalse,
				PreMajorFeaturesBumpPatch:  &featuresFalse,
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target overrides with false
		testastic.NoError(t, err)
		testastic.False(t, resolved["app"].PreMajorBreakingBumpsMinor)
		testastic.False(t, resolved["app"].PreMajorFeaturesBumpPatch)
	})

	t.Run("target overrides top-level false with true", func(t *testing.T) {
		t.Parallel()

		// given: top-level set to false, target overrides with true
		cfg := config.Default()
		cfg.PreMajorBreakingBumpsMinor = false
		cfg.PreMajorFeaturesBumpPatch = false
		breakingTrue := true
		featuresTrue := true
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:                       config.TargetTypePath,
				Path:                       ".",
				TagPrefix:                  "v",
				PreMajorBreakingBumpsMinor: &breakingTrue,
				PreMajorFeaturesBumpPatch:  &featuresTrue,
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target overrides with true
		testastic.NoError(t, err)
		testastic.True(t, resolved["app"].PreMajorBreakingBumpsMinor)
		testastic.True(t, resolved["app"].PreMajorFeaturesBumpPatch)
	})

	t.Run("target inherits top-level references and merges overrides", func(t *testing.T) {
		t.Parallel()

		// given: top-level references with patterns and footers, target adds a footer override
		cfg := config.Default()
		cfg.Changelog.References = config.ReferencesConfig{
			Patterns: []config.ReferencePattern{
				{Pattern: `JIRA-\d+`, URL: "https://jira.example.com/browse/{value}"},
			},
			Footers: map[string]string{
				"Refs": "https://jira.example.com/browse/{value}",
			},
		}
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				Changelog: config.ChangelogConfig{
					References: config.ReferencesConfig{
						Footers: map[string]string{
							"Closes": "",
						},
					},
				},
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target inherits top-level patterns (no override) and merges footers
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(resolved["app"].Changelog.References.Patterns))
		testastic.Equal(t, `JIRA-\d+`, resolved["app"].Changelog.References.Patterns[0].Pattern)
		testastic.Equal(t, 2, len(resolved["app"].Changelog.References.Footers))
		testastic.Equal(t, "https://jira.example.com/browse/{value}", resolved["app"].Changelog.References.Footers["Refs"])
		testastic.Equal(t, "", resolved["app"].Changelog.References.Footers["Closes"])
	})

	t.Run("rejects pre_major_breaking_bumps_minor on calver target", func(t *testing.T) {
		t.Parallel()

		// given: a calver target with explicit pre_major_breaking_bumps_minor
		cfg := config.Default()
		breakingTrue := true
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:                       config.TargetTypePath,
				Path:                       ".",
				TagPrefix:                  "v",
				Versioning:                 config.VersioningCalVer,
				PreMajorBreakingBumpsMinor: &breakingTrue,
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: error mentions the incompatibility
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"invalid config: targets.app.pre_major_breaking_bumps_minor has no effect with calver "+
				"versioning",
			err.Error(),
		)
	})

	t.Run("rejects pre_major_features_bump_patch on calver target", func(t *testing.T) {
		t.Parallel()

		// given: a calver target with explicit pre_major_features_bump_patch
		cfg := config.Default()
		featuresFalse := false
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:                      config.TargetTypePath,
				Path:                      ".",
				TagPrefix:                 "v",
				Versioning:                config.VersioningCalVer,
				PreMajorFeaturesBumpPatch: &featuresFalse,
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: error mentions the incompatibility
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"invalid config: targets.app.pre_major_features_bump_patch has no effect with calver "+
				"versioning",
			err.Error(),
		)
	})

	t.Run("allows calver target without pre_major overrides", func(t *testing.T) {
		t.Parallel()

		// given: a calver target that inherits pre_major options (does not set them)
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: no error, inherited options are silently ignored
		testastic.NoError(t, err)
	})

	t.Run("rejects invalid top-level calver format", func(t *testing.T) {
		t.Parallel()

		// given: a config with an unsupported calver format
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer
		cfg.CalVer.Format = "YYYY.QQ.MICRO"
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation fails before release planning
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: calver.format: invalid version: calver format only supports dots as "+
				"separators: \"YYYY.QQ.MICRO\"",
			err.Error(),
		)
	})

	t.Run("rejects invalid target calver format", func(t *testing.T) {
		t.Parallel()

		// given: a target with an unsupported calver format override
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				CalVer: config.CalVerConfig{
					Format: "YYYY.0M",
				},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation fails with the target path
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"invalid config: targets.app.calver.format: invalid version: calver format must include "+
				"MICRO: \"YYYY.0M\"",
			err.Error(),
		)
	})
}

func TestResolvedTargets_Merging(t *testing.T) {
	t.Parallel()

	t.Run("target overrides changelog file path", func(t *testing.T) {
		t.Parallel()

		// given: a target overriding the top-level changelog file path
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				Changelog: config.ChangelogConfig{
					File: " ./docs/../docs/RELEASES.md ",
				},
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target uses the override
		testastic.NoError(t, err)
		testastic.Equal(t, "docs/RELEASES.md", resolved["app"].Changelog.File)
	})

	t.Run("normalizes inherited version file paths", func(t *testing.T) {
		t.Parallel()

		cfg := config.Default()
		cfg.VersionFiles = []config.VersionFile{{Path: " ./config/../VERSION "}}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		resolved, err := cfg.ResolvedTargets(t.Context())

		testastic.NoError(t, err)
		testastic.Equal(t, "VERSION", resolved["app"].VersionFiles[0].Path)
	})

	t.Run("target overrides changelog include list", func(t *testing.T) {
		t.Parallel()

		// given: a target overriding the include list
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				Changelog: config.ChangelogConfig{
					Include: []string{"feat", "fix"},
				},
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: include is replaced (not merged)
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{"feat", "fix"}, resolved["app"].Changelog.Include)
	})

	t.Run("target merges changelog sections over defaults", func(t *testing.T) {
		t.Parallel()

		// given: a target overriding only one section label
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				Changelog: config.ChangelogConfig{
					Sections: map[string]string{"feat": "Shiny New Things"},
				},
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target keeps inherited sections and overrides only the specified one
		testastic.NoError(t, err)
		testastic.Equal(t, "Shiny New Things", resolved["app"].Changelog.Sections["feat"])
		testastic.Equal(t, "Bug Fixes", resolved["app"].Changelog.Sections["fix"])
	})

	t.Run("target overrides reference patterns", func(t *testing.T) {
		t.Parallel()

		// given: top-level patterns and a target with its own pattern list
		cfg := config.Default()
		cfg.Changelog.References = config.ReferencesConfig{
			Patterns: []config.ReferencePattern{{Pattern: `JIRA-\d+`, URL: "https://jira/{value}"}},
			Footers:  map[string]string{"Refs": "https://jira/{value}"},
		}
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "v",
				Changelog: config.ChangelogConfig{
					References: config.ReferencesConfig{
						Patterns: []config.ReferencePattern{{Pattern: `#\d+`, URL: ""}},
					},
				},
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target patterns replace the inherited list and footers stay inherited
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(resolved["app"].Changelog.References.Patterns))
		testastic.Equal(t, `#\d+`, resolved["app"].Changelog.References.Patterns[0].Pattern)
		testastic.Equal(t, "https://jira/{value}", resolved["app"].Changelog.References.Footers["Refs"])
	})

	t.Run("target overrides calver format", func(t *testing.T) {
		t.Parallel()

		// given: a calver target overriding the top-level format
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:       config.TargetTypePath,
				Path:       ".",
				TagPrefix:  "v",
				Versioning: config.VersioningCalVer,
				CalVer:     config.CalVerConfig{Format: "YYYY.MM.MICRO"},
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target keeps its own format
		testastic.NoError(t, err)
		testastic.Equal(t, "YYYY.MM.MICRO", resolved["app"].CalVer.Format)
	})

	t.Run("target overrides version_files entirely", func(t *testing.T) {
		t.Parallel()

		// given: top-level version files and a target overriding them
		cfg := config.Default()
		cfg.VersionFiles = []config.VersionFile{{Path: "VERSION"}}
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:         config.TargetTypePath,
				Path:         ".",
				TagPrefix:    "v",
				VersionFiles: []config.VersionFile{{Path: "package.json"}},
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target uses its own list (not merged)
		testastic.NoError(t, err)
		testastic.Equal(t, "package.json", resolved["app"].VersionFiles[0].Path)
	})

	t.Run("target inherits top-level versioning when unset", func(t *testing.T) {
		t.Parallel()

		// given: top-level set to calver, target leaves versioning unset
		cfg := config.Default()
		cfg.Versioning = config.VersioningCalVer
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: target inherits calver
		testastic.NoError(t, err)
		testastic.Equal(t, config.VersioningCalVer, resolved["app"].Versioning)
	})

	t.Run("target tag prefix is trimmed", func(t *testing.T) {
		t.Parallel()

		// given: a tag prefix with surrounding whitespace
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"app": {
				Type:      config.TargetTypePath,
				Path:      ".",
				TagPrefix: "  v  ",
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: tag prefix is normalized
		testastic.NoError(t, err)
		testastic.Equal(t, "v", resolved["app"].TagPrefix)
	})

	t.Run("normalizes nested target path slashes", func(t *testing.T) {
		t.Parallel()

		// given: a target path with redundant separators
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services//api/",
				TagPrefix: "api-v",
			},
		}

		// when: resolving targets
		resolved, err := cfg.ResolvedTargets(t.Context())

		// then: path is cleaned
		testastic.NoError(t, err)
		testastic.Equal(t, "services/api", resolved["api"].Path)
	})

	t.Run("derived target with non-overlapping include skips ownership conflict", func(t *testing.T) {
		t.Parallel()

		// given: a derived target whose included path is excluded
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"root": {
				Type:         config.TargetTypeDerived,
				Path:         ".",
				TagPrefix:    "v",
				ExcludePaths: []string{"services/api"},
				Includes:     []string{"api"},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: ownership is unambiguous
		testastic.NoError(t, err)
	})

	t.Run("derived target without exclude overlapping included path fails", func(t *testing.T) {
		t.Parallel()

		// given: a derived root target that does not exclude the included path
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"root": {
				Type:      config.TargetTypeDerived,
				Path:      ".",
				TagPrefix: "v",
				Includes:  []string{"api"},
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: overlap with the include is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: direct path ownership overlaps between targets.api and targets.root", err.Error())
	})

	t.Run("disjoint targets do not overlap", func(t *testing.T) {
		t.Parallel()

		// given: two targets in unrelated paths
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "services/api",
				TagPrefix: "api-v",
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "apps/web",
				TagPrefix: "web-v",
			},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation passes
		testastic.NoError(t, err)
	})
}

func TestRepoPathContains(t *testing.T) {
	t.Parallel()

	// given: a base path and candidates with various relationships
	cases := map[string]struct {
		base, candidate string
		want            bool
	}{
		"root contains everything":  {".", "anything", true},
		"identical paths":           {"services/api", "services/api", true},
		"prefix containment":        {"services", "services/api", true},
		"prefix substring no match": {"service", "services/api", false},
		"unrelated paths":           {"apps", "services/api", false},
		"candidate above base":      {"services/api", "services", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// when: checking containment
			got := config.RepoPathContains(tc.base, tc.candidate)

			// then: result matches expectation
			testastic.Equal(t, tc.want, got)
		})
	}
}

func TestReleaseReviewers(t *testing.T) {
	t.Parallel()

	t.Run("no reviewers by default", func(t *testing.T) {
		t.Parallel()

		// given: nothing

		// when: creating a default config
		cfg := config.Default()

		// then: no reviewers are configured
		testastic.Equal(t, 0, len(cfg.Release.Reviewers))
	})

	t.Run("blank reviewer entry fails", func(t *testing.T) {
		t.Parallel()

		// given: config with a whitespace-only reviewer
		cfg := config.Default()
		cfg.Release.Reviewers = []string{"alice", "   "}
		cfg.Targets = map[string]config.Target{
			"app": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		// when: validating
		err := cfg.Validate()

		// then: validation fails
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, "invalid config: release.reviewers must not contain empty strings", err.Error())
	})
}
