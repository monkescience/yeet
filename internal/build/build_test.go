package build_test

import (
	"testing"

	"github.com/monkescience/yeet/internal/build"
)

func TestServiceName(t *testing.T) {
	t.Parallel()

	// then: ServiceName is the constant "yeet"
	if build.ServiceName != "yeet" {
		t.Errorf("ServiceName = %q, want %q", build.ServiceName, "yeet")
	}
}

func TestVersion_ldflagTakesPrecedence(t *testing.T) {
	// given: an ldflag-injected version
	t.Cleanup(build.SetForTest("v1.2.3", "", ""))

	// when: reading Version
	got := build.Version()

	// then: the ldflag value is returned
	if got != "v1.2.3" {
		t.Errorf("Version() = %q, want %q", got, "v1.2.3")
	}
}

func TestVersion_fallsBackToBuildInfo(t *testing.T) {
	// given: no ldflag-injected version
	t.Cleanup(build.SetForTest("", "", ""))

	// when: reading Version
	got := build.Version()

	// then: debug.ReadBuildInfo() provides a non-empty fallback
	if got == "" {
		t.Error("Version() returned empty string; expected debug.ReadBuildInfo() fallback to provide a value")
	}
}

func TestCommit_ldflagTakesPrecedence(t *testing.T) {
	// given: an ldflag-injected commit
	t.Cleanup(build.SetForTest("", "abc1234", ""))

	// when: reading Commit
	got := build.Commit()

	// then: the ldflag value is returned
	if got != "abc1234" {
		t.Errorf("Commit() = %q, want %q", got, "abc1234")
	}
}

func TestCommit_fallsBackToVCSRevision(t *testing.T) {
	// given: no ldflag-injected commit
	t.Cleanup(build.SetForTest("", "", ""))

	// when: reading Commit
	got := build.Commit()

	// then: the vcs.revision fallback provides a non-empty value
	if got == "" {
		t.Error("Commit() returned empty string; expected vcs.revision fallback to provide a value")
	}
}

func TestDate_ldflagTakesPrecedence(t *testing.T) {
	// given: an ldflag-injected build date
	t.Cleanup(build.SetForTest("", "", "2026-03-20T12:34:56Z"))

	// when: reading Date
	got := build.Date()

	// then: the ldflag value is returned
	if got != "2026-03-20T12:34:56Z" {
		t.Errorf("Date() = %q, want %q", got, "2026-03-20T12:34:56Z")
	}
}

func TestDate_fallsBackToVCSTime(t *testing.T) {
	// given: no ldflag-injected build date
	t.Cleanup(build.SetForTest("", "", ""))

	// when: reading Date
	got := build.Date()

	// then: the vcs.time fallback provides a non-empty value
	if got == "" {
		t.Error("Date() returned empty string; expected vcs.time fallback to provide a value")
	}
}
