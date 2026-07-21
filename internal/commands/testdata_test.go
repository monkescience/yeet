package commands //nolint:testpackage // validates unexported command behavior directly

import (
	"path/filepath"
	"runtime"
	"testing"
)

func commandTestFilePath(t *testing.T, path string) string {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve command testdata directory")
	}

	return filepath.Join(filepath.Dir(sourceFile), path)
}
