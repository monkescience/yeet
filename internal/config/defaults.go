package config

import (
	"time"

	"github.com/monkescience/yeet/internal/version"
)

const (
	defaultBreakingChangesHeading      = "⚠ BREAKING CHANGES"
	defaultNetworkRequestTimeout       = 30 * time.Second
	defaultNetworkRetryMaxAttempts     = 4
	defaultNetworkRetryMinBackoff      = time.Second
	defaultNetworkRetryMaxBackoff      = 10 * time.Second
	defaultMergePollingInitialInterval = 250 * time.Millisecond
	defaultMergePollingMaxInterval     = 5 * time.Second
	defaultMergePollingTimeout         = 2 * time.Minute
)

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
		"breaking":       defaultBreakingChangesHeading,
	}
}

func Default() *Config {
	return &Config{
		Versioning:                 VersioningSemver,
		Branch:                     "main",
		Timezone:                   "Local",
		Provider:                   ProviderAuto,
		PreMajorBreakingBumpsMinor: true,
		PreMajorFeaturesBumpPatch:  true,
		BumpTypes:                  defaultBumpTypes(),
		Repository: RepositoryConfig{
			Remote: "origin",
		},
		Network: NetworkConfig{
			RequestTimeout: defaultNetworkRequestTimeout,
			Retry: NetworkRetryConfig{
				MaxAttempts: defaultNetworkRetryMaxAttempts,
				MinBackoff:  defaultNetworkRetryMinBackoff,
				MaxBackoff:  defaultNetworkRetryMaxBackoff,
			},
		},
		Release: ReleaseConfig{
			Labels: ReleaseLabelsConfig{
				Pending: "autorelease: pending",
				Tagged:  "autorelease: tagged",
				Yeet:    true,
			},
			BranchTemplate: "yeet/release-{{ .Branch }}",
			MergePolling: ReleaseMergePollingConfig{
				InitialInterval: defaultMergePollingInitialInterval,
				MaxInterval:     defaultMergePollingMaxInterval,
				Timeout:         defaultMergePollingTimeout,
			},
			AutoMerge:       false,
			AutoMergeForce:  false,
			AutoMergeMethod: AutoMergeMethodAuto,
			PRBodyHeader:    "## ٩(^ᴗ^)۶ release created",
			PRBodyFooter: "_Auto-generated preview, edit `CHANGELOG.md` to customize release notes._\n\n" +
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
