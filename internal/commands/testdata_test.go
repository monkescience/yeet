package commands //nolint:testpackage // validates unexported command behavior directly

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/monkescience/testastic"
)

func commandTestFilePath(t *testing.T, path string) string {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	testastic.True(t, ok)

	if !ok {
		t.FailNow()
	}

	return filepath.Join(filepath.Dir(sourceFile), path)
}
