//nolint:testpackage // This test validates unexported branch template behavior.
package release

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestReleaseBranchTemplate(t *testing.T) {
	t.Parallel()

	t.Run("renders the compatibility default", func(t *testing.T) {
		t.Parallel()

		// given: the default release configuration
		cfg := config.Default()

		// when: resolving the release branch
		run, err := resolveRun(cfg, cfg.Branch, Options{})

		// then: the existing branch convention is preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "yeet/release-main", run.releaseBranch)
	})

	t.Run("renders branch and channel fields", func(t *testing.T) {
		t.Parallel()

		// given: an active beta channel and a custom branch template
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "release/beta", Prerelease: "beta"},
		}
		cfg.Release.BranchTemplate = "automation/{{ .Channel }}/{{ .Branch }}"

		// when: resolving the release branch
		run, err := resolveRun(cfg, "release/beta", Options{})

		// then: both allowlisted fields are rendered
		testastic.NoError(t, err)
		testastic.Equal(t, "automation/beta/release/beta", run.releaseBranch)
	})
}

func TestReleaseBranchTemplateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
	}{
		{name: "invalid syntax", template: "release/{{"},
		{name: "unavailable field", template: "release/{{ .Target }}"},
		{name: "blank output", template: "{{ if .Channel }}release/{{ .Channel }}{{ end }}"},
		{name: "multiline output", template: "release/{{ .Branch }}\nother"},
		{name: "invalid git branch", template: "release/{{ .Branch }}..next"},
		{name: "base branch", template: "{{ .Branch }}"},
		{name: "symbolic head", template: "HEAD"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a release branch template with an invalid result or field
			cfg := config.Default()
			cfg.Release.BranchTemplate = test.template

			// when: validating every configured release mode
			err := validateReleaseBranchTemplates(cfg)

			// then: the invalid template is rejected before provider activity
			testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		})
	}

	t.Run("rejects duplicate stable and channel release branches", func(t *testing.T) {
		t.Parallel()

		// given: a template that ignores branch and channel identity
		cfg := config.Default()
		cfg.Release.BranchTemplate = "automation/release"
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: validating all configured release modes
		err := validateReleaseBranchTemplates(cfg)

		// then: two modes cannot claim the same release branch
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.ErrorContains(t, err, "duplicates branch")
	})
}
