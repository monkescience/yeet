package build

import (
	"runtime"
	"runtime/debug"
)

const (
	ServiceName  = "yeet"
	unknownValue = "unknown"
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

// Module returns the module checksum (e.g. "h1:abc...") embedded by `go install`
// builds. Empty for `go build` / release / ko builds where the binary is not
// sourced from a module tarball. Callers decide whether to display it.
func Module() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	return info.Main.Sum
}

func Platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func GoVersion() string {
	return runtime.Version()
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
