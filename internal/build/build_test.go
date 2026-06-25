package build_test

import (
	"testing"

	"github.com/monkescience/testastic"

	"github.com/monkescience/yeet/internal/build"
)

func TestServiceName(t *testing.T) {
	t.Parallel()

	// then: ServiceName is the constant "yeet"
	testastic.Equal(t, "yeet", build.ServiceName)
}

func TestVersion_ldflagTakesPrecedence(t *testing.T) {
	// given: an ldflag-injected version
	t.Cleanup(build.SetForTest("v1.2.3", "", ""))

	// when: reading Version
	got := build.Version()

	// then: the ldflag value is returned
	testastic.Equal(t, "v1.2.3", got)
}

func TestVersion_fallsBackToBuildInfo(t *testing.T) {
	// given: no ldflag-injected version
	t.Cleanup(build.SetForTest("", "", ""))

	// when: reading Version
	got := build.Version()

	// then: debug.ReadBuildInfo() provides a non-empty fallback
	testastic.NotEqual(t, "", got)
}

func TestCommit_ldflagTakesPrecedence(t *testing.T) {
	// given: an ldflag-injected commit
	t.Cleanup(build.SetForTest("", "abc1234", ""))

	// when: reading Commit
	got := build.Commit()

	// then: the ldflag value is returned
	testastic.Equal(t, "abc1234", got)
}

func TestCommit_fallsBackToVCSRevision(t *testing.T) {
	// given: no ldflag-injected commit
	t.Cleanup(build.SetForTest("", "", ""))

	// when: reading Commit
	got := build.Commit()

	// then: the vcs.revision fallback provides a non-empty value
	testastic.NotEqual(t, "", got)
}

func TestDate_ldflagTakesPrecedence(t *testing.T) {
	// given: an ldflag-injected build date
	t.Cleanup(build.SetForTest("", "", "2026-03-20T12:34:56Z"))

	// when: reading Date
	got := build.Date()

	// then: the ldflag value is returned
	testastic.Equal(t, "2026-03-20T12:34:56Z", got)
}

func TestDate_fallsBackToVCSTime(t *testing.T) {
	// given: no ldflag-injected build date
	t.Cleanup(build.SetForTest("", "", ""))

	// when: reading Date
	got := build.Date()

	// then: the vcs.time fallback provides a non-empty value
	testastic.NotEqual(t, "", got)
}
