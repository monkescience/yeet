//nolint:testpackage // This test validates unexported title rendering behavior.
package release

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestReleasePRTitleTemplates(t *testing.T) {
	t.Parallel()

	t.Run("renders every single-target field", func(t *testing.T) {
		t.Parallel()

		// given: a single-target title template and release plan
		cfg := config.Default()
		cfg.Branch = "beta"
		cfg.ActiveChannel = "beta"
		cfg.Release.PRTitle = " {{ .Branch }}|{{ .Channel }}|{{ .Target }}|{{ .Version }}|{{ .Tag }} "
		titles, err := newReleaseTitleTemplates(cfg.Release)
		testastic.NoError(t, err)

		core := &releaseCore{cfg: cfg, titles: titles}
		plans := []TargetPlan{{
			ID:          "api",
			NextVersion: "1.2.3-beta.1",
			NextTag:     "api-v1.2.3-beta.1",
		}}

		// when: rendering the PR title
		title, err := core.releasePRTitle(plans)

		// then: values are rendered as data and surrounding whitespace is trimmed
		testastic.NoError(t, err)
		testastic.Equal(t, "beta|beta|api|1.2.3-beta.1|api-v1.2.3-beta.1", title)
	})

	t.Run("renders every group field", func(t *testing.T) {
		t.Parallel()

		// given: a group title template and two plans
		cfg := config.Default()
		cfg.Release.PRTitleGroup = "{{ .Branch }}|{{ .Channel }}|{{ .TargetCount }}"
		titles, err := newReleaseTitleTemplates(cfg.Release)
		testastic.NoError(t, err)

		core := &releaseCore{cfg: cfg, titles: titles}
		plans := []TargetPlan{{ID: "api"}, {ID: "web"}}

		// when: rendering the PR title
		title, err := core.releasePRTitle(plans)

		// then: group data excludes target-specific values
		testastic.NoError(t, err)
		testastic.Equal(t, "main||2", title)
	})

	t.Run("rejects a single-target field in a group template", func(t *testing.T) {
		t.Parallel()

		// given: a group template that references unavailable data
		cfg := config.Default()
		cfg.Release.PRTitleGroup = "release {{ .Version }}"

		// when: preparing templates
		_, err := newReleaseTitleTemplates(cfg.Release)

		// then: configuration validation fails before release mutation
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("rejects invalid template syntax", func(t *testing.T) {
		t.Parallel()

		// given: malformed template syntax
		cfg := config.Default()
		cfg.Release.PRTitle = "release {{"

		// when: preparing templates
		_, err := newReleaseTitleTemplates(cfg.Release)

		// then: configuration validation fails
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	t.Run("accepts a branch restricted template", func(t *testing.T) {
		t.Parallel()

		// given: a template that only renders a title for the actual release branch
		cfg := config.Default()
		cfg.Release.PRTitle = `{{ if eq .Branch "main" }}release from main{{ end }}`

		// when: preparing and rendering the template with matching release data
		titles, err := newReleaseTitleTemplates(cfg.Release)
		testastic.NoError(t, err)

		core := &releaseCore{cfg: cfg, titles: titles}
		title, err := core.releasePRTitle([]TargetPlan{{ID: "root", NextVersion: "1.0.0", NextTag: "v1.0.0"}})

		// then: validation uses the actual branch instead of synthetic data
		testastic.NoError(t, err)
		testastic.Equal(t, "release from main", title)
	})

	t.Run("rejects an unavailable root variable field in a group template", func(t *testing.T) {
		t.Parallel()

		// given: a group template that hides an unavailable field behind an inactive branch
		cfg := config.Default()
		cfg.Release.PRTitleGroup = `{{ if false }}{{ $.Version }}{{ end }}release`

		// when: preparing templates
		_, err := newReleaseTitleTemplates(cfg.Release)

		// then: root variable fields receive the same static validation as direct fields
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})

	for _, template := range []string{"{{ if false }}{{ .Unknown }}{{ end }}release", "   ", "one\ntwo"} {
		t.Run("rejects invalid rendered title "+template, func(t *testing.T) {
			t.Parallel()

			// given: an invalid custom title template
			cfg := config.Default()
			cfg.Release.PRTitle = template

			// when: preparing or rendering the template
			titles, err := newReleaseTitleTemplates(cfg.Release)
			if err == nil {
				core := &releaseCore{cfg: cfg, titles: titles}
				_, err = core.releasePRTitle([]TargetPlan{{ID: "root", NextVersion: "1.0.0", NextTag: "v1.0.0"}})
			}

			// then: invalid fields, empty results, and multiline results are rejected
			testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		})
	}
}

func TestReleaseCommitSubjectTemplates(t *testing.T) {
	t.Parallel()

	t.Run("renders independently from single-target PR title", func(t *testing.T) {
		t.Parallel()

		// given: distinct PR title and commit subject templates
		cfg := config.Default()
		cfg.Release.PRTitle = "PR {{ .Tag }}"
		cfg.Release.CommitSubject = "commit {{ .Branch }} {{ .Target }} {{ .Version }}"
		titles, err := newReleaseTitleTemplates(cfg.Release)
		testastic.NoError(t, err)

		core := &releaseCore{cfg: cfg, titles: titles}
		plans := []TargetPlan{{
			ID:          "api",
			NextVersion: "1.2.3",
			NextTag:     "api-v1.2.3",
		}}

		// when: rendering both values
		prTitle, err := core.releasePRTitle(plans)
		testastic.NoError(t, err)
		commitSubject, err := core.releaseCommitSubject(plans)

		// then: each output uses its own template
		testastic.NoError(t, err)
		testastic.Equal(t, "PR api-v1.2.3", prTitle)
		testastic.Equal(t, "commit main api 1.2.3", commitSubject)
	})

	t.Run("renders group data", func(t *testing.T) {
		t.Parallel()

		// given: a group commit subject template
		cfg := config.Default()
		cfg.Branch = "beta"
		cfg.ActiveChannel = "rc"
		cfg.Release.CommitSubjectGroup = "commit {{ .Branch }} {{ .Channel }} {{ .TargetCount }}"
		titles, err := newReleaseTitleTemplates(cfg.Release)
		testastic.NoError(t, err)

		core := &releaseCore{cfg: cfg, titles: titles}
		plans := []TargetPlan{{ID: "api"}, {ID: "web"}}

		// when: rendering the commit subject
		subject, err := core.releaseCommitSubject(plans)

		// then: group values are available
		testastic.NoError(t, err)
		testastic.Equal(t, "commit beta rc 2", subject)
	})

	t.Run("rejects a single-target field in a group commit template", func(t *testing.T) {
		t.Parallel()

		// given: a group commit template that references unavailable data
		cfg := config.Default()
		cfg.Release.CommitSubjectGroup = "commit {{ .Version }}"

		// when: preparing templates
		_, err := newReleaseTitleTemplates(cfg.Release)

		// then: configuration validation fails before release mutation
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})
}

func TestReleaseNameTemplate(t *testing.T) {
	t.Parallel()

	t.Run("uses tag by default", func(t *testing.T) {
		t.Parallel()

		// given: the default release name template and a single-target plan
		cfg := config.Default()
		titles, err := newReleaseTitleTemplates(cfg.Release)
		testastic.NoError(t, err)

		core := &releaseCore{cfg: cfg, titles: titles}

		// when: rendering the release name
		name, err := core.releaseNameForPlan(TargetPlan{
			ID: "api", NextVersion: "1.2.3", NextTag: "api-v1.2.3",
		})

		// then: the plan tag becomes the release name
		testastic.NoError(t, err)
		testastic.Equal(t, "api-v1.2.3", name)
	})

	t.Run("renders every allowlisted field", func(t *testing.T) {
		t.Parallel()

		// given: a release name template containing every allowed field
		cfg := config.Default()
		cfg.Branch = "beta"
		cfg.ActiveChannel = "beta"
		cfg.Release.NameTemplate = "{{ .Branch }}|{{ .Channel }}|{{ .Target }}|{{ .Version }}|{{ .Tag }}"
		titles, err := newReleaseTitleTemplates(cfg.Release)
		testastic.NoError(t, err)

		core := &releaseCore{cfg: cfg, titles: titles}

		// when: rendering the name for an active channel plan
		name, err := core.releaseNameForPlan(TargetPlan{
			ID: "api", NextVersion: "1.2.3-beta.1", NextTag: "api-v1.2.3-beta.1",
		})

		// then: every field uses its release context value
		testastic.NoError(t, err)
		testastic.Equal(t, "beta|beta|api|1.2.3-beta.1|api-v1.2.3-beta.1", name)
	})

	for _, source := range []string{"{{ .TargetCount }}", " ", "one\ntwo"} {
		t.Run("rejects invalid release name "+source, func(t *testing.T) {
			t.Parallel()

			// given: a release name template that is invalid before or after rendering
			cfg := config.Default()
			cfg.Release.NameTemplate = source

			// when: preparing or rendering the release name
			titles, err := newReleaseTitleTemplates(cfg.Release)
			if err == nil {
				core := &releaseCore{cfg: cfg, titles: titles}
				_, err = core.releaseNameForPlan(TargetPlan{
					ID: "api", NextVersion: "1.2.3", NextTag: "api-v1.2.3",
				})
			}

			// then: the invalid release name is rejected
			testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		})
	}
}
