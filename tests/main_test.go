// Package integration_test contains end-to-end blackbox tests that exercise
// the compiled yeet binary as a subprocess and assert against its real
// stdout, stderr, and exit codes.
package integration_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
)

var binary *testastic.Binary

func TestMain(m *testing.M) {
	testing.Init()
	flag.Parse()

	if err := clearInheritedBranchEnv(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}

	if testing.Short() {
		os.Exit(0)
	}

	binary = testastic.BuildBinaryMain(
		m,
		"./cmd/yeet",
		testastic.WithWorkDir(".."),
		testastic.WithBuildArgs("-ldflags", "-X github.com/monkescience/yeet/internal/build.version=dev"),
	)

	code := testastic.CollectSubprocessCoverage(m, "../coverage/coverage.out")

	binary.Cleanup()

	os.Exit(code)
}

func clearInheritedBranchEnv() error {
	for _, envName := range []string{
		"GITHUB_REF",
		"GITHUB_REF_NAME",
		"CI_COMMIT_BRANCH",
		"BRANCH_NAME",
		"BUILD_SOURCEBRANCH",
	} {
		if err := os.Unsetenv(envName); err != nil {
			return fmt.Errorf("unset inherited branch environment %s: %w", envName, err)
		}
	}

	return nil
}

func absoluteTestFile(t *testing.T, path string) string {
	t.Helper()

	absolutePath, err := filepath.Abs(path)
	testastic.NoError(t, err)

	return absolutePath
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	testastic.NoError(t, err)

	return string(contents)
}
