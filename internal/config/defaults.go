package config

import (
	"time"

	"github.com/monkescience/yeet/internal/version"
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
		"breaking":       "⚠ BREAKING CHANGES",
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
			RequestTimeout: 30 * time.Second,
			Retry: NetworkRetryConfig{
				MaxAttempts: 4,
				MinBackoff:  1 * time.Second,
				MaxBackoff:  10 * time.Second,
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
				InitialInterval: 250 * time.Millisecond,
				MaxInterval:     5 * time.Second,
				Timeout:         2 * time.Minute,
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
