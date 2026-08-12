package release

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
)

// captureWarnings redirects the default logger for one test and returns what it
// wrote. The default logger is process wide, so callers must not run parallel.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()

	buffer := &bytes.Buffer{}
	previous := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(buffer, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return buffer
}

func releaseAsCommit(hash, value string) commit.Commit {
	return commit.Commit{
		Hash:    hash,
		Type:    "feat",
		Footers: []commit.Footer{{Key: "Release-As", Value: value}},
	}
}

func TestReleaseAsOverride(t *testing.T) {
	t.Run("reports every commit a calver target cannot honour", func(t *testing.T) {
		// given: a calver target and two commits asking for an explicit version
		logs := captureWarnings(t)

		target := config.ResolvedTarget{ID: "app", Versioning: config.VersioningCalVer, TagPrefix: "v"}
		commits := []commit.Commit{
			releaseAsCommit("abc1234", "2.0.0"),
			releaseAsCommit("def5678", "3.0.0"),
		}

		// when: the override is read for that target
		override, err := releaseAsOverride(
			t.Context(),
			versionStrategyForResolvedTarget(target).strategy,
			target,
			commits,
		)

		// then: no version is overridden and neither footer is dropped in silence
		testastic.NoError(t, err)
		testastic.Equal(t, "", override)
		testastic.Equal(t, 2, strings.Count(logs.String(), "ignoring Release-As footer"))
		testastic.Contains(t, logs.String(), "abc1234")
		testastic.Contains(t, logs.String(), "def5678")
		testastic.Contains(t, logs.String(), "calver")
	})

	t.Run("honours the footer on a semver target", func(t *testing.T) {
		// given: a semver target and a commit asking for an explicit version
		logs := captureWarnings(t)

		target := config.ResolvedTarget{ID: "app", Versioning: config.VersioningSemver, TagPrefix: "v"}

		// when: the override is read for that target
		override, err := releaseAsOverride(
			t.Context(),
			versionStrategyForResolvedTarget(target).strategy,
			target,
			[]commit.Commit{releaseAsCommit("abc1234", "2.0.0")},
		)

		// then: the requested version is returned and nothing is reported
		testastic.NoError(t, err)
		testastic.Equal(t, "2.0.0", override)
		testastic.Equal(t, "", logs.String())
	})
}
