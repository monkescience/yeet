//nolint:testpackage // This test validates unexported release analyzer functions.
package release

import (
	"context"
	"fmt"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

func TestOrderedPlans(t *testing.T) {
	t.Parallel()

	t.Run("empty map yields empty slice", func(t *testing.T) {
		t.Parallel()

		// given: no plans
		plans := map[string]TargetPlan{}

		// when: ordering
		ordered := orderedPlans(plans)

		// then: the slice is empty
		testastic.Equal(t, 0, len(ordered))
	})

	t.Run("single entry is returned as is", func(t *testing.T) {
		t.Parallel()

		// given: one plan
		plans := map[string]TargetPlan{
			"only": {ID: "only", Type: "service"},
		}

		// when: ordering
		ordered := orderedPlans(plans)

		// then: that plan is the sole element
		testastic.Equal(t, 1, len(ordered))
		testastic.Equal(t, "only", ordered[0].ID)
	})

	t.Run("sorts by type before id", func(t *testing.T) {
		t.Parallel()

		// given: plans with mixed types and ids
		plans := map[string]TargetPlan{
			"svc-b": {ID: "svc-b", Type: "service"},
			"lib-a": {ID: "lib-a", Type: "library"},
			"svc-a": {ID: "svc-a", Type: "service"},
			"lib-b": {ID: "lib-b", Type: "library"},
		}

		// when: ordering
		ordered := orderedPlans(plans)

		// then: type is the primary key, id breaks ties
		testastic.Equal(t, 4, len(ordered))
		testastic.Equal(t, "lib-a", ordered[0].ID)
		testastic.Equal(t, "lib-b", ordered[1].ID)
		testastic.Equal(t, "svc-a", ordered[2].ID)
		testastic.Equal(t, "svc-b", ordered[3].ID)
	})

	t.Run("same type sorts by id lexicographically", func(t *testing.T) {
		t.Parallel()

		// given: plans sharing a type
		plans := map[string]TargetPlan{
			"gamma": {ID: "gamma", Type: "service"},
			"alpha": {ID: "alpha", Type: "service"},
			"beta":  {ID: "beta", Type: "service"},
		}

		// when: ordering
		ordered := orderedPlans(plans)

		// then: ids appear in ascending order
		testastic.Equal(t, "alpha", ordered[0].ID)
		testastic.Equal(t, "beta", ordered[1].ID)
		testastic.Equal(t, "gamma", ordered[2].ID)
	})
}

func TestReleaseAnalyzerSharedMonorepoHistoryIndex(t *testing.T) {
	t.Parallel()

	// given: many path targets that share the same branch history but have distinct release tags
	cfg := config.Default()
	cfg.Targets = make(map[string]config.Target)

	const targetCount = 100

	stub := newProviderStub()
	stub.commits = make([]provider.CommitEntry, 0, targetCount)

	for idx := range targetCount {
		targetID := fmt.Sprintf("service-%03d", idx)
		tagPrefix := fmt.Sprintf("service-%03d-v", idx)
		servicePath := fmt.Sprintf("services/%03d", idx)

		cfg.Targets[targetID] = config.Target{
			Type:      config.TargetTypePath,
			Path:      servicePath,
			TagPrefix: tagPrefix,
			Changelog: config.ChangelogConfig{File: servicePath + "/CHANGELOG.md"},
		}

		stub.tagList = append(stub.tagList, tagPrefix+"1.0.0")
		stub.commits = append(stub.commits, provider.CommitEntry{
			Hash:    fmt.Sprintf("%040d", idx),
			Message: "fix: patch service",
			Paths:   []string{servicePath + "/main.go"},
		})
	}

	r := newTestReleaser(t, cfg, stub)

	// when: analyzing the monorepo release wave
	result, err := r.Release(context.Background(), true)

	// then: tags and branch history are fetched once for the whole release run
	testastic.NoError(t, err)
	testastic.Equal(t, targetCount, len(result.Plans))
	testastic.Equal(t, 1, stub.getLatestVersionRefCalls)
	testastic.Equal(t, 1, stub.listTagsCalls)
	testastic.Equal(t, 1, stub.getCommitsSinceRefsCalls)
	testastic.SliceEqual(t, []bool{true}, stub.getCommitsSinceIncludePath)
	testastic.Equal(t, 0, stub.commitOverrideCalls)
}

func TestReleaseAnalyzerSharedHistoryUsesPerTargetBoundaries(t *testing.T) {
	t.Parallel()

	// given: path targets whose latest tags sit at different branch boundaries
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

	stub := newProviderStub()
	stub.tagList = []string{"web-v2.0.0", "api-v1.0.0"}
	stub.commitsByRef = map[string][]provider.CommitEntry{
		"api-v1.0.0": {
			{Hash: "api-new", Message: "fix: patch api", Paths: []string{"services/api/main.go"}},
			{Hash: "web-new", Message: "feat: refresh web", Paths: []string{"apps/web/app.tsx"}},
		},
		"web-v2.0.0": {
			{Hash: "web-new", Message: "feat: refresh web", Paths: []string{"apps/web/app.tsx"}},
		},
	}

	r := newTestReleaser(t, cfg, stub)

	// when: analyzing the shared branch history
	result, err := r.Release(context.Background(), true)

	// then: each target plans from its own selected boundary while sharing one provider scan
	testastic.NoError(t, err)
	testastic.Equal(t, 2, len(result.Plans))
	testastic.Equal(t, "api", result.Plans[0].ID)
	testastic.Equal(t, "1.0.0", result.Plans[0].CurrentVersion)
	testastic.Equal(t, "api-v1.0.1", result.Plans[0].NextTag)
	testastic.Equal(t, "web", result.Plans[1].ID)
	testastic.Equal(t, "2.0.0", result.Plans[1].CurrentVersion)
	testastic.Equal(t, "web-v2.1.0", result.Plans[1].NextTag)
	testastic.Equal(t, 1, stub.getCommitsSinceRefsCalls)
}

func TestReleaseAnalyzerSharedHistoryExcludesUnboundedTargets(t *testing.T) {
	t.Parallel()

	// given: one target with a tag and one brand-new target with no tags
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

	stub := newProviderStub()
	stub.tagList = []string{"api-v1.0.0"}
	stub.commitsByRef = map[string][]provider.CommitEntry{
		"api-v1.0.0": {
			{Hash: "api-new", Message: "fix: patch api", Paths: []string{"services/api/main.go"}},
		},
		"": {
			{Hash: "api-new", Message: "fix: patch api", Paths: []string{"services/api/main.go"}},
			{Hash: "web-new", Message: "feat: refresh web", Paths: []string{"apps/web/app.tsx"}},
		},
	}

	r := newTestReleaser(t, cfg, stub)

	// when: analyzing the shared branch history
	result, err := r.Release(context.Background(), true)

	// then: shared scan stays bounded and the unbounded target runs in its own scan
	testastic.NoError(t, err)
	testastic.Equal(t, 2, len(result.Plans))
	testastic.SliceEqual(t, []string{"api-v1.0.0"}, stub.getCommitsSinceRefsOf[0])
	testastic.SliceEqual(t, []string{""}, stub.getCommitsSinceRefsOf[1])
}

func TestReleaseAnalyzerSharedHistoryFallsBackBeyondTopRefs(t *testing.T) {
	t.Parallel()

	// given: an api target whose three newest tags are unreachable (hotfix branches),
	//        a fourth tag that is reachable, and a separate web target with one tag
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

	stub := newProviderStub()
	stub.tagList = []string{
		"api-v1.1.0",
		"api-v1.0.2",
		"api-v1.0.1",
		"api-v1.0.0",
		"web-v1.0.0",
	}
	stub.commitsErrByRef["api-v1.1.0"] = provider.ErrCommitBoundaryNotFound
	stub.commitsErrByRef["api-v1.0.2"] = provider.ErrCommitBoundaryNotFound
	stub.commitsErrByRef["api-v1.0.1"] = provider.ErrCommitBoundaryNotFound
	stub.commitsByRef = map[string][]provider.CommitEntry{
		"api-v1.0.0": {
			{Hash: "api-new", Message: "fix: patch api", Paths: []string{"services/api/main.go"}},
		},
		"web-v1.0.0": {
			{Hash: "web-new", Message: "feat: refresh web", Paths: []string{"apps/web/app.tsx"}},
		},
	}

	r := newTestReleaser(t, cfg, stub)

	// when: analyzing the shared branch history
	result, err := r.Release(context.Background(), true)

	// then: api falls back to v1.0.0 once the top-3 refs are missing from the shared scan
	testastic.NoError(t, err)
	testastic.Equal(t, 2, len(result.Plans))
	testastic.Equal(t, "api", result.Plans[0].ID)
	testastic.Equal(t, "1.0.0", result.Plans[0].CurrentVersion)
	testastic.Equal(t, "api-v1.0.1", result.Plans[0].NextTag)
	testastic.Equal(t, "web", result.Plans[1].ID)
	testastic.Equal(t, "1.0.0", result.Plans[1].CurrentVersion)
	testastic.Equal(t, "web-v1.1.0", result.Plans[1].NextTag)
	testastic.SliceEqual(
		t,
		[]string{"api-v1.0.1", "api-v1.0.2", "api-v1.1.0", "web-v1.0.0"},
		stub.getCommitsSinceRefsOf[0],
	)
}

func TestReleaseAnalyzerSharedHistoryFallbackReusesScan(t *testing.T) {
	t.Parallel()

	// given: api top-3 refs are unreachable per the shared scan, with a deeper reachable ref
	//        outside the top-N, plus a separate web target with one tag
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

	stub := newProviderStub()
	stub.tagList = []string{
		"api-v1.1.0",
		"api-v1.0.2",
		"api-v1.0.1",
		"api-v1.0.0",
		"web-v1.0.0",
	}
	stub.commitsErrByRef["api-v1.1.0"] = provider.ErrCommitBoundaryNotFound
	stub.commitsErrByRef["api-v1.0.2"] = provider.ErrCommitBoundaryNotFound
	stub.commitsErrByRef["api-v1.0.1"] = provider.ErrCommitBoundaryNotFound
	stub.commitsByRef = map[string][]provider.CommitEntry{
		"api-v1.0.0": {
			{Hash: "api-new", Message: "fix: patch api", Paths: []string{"services/api/main.go"}},
		},
		"web-v1.0.0": {
			{Hash: "web-new", Message: "feat: refresh web", Paths: []string{"apps/web/app.tsx"}},
		},
	}

	r := newTestReleaser(t, cfg, stub)

	// when: analyzing the shared branch history
	_, err := r.Release(context.Background(), true)

	// then: the fallback path reuses the shared scan's reachability and entries,
	//       probing only the deeper ref the shared scan did not cover
	testastic.NoError(t, err)
	testastic.SliceEqual(t, []string{"api-v1.0.0", "api-v1.0.0"}, stub.singleRefProbes())
}
