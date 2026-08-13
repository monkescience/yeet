package config

import "github.com/monkescience/yeet/internal/version"

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
			Labels: ReleaseLabelsConfig{
				Pending: "autorelease: pending",
				Tagged:  "autorelease: tagged",
				Yeet:    true,
			},
			BranchTemplate:  "yeet/release-{{ .Branch }}",
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
