package commands //nolint:testpackage // validates unexported printVersion behavior directly

import (
	"bytes"
	"testing"
)

func TestPrintVersion_withoutModule(t *testing.T) {
	t.Parallel()

	// given: build info without a module sum
	var buf bytes.Buffer

	info := buildInfo{
		version: "v1.2.3",
		commit:  "abc1234",
		built:   "2026-03-20T12:34:56Z",
	}

	// when: printVersion is invoked
	err := printVersion(&buf, info)
	if err != nil {
		t.Fatalf("printVersion returned error: %v", err)
	}

	// then: output omits the module line
	want := "version: v1.2.3\ncommit: abc1234\nbuilt: 2026-03-20T12:34:56Z\n"
	if got := buf.String(); got != want {
		t.Errorf("printVersion output = %q, want %q", got, want)
	}
}

func TestPrintVersion_withModule(t *testing.T) {
	t.Parallel()

	// given: build info with a module sum
	var buf bytes.Buffer

	info := buildInfo{
		version: "v1.2.3",
		commit:  "abc1234",
		built:   "2026-03-20T12:34:56Z",
		module:  "h1:ModuleSumExample=",
	}

	// when: printVersion is invoked
	err := printVersion(&buf, info)
	if err != nil {
		t.Fatalf("printVersion returned error: %v", err)
	}

	// then: output includes the module line
	want := "version: v1.2.3\ncommit: abc1234\nbuilt: 2026-03-20T12:34:56Z\nmodule: h1:ModuleSumExample=\n"
	if got := buf.String(); got != want {
		t.Errorf("printVersion output = %q, want %q", got, want)
	}
}
