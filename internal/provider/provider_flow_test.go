package provider_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
)

func decodedPathTail(t *testing.T, request *http.Request) string {
	t.Helper()

	segments := strings.Split(request.URL.EscapedPath(), "/")

	label, err := url.PathUnescape(segments[len(segments)-1])
	testastic.NoError(t, err)

	return label
}

func isGitLabRawFilePath(request *http.Request, path string) bool {
	if request.URL.Query().Get("ref") != "main" {
		return false
	}

	prefix := "/api/v4/projects/o%2Fr/repository/files/"
	suffix := "/raw"

	escapedPath := request.URL.EscapedPath()
	if !strings.HasPrefix(escapedPath, prefix) || !strings.HasSuffix(escapedPath, suffix) {
		return false
	}

	rawPath := strings.TrimSuffix(strings.TrimPrefix(escapedPath, prefix), suffix)

	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return false
	}

	return decodedPath == path
}
