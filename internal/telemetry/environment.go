package telemetry

import "strings"

func doNotTrack(values []string) bool {
	setting := ""

	for _, value := range values {
		name, value, found := strings.Cut(value, "=")
		if found && name == "DO_NOT_TRACK" {
			setting = value
		}
	}

	return isTruthy(setting)
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
