package version_test

import (
	"slices"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/version"
)

func TestSemVerOrderedRefs(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name, channel string
		want          []string
	}{
		{
			name: "stable releases and lexical ties",
			want: []string{"v3.0.0", "v1.10.0", "v1.9.0", "v1.2.3+zzz", "v1.2.3+aaa", "v1.2.3", "1.2.3"},
		},
		{
			name: "beta releases and stable boundaries", channel: "beta",
			want: []string{
				"v3.0.0", "v2.0.0-beta.10", "v2.0.0-beta.2", "v2.0.0-beta",
				"v1.10.0", "v1.9.0", "v1.2.3+zzz", "v1.2.3+aaa", "v1.2.3", "1.2.3",
			},
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: mixed channels, equal versions, invalid refs and duplicate tags
			strategy := version.NewSemVer(version.SemVerOptions{Prefix: "v"})
			refs := []string{
				" v1.2.3 ", "v1.2.3+aaa", "v1.2.3+zzz", "1.2.3", "v1.10.0", "v1.9.0",
				"v2.0.0-beta.2", "v2.0.0-beta.10", "v2.0.0-beta", "v9.0.0-alpha.1",
				"v2.0.0-betaish.1", "v3.0.0", "v1.2.3", "v1.2", "other/1.2.3", "", " ",
			}
			original := slices.Clone(refs)

			// when: ordering history for the active channel
			ordered := strategy.OrderedRefs(refs, scenario.channel)

			// then: eligible refs descend by version and original ref without modifying the input
			testastic.SliceEqual(t, scenario.want, ordered)
			testastic.SliceEqual(t, original, refs)
		})
	}
}

func TestCalVerOrderedRefs(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name, format string
		refs, want   []string
	}{
		{
			name: "normalizes versions and breaks ties by original ref",
			refs: []string{
				" v2026.2.9 ", "v2026.02.9", "2026.02.9", "v2026.10.1", "v2026.2.10",
				"v2026.2.9", "v2026.13.1", "other/2026.02.1", "v2026.02.-1", "", " ",
			},
			want: []string{"v2026.10.1", "v2026.2.10", "v2026.2.9", "v2026.02.9", "2026.02.9"},
		},
		{
			name: "orders calendar periods across years",
			refs: []string{"v2026.12.9", "v2027.01.1", "v2026.01.5", "v2026.02.1"},
			want: []string{"v2027.01.1", "v2026.12.9", "v2026.02.1", "v2026.01.5"},
		},
		{
			name: "orders short years and unpadded months numerically", format: "YY.MM.MICRO",
			refs: []string{"v26.2.9", "v26.10.1", "v27.1.1", "v25.12.9"},
			want: []string{"v27.1.1", "v26.10.1", "v26.2.9", "v25.12.9"},
		},
		{
			name: "honors configured token order", format: "0M.YYYY.MICRO",
			refs: []string{"v01.2027.1", "v12.2026.1", "v02.2026.10", "v02.2026.2"},
			want: []string{"v12.2026.1", "v02.2026.10", "v02.2026.2", "v01.2027.1"},
		},
		{
			name: "filters impossible dates", format: "YYYY.0M.0D.MICRO",
			refs: []string{"v2026.02.28.1", "v2026.02.30.1", "v2026.03.01.1", "v2026.02.27.5"},
			want: []string{"v2026.03.01.1", "v2026.02.28.1", "v2026.02.27.5"},
		},
		{
			name: "orders weeks numerically", format: "YY.WW.MICRO",
			refs: []string{"v26.5.1", "v26.10.1", "v26.54.1", "v26.0.1"},
			want: []string{"v26.10.1", "v26.5.1"},
		},
		{
			name: "filters all invalid refs",
			refs: []string{"bad", "v2026.13.1"}, want: []string{},
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a compiled calendar format and mixed version history
			strategy := newCalVer(t, scenario.format, "v", nil)
			original := slices.Clone(scenario.refs)

			// when: ordering refs with the existing CalVer channel policy
			ordered := strategy.OrderedRefs(scenario.refs, "beta")

			// then: the configured format controls ordering and invalid refs are filtered
			testastic.SliceEqual(t, scenario.want, ordered)
			testastic.SliceEqual(t, original, scenario.refs)
		})
	}
}

func TestStrategyTag(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{"", "v", "app/"} {
		t.Run("prefix "+prefix, func(t *testing.T) {
			t.Parallel()

			// given: both schemes configured with the same tag prefix
			semver := version.NewSemVer(version.SemVerOptions{Prefix: prefix})
			calver := newCalVer(t, "", prefix, nil)

			// when: formatting a tag for each scheme
			semverTag := semver.Tag("1.2.3")
			calverTag := calver.Tag("2026.02.1")

			// then: each strategy owns its configured prefix
			testastic.Equal(t, prefix+"1.2.3", semverTag)
			testastic.Equal(t, prefix+"2026.02.1", calverTag)
		})
	}
}
