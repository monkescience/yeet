package logattr

import "log/slog"

func Provider(name string) slog.Attr {
	return slog.String("provider", name)
}

func PullRequestNumber(number int64) slog.Attr {
	return slog.Int64("pr_number", number)
}

func Hint(message string) slog.Attr {
	return slog.String("hint", message)
}
