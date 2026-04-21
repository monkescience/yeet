package build_test

import (
	"testing"

	"github.com/monkescience/yeet/internal/build"
)

func TestServiceName(t *testing.T) {
	if build.ServiceName != "yeet" {
		t.Errorf("ServiceName = %q, want %q", build.ServiceName, "yeet")
	}
}

func TestVersion_ldflagTakesPrecedence(t *testing.T) {
	t.Cleanup(build.SetForTest("v1.2.3", "", ""))

	got := build.Version()
	if got != "v1.2.3" {
		t.Errorf("Version() = %q, want %q", got, "v1.2.3")
	}
}

func TestVersion_fallsBackToBuildInfo(t *testing.T) {
	t.Cleanup(build.SetForTest("", "", ""))

	got := build.Version()
	if got == "" {
		t.Error("Version() returned empty string; expected debug.ReadBuildInfo() fallback to provide a value")
	}
}

func TestCommit_ldflagTakesPrecedence(t *testing.T) {
	t.Cleanup(build.SetForTest("", "abc1234", ""))

	got := build.Commit()
	if got != "abc1234" {
		t.Errorf("Commit() = %q, want %q", got, "abc1234")
	}
}

func TestCommit_fallsBackToVCSRevision(t *testing.T) {
	t.Cleanup(build.SetForTest("", "", ""))

	got := build.Commit()
	if got == "" {
		t.Error("Commit() returned empty string; expected vcs.revision fallback to provide a value")
	}
}

func TestDate_ldflagTakesPrecedence(t *testing.T) {
	t.Cleanup(build.SetForTest("", "", "2026-03-20T12:34:56Z"))

	got := build.Date()
	if got != "2026-03-20T12:34:56Z" {
		t.Errorf("Date() = %q, want %q", got, "2026-03-20T12:34:56Z")
	}
}

func TestDate_fallsBackToVCSTime(t *testing.T) {
	t.Cleanup(build.SetForTest("", "", ""))

	got := build.Date()
	if got == "" {
		t.Error("Date() returned empty string; expected vcs.time fallback to provide a value")
	}
}
