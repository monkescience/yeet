// Package integration_test contains end-to-end blackbox tests that exercise
// the compiled yeet binary as a subprocess and assert against its real
// stdout, stderr, and exit codes.
package integration_test

import (
	"flag"
	"os"
	"testing"

	"github.com/monkescience/testastic"
)

var binary *testastic.Binary

func TestMain(m *testing.M) {
	testing.Init()
	flag.Parse()

	if testing.Short() {
		os.Exit(m.Run())
	}

	binary = testastic.BuildBinaryMain(m, "./cmd/yeet", testastic.WithWorkDir(".."))

	code := testastic.CollectSubprocessCoverage(m, "../coverage/process.out")

	binary.Cleanup()

	os.Exit(code)
}
