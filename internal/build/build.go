// Package build exposes the binary's identity and version metadata.
//
// Values injected via -ldflags (at release or Makefile build time) take
// precedence. When a value is not injected (e.g. `go install`, `go build`,
// or a ko image build without ldflags), the accessors fall back to
// runtime/debug.ReadBuildInfo so `yeet version` still reports meaningful
// module and VCS information.
package build

import "runtime/debug"

const (
	ServiceName  = "yeet"
	unknownValue = "(unknown)"
)

var (
	version string
	commit  string
	date    string
)

func Version() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return unknownValue
	}

	return info.Main.Version
}

func Commit() string {
	if commit != "" {
		return commit
	}

	return buildInfoSetting("vcs.revision")
}

func Date() string {
	if date != "" {
		return date
	}

	return buildInfoSetting("vcs.time")
}

func buildInfoSetting(key string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return unknownValue
	}

	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}

	return unknownValue
}
