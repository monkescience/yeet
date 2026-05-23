package commands //nolint:testpackage // validates unexported printVersion behavior directly

import (
	"bytes"
	"testing"
)

func TestPrintVersion_buildBinary(t *testing.T) {
	t.Parallel()

	// given: build info from a -ldflags-injected binary (no module sum)
	var buf bytes.Buffer

	info := buildInfo{
		name:      "yeet",
		version:   "v1.2.3",
		commit:    "abc1234",
		built:     "2026-03-20T12:34:56Z",
		platform:  "linux/amd64",
		goVersion: "go1.26.0",
	}

	// when: printVersion is invoked
	err := printVersion(&buf, info)
	if err != nil {
		t.Fatalf("printVersion returned error: %v", err)
	}

	// then: output emits commit (not module-sum) and the full provenance block
	want := "name: yeet\n" +
		"version: v1.2.3\n" +
		"commit: abc1234\n" +
		"built: 2026-03-20T12:34:56Z\n" +
		"platform: linux/amd64\n" +
		"go-version: go1.26.0\n"
	if got := buf.String(); got != want {
		t.Errorf("printVersion output = %q, want %q", got, want)
	}
}

func TestPrintVersion_goInstallBinary(t *testing.T) {
	t.Parallel()

	// given: build info from a `go install` binary (module sum present, no commit)
	var buf bytes.Buffer

	info := buildInfo{
		name:      "yeet",
		version:   "v1.2.3",
		commit:    "unknown",
		built:     "unknown",
		platform:  "darwin/arm64",
		goVersion: "go1.26.0",
		moduleSum: "h1:ModuleSumExample=",
	}

	// when: printVersion is invoked
	err := printVersion(&buf, info)
	if err != nil {
		t.Fatalf("printVersion returned error: %v", err)
	}

	// then: output emits module-sum in place of commit
	want := "name: yeet\n" +
		"version: v1.2.3\n" +
		"module-sum: h1:ModuleSumExample=\n" +
		"built: unknown\n" +
		"platform: darwin/arm64\n" +
		"go-version: go1.26.0\n"
	if got := buf.String(); got != want {
		t.Errorf("printVersion output = %q, want %q", got, want)
	}
}
